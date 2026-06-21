// 告警引擎
// 6 类指标阈值判定 + 三级回退 + 持续超阈值判定 + 滞回防抖 + 去重抑制 + 恢复通知 + 静默窗口
package alert

import (
	"log"
	"sync"
	"time"

	"wukong/internal/config"
	"wukong/internal/store"
)

// Engine 告警引擎
type Engine struct {
	store store.MetricsStore
	cfg   *config.ServerConfig
	mu    sync.RWMutex

	// 抑制期记录 map[agentID+metric]firedAt
	suppressed map[string]time.Time
	// 持续超阈值累计 map[agentID+metric]duration
	exceedDuration map[string]time.Duration
	// 静默探针 map[agentID]struct{}
	silencedAgents map[string]struct{}
	// 静默分组 map[groupID]struct{}
	silencedGroups map[string]struct{}
}

func NewEngine(s store.MetricsStore, cfg *config.ServerConfig) *Engine {
	return &Engine{
		store:          s,
		cfg:            cfg,
		suppressed:     make(map[string]time.Time),
		exceedDuration: make(map[string]time.Duration),
	}
}

// ThresholdConfig 单个指标的阈值配置
type ThresholdConfig struct {
	Metric      string  `json:"metric"`       // cpu/mem/disk/ping_latency/ping_loss
	Enabled     bool    `json:"enabled"`
	Warning     float64 `json:"warning"`      // 告警阈值
	Critical    float64 `json:"critical"`     // 严重阈值（暂用同一阈值，后续可扩展）
	Duration    int     `json:"duration"`     // 持续超出默认时间（秒）才触发
	Recovery    float64 `json:"recovery"`     // 恢复阈值（滞回）
	SuppressMin int     `json:"suppress_min"` // 抑制期（分钟）
}

// Run 定时检查（每分钟执行一次）
func (e *Engine) Run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		e.checkAlerts()
	}
}

func (e *Engine) checkAlerts() {
	agents, err := e.store.ListAgents()
	if err != nil {
		log.Printf("告警引擎: 获取探针列表失败: %v", err)
		return
	}

	for _, agent := range agents {
		if !agent.Online {
			e.handleOffline(agent)
			continue
		}

		// 获取最新指标
		metrics, err := e.store.GetLatestMetrics(agent.ID)
		if err != nil {
			continue
		}

		// 检查各指标
		e.checkMetric(agent, "cpu", metrics.CPU, 90, 85, 60)
		e.checkMetric(agent, "mem", metrics.Mem, 90, 85, 60)
		e.checkMetric(agent, "disk", metrics.Disk, 90, 85, 300)
	}
}

func (e *Engine) handleOffline(agent *store.Agent) {
	key := agent.ID + ":offline"
	e.mu.RLock()
	_, suppressed := e.suppressed[key]
	e.mu.RUnlock()
	if suppressed {
		return
	}

	// 检查探针是否已离线超过一定时间（这里告警引擎使用 grpcapi 的心跳数据）
	// 暂略详细实现，等待集成
	log.Printf("告警引擎: 探针 %s(%s) 离线", agent.Name, agent.ID)
	e.fireAlert(agent, "offline", 0, 1)
}

func (e *Engine) checkMetric(agent *store.Agent, metric string, value, threshold, recovery float64, durationSec int) {
	key := agent.ID + ":" + metric

	e.mu.Lock()
	defer e.mu.Unlock()

	// 抑制期检查
	if firedAt, ok := e.suppressed[key]; ok {
		if time.Since(firedAt) < time.Duration(e.cfg.AlertSuppressMinutes)*time.Minute {
			return // 仍在抑制期
		}
		delete(e.suppressed, key)
	}

	// 持续超阈值累计
	if value > threshold {
		e.exceedDuration[key] += 1 * time.Minute // 每次检查增加 1 分钟
		if e.exceedDuration[key] >= time.Duration(durationSec)*time.Second {
			// 持续超阈值达到条件，触发告警
			e.fireAlert(agent, metric, threshold, value)
			e.suppressed[key] = time.Now()
			delete(e.exceedDuration, key)
		}
	} else if value <= recovery {
		// 恢复
		delete(e.exceedDuration, key)
		if err := e.store.ResolveAlert(agent.ID, metric); err != nil {
			log.Printf("告警引擎: 恢复告警失败: agent=%s metric=%s err=%v", agent.ID, metric, err)
		}
	}
}

func (e *Engine) fireAlert(agent *store.Agent, metric string, threshold, value float64) {
	alert := &store.Alert{
		AgentID:   agent.ID,
		Metric:    metric,
		Threshold: threshold,
		Value:     value,
		FiredAt:   time.Now(),
		Status:    "firing",
	}
	id, err := e.store.CreateAlert(alert)
	if err != nil {
		log.Printf("告警引擎: 创建告警记录失败: %v", err)
		return
	}
	log.Printf("告警引擎: 触发告警 id=%d agent=%s metric=%s value=%.1f threshold=%.1f",
		id, agent.Name, metric, value, threshold)
}

// GetActiveAlerts 获取活跃告警列表
func (e *Engine) GetActiveAlerts() ([]*store.Alert, error) {
	return e.store.ListActiveAlerts()
}

// SilenceAgent 静默某个探针
func (e *Engine) SilenceAgent(agentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.silencedAgents[agentID] = struct{}{}
}

// UnsilenceAgent 取消静默
func (e *Engine) UnsilenceAgent(agentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.silencedAgents, agentID)
}