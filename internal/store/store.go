// 存储层接口定义
// MetricsStore 抽象存储层，支持 SQLite 实现，预留 PG 升级路径
// 包含元数据（探针/分组/用户/配置）和时序数据（系统指标/Ping）的存储接口
package store

import (
	"time"
)

// ============ 元数据类型 ============

// Agent 探针（被监控节点）
type Agent struct {
	ID          string     `json:"id"`           // UUID
	Name        string     `json:"name"`         // 显示名称
	Hostname    string     `json:"hostname"`     // 系统主机名
	GroupID     *string    `json:"group_id"`     // 所属分组（nil=未分组）
	Secret      string     `json:"-"`            // 个体凭证密钥（bcrypt）
	OSVersion   string     `json:"os_version"`   // 操作系统版本
	AgentVer    string     `json:"agent_ver"`    // 探针版本
	Arch        string     `json:"arch"`         // 架构
	IPv4        string     `json:"ip_v4"`        // 公网 IPv4 地址（探针上报，不显示前端避免暴露）
	IPv6        string     `json:"ip_v6"`        // 公网 IPv6 地址（有则上报，不显示前端避免暴露）
	CollectIntv *int       `json:"collect_intv"` // 自定义采集频率（覆盖分组/全局，秒）
	PingIntv    *int       `json:"ping_intv"`    // 自定义 Ping 频率
	Online      bool       `json:"online"`       // 是否在线
	LastSeenAt  *time.Time `json:"last_seen_at"` // 最后心跳时间
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Group 分组
type Group struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CollectIntv    *int   `json:"collect_intv"`     // 分组采集频率（覆盖全局）
	PingIntv       *int   `json:"ping_intv"`        // 分组 Ping 频率
	TelegramConfID *int64 `json:"telegram_conf_id"` // 分组绑定的 Telegram 配置
}

// ISP 运营商 Ping 目标
type ISPTarget struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"` // 运营商名称（如"电信"）
	IP      string `json:"ip"`   // 目标 IP
	Port    int    `json:"port"` // TCP ping 端口
	Mode    string `json:"mode"` // "icmp" / "tcp" / "auto"
	Enabled bool   `json:"enabled"`
}

// ============ 时序数据类型 ============

// LatestMetric 最新一帧系统指标（存内存 map，同步写 SQLite）
type LatestMetric struct {
	AgentID           string    `json:"agent_id"`
	CPU               float64   `json:"cpu"`
	Mem               float64   `json:"mem"`
	Disk              float64   `json:"disk"`
	NetUp             int64     `json:"net_up"`
	NetDown           int64     `json:"net_down"`
	OSVersion         string    `json:"os_version"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	BootTime          int64     `json:"boot_time"`
	MemTotalBytes     int64     `json:"mem_total_bytes"`
	DiskTotalBytes    int64     `json:"disk_total_bytes"`
	CPUModel          string    `json:"cpu_model"`
	CPUCores          int       `json:"cpu_cores"`
	Load1             float64   `json:"load1"`
	Load5             float64   `json:"load5"`
	Load15            float64   `json:"load15"`
	NetUpTotalBytes   int64     `json:"net_up_total_bytes"`
	NetDownTotalBytes int64     `json:"net_down_total_bytes"`
	Region            string    `json:"region"`
	Platform          string    `json:"platform"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// SystemMetricInput 是系统指标写入模型。
// 原因：系统详情字段会持续扩展，使用结构体比不断扩展函数参数更容易保持调用方清晰。
type SystemMetricInput struct {
	AgentID           string
	Timestamp         time.Time
	CPU               float64
	Mem               float64
	Disk              float64
	NetUp             int64
	NetDown           int64
	OSVersion         string
	UptimeSeconds     int64
	BootTime          int64
	MemTotalBytes     int64
	DiskTotalBytes    int64
	CPUModel          string
	CPUCores          int
	Load1             float64
	Load5             float64
	Load15            float64
	NetUpTotalBytes   int64
	NetDownTotalBytes int64
	Region            string
	Platform          string
}

// PingAggMin 1 分钟预聚合的 Ping 数据（用于 24h K线查询）
type PingAggMin struct {
	BucketMin time.Time `json:"bucket_min"` // 分钟桶时间
	AgentID   string    `json:"agent_id"`
	ISP       string    `json:"isp"`
	Count     int       `json:"count"`
	AvgLat    float64   `json:"avg_lat"`
	MinLat    float64   `json:"min_lat"`
	MaxLat    float64   `json:"max_lat"`
	LossRate  float64   `json:"loss_rate"`
}

// ============ 告警类型 ============

// Alert 告警记录
type Alert struct {
	ID         int64      `json:"id"`
	AgentID    string     `json:"agent_id"`
	Metric     string     `json:"metric"`    // cpu/mem/disk/ping_loss/ping_latency/offline
	Threshold  float64    `json:"threshold"` // 触发阈值
	Value      float64    `json:"value"`     // 触发时的实际值
	Status     string     `json:"status"`    // firing / resolved
	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Notified   bool       `json:"notified"` // 是否已通知
}

// ============ 接口定义 ============

type MetricsStore interface {
	// 初始化表结构
	InitSchema() error
	// 关闭连接
	Close() error

	// ----- 管理员/鉴权 -----
	SetAdminPassword(hash string) error
	GetAdminPassword() (string, error)
	SetTOTPSecret(secret string) error
	GetTOTPSecret() (string, error)

	// ----- 探针管理 -----
	RegisterAgent(token, hostname, agentVer, arch, ipV4, ipV6 string) (*Agent, string, error)
	ValidateAgent(agentID, secret string) bool
	GetAgent(id string) (*Agent, error)
	ListAgents() ([]*Agent, error)
	UpdateAgent(agent *Agent) error
	DeleteAgent(id string) error
	SetAgentOnline(id string, online bool, seenAt time.Time) error

	// ----- 分组管理 -----
	ListGroups() ([]*Group, error)
	GetGroup(id string) (*Group, error)
	CreateGroup(name string) (*Group, error)
	UpdateGroup(group *Group) error
	DeleteGroup(id string) error

	// ----- 运营商 Ping 目标 -----
	ListISPTargets() ([]*ISPTarget, error)
	CreateISPTarget(target *ISPTarget) (int64, error)
	UpdateISPTarget(target *ISPTarget) error
	DeleteISPTarget(id int64) error

	// ----- 设置（KV）-----
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error

	// ----- 时序数据写入 -----
	WriteSystemMetric(metric *SystemMetricInput) error
	WritePingMetric(agentID string, ts time.Time, isp, targetIP string, latency, loss, jitter float64) error
	AggregatePingMin() error // 每分钟聚合上一分钟的 Ping 到 ping_agg_1min 表

	// ----- 时序数据查询 -----
	GetLatestMetrics(agentID string) (*LatestMetric, error)
	GetAllLatestMetrics() (map[string]*LatestMetric, error)
	GetPingAgg(agentID, isp string, since, until time.Time) ([]*PingAggMin, error)
	GetSystemMetrics(agentID string, since, until time.Time) ([]*RawSystemMetric, error)

	// ----- 告警 -----
	CreateAlert(alert *Alert) (int64, error)
	ResolveAlert(agentID, metric string) error
	ListActiveAlerts() ([]*Alert, error)
	GetActiveAlert(agentID, metric string) (*Alert, error)

	// ----- 安装 Token（一次性）-----
	CreateInstallToken() (string, error)
	ConsumeInstallToken(token string) (bool, error)

	// ----- 数据维护 -----
	DropOldHourlyTables(keepHours int) error
	CleanOldAggData(hours int) error
}

// RawSystemMetric 系统指标原始行（明细查询用）
type RawSystemMetric struct {
	Timestamp time.Time `json:"ts"`
	CPU       float64   `json:"cpu"`
	Mem       float64   `json:"mem"`
	Disk      float64   `json:"disk"`
	NetUp     int64     `json:"net_up"`
	NetDown   int64     `json:"net_down"`
}
