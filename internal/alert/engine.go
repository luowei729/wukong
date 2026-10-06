// 告警引擎
// 6 类指标阈值判定 + 三级回退 + 持续超阈值判定 + 滞回防抖 + 去重抑制 + 恢复通知 + 静默窗口
package alert

import (
	"context"
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
	// pushplusAgg 微信 ClawBot 等 pushplus 渠道的合并节流器。
	// 原因：这类渠道有严格条数上限（ClawBot 每 10 条要人工激活，pushplus 自身还有分钟/日频次红线），
	// 逐条推送会在告警风暴时瞬间打满配额，导致最关键的告警反而发不出去。
	pushplusAgg *notify.AlertAggregator
	// ruleEnabled 记录上一轮各项规则的开关，用于捕捉"启用→关闭"的瞬间。
	// 原因：关掉某项告警后引擎不再检查它，如果不清理，已存在的 firing 记录会永远挂在告警中心。
	ruleEnabled map[string]bool
}

func NewEngine(s store.MetricsStore, cfg *config.ServerConfig) *Engine {
	e := &Engine{
		store:          s,
		cfg:            cfg,
		suppressed:     make(map[string]time.Time),
		exceedDuration: make(map[string]time.Duration),
		silencedAgents: make(map[string]struct{}),
		silencedGroups: make(map[string]struct{}),
		ruleEnabled:    make(map[string]bool),
	}
	// 合并器的三个回调都指向 Engine 方法：
	// 窗口时长和发送动作都要在发送当时读最新设置，后台改了立即生效，不需要重启主控。
	e.pushplusAgg = notify.NewAlertAggregator(e.pushplusWindow, pushplusAccept, e.sendPushplus)
	return e
}

// pushplusWindow 读取合并窗口分钟数（默认 5 分钟），并限制在 1~60 分钟。
// 原因：填 0 或负数会让每条告警都立即发送，失去节流意义；
// 超过 1 小时则告警压得太久，运维价值已经消失，所以两侧都兜底。
func (e *Engine) pushplusWindow() time.Duration {
	minutes := e.settingInt("pushplus_merge_minutes", 5)
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 60 {
		minutes = 60
	}
	return time.Duration(minutes) * time.Minute
}

// pushplusAccept 只推关键消息：warning/critical 的触发 + 全部恢复通知。
// 原因：ClawBot 每 10 条需要用户在微信里主动发一条消息激活，配额必须留给真正需要人处理的消息；
// 恢复通知的 Level 是 info，但"已恢复"直接决定用户要不要继续排查，所以按 Kind 放行。
func pushplusAccept(msg *notify.Message) bool {
	if msg.Kind == "resolved" || msg.Kind == "summary" {
		return true
	}
	return msg.Level == "warning" || msg.Level == "critical"
}

// settingString 读取字符串型设置，出错一律当空处理（推送链路不能因为读配置失败而 panic）
func (e *Engine) settingString(key string) string {
	value, err := e.store.GetSetting(key)
	if err != nil {
		return ""
	}
	return value
}

func (e *Engine) settingBool(key string, fallback bool) bool {
	value, err := e.store.GetSetting(key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	v, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return v
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

// Run 定时检查告警状态，支持 context 取消以实现优雅退出。
// 原因：旧代码 Run() 无 context 参数，主控退出时无法等待告警引擎停止，
// 可能导致正在发送的 Telegram 通知被强制中断。
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(alertCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 退出前把合并窗口里缓存的告警一次性发出，不能因为进程结束而静默丢失
			if e.pushplusAgg != nil {
				e.pushplusAgg.Close()
			}
			log.Println("告警引擎已停止")
			return
		case <-ticker.C:
			e.checkAlerts()
		}
	}
}

func (e *Engine) checkAlerts() {
	agents, err := e.store.ListAgents()
	if err != nil {
		log.Printf("告警引擎: 获取探针列表失败: %v", err)
		return
	}

	// 每轮开头读一次规则（6 条 setting，开销远小于逐节点重复读）。
	// 从 SQLite 而不是配置文件取值：后台改完下一轮（≤5s）就生效，不需要重启主控。
	rules := LoadRules(e.store, e.cfg.AlertSuppressMinutes)
	for _, spec := range RuleSpecs() {
		rule := rules[spec.Metric]
		prev, seen := e.ruleEnabled[spec.Metric]
		e.ruleEnabled[spec.Metric] = rule.Enabled
		if !rule.Enabled {
			// 只在"启用→关闭"这一次做清理，避免每 5 秒重复扫活跃告警
			if seen && prev {
				e.resolveMetricAlerts(spec.Metric)
			}
		}
	}

	for _, agent := range agents {
		if !agent.Online {
			if rules[MetricOffline].Enabled {
				e.handleOffline(agent, rules[MetricOffline])
			}
			continue
		}

		// 节点恢复在线时，必须恢复 offline 告警并发送恢复通知。
		// 即使离线告警被关掉也要清理，否则记录会永远挂在 firing。
		e.resolveAlert(agent, "offline", 0)

		// 获取最新指标
		metrics, err := e.store.GetLatestMetrics(agent.ID)
		if err != nil {
			continue
		}

		// 资源类指标：每项用自己的规则（阈值/持续/滞回/抑制期），关掉就完全跳过
		if rules[MetricCPU].Enabled {
			e.checkMetric(agent, "cpu", metrics.CPU, rules[MetricCPU])
		}
		if rules[MetricMem].Enabled {
			e.checkMetric(agent, "mem", metrics.Mem, rules[MetricMem])
		}
		if rules[MetricDisk].Enabled {
			e.checkMetric(agent, "disk", metrics.Disk, rules[MetricDisk])
		}
		e.checkPingMetrics(agent, rules)
	}
}

// resolveMetricAlerts 在某项告警被关闭时，静默清理它遗留的 firing 记录。
// 不发恢复通知的原因：一次开关可能涉及十几个节点，全部推送会瞬间打爆
// 微信 ClawBot 的条数配额，而且"用户自己关掉的规则"本来也不需要通知。
func (e *Engine) resolveMetricAlerts(metric string) {
	active, err := e.store.ListActiveAlerts()
	if err != nil {
		log.Printf("告警引擎: 清理已关闭告警 %s 时查询活跃记录失败: %v", metric, err)
		return
	}
	count := 0
	for _, alert := range active {
		if alert == nil || alert.Metric != metric {
			continue
		}
		if err := e.store.ResolveAlert(alert.AgentID, metric); err != nil {
			log.Printf("告警引擎: 清理告警记录失败 agent=%s metric=%s err=%v", alert.AgentID, metric, err)
			continue
		}
		// 同步清掉内存状态，重新开启时不会被旧抑制期挡住
		key := alert.AgentID + ":" + metric
		e.mu.Lock()
		delete(e.suppressed, key)
		delete(e.exceedDuration, key)
		e.mu.Unlock()
		count++
	}
	if count > 0 {
		log.Printf("告警引擎: 已关闭 %s 告警，静默清理 %d 条活跃记录", metric, count)
	}
}

func (e *Engine) handleOffline(agent *store.Agent, rule Rule) {
	key := agent.ID + ":offline"
	// 离线告警的抑制期也改成该项自己的配置，不再共用一个全局值
	suppressWindow := time.Duration(rule.SuppressMin) * time.Minute
	e.mu.RLock()
	firedAt, suppressed := e.suppressed[key]
	e.mu.RUnlock()
	if suppressed && time.Since(firedAt) < suppressWindow {
		return
	}
	if suppressed {
		// 抑制期过后允许再次触发离线告警，避免节点长期离线却永远只报一次。
		e.mu.Lock()
		delete(e.suppressed, key)
		e.mu.Unlock()
	}

	// 离线阈值就是"最后心跳超过多少秒算离线"，按后台配置判断，避免节点刚短暂重连就立刻报警。
	offlineSeconds := int(rule.Warning)
	if offlineSeconds <= 0 {
		offlineSeconds = e.cfg.HeartbeatTimeout
	}
	if agent.LastSeenAt != nil && time.Since(*agent.LastSeenAt) < time.Duration(offlineSeconds)*time.Second {
		return
	}
	log.Printf("告警引擎: 探针 %s(%s) 离线超过 %d 秒", agent.Name, agent.ID, offlineSeconds)
	e.fireAlert(agent, "offline", float64(offlineSeconds), 1)
	e.mu.Lock()
	e.suppressed[key] = time.Now()
	e.mu.Unlock()
}

func (e *Engine) checkPingMetrics(agent *store.Agent, rules map[string]Rule) {
	latencyRule := rules[MetricPingLatency]
	lossRule := rules[MetricPingLoss]
	// 两项都关掉时直接返回，连 ISP 目标列表和聚合查询都不用发
	if !latencyRule.Enabled && !lossRule.Enabled {
		return
	}
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
	// 延迟与丢包各自用自己的规则：旧版共用资源告警的持续时间、滞回还是写成阈值乘系数，
	// 现在两项参数完全独立（滞回默认 150ms / 5%，均可在后台改）
	if latencyRule.Enabled {
		e.checkMetric(agent, "ping_latency", worstLatency, latencyRule)
	}
	if lossRule.Enabled {
		e.checkMetric(agent, "ping_loss", worstLoss, lossRule)
	}
}

func (e *Engine) checkMetric(agent *store.Agent, metric string, value float64, rule Rule) {
	key := agent.ID + ":" + metric
	threshold := rule.Warning
	recovery := rule.Recovery
	durationSec := rule.Duration
	shouldFire := false
	shouldResolve := false

	e.mu.Lock()
	// 抑制期只用于压制“重复触发/重复通知”，绝不能跳过恢复判断。
	// 旧实现在抑制期内直接 return，导致改了运营商目标、指标早已回落到 0% 丢包后，
	// 告警记录仍挂 firing 最长 30 分钟（用户反馈的“改了 IP 还在报警”）。
	// 抑制期改成按该项规则取值，不同告警可以有不同的防抖节奏。
	suppressed := false
	if firedAt, ok := e.suppressed[key]; ok {
		if time.Since(firedAt) < time.Duration(rule.SuppressMin)*time.Minute {
			suppressed = true
		} else {
			delete(e.suppressed, key)
		}
	}

	// 持续超阈值累计；检查周期是 5 秒，所以每轮只累加实际检查间隔。
	if value > threshold {
		if suppressed {
			// 仍在抑制期：不重复告警，也不累计持续时间，避免解除瞬间误触发
			e.mu.Unlock()
			return
		}
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
	e.notifyChannels(&notify.Message{
		Title:   fmt.Sprintf("%s 触发%s告警", agent.Name, metricName(metric)),
		Body:    fmt.Sprintf("当前值 %.1f，阈值 %.1f", value, threshold),
		Level:   alertLevel(metric),
		AgentID: agent.ID,
		Metric:  metric,
		Kind:    "firing", // 触发类：pushplus 渠道按级别判定是否推送
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
	e.notifyChannels(&notify.Message{
		Title:   fmt.Sprintf("%s %s已恢复", agent.Name, metricName(metric)),
		Body:    fmt.Sprintf("当前值 %.1f，告警已恢复", value),
		Level:   "info",
		AgentID: agent.ID,
		Metric:  metric,
		Kind:    "resolved", // 恢复通知：不受级别过滤，pushplus 渠道必须送达
	})
}

// notifyChannels 把一条告警分发给所有已配置渠道。
// 原因：原来只有 sendTelegramNotification 一个硬出口，加新渠道就要改每个调用点；
// 收敛成一个分发函数后渠道之间互不影响：Telegram 保持逐条即时，pushplus 走合并节流。
func (e *Engine) notifyChannels(msg *notify.Message) {
	e.sendTelegramNotification(msg)
	if e.pushplusAgg != nil {
		e.pushplusAgg.Submit(msg)
	}
}

// sendPushplus 通过 pushplus 中转推送（微信 ClawBot / 公众号 / 企业微信应用等）。
// 每次发送时才读设置：渠道、令牌、站点域名在后台改完立即生效，不需要重启主控。
func (e *Engine) sendPushplus(msg *notify.Message) {
	token := strings.TrimSpace(e.settingString("pushplus_token"))
	if token == "" {
		return
	}
	// 未启用时直接跳过：设置页允许先存令牌再开关试推
	if !e.settingBool("pushplus_enabled", false) {
		return
	}
	channel := strings.TrimSpace(e.settingString("pushplus_channel"))
	n := notify.NewPushplusNotifier(token, channel)
	// 详情链接依赖 site_domain；未配置时 Notifier 内部会自动不输出链接
	n.SiteURL = strings.TrimSpace(e.settingString("site_domain"))
	if err := notify.SendWithRetry(n, msg); err != nil {
		log.Printf("告警引擎: %s 通知发送失败: %v", n.Name(), err)
	}
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
