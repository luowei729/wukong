// 鉴权服务
// JWT 令牌 + TOTP 二步验证 + 登录限流
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"wukong/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	cfg     *config.ServerConfig
	// 登录限流（内存 map，重启清空）
	failures map[string]*failRecord
	mu       sync.Mutex
}

type failRecord struct {
	count    int
	firstAt  time.Time
	lockedAt time.Time
}

func NewService(cfg *config.ServerConfig) *Service {
	return &Service{
		cfg:      cfg,
		failures: make(map[string]*failRecord),
	}
}

// 自定义 Claims
type Claims struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	// TokenType 区分 access 和 refresh token，防止攻击者用 refresh token 当 access token 访问 API。
	// 原因：旧代码两种 token 的 Claims 结构完全相同，ValidateToken 无法区分，
	// 只要 token 未过期就能任意使用，降低了安全性。
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	TOTPCode   string `json:"totp_code,omitempty"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // 秒
}

// Authenticate 验证用户名密码和 TOTP
// ip 参数用于登录限流，从 HTTP 请求的 X-Forwarded-For / X-Real-IP / RemoteAddr 获取
func (s *Service) Authenticate(username, password, totpCode, ip string) (*LoginResponse, error) {
	// 限流检查
	if ip == "" {
		ip = "global" // 兜底：无 IP 时按全局限流
	}
	if err := s.checkRateLimit(ip); err != nil {
		return nil, err
	}

	// 验证用户名（配置中读取）
	if username != s.cfg.AdminUsername {
		s.recordFailure(ip)
		return nil, errors.New("用户名或密码错误")
	}

	// 验证密码
	if s.cfg.AdminPasswordHash == "" {
		s.recordFailure(ip)
		return nil, errors.New("管理员密码未设置")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.cfg.AdminPasswordHash), []byte(password)); err != nil {
		s.recordFailure(ip)
		return nil, errors.New("用户名或密码错误")
	}

	// 验证 TOTP（如果已配置）
	if s.cfg.AdminTOTPSecret != "" {
		if totpCode == "" {
			return nil, errors.New("需要 TOTP 验证码")
		}
		if !totp.Validate(totpCode, s.cfg.AdminTOTPSecret) {
			s.recordFailure(ip)
			return nil, errors.New("TOTP 验证码错误")
		}
	}

	// 清空失败记录
	s.clearFailure(ip)

	// 生成令牌
	return s.GenerateTokens(username)
}

// GenerateTokens 生成 access + refresh 令牌
func (s *Service) GenerateTokens(username string) (*LoginResponse, error) {
	now := time.Now()

	// Access Token
	accessClaims := &Claims{
		Username:  username,
		IsAdmin:   true,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.GetAccessExpiry())),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        newJWTID(),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessStr, err := accessToken.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("生成 access token 失败: %w", err)
	}

	// Refresh Token
	refreshClaims := &Claims{
		Username:  username,
		IsAdmin:   true,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.GetRefreshExpiry())),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        newJWTID(),
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := refreshToken.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("生成 refresh token 失败: %w", err)
	}

	return &LoginResponse{
		AccessToken:  accessStr,
		RefreshToken: refreshStr,
		ExpiresIn:    int(s.cfg.GetAccessExpiry().Seconds()),
	}, nil
}

// ValidateToken 验证 JWT 并返回 Claims
func (s *Service) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("token 解析失败: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token 无效")
	}
	return claims, nil
}

// ValidateAccessToken 验证 access token，拒绝 refresh token 用于 API 访问。
// 原因：旧代码 ValidateToken 不区分 token 类型，refresh token 也可当 access token 使用，
// 降低安全性。分开校验后，API 中间件只接受 access token，刷新接口只接受 refresh token。
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	claims, err := s.ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "access" {
		return nil, errors.New("该 token 类型不是 access token")
	}
	return claims, nil
}

// ValidateRefreshToken 验证 refresh token，拒绝 access token 用于刷新。
func (s *Service) ValidateRefreshToken(tokenStr string) (*Claims, error) {
	claims, err := s.ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "refresh" {
		return nil, errors.New("该 token 类型不是 refresh token")
	}
	return claims, nil
}

// SetupTOTP 生成 TOTP 密钥，但不在内存中生效。
// 原因：2FA 设置流程是“生成密钥 → 用户扫码 → 输入验证码确认 → 写入数据库”，
// 未确认前密钥只是临时返回给前端，不应直接启用。
func (s *Service) SetupTOTP() (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "wukong",
		AccountName: s.cfg.AdminUsername,
	})
	if err != nil {
		return "", "", fmt.Errorf("生成 TOTP 密钥失败: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// SetTOTPSecret 将 TOTP 密钥写入内存配置（配合数据库持久化使用）。
// 原因：SetupTOTP 只生成密钥，必须在用户确认验证码后才调用本方法 + 写入数据库，
// 保证密钥不会在未确认前意外生效。
func (s *Service) SetTOTPSecret(secret string) {
	s.cfg.AdminTOTPSecret = secret
}

// VerifyTOTP 验证 TOTP 验证码
func (s *Service) VerifyTOTP(code string) bool {
	if s.cfg.AdminTOTPSecret == "" {
		log.Println("TOTP 未配置，跳过验证")
		return true
	}
	return totp.Validate(code, s.cfg.AdminTOTPSecret)
}

// ---- 限流 ----

func (s *Service) checkRateLimit(ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.failures[ip]
	if !ok {
		return nil
	}

	// 如果已锁定且锁定时间未过 15 分钟
	if !r.lockedAt.IsZero() && time.Since(r.lockedAt) < 15*time.Minute {
		return errors.New("登录太频繁，请 15 分钟后重试")
	}
	// 锁定时间已过，清空
	if !r.lockedAt.IsZero() {
		delete(s.failures, ip)
		return nil
	}

	return nil
}

func (s *Service) recordFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.failures[ip]
	if !ok {
		s.failures[ip] = &failRecord{count: 1, firstAt: time.Now()}
		return
	}
	r.count++

	// 5 分钟内失败 5 次，锁定 15 分钟
	if r.count >= 5 && time.Since(r.firstAt) < 5*time.Minute {
		r.lockedAt = time.Now()
		log.Printf("登录限流触发: IP=%s", ip)
	}
}

func (s *Service) clearFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, ip)
}

func newJWTID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}