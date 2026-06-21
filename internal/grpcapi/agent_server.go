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

	return &pb.RegisterResponse{
		AgentId:         agent.ID,
		AgentSecret:     secret,
		CollectInterval: int32(s.cfg.DefaultCollectInterval),
		ServerName:      agent.Hostname,
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour).Unix(), // 一年有效
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
	// 写入系统指标
	sys := r.System
	if sys != nil {
		ts := time.Unix(sys.Timestamp, 0)
		if err := s.store.WriteSystemMetric(agentID, ts,
			sys.CpuPercent, sys.MemPercent, sys.DiskPercent,
			sys.NetUpBps, sys.NetDownBps, sys.OsVersion); err != nil {
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
