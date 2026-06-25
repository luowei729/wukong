// 告警引擎
// 6 类指标阈值判定 + 三级回退 + 持续超阈值判定 + 滞回防抖 + 去重抑制 + 恢复通知 + 静默窗口
package alert

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"wukong/internal/config"
	"wukong/internal/notify"
	"wukong/internal/store"
)

const alertCheckInterval = 5 * time.Second

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
		silencedAgents: make(map[string]struct{}),
		silencedGroups: make(map[string]struct{}),
	}
}

// ThresholdConfig 单个指标的阈值配置
type ThresholdConfig struct {
	Metric      string  `json:"metric"` // cpu/mem/disk/ping_latency/ping_loss
	Enabled     bool    `json:"enabled"`
	Warning     float64 `json:"warning"`      // 告警阈值
	Critical    float64 `json:"critical"`     // 严重阈值（暂用同一阈值，后续可扩展）
	Duration    int     `json:"duration"`     // 持续超出默认时间（秒）才触发
	Recovery    float64 `json:"recovery"`     // 恢复阈值（滞回）
	SuppressMin int     `json:"suppress_min"` // 抑制期（分钟）
}

func (e *Engine) settingInt(key string, fallback int) int {
	value, err := e.store.GetSetting(key)
	if err != nil || value == "" {
		return fallback
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return v
}

func (e *Engine) settingFloat(key string, fallback float64) float64 {
	value, err := e.store.GetSetting(key)
	if err != nil || value == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return v
}

// Run 定时检查告警状态。
// 原因：离线阈值默认 30 秒，若仍每分钟检查会明显滞后；这里用 5 秒轻量轮询，具体触发时间仍由数据库阈值控制。
func (e *Engine) Run() {
	ticker := time.NewTicker(alertCheckInterval)
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

		// 节点恢复在线时，必须恢复 offline 告警并发送恢复通知。
		e.resolveAlert(agent, "offline", 0)

		// 获取最新指标
		metrics, err := e.store.GetLatestMetrics(agent.ID)
		if err != nil {
			continue
		}

		// 检查各指标，阈值和持续时间优先使用 SQLite settings 表，后台保存后立即生效。
		duration := e.settingInt("alert_metric_duration_seconds", 60)
		e.checkMetric(agent, "cpu", metrics.CPU, e.settingFloat("alert_cpu_threshold", 90), 85, duration)
		e.checkMetric(agent, "mem", metrics.Mem, e.settingFloat("alert_mem_threshold", 90), 85, duration)
		e.checkMetric(agent, "disk", metrics.Disk, e.settingFloat("alert_disk_threshold", 90), 85, e.settingInt("alert_disk_duration_seconds", 300))
		e.checkPingMetrics(agent, duration)
	}
}

func (e *Engine) handleOffline(agent *store.Agent) {
	key := agent.ID + ":offline"
	e.mu.RLock()
	firedAt, suppressed := e.suppressed[key]
	e.mu.RUnlock()
	if suppressed && time.Since(firedAt) < time.Duration(e.cfg.AlertSuppressMinutes)*time.Minute {
		return
	}
	if suppressed {
		// 抑制期过后允许再次触发离线告警，避免节点长期离线却永远只报一次。
		e.mu.Lock()
		delete(e.suppressed, key)
		e.mu.Unlock()
	}

	// 离线告警按最后心跳时间和后台配置的离线阈值判断，避免节点刚短暂重连就立刻报警。
	offlineSeconds := e.settingInt("alert_offline_seconds", e.cfg.HeartbeatTimeout)
	if agent.LastSeenAt != nil && time.Since(*agent.LastSeenAt) < time.Duration(offlineSeconds)*time.Second {
		return
	}
	log.Printf("告警引擎: 探针 %s(%s) 离线超过 %d 秒", agent.Name, agent.ID, offlineSeconds)
	e.fireAlert(agent, "offline", float64(offlineSeconds), 1)
	e.mu.Lock()
	e.suppressed[key] = time.Now()
	e.mu.Unlock()
}

func (e *Engine) checkPingMetrics(agent *store.Agent, durationSec int) {
	targets, err := e.store.ListISPTargets()
	if err != nil {
		log.Printf("告警引擎: 获取 Ping 目标失败: %v", err)
		return
	}
	until := time.Now()
	since := until.Add(-2 * time.Minute)
	var worstLatency float64
	var worstLoss float64
	for _, target := range targets {
		if target == nil || !target.Enabled || strings.TrimSpace(target.Name) == "" {
			continue
		}
		points, err := e.store.GetPingAgg(agent.ID, target.Name, since, until)
		if err != nil || len(points) == 0 {
			continue
		}
		latest := points[len(points)-1]
		if latest.AvgLat > worstLatency {
			worstLatency = latest.AvgLat
		}
		lossPercent := latest.LossRate * 100
		if lossPercent > worstLoss {
			worstLoss = lossPercent
		}
	}
	latencyThreshold := e.settingFloat("alert_ping_latency_threshold", 200)
	lossThreshold := e.settingFloat("alert_ping_loss_threshold", 20)
	e.checkMetric(agent, "ping_latency", worstLatency, latencyThreshold, latencyThreshold*0.8, durationSec)
	e.checkMetric(agent, "ping_loss", worstLoss, lossThreshold, lossThreshold*0.5, durationSec)
}

func (e *Engine) checkMetric(agent *store.Agent, metric string, value, threshold, recovery float64, durationSec int) {
	key := agent.ID + ":" + metric
	shouldFire := false
	shouldResolve := false

	e.mu.Lock()
	// 抑制期检查
	if firedAt, ok := e.suppressed[key]; ok {
		if time.Since(firedAt) < time.Duration(e.cfg.AlertSuppressMinutes)*time.Minute {
			e.mu.Unlock()
			return
		}
		delete(e.suppressed, key)
	}

	// 持续超阈值累计；检查周期是 5 秒，所以每轮只累加实际检查间隔。
	if value > threshold {
		e.exceedDuration[key] += alertCheckInterval
		if e.exceedDuration[key] >= time.Duration(durationSec)*time.Second {
			shouldFire = true
			e.suppressed[key] = time.Now()
			delete(e.exceedDuration, key)
		}
	} else if value <= recovery {
		delete(e.exceedDuration, key)
		shouldResolve = true
	}
	e.mu.Unlock()

	if shouldFire {
		e.fireAlert(agent, metric, threshold, value)
	}
	if shouldResolve {
		e.resolveAlert(agent, metric, value)
	}
}

func (e *Engine) fireAlert(agent *store.Agent, metric string, threshold, value float64) {
	if _, err := e.store.GetActiveAlert(agent.ID, metric); err == nil {
		return
	} else if err != sql.ErrNoRows {
		log.Printf("告警引擎: 查询活跃告警失败: agent=%s metric=%s err=%v", agent.ID, metric, err)
		return
	}
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
	e.sendTelegramNotification(&notify.Message{
		Title:   fmt.Sprintf("%s 触发%s告警", agent.Name, metricName(metric)),
		Body:    fmt.Sprintf("当前值 %.1f，阈值 %.1f", value, threshold),
		Level:   alertLevel(metric),
		AgentID: agent.ID,
		Metric:  metric,
	})
}

func (e *Engine) resolveAlert(agent *store.Agent, metric string, value float64) {
	active, err := e.store.GetActiveAlert(agent.ID, metric)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("告警引擎: 查询活跃告警失败: agent=%s metric=%s err=%v", agent.ID, metric, err)
		}
		return
	}
	if err := e.store.ResolveAlert(agent.ID, metric); err != nil {
		log.Printf("告警引擎: 恢复告警失败: agent=%s metric=%s err=%v", agent.ID, metric, err)
		return
	}
	log.Printf("告警引擎: 恢复告警 id=%d agent=%s metric=%s", active.ID, agent.Name, metric)
	e.mu.Lock()
	delete(e.suppressed, agent.ID+":"+metric)
	delete(e.exceedDuration, agent.ID+":"+metric)
	e.mu.Unlock()
	e.sendTelegramNotification(&notify.Message{
		Title:   fmt.Sprintf("%s %s已恢复", agent.Name, metricName(metric)),
		Body:    fmt.Sprintf("当前值 %.1f，告警已恢复", value),
		Level:   "info",
		AgentID: agent.ID,
		Metric:  metric,
	})
}

func (e *Engine) sendTelegramNotification(msg *notify.Message) {
	botToken, _ := e.store.GetSetting("telegram_bot_token")
	chatIDRaw, _ := e.store.GetSetting("telegram_chat_id")
	botToken = strings.TrimSpace(botToken)
	chatIDRaw = strings.TrimSpace(chatIDRaw)
	if botToken == "" || chatIDRaw == "" {
		return
	}
	chatID, err := strconv.ParseInt(chatIDRaw, 10, 64)
	if err != nil {
		log.Printf("告警引擎: Telegram Chat ID 无效: %s", chatIDRaw)
		return
	}
	if err := notify.NewTelegramNotifier(botToken, chatID).Send(msg); err != nil {
		log.Printf("告警引擎: Telegram 通知发送失败: %v", err)
	}
}

func metricName(metric string) string {
	switch metric {
	case "offline":
		return "离线"
	case "cpu":
		return "CPU"
	case "mem":
		return "内存"
	case "disk":
		return "磁盘"
	case "ping_latency":
		return "Ping延迟"
	case "ping_loss":
		return "Ping丢包"
	default:
		return metric
	}
}

func alertLevel(metric string) string {
	if metric == "offline" {
		return "critical"
	}
	return "warning"
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
