// 配置管理模块
// 负责加载和保存主控/探针配置，支持配置文件 + 环境变量 + 命令行参数
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// 默认路径
const (
	DefaultDir        = "/opt/wukong"
	DefaultConfigFile = "/opt/wukong/wukong.conf"
	DefaultDBPath     = "/opt/wukong/data/wukong.db"
	DefaultListenAddr = "127.0.0.1:64443" // 主控监听地址，默认仅本地，nginx 反代
	DefaultPIDFile    = "/opt/wukong/wukong.pid"
)

// 主控配置
type ServerConfig struct {
	// 监听
	ListenAddr string `json:"listen_addr"` // 监听地址，默认 127.0.0.1:64443

	// 数据目录
	DataDir string `json:"data_dir"` // 数据目录，默认 /opt/wukong/data
	DBPath  string `json:"db_path"`  // SQLite 路径，默认 /opt/wukong/data/wukong.db

	// 签名服务
	SignerSocket string `json:"signer_socket"` // 签名服务 Unix Socket 路径

	// 管理员鉴权
	AdminUsername     string `json:"admin_username"`     // 管理员用户名（默认 "admin"）
	AdminPasswordHash string `json:"-"`                  // 密码 bcrypt 哈希（不从配置读取）
	AdminTOTPSecret   string `json:"-"`                  // TOTP 密钥（不从配置读取）
	JWTSecret         string `json:"jwt_secret"`         // JWT 签名密钥
	JWTAccessExpiry   string `json:"jwt_access_expiry"`  // access token 有效期（默认 15m）
	JWTRefreshExpiry  string `json:"jwt_refresh_expiry"` // refresh token 有效期（默认 7d）

	// 数据库
	DBMaxConnections int `json:"db_max_connections"` // SQLite 最大连接数（写用，默认 1）

	// 探针相关默认值
	DefaultCollectInterval int `json:"default_collect_interval"` // 默认采集频率（秒，1）
	DefaultPingInterval    int `json:"default_ping_interval"`    // 默认 Ping 频率（秒，60）
	HeartbeatTimeout       int `json:"heartbeat_timeout"`        // 心跳超时判定离线（秒，30）

	// 告警
	AlertSuppressMinutes int `json:"alert_suppress_minutes"` // 告警去重抑制期（分钟，30）

	// 日志
	LogLevel string `json:"log_level"` // 日志级别 debug/info/warn/error

	// Telemetry
	DefaultTelegramBotToken string `json:"-"` // 默认 Telegram bot token（开配置不读，从环境变量 TG_BOT_TOKEN 读）
	DefaultTelegramChatID   int64  `json:"-"` // 默认 Telegram chat ID（从环境变量 TG_CHAT_ID 读）
}

// 探针配置
type AgentConfig struct {
	ServerAddr  string `json:"server_addr"`  // 主控地址（域名:443）
	AgentID     string `json:"agent_id"`     // 个体凭证 ID
	AgentSecret string `json:"agent_secret"` // 个体凭证密钥
	DataDir     string `json:"data_dir"`     // 探针本地数据目录

	CollectInterval int `json:"collect_interval"` // 采集频率（秒）
	PingInterval    int `json:"ping_interval"`    // Ping 频率（秒）

	BufferMinutes int    `json:"buffer_minutes"` // 本地缓冲分钟数（默认 10）
	LogLevel      string `json:"log_level"`
}

// 默认配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		ListenAddr:             DefaultListenAddr,
		DataDir:                filepath.Join(DefaultDir, "data"),
		DBPath:                 DefaultDBPath,
		AdminUsername:          "admin",
		JWTSecret:              randomHex(32),
		JWTAccessExpiry:        "15m",
		JWTRefreshExpiry:       "168h",
		DBMaxConnections:       1,
		DefaultCollectInterval: 1,
		DefaultPingInterval:    60,
		HeartbeatTimeout:       30,
		AlertSuppressMinutes:   30,
		LogLevel:               "info",
	}
}

func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		ServerAddr:      "",
		DataDir:         filepath.Join(DefaultDir, "agent", "data"),
		CollectInterval: 1,
		PingInterval:    60,
		BufferMinutes:   10,
		LogLevel:        "info",
	}
}

// LoadServerConfig 从文件加载主控配置，缺失字段用默认值填充
func LoadServerConfig(path string) (*ServerConfig, error) {
	cfg := DefaultServerConfig()
	if path == "" {
		path = DefaultConfigFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
		// 配置文件不存在，使用默认值，继续执行环境变量覆盖
	} else if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	// 环境变量覆盖（支持 Docker 等场景，始终执行）
	if v := os.Getenv("WUKONG_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("WUKONG_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("WUKONG_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("WUKONG_JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := os.Getenv("WUKONG_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	// 从环境变量读取敏感信息（不落盘）
	if v := os.Getenv("WUKONG_ADMIN_PASSWORD"); v != "" {
		cfg.AdminPasswordHash = v
	}
	if v := os.Getenv("WUKONG_TOTP_SECRET"); v != "" {
		cfg.AdminTOTPSecret = v
	}
	if v := os.Getenv("WUKONG_TG_BOT_TOKEN"); v != "" {
		cfg.DefaultTelegramBotToken = v
	}

	// === 自动生成默认值（用于 Docker 等无配置场景） ===

	// 如果 JWT 密钥为空，自动生成随机密钥
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = randomHex(32)
		log.Printf("[自动生成] JWT_SECRET=%s", cfg.JWTSecret)
	}

	// 如果管理员密码为空，自动生成随机密码并 bcrypt 哈希
	if cfg.AdminPasswordHash == "" {
		plainPwd := randomHex(16) // 16 字符随机密码
		hash, err := bcrypt.GenerateFromPassword([]byte(plainPwd), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("生成管理员密码 hash 失败: %w", err)
		}
		cfg.AdminPasswordHash = string(hash)
		// 日志打印明文密码，首次启动时展示给用户
		log.Printf("========================================")
		log.Printf("  管理员用户名: %s", cfg.AdminUsername)
		log.Printf("  管理员密码:   %s", plainPwd)
		log.Printf("  （请登录后立即修改密码）")
		log.Printf("========================================")
	}

	return cfg, nil
}

// SaveServerConfig 保存主控配置到文件
func SaveServerConfig(cfg *ServerConfig, path string) error {
	if path == "" {
		path = DefaultConfigFile
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	return os.WriteFile(path, data, 0600) // 配置含密钥，权限 600
}

// LoadAgentConfig 加载探针配置（JSON 格式）
func LoadAgentConfig(path string) (*AgentConfig, error) {
	cfg := DefaultAgentConfig()
	if path == "" {
		path = filepath.Join(DefaultDir, "agent", "agent.conf")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取探针配置 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析探针配置 %s 失败: %w", path, err)
	}
	return cfg, nil
}

// SaveAgentConfig 保存探针配置（权限 600）
func SaveAgentConfig(cfg *AgentConfig, path string) error {
	if path == "" {
		path = filepath.Join(DefaultDir, "agent", "agent.conf")
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化探针配置失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建探针配置目录失败: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// ParseDuration 解析配置中的持续时间字符串
func ParseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// GetAccessExpiry 解析 JWT access token 有效期
func (c *ServerConfig) GetAccessExpiry() time.Duration {
	return ParseDuration(c.JWTAccessExpiry, 15*time.Minute)
}

// GetRefreshExpiry 解析 JWT refresh token 有效期
func (c *ServerConfig) GetRefreshExpiry() time.Duration {
	return ParseDuration(c.JWTRefreshExpiry, 7*24*time.Hour)
}

// randomHex 生成随机十六进制字符串（用于默认密钥和密码生成）
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 极端情况回退到时间种子（几乎不会发生）
		log.Printf("[警告] crypto/rand 读取失败: %v, 使用备用方案", err)
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (i % 8))
		}
	}
	return hex.EncodeToString(b)
}
