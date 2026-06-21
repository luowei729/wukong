// gRPC API - 探针通道服务
// 处理探针注册和双向流上报
package grpcapi

import (
	"context"
	"io"
	"log"
	"time"

	"wukong/internal/config"
	"wukong/internal/store"
	pb "wukong/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	store       store.MetricsStore
	alertEngine interface{}
	cfg         *config.ServerConfig
	// 在线探针映射（agentID -> 最近心跳时间）
	onlineAgents map[string]time.Time
}

func RegisterService(grpcServer *grpc.Server, s store.MetricsStore, alert interface{}, cfg *config.ServerConfig) {
	server := &AgentServer{
		store:        s,
		alertEngine:  alert,
		cfg:          cfg,
		onlineAgents: make(map[string]time.Time),
	}
	pb.RegisterAgentServiceServer(grpcServer, server)
	log.Println("gRPC AgentService 已注册")
}

// Register 探针一次性 token 注册
func (s *AgentServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Printf("探针注册请求: token=%s hostname=%s arch=%s", req.Token[:min(16, len(req.Token))]+"...", req.Hostname, req.Arch)

	agent, secret, err := s.store.RegisterAgent(req.Token, req.Hostname, req.AgentVersion, req.Arch)
	if err != nil {
		log.Printf("注册失败: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "注册失败: %v", err)
	}

	collectInterval := s.effectiveCollectInterval(agent)
	pingInterval := s.effectivePingInterval(agent)
	pingTargets := s.enabledPingTargets()

	return &pb.RegisterResponse{
		AgentId:         agent.ID,
		AgentSecret:     secret,
		CollectInterval: int32(collectInterval),
		ServerName:      agent.Hostname,
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour).Unix(), // 一年有效
		PingInterval:    int32(pingInterval),
		PingTargets:     pingTargets,
	}, nil
}

// ReportStream 双向流：探针上报指标 + 接收主控指令
func (s *AgentServer) ReportStream(stream pb.AgentService_ReportStreamServer) error {
	// 首个消息必须包含认证
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "需要认证")
	}

	report := firstMsg.GetMetricsReport()
	if report == nil {
		return status.Errorf(codes.InvalidArgument, "首个消息必须是 MetricsReport")
	}

	// 验证探针身份。
	// 当前 proto 的 MetricsReport 只有 agent_id，没有携带 agent_secret；因此先确认 agent_id 已注册，
	// 避免注册成功后的本机探针因为空 secret 校验被拒绝。后续应在协议中补充签名或凭证字段。
	if _, err := s.store.GetAgent(report.AgentId); err != nil {
		return status.Errorf(codes.PermissionDenied, "身份验证失败")
	}

	agentID := report.AgentId
	log.Printf("探针 %s 已连接", agentID)

	// 标记在线
	s.onlineAgents[agentID] = time.Now()
	s.store.SetAgentOnline(agentID, true, time.Now())
	defer func() {
		delete(s.onlineAgents, agentID)
		s.store.SetAgentOnline(agentID, false, time.Now())
		log.Printf("探针 %s 已断开", agentID)
	}()

	// 处理上行数据流
	done := make(chan error, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				done <- nil
				return
			}
			if err != nil {
				done <- err
				return
			}

			// 更新在线状态
			s.onlineAgents[agentID] = time.Now()
			s.store.SetAgentOnline(agentID, true, time.Now())

			if r := msg.GetMetricsReport(); r != nil {
				s.handleMetricsReport(agentID, r)
			}
			if cr := msg.GetCommandResult(); cr != nil {
				log.Printf("探针 %s 指令结果: cmd=%s success=%v err=%s",
					agentID, cr.CommandId, cr.Success, cr.ErrorMessage)
			}
		}
	}()

	// 定期发送心跳确认
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			// 发送心跳确认
			if err := stream.Send(&pb.ServerFrame{
				Frame: &pb.ServerFrame_HeartbeatAck{
					HeartbeatAck: true,
				},
			}); err != nil {
				return err
			}
		}
	}
}

// handleMetricsReport 处理探针上报的指标数据
func (s *AgentServer) handleMetricsReport(agentID string, r *pb.MetricsReport) {
	// 写入系统指标。使用结构体映射，后续继续扩展公开详情字段时不再膨胀存储接口参数。
	sys := r.System
	if sys != nil {
		ts := time.Unix(sys.Timestamp, 0)
		if err := s.store.WriteSystemMetric(&store.SystemMetricInput{
			AgentID:           agentID,
			Timestamp:         ts,
			CPU:               sys.CpuPercent,
			Mem:               sys.MemPercent,
			Disk:              sys.DiskPercent,
			NetUp:             sys.NetUpBps,
			NetDown:           sys.NetDownBps,
			OSVersion:         sys.OsVersion,
			UptimeSeconds:     sys.UptimeSeconds,
			BootTime:          sys.BootTime,
			MemTotalBytes:     sys.MemTotalBytes,
			DiskTotalBytes:    sys.DiskTotalBytes,
			CPUModel:          sys.CpuModel,
			CPUCores:          int(sys.CpuCores),
			Load1:             sys.Load1,
			Load5:             sys.Load5,
			Load15:            sys.Load15,
			NetUpTotalBytes:   sys.NetUpTotalBytes,
			NetDownTotalBytes: sys.NetDownTotalBytes,
			Region:            sys.Region,
			Platform:          sys.Platform,
		}); err != nil {
			log.Printf("写入系统指标失败: agent=%s err=%v", agentID, err)
		}
	}

	// 写入 Ping 指标
	for _, ping := range r.Pings {
		ts := time.Unix(ping.Timestamp, 0)
		if err := s.store.WritePingMetric(agentID, ts,
			ping.IspName, ping.TargetIp, ping.LatencyMs, ping.LossRate, ping.JitterMs); err != nil {
			log.Printf("写入 Ping 指标失败: agent=%s isp=%s err=%v", agentID, ping.IspName, err)
		}
	}
}

func (s *AgentServer) effectiveCollectInterval(agent *store.Agent) int {
	// 注册阶段先按探针自定义值优先，未配置时回退主控默认值；分组热更新后续通过签名配置下发补齐。
	if agent != nil && agent.CollectIntv != nil && *agent.CollectIntv > 0 {
		return *agent.CollectIntv
	}
	if s.cfg.DefaultCollectInterval > 0 {
		return s.cfg.DefaultCollectInterval
	}
	return 1
}

func (s *AgentServer) effectivePingInterval(agent *store.Agent) int {
	// Ping 频率也必须随注册响应下发并写入探针本地配置，避免探针重启后丢失运营商探测周期。
	if agent != nil && agent.PingIntv != nil && *agent.PingIntv > 0 {
		return *agent.PingIntv
	}
	if s.cfg.DefaultPingInterval > 0 {
		return s.cfg.DefaultPingInterval
	}
	return 60
}

func (s *AgentServer) enabledPingTargets() []*pb.PingTarget {
	targets, err := s.store.ListISPTargets()
	if err != nil {
		log.Printf("读取 Ping 运营商目标失败: %v", err)
		return nil
	}
	result := make([]*pb.PingTarget, 0, len(targets))
	for _, target := range targets {
		if target == nil || !target.Enabled {
			continue
		}
		// 只下发启用目标；目标来自 SQLite 配置，不包含任何管理端密钥。
		result = append(result, &pb.PingTarget{
			Name:    target.Name,
			Ip:      target.IP,
			Port:    int32(target.Port),
			Mode:    target.Mode,
			Enabled: target.Enabled,
		})
	}
	return result
}

// CheckAgentOnline 检查探针是否在线（告警引擎用）
func (s *AgentServer) CheckAgentOnline(agentID string, timeout time.Duration) bool {
	lastSeen, ok := s.onlineAgents[agentID]
	if !ok {
		return false
	}
	return time.Since(lastSeen) < timeout
}

// GetOnlineAgents 获取所有在线探针（Web API 用）
func (s *AgentServer) GetOnlineAgents() map[string]time.Time {
	result := make(map[string]time.Time)
	for k, v := range s.onlineAgents {
		result[k] = v
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
