// 告警合并器（节流 + 聚合）
// 把短时间窗口内的多条告警合成一条推送，避免撞渠道侧的条数上限
//
// 为什么必须有这一层：
//  1. 微信 ClawBot 官方限制"每下发 10 条消息需用户主动在微信里发一条消息激活"，
//     且每 24 小时也要一次主动对话，否则推送直接失败；
//  2. pushplus 自身还有硬红线：实名用户 1 分钟 5 次、相同内容 1 小时 3 条、
//     单日超 1000 次请求封号 7 天（返回码 900）；
//  3. 本项目采集频率 1 秒、告警检查 5 秒一轮，一次机房网络抖动就可能几十个节点同时超阈，
//     逐条推送必然瞬间打满配额，结果是"最关键的告警反而发不出去"。
//
// 策略（对齐 Alertmanager 的 group_wait / group_interval 语义）：
//   - 空闲时第一条立即发出，保证 critical 的即时性；
//   - 发出后进入合并窗口（默认 5 分钟），窗口内新到的告警只缓存不发送；
//   - 窗口结束时若缓存非空，合并成一条汇总发出，并重新开一个窗口
//     （持续风暴时表现为"每 5 分钟一条汇总"，而不是每来一条发一条）。
package notify

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// 汇总消息里最多列出的明细条数，超出部分折叠成"另有 N 条"
const aggregatorMaxLines = 10

// 缓存上限：极端风暴下防止 pending 无限增长，超出只计数不缓存
const aggregatorMaxPending = 200

// AlertAggregator 告警合并器
type AlertAggregator struct {
	// windowFunc 每次开新窗口时回调读取最新窗口时长。
	// 原因：窗口分钟数存在 SQLite 设置里、后台可改，用回调而不是固定值才能热生效。
	windowFunc func() time.Duration
	// accept 判定这条消息是否值得推送到该渠道（级别/种类过滤）
	accept func(*Message) bool
	// sender 真正的发送动作，由调用方注入（内部负责读配置、构造 Notifier、重试）
	sender func(*Message)

	mu      sync.Mutex
	pending []*Message
	timer   *time.Timer
	closed  bool
	dropped int // 因超出缓存上限被丢弃的条数，汇总时如实告知
}

// NewAlertAggregator 构造合并器。windowFunc/accept/sender 均不可为 nil，
// 由调用方保证（这里不做 nil 校验是为了避免在每个发送路径上反复判断，
// 传错会在启动后第一次 Submit 就 panic 暴露问题，比静默不推送更好排查）。
func NewAlertAggregator(windowFunc func() time.Duration, accept func(*Message) bool, sender func(*Message)) *AlertAggregator {
	return &AlertAggregator{
		windowFunc: windowFunc,
		accept:     accept,
		sender:     sender,
	}
}

// Submit 提交一条告警：空闲则立即发送，窗口期内则进缓存等待合并。
// 非阻塞：真正的 HTTP 发送放到 goroutine，避免拖慢告警引擎 5 秒一轮的检查节奏。
func (a *AlertAggregator) Submit(msg *Message) {
	if msg == nil {
		return
	}
	if !a.accept(msg) {
		return
	}

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	// 已有窗口在跑：本条进缓存，等窗口结束一起发
	if a.timer != nil {
		if len(a.pending) < aggregatorMaxPending {
			a.pending = append(a.pending, msg)
		} else {
			a.dropped++
		}
		a.mu.Unlock()
		return
	}
	// 空闲状态：立即发这一条，并开启合并窗口
	a.timer = time.AfterFunc(a.window(), a.flush)
	a.mu.Unlock()

	go a.sender(msg)
}

// Close 停止合并器并把缓存里的告警立即合并发出。
// 原因：主控退出时不能把用户还没收到的告警丢掉，做一次同步 flush。
func (a *AlertAggregator) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	buffered := a.pending
	a.pending = nil
	a.mu.Unlock()

	if len(buffered) == 0 {
		return
	}
	// 这里同步发送：进程正在退出，再起 goroutine 可能来不及跑完就被终止
	a.sender(a.merge(buffered, a.droppedCount()))
}

// droppedCount 读取当前因超出缓存上限而未发送的条数（Close 路径合并时用）
func (a *AlertAggregator) droppedCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}

// window 读取当前合并窗口时长，非法值兜底为 5 分钟
func (a *AlertAggregator) window() time.Duration {
	d := a.windowFunc()
	if d <= 0 {
		return 5 * time.Minute
	}
	return d
}

// flush 窗口结束回调：把缓存合并成一条发出，并按需续开窗口
func (a *AlertAggregator) flush() {
	a.mu.Lock()
	buffered := a.pending
	dropped := a.dropped
	a.pending = nil
	a.dropped = 0
	a.timer = nil
	closed := a.closed
	a.mu.Unlock()

	if closed {
		return
	}
	// 窗口内一条都没有：回到空闲，下一条告警会再次立即发出
	if len(buffered) == 0 {
		return
	}

	merged := a.merge(buffered, dropped)
	// 缓存里还有（合并期间新到的）就续开窗口，保证风暴期按窗口节奏输出而不是逐条轰炸
	a.mu.Lock()
	if len(a.pending) > 0 {
		a.timer = time.AfterFunc(a.window(), a.flush)
	}
	a.mu.Unlock()

	go a.sender(merged)
}

// merge 把多条告警合成一条汇总消息。dropped 是因超出缓存上限而未列出的条数。
func (a *AlertAggregator) merge(items []*Message, dropped int) *Message {
	level := "info"
	lines := make([]string, 0, len(items))
	for i, m := range items {
		if m.Level == "critical" {
			level = "critical"
		} else if m.Level == "warning" && level != "critical" {
			level = "warning"
		}
		if i >= aggregatorMaxLines {
			continue
		}
		line := m.Title
		if strings.TrimSpace(m.Body) != "" {
			line = fmt.Sprintf("%s（%s）", m.Title, strings.TrimSpace(m.Body))
		}
		lines = append(lines, "· "+line)
	}

	title := fmt.Sprintf("wukong 告警汇总（%d 条）", len(items))
	if len(items) > aggregatorMaxLines {
		lines = append(lines, fmt.Sprintf("… 另有 %d 条已合并，请登录后台查看", len(items)-aggregatorMaxLines))
	}
	// 超出缓存上限的部分不隐盖，否则用户会以为告警只有这么多
	if dropped > 0 {
		lines = append(lines, fmt.Sprintf("… 另有 %d 条因短时告警过多未缓存", dropped))
	}

	icon := "ℹ️"
	switch level {
	case "warning":
		icon = "⚠️"
	case "critical":
		icon = "🔴"
	}

	return &Message{
		Title:  title,
		Body:   icon + "\n" + strings.Join(lines, "\n"),
		Level:  level,
		Kind:   "summary",
		Metric: "batch",
	}
}
