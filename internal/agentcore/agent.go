// 探针核心模块
// 负责采集系统指标 + Ping 探测 + gRPC 上报 + 指令接收 + 本地 10min 缓冲
package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"wukong/internal/config"
	pb "wukong/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Agent 探针实例
type Agent struct {
	cfg    *config.AgentConfig
	client pb.AgentServiceClient
	conn   *grpc.ClientConn

	mu          sync.RWMutex
	agentID     string
	agentSecret string
	registered  bool

	// 缓冲
	buffer     []*pb.MetricsReport
	bufferMu   sync.Mutex
	bufferSize int // 最大缓冲条数

	// 采集器
	collectors []Collector

	// 探针版本
	version string

	// 当前公网出口 IP，定期自测并随 MetricsReport 上报。
	ipMu sync.RWMutex
	ipV4 string
	ipV6 string

	// 目标版本（主控下发，用于自升级）
	targetVersion string

	cancel context.CancelFunc
}

// Collector 采集器接口
type Collector interface {
	Name() string
	Collect() (*CollectResult, error)
}

// CollectResult 采集结果
type CollectResult struct {
	System *pb.SystemMetric
	Pings  []*pb.PingMetric
}

func NewAgent(cfg *config.AgentConfig) (*Agent, error) {
	return NewAgentWithVersion(cfg, "dev")
}

// NewAgentWithVersion 创建探针实例，使用构建时注入的版本号。
// 原因：cmd/agent/main.go 通过 ldflags 注入版本号，需要传入而非硬编码。
func NewAgentWithVersion(cfg *config.AgentConfig, ver string) (*Agent, error) {
	if ver == "" {
		ver = "dev"
	}
	return &Agent{
		cfg:        cfg,
		bufferSize: (cfg.BufferMinutes * 60) / cfg.CollectInterval, // 缓冲条数
		version:    ver,
	}, nil
}

// Register 使用一次性 token 注册到主控
func (a *Agent) Register(token string) error {
	// 连接主控 gRPC 服务；443 通过公网 nginx TLS 反代，内网/直连端口继续使用明文 gRPC。
	conn, err := grpc.Dial(a.cfg.ServerAddr,
		append(a.grpcDialOptions(), grpc.WithBlock(), grpc.WithTimeout(10*time.Second))...)
	if err != nil {
		return fmt.Errorf("连接主控失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	hostname, _ := os.Hostname()

	// 获取本机公网 IPv4/IPv6 地址，注册时上报给主控存储；后续运行中也会周期自测并随指标上报。
	ipV4, ipV6 := getPublicIPs()
	a.setPublicIPs(ipV4, ipV6)

	// 发送注册请求
	resp, err := client.Register(context.Background(), &pb.RegisterRequest{
		Token:        token,
		Hostname:     hostname,
		AgentVersion: a.version,
		Arch:         getArch(),
		IpV4:         ipV4,
		IpV6:         ipV6,
	})
	if err != nil {
		return fmt.Errorf("注册失败: %w", err)
	}

	// 保存个体凭证到配置
	a.cfg.AgentID = resp.AgentId
	a.cfg.AgentSecret = resp.AgentSecret
	if resp.CollectInterval > 0 {
		a.cfg.CollectInterval = int(resp.CollectInterval)
	}
	if resp.PingInterval > 0 {
		a.cfg.PingInterval = int(resp.PingInterval)
	}
	a.cfg.PingTargets = make([]config.PingTargetConfig, 0, len(resp.PingTargets))
	for _, target := range resp.PingTargets {
		// 注册响应里的 Ping 目标来自主控 SQLite，写入本地配置后探针重启仍可继续探测。
		a.cfg.PingTargets = append(a.cfg.PingTargets, config.PingTargetConfig{
			Name:    target.Name,
			IP:      target.Ip,
			Port:    int(target.Port),
			Mode:    target.Mode,
			Enabled: target.Enabled,
		})
	}

	if resp.TargetVersion != "" {
		a.targetVersion = resp.TargetVersion
	}

	// 保存配置到文件
	if err := config.SaveAgentConfig(a.cfg, ""); err != nil {
		return fmt.Errorf("保存探针配置失败: %w", err)
	}

	log.Printf("探针注册成功: id=%s hostname=%s", resp.AgentId, hostname)
	return nil
}

// Start 启动探针（采集 + 上报 + 指令接收）
func (a *Agent) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	// 尝试加载个体凭证
	if a.cfg.AgentID == "" || a.cfg.AgentSecret == "" {
		return fmt.Errorf("个体凭证未设置，请先注册（使用 --token 参数）")
	}

	// 初始化采集器
	a.initCollectors()

	// 启动公网出口 IP 自测循环，确保旧节点无需重新注册也能把 IPv4/IPv6 上报给主站。
	go a.publicIPLoop(ctx)

	// 每 5 分钟自动检查是否有新版本可升级
	go a.autoUpgradeCheck(ctx)

	// 启动 gRPC 连接和上报循环
	go a.reportLoop(ctx)

	log.Printf("探针已启动，采集频率: %ds", a.cfg.CollectInterval)
	return nil
}

func (a *Agent) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.conn != nil {
		a.conn.Close()
	}
}

// initCollectors 初始化采集器
func (a *Agent) initCollectors() {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 系统采集器负责资源使用率、硬件规格、启动时间和区域等公开详情字段。
	collectors := []Collector{&SystemCollector{region: a.cfg.Region}}
	if len(a.cfg.PingTargets) > 0 {
		// Ping 采集器内部按 PingInterval 限频；当前生产默认 1 秒，图表仍按 1 分钟聚合展示。
		collectors = append(collectors, NewPingCollector(a.cfg.PingInterval, a.cfg.PingTargets))
	}
	a.collectors = collectors
}

// reportLoop 采集并上报循环
func (a *Agent) reportLoop(ctx context.Context) {
	// 重连指数退避
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 连接主控
		conn, err := a.connect(ctx)
		if err != nil {
			log.Printf("连接主控失败: %v (%.0fs 后重试)", err, backoff.Seconds())
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}
		backoff = 1 * time.Second // 连接成功，重置退避

		a.conn = conn
		client := pb.NewAgentServiceClient(conn)
		a.client = client

		// 启动双向流
		stream, err := client.ReportStream(ctx)
		if err != nil {
			log.Printf("建立上报流失败: %v", err)
			conn.Close()
			continue
		}

		log.Printf("已连接到主控 %s", a.cfg.ServerAddr)

		// 发送缓冲数据（如果有）
		a.flushBuffer(stream)

		// 定时采集并上报
		ticker := time.NewTicker(time.Duration(a.cfg.CollectInterval) * time.Second)
		defer ticker.Stop()

		// 接收指令的 goroutine
		cmdDone := make(chan error, 1)
		go func() {
			for {
				frame, err := stream.Recv()
				if err != nil {
					cmdDone <- err
					return
				}
				a.handleServerFrame(frame)
			}
		}()

	reportLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 采集并上报
				report := a.collectAndReport()
				if report != nil {
					if err := stream.Send(&pb.AgentFrame{
						Frame: &pb.AgentFrame_MetricsReport{
							MetricsReport: report,
						},
					}); err != nil {
						log.Printf("上报失败: %v", err)
						conn.Close()
						break reportLoop
					}
				}
			case err := <-cmdDone:
				if err != nil {
					log.Printf("流连接断开: %v", err)
				}
				conn.Close()
				break reportLoop
			}
		}
	}
}

// grpcDialOptions 按连接地址选择 gRPC 传输方式。
// 原因：生产环境要求探针通过 443 连接 nginx，此时客户端必须使用 TLS；本地 64443 或内网直连仍保持明文 gRPC。
func (a *Agent) grpcDialOptions() []grpc.DialOption {
	_, port, err := net.SplitHostPort(a.cfg.ServerAddr)
	if err == nil && port == "443" {
		return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, ""))}
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
}

// connect 连接到主控
func (a *Agent) connect(ctx context.Context) (*grpc.ClientConn, error) {
	options := append(a.grpcDialOptions(), grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	}))
	return grpc.DialContext(ctx, a.cfg.ServerAddr, options...)
}

// collectAndReport 采集一次并构造上报消息
func (a *Agent) collectAndReport() *pb.MetricsReport {
	now := time.Now()
	report := &pb.MetricsReport{
		AgentId:      a.cfg.AgentID,
		ReportSeq:    now.UnixNano(),
		AgentVersion: a.version,
		Arch:         getArch(),
	}
	ipV4, ipV6 := a.publicIPs()
	report.IpV4 = ipV4
	report.IpV6 = ipV6

	a.mu.RLock()
	collectors := append([]Collector(nil), a.collectors...)
	a.mu.RUnlock()
	for _, c := range collectors {
		result, err := c.Collect()
		if err != nil {
			log.Printf("采集器 %s 失败: %v", c.Name(), err)
			continue
		}
		if result.System != nil {
			report.System = result.System
			report.System.Timestamp = now.Unix()
		}
		if len(result.Pings) > 0 {
			report.Pings = append(report.Pings, result.Pings...)
		}
	}

	return report
}

// handleServerFrame 处理主控下发帧
func (a *Agent) handleServerFrame(frame *pb.ServerFrame) {
	switch f := frame.Frame.(type) {
	case *pb.ServerFrame_HeartbeatAck:
		// 心跳确认，无需处理
	case *pb.ServerFrame_SignedCommand:
		a.handleSignedCommand(f.SignedCommand)
	}
}

// handleSignedCommand 处理签名指令
func (a *Agent) handleSignedCommand(cmd *pb.SignedCommand) {
	// 验证签名（探针侧需要预置公钥）
	// 此处先简化处理，后续实现完整验签逻辑
	log.Printf("收到指令: cmd=%s type=%v", cmd.CommandId, cmd.CommandType)

	switch cmd.CommandType {
	case pb.CommandType_COMMAND_UPDATE_CONFIG:
		a.handleUpdateConfig(cmd.Payload)
	case pb.CommandType_COMMAND_RESTART_AGENT:
		a.handleRestartAgent()
	case pb.CommandType_COMMAND_UPGRADE_AGENT:
		a.handleUpgradeAgent(cmd.Payload)
	}
}

// handleUpdateConfig 处理配置更新
func (a *Agent) handleUpdateConfig(payload []byte) {
	var newCfg config.AgentConfig
	if err := json.Unmarshal(payload, &newCfg); err != nil {
		log.Printf("解析新配置失败: %v", err)
		return
	}

	// 更新采集频率和 Ping 配置，并立即重建采集器；Ping 频率可无需重启立即生效。
	if newCfg.CollectInterval > 0 {
		a.cfg.CollectInterval = newCfg.CollectInterval
	}
	if newCfg.PingInterval > 0 {
		a.cfg.PingInterval = newCfg.PingInterval
	}
	if len(newCfg.PingTargets) > 0 {
		a.cfg.PingTargets = newCfg.PingTargets
	}
	if newCfg.Region != "" {
		a.cfg.Region = newCfg.Region
	}

	// 保存配置并重建采集器，让 ping_interval=1 和最新运营商目标立即生效。
	config.SaveAgentConfig(a.cfg, "")
	a.initCollectors()
	log.Printf("配置已更新，采集频率: %ds，Ping 频率: %ds，Ping目标数: %d", a.cfg.CollectInterval, a.cfg.PingInterval, len(a.cfg.PingTargets))
}

// handleRestartAgent 重启探针自身
func (a *Agent) handleRestartAgent() {
	log.Println("收到重启指令，探针 3 秒后退出（由 systemd 自动重启）")
	time.AfterFunc(3*time.Second, func() {
		os.Exit(0)
	})
}

// handleUpgradeAgent 处理探针自升级指令。
// payload 为 JSON 格式: {"target_version":"0.2.0","download_url":"https://..."}
// 流程：下载新二进制 → 备份当前二进制 → 原子替换 → 退出并交给 systemd 拉起新版本。
func (a *Agent) handleUpgradeAgent(payload []byte) {
	var req struct {
		TargetVersion string `json:"target_version"`
		DownloadURL   string `json:"download_url"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("[升级] 指令 payload 解析失败: %v", err)
		return
	}
	if req.TargetVersion == "" {
		log.Println("[升级] target_version 为空，忽略升级指令")
		return
	}
	if a.version == req.TargetVersion {
		log.Printf("[升级] 当前版本 %s 已是目标版本，无需升级", a.version)
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[升级] 获取当前可执行文件路径失败: %v", err)
		return
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		log.Printf("[升级] 解析当前可执行文件路径失败: %v", err)
		return
	}

	downloadURL := req.DownloadURL
	if downloadURL == "" {
		downloadURL = fmt.Sprintf("https://github.com/luowei729/wukong/releases/download/v%s/wukong-agent-%s-%s", req.TargetVersion, runtime.GOOS, runtime.GOARCH)
	}
	log.Printf("[升级] 开始升级 %s -> %s，下载地址: %s", a.version, req.TargetVersion, downloadURL)

	resp, err := http.Get(downloadURL)
	if err != nil {
		log.Printf("[升级] 下载新版本失败: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[升级] 下载新版本失败，HTTP 状态码: %d", resp.StatusCode)
		return
	}

	tmpPath := exePath + ".new"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		log.Printf("[升级] 创建临时文件失败: %v", err)
		return
	}
	written, err := io.Copy(tmpFile, resp.Body)
	closeErr := tmpFile.Close()
	if err != nil || closeErr != nil {
		log.Printf("[升级] 写入临时文件失败: copy=%v close=%v", err, closeErr)
		os.Remove(tmpPath)
		return
	}
	if written <= 0 {
		log.Printf("[升级] 新版本二进制为空，取消升级")
		os.Remove(tmpPath)
		return
	}

	backupPath := exePath + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(exePath, backupPath); err != nil {
		log.Printf("[升级] 备份当前二进制失败: %v", err)
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		log.Printf("[升级] 替换二进制失败，开始回滚: %v", err)
		_ = os.Rename(backupPath, exePath)
		_ = os.Remove(tmpPath)
		return
	}
	_ = os.Chmod(exePath, 0755)
	log.Printf("[升级] 升级完成，写入 %d bytes，3 秒后重启", written)
	time.AfterFunc(3*time.Second, func() {
		os.Exit(0)
	})
}

// flushBuffer 发送缓冲数据
func (a *Agent) flushBuffer(stream pb.AgentService_ReportStreamClient) {
	a.bufferMu.Lock()
	defer a.bufferMu.Unlock()

	for _, report := range a.buffer {
		if err := stream.Send(&pb.AgentFrame{
			Frame: &pb.AgentFrame_MetricsReport{
				MetricsReport: report,
			},
		}); err != nil {
			log.Printf("发送缓冲数据失败: %v", err)
			return
		}
	}
	a.buffer = nil
}

func getArch() string {
	// 使用 runtime.GOARCH 获取实际系统架构，替代之前的硬编码 "amd64"
	return runtime.GOARCH
}

// publicIPLoop 定期自测公网出口 IP。
// 原因：出口 IP 可能变化，且旧节点注册时没有 ip_v4/ip_v6；运行中上报可补齐主站后台显示。
func (a *Agent) publicIPLoop(ctx context.Context) {
	refresh := func() {
		ipV4, ipV6 := getPublicIPs()
		a.setPublicIPs(ipV4, ipV6)
		if ipV4 != "" || ipV6 != "" {
			log.Printf("公网出口 IP 已更新: ipv4=%s ipv6=%s", ipV4, ipV6)
		}
	}
	refresh()
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

func (a *Agent) setPublicIPs(ipV4, ipV6 string) {
	a.ipMu.Lock()
	defer a.ipMu.Unlock()
	if ipV4 != "" {
		a.ipV4 = ipV4
	}
	if ipV6 != "" {
		a.ipV6 = ipV6
	}
}

func (a *Agent) publicIPs() (string, string) {
	a.ipMu.RLock()
	defer a.ipMu.RUnlock()
	return a.ipV4, a.ipV6
}

// autoUpgradeCheck 每 5 分钟自动检查主控是否有新版本可升级。
// 原因：探针应主动检查版本而非完全依赖主控下发升级指令，确保即使指令下发失败也能及时升级。
// 流程：通过 gRPC 查询主控的 target_version → 与当前版本比较 → 下载新二进制 → 替换重启。
func (a *Agent) autoUpgradeCheck(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.client == nil {
				continue
			}
			// 通过 gRPC 注册接口查询主控当前 target_version
			// 原因：没有独立的版本查询 API，复用注册响应中的 target_version 字段
			// 实际上主控在心跳中会下发升级指令，这里作为主动补充检查
			a.checkAndUpgrade()
		}
	}
}

// checkAndUpgrade 检查主控是否有新版本并执行升级
func (a *Agent) checkAndUpgrade() {
	if a.targetVersion == "" || a.targetVersion == a.version {
		return
	}
	log.Printf("[自动升级] 发现新版本: %s -> %s，开始升级", a.version, a.targetVersion)

	// 构造升级 payload
	payload, _ := json.Marshal(map[string]string{
		"target_version": a.targetVersion,
	})

	cmd := &pb.SignedCommand{
		CommandId:   fmt.Sprintf("auto-upgrade-%d", time.Now().Unix()),
		CommandType: pb.CommandType_COMMAND_UPGRADE_AGENT,
		Payload:     payload,
		IssuedAt:    time.Now().Unix(),
		ExpiresAt:   time.Now().Add(10 * time.Minute).Unix(),
	}
	a.handleSignedCommand(cmd)
}

// getPublicIPs 获取本机公网 IPv4 和 IPv6 地址。
// 原因：探针注册时需要上报公网 IP 给主控存储，用于后端管理。
// 方法：并发请求外部 API 获取，失败则回退到本机网卡地址，超时不阻塞注册流程。
func getPublicIPs() (string, string) {
	type ipResult struct {
		v4  string
		v6  string
		err error
	}
	ch := make(chan ipResult, 1)

	go func() {
		var ipV4, ipV6 string
		var errV4, errV6 error

		// 并发获取 IPv4 和 IPv6
		v4Ch := make(chan string, 1)
		v6Ch := make(chan string, 1)

		go func() {
			// 通过外部 API 获取公网 IPv4
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get("https://api4.ipify.org")
			if err != nil {
				v4Ch <- ""
				return
			}
			defer resp.Body.Close()
			buf := make([]byte, 64)
			n, _ := resp.Body.Read(buf)
			v4Ch <- strings.TrimSpace(string(buf[:n]))
		}()

		go func() {
			// 通过外部 API 获取公网 IPv6
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get("https://api6.ipify.org")
			if err != nil {
				v6Ch <- ""
				return
			}
			defer resp.Body.Close()
			buf := make([]byte, 64)
			n, _ := resp.Body.Read(buf)
			v6Ch <- strings.TrimSpace(string(buf[:n]))
		}()

		ipV4 = <-v4Ch
		ipV6 = <-v6Ch
		if !isPublicIP(ipV4) {
			ipV4 = ""
		}
		if !isPublicIP(ipV6) {
			ipV6 = ""
		}

		// 如果外部 API 都失败，回退到本机网卡地址（仅接受公网地址）
		if ipV4 == "" && ipV6 == "" {
			ipV4, ipV6 = getLocalIPs()
		}

		ch <- ipResult{v4: ipV4, v6: ipV6, err: fmt.Errorf("ipv4 err=%v ipv6 err=%v", errV4, errV6)}
	}()

	// 最多等待 6 秒，超时则回退到本机地址
	select {
	case result := <-ch:
		return result.v4, result.v6
	case <-time.After(6 * time.Second):
		log.Println("获取公网 IP 超时，回退到本机地址")
		return getLocalIPs()
	}
}

// getLocalIPs 从本机网卡获取 IPv4/IPv6 地址（回退方案）
func getLocalIPs() (string, string) {
	var ipV4, ipV6 string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", ""
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ipNet.IP.To4() != nil && ipV4 == "" && isPublicIP(ipNet.IP.String()) {
			ipV4 = ipNet.IP.String()
		}
		if ipNet.IP.To4() == nil && ipV6 == "" && isPublicIP(ipNet.IP.String()) {
			ipV6 = ipNet.IP.String()
		}
	}
	return ipV4, ipV6
}

// isPublicIP 只允许公网出口 IP 上报。
// 过滤 10/172.16/192.168、loopback、link-local(fe80::/10)、ULA(fc00::/7) 等非公网地址。
func isPublicIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
