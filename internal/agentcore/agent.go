// 探针核心模块
// 负责采集系统指标 + Ping 探测 + gRPC 上报 + 指令接收 + 本地 10min 缓冲
package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
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
	return &Agent{
		cfg:        cfg,
		bufferSize: (cfg.BufferMinutes * 60) / cfg.CollectInterval, // 缓冲条数
		version:    "0.1.0",
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

	// 发送注册请求
	resp, err := client.Register(context.Background(), &pb.RegisterRequest{
		Token:        token,
		Hostname:     hostname,
		AgentVersion: a.version,
		Arch:         getArch(),
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
	a.collectors = append(a.collectors, &SystemCollector{})
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
		AgentId:   a.cfg.AgentID,
		ReportSeq: now.UnixNano(),
	}

	for _, c := range a.collectors {
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
	}
}

// handleUpdateConfig 处理配置更新
func (a *Agent) handleUpdateConfig(payload []byte) {
	var newCfg config.AgentConfig
	if err := json.Unmarshal(payload, &newCfg); err != nil {
		log.Printf("解析新配置失败: %v", err)
		return
	}

	// 更新采集频率
	if newCfg.CollectInterval > 0 {
		a.cfg.CollectInterval = newCfg.CollectInterval
	}

	// 保存配置
	config.SaveAgentConfig(a.cfg, "")
	log.Printf("配置已更新，新采集频率: %ds", a.cfg.CollectInterval)
}

// handleRestartAgent 重启探针自身
func (a *Agent) handleRestartAgent() {
	log.Println("收到重启指令，探针将退出（由 systemd 自动重启）")
	os.Exit(0)
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
	// 简化实现
	return "amd64"
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
