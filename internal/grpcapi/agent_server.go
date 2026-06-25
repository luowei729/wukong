// gRPC API - 探针通道服务
// 处理探针注册和双向流上报
package grpcapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"wukong/internal/config"
	"wukong/internal/store"
	pb "wukong/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AgentServer 探针 gRPC 服务

type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	store       store.MetricsStore
	alertEngine interface{}
	cfg         *config.ServerConfig
	// 在线探针映射（agentID -> 最近心跳时间）
	onlineAgents map[string]time.Time
	onlineMu     sync.RWMutex // 保护 onlineAgents 并发读写
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
	log.Printf("探针注册请求: token=%s hostname=%s arch=%s ipv4=%s ipv6=%s",
		req.Token[:min(16, len(req.Token))]+"...", req.Hostname, req.Arch, req.IpV4, req.IpV6)

	// 注册时保存探针上报的公网 IPv4/IPv6 地址，用于后端管理，不显示前端避免暴露
	agent, secret, err := s.store.RegisterAgent(req.Token, req.Hostname, req.AgentVersion, req.Arch, req.IpV4, req.IpV6)
	if err != nil {
		log.Printf("注册失败: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "注册失败: %v", err)
	}

	collectInterval := s.effectiveCollectInterval(agent)
	pingInterval := s.effectivePingInterval(agent)
	pingTargets := s.enabledPingTargets()
	targetVersion, _ := s.store.GetSetting("agent_target_version")

	return &pb.RegisterResponse{
		AgentId:         agent.ID,
		AgentSecret:     secret,
		CollectInterval: int32(collectInterval),
		ServerName:      agent.Hostname,
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour).Unix(), // 一年有效
		PingInterval:    int32(pingInterval),
		PingTargets:     pingTargets,
		TargetVersion:   targetVersion,
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

	// 验证探针身份：优先使用 agent_secret bcrypt 校验
	// 原因：旧版探针（不含 agent_secret 字段）会在升级前被拒绝连不上，
	// 连不上就收不到升级指令 → 死锁。因此对 secret 为空的旧探针退回
	// 只检查 agent_id 是否已注册的兼容逻辑，让它连上并收到升级指令；
	// 新版探针（含 agent_secret）则强制校验，安全无妥协。
	if report.AgentId == "" {
		return status.Errorf(codes.PermissionDenied, "缺少探针身份凭证")
	}
	if report.AgentSecret != "" {
		// 新探针：强制 bcrypt 校验
		if !s.store.ValidateAgent(report.AgentId, report.AgentSecret) {
			log.Printf("探针 %s 身份验证失败: secret 不匹配", report.AgentId)
			return status.Errorf(codes.PermissionDenied, "探针身份验证失败")
		}
	} else {
		// 旧探针兼容：只检查 agent_id 是否已注册，允许连上接收升级指令
		if _, err := s.store.GetAgent(report.AgentId); err != nil {
			return status.Errorf(codes.PermissionDenied, "探针身份验证失败")
		}
		log.Printf("探针 %s 使用兼容模式连接（无 agent_secret），下发升级指令", report.AgentId)
	}

	agentID := report.AgentId
	log.Printf("探针 %s 已连接", agentID)

	// 标记在线
	s.onlineMu.Lock()
	s.onlineAgents[agentID] = time.Now()
	s.onlineMu.Unlock()
	s.store.SetAgentOnline(agentID, true, time.Now())
	defer func() {
		s.onlineMu.Lock()
		delete(s.onlineAgents, agentID)
		s.onlineMu.Unlock()
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
			s.onlineMu.Lock()
			s.onlineAgents[agentID] = time.Now()
			s.onlineMu.Unlock()
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

	// 定期发送心跳确认，并在需要时下发配置/升级指令。
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	configSent := false
	upgradeSent := false

	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			// 每次连接先下发一次最新配置，确保旧探针本地 ping_interval=60 会被同步为 1 秒。
			if !configSent {
				if frame := s.buildConfigFrame(agentID); frame != nil {
					if err := stream.Send(frame); err != nil {
						return err
					}
					configSent = true
					continue
				}
				configSent = true
			}
			if !upgradeSent {
				if frame := s.buildUpgradeFrame(agentID); frame != nil {
					if err := stream.Send(frame); err != nil {
						return err
					}
					upgradeSent = true
					continue
				}
			}
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

// buildConfigFrame 构造配置热更新指令。
// 原因：已安装探针的 agent.conf 可能仍是 ping_interval=60；连接后下发一次最新配置，让 Ping/TCP 探测立即按 1 秒执行。
func (s *AgentServer) buildConfigFrame(agentID string) *pb.ServerFrame {
	agent, err := s.store.GetAgent(agentID)
	if err != nil {
		return nil
	}
	cfg := config.AgentConfig{
		CollectInterval: s.effectiveCollectInterval(agent),
		PingInterval:    s.effectivePingInterval(agent),
	}
	for _, target := range s.enabledPingTargets() {
		cfg.PingTargets = append(cfg.PingTargets, config.PingTargetConfig{
			Name:    target.Name,
			IP:      target.Ip,
			Port:    int(target.Port),
			Mode:    target.Mode,
			Enabled: target.Enabled,
		})
	}
	payload, _ := json.Marshal(cfg)
	log.Printf("下发探针配置: agent=%s collect=%ds ping=%ds targets=%d", agentID, cfg.CollectInterval, cfg.PingInterval, len(cfg.PingTargets))
	return &pb.ServerFrame{
		Frame: &pb.ServerFrame_SignedCommand{
			SignedCommand: &pb.SignedCommand{
				CommandId:   fmt.Sprintf("config-%s-%d", agentID, time.Now().Unix()),
				CommandType: pb.CommandType_COMMAND_UPDATE_CONFIG,
				Payload:     payload,
				IssuedAt:    time.Now().Unix(),
				ExpiresAt:   time.Now().Add(10 * time.Minute).Unix(),
			},
		},
	}
}

// buildUpgradeFrame 按数据库设置构造探针升级指令。
// settings.agent_target_version 为空时不升级；download_url 可选，默认走主控公开二进制接口。
func (s *AgentServer) buildUpgradeFrame(agentID string) *pb.ServerFrame {
	targetVersion, _ := s.store.GetSetting("agent_target_version")
	if targetVersion == "" {
		return nil
	}
	agent, err := s.store.GetAgent(agentID)
	if err != nil {
		return nil
	}
	if agent.AgentVer == targetVersion {
		return nil
	}

	downloadURL, _ := s.store.GetSetting("agent_upgrade_url")
	if downloadURL == "" {
		siteDomain, _ := s.store.GetSetting("site_domain")
		if siteDomain != "" {
			downloadURL = fmt.Sprintf("%s/api/agent/binary/%s/%s", siteDomain, targetVersion, agent.Arch)
		}
	}
	payload, _ := json.Marshal(map[string]string{
		"target_version": targetVersion,
		"download_url":   downloadURL,
	})

	log.Printf("探针 %s 版本 %s 需要升级到 %s", agentID, agent.AgentVer, targetVersion)
	return &pb.ServerFrame{
		Frame: &pb.ServerFrame_SignedCommand{
			SignedCommand: &pb.SignedCommand{
				CommandId:   fmt.Sprintf("upgrade-%s-%d", agentID, time.Now().Unix()),
				CommandType: pb.CommandType_COMMAND_UPGRADE_AGENT,
				Payload:     payload,
				IssuedAt:    time.Now().Unix(),
				ExpiresAt:   time.Now().Add(10 * time.Minute).Unix(),
			},
		},
	}
}

// handleMetricsReport 处理探针上报的指标数据
func (s *AgentServer) handleMetricsReport(agentID string, r *pb.MetricsReport) {
	// 每次上报都同步探针版本、架构和公网出口 IP。
	if r.AgentVersion != "" || r.Arch != "" || r.IpV4 != "" || r.IpV6 != "" {
		if agent, err := s.store.GetAgent(agentID); err == nil {
			changed := false
			if r.AgentVersion != "" && agent.AgentVer != r.AgentVersion {
				agent.AgentVer = r.AgentVersion
				changed = true
			}
			if r.Arch != "" && agent.Arch != r.Arch {
				agent.Arch = r.Arch
				changed = true
			}
			if r.IpV4 != "" && isPublicIP(r.IpV4) && agent.IPv4 != r.IpV4 {
				agent.IPv4 = r.IpV4
				changed = true
			}
			if r.IpV6 != "" && isPublicIP(r.IpV6) && agent.IPv6 != r.IpV6 {
				agent.IPv6 = r.IpV6
				changed = true
			}
			if changed {
				if err := s.store.UpdateAgent(agent); err != nil {
					log.Printf("同步探针版本/架构/IP失败: agent=%s err=%v", agentID, err)
				}
			}
		}
	}

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
	return 1
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
	s.onlineMu.RLock()
	lastSeen, ok := s.onlineAgents[agentID]
	s.onlineMu.RUnlock()
	if !ok {
		return false
	}
	return time.Since(lastSeen) < timeout
}

// GetOnlineAgents 获取所有在线探针（Web API 用）
func (s *AgentServer) GetOnlineAgents() map[string]time.Time {
	result := make(map[string]time.Time)
	s.onlineMu.RLock()
	for k, v := range s.onlineAgents {
		result[k] = v
	}
	s.onlineMu.RUnlock()
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isPublicIP(value string) bool {
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}
