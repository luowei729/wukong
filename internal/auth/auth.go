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
func (s *Service) Authenticate(username, password, totpCode string) (*LoginResponse, error) {
	// 限流检查
	ip := "global" // 生产环境从请求中取 IP
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
	return s.generateTokens(username)
}

// generateTokens 生成 access + refresh 令牌
func (s *Service) generateTokens(username string) (*LoginResponse, error) {
	now := time.Now()

	// Access Token
	accessClaims := &Claims{
		Username: username,
		IsAdmin:  true,
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
		Username: username,
		IsAdmin:  true,
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

// SetupTOTP 设置 TOTP 密钥（首次配置）
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