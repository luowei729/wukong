// 通知管理器
// Notifier 接口 + Telegram 实现（含重试）+ 预留多渠道扩展
package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 通知重试参数
const (
	maxRetries     = 3               // 最大重试次数
	retryBaseDelay = 2 * time.Second // 首次重试延迟
)

// Notifier 通知渠道接口
type Notifier interface {
	// Send 发送通知
	Send(message *Message) error
	// Name 渠道名称
	Name() string
}

// Message 通知消息
type Message struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Level   string `json:"level"` // info/warning/critical
	AgentID string `json:"agent_id"`
	Metric  string `json:"metric"`
	// Kind 消息种类：firing（触发）/ resolved（恢复）。
	// 原因：微信 ClawBot 这类渠道有严格条数上限，必须按"级别 + 种类"过滤；
	// 恢复通知的 Level 是 info，但运维上必须送达，只按 Level 判断会把它误杀。
	Kind string `json:"kind"`
}

// retryableError 渠道错误可以自行声明"这次失败到底值不值得重试"。
// 原因：pushplus 的 900（账号受限）/903（无效令牌）/905（未实名）属于账号层面不可用，
// 官方明确说明继续请求会加重限制甚至封号，按 HTTP 状态码一律重试反而会适得其反。
type retryableError interface {
	Retryable() bool
}

// SendWithRetry 供外部（如告警引擎）复用的带退避重试发送。
// 原因：告警引擎是按渠道逐个构造 Notifier 的，不走 Manager 的注册列表，
// 但重试策略必须与 Manager 一致，否则各渠道的可靠性行为会分叉。
func SendWithRetry(n Notifier, msg *Message) error {
	return sendWithRetry(n, msg)
}

// Manager 通知管理器
type Manager struct {
	notifiers []Notifier
}

func NewManager() *Manager {
	return &Manager{
		notifiers: make([]Notifier, 0),
	}
}

// Register 注册通知渠道
func (m *Manager) Register(n Notifier) {
	m.notifiers = append(m.notifiers, n)
	log.Printf("通知渠道已注册: %s", n.Name())
}

// Send 遍历所有渠道发送（含重试）
// 原因：Telegram API 可能因网络抖动或限流返回 429/5xx，单次失败就丢弃通知会导致关键告警漏报。
func (m *Manager) Send(msg *Message) {
	for _, n := range m.notifiers {
		if err := sendWithRetry(n, msg); err != nil {
			log.Printf("通知渠道 %s 发送失败（已重试 %d 次）: %v", n.Name(), maxRetries, err)
		}
	}
}

// sendWithRetry 带指数退避重试的通知发送。
// 原因：网络瞬断或 Telegram 限流（429）是暂时性错误，重试可大幅提高通知到达率。
// 退避策略：2s → 4s → 8s，避免在限流期间密集请求加剧问题。
func sendWithRetry(n Notifier, msg *Message) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1)) // 指数退避: 2s, 4s, 8s
			log.Printf("通知渠道 %s 第 %d 次重试（延迟 %v）: %s", n.Name(), attempt, delay, msg.Title)
			time.Sleep(delay)
		}
		lastErr = n.Send(msg)
		if lastErr == nil {
			return nil
		}
		// 渠道自己声明不可重试时立即返回：例如 pushplus 返回账号受限/令牌无效，
		// 再打三次只会加重账号限制，不如直接失败并记日志。
		if re, ok := lastErr.(retryableError); ok && !re.Retryable() {
			return lastErr
		}
		// 如果是 4xx 客户端错误（除 429 限流外），不重试，因为请求本身有问题
		if isClientError(lastErr) && !isRateLimitError(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

// httpStatusOf 从各渠道错误类型里取出 HTTP 语义状态码。
// 原因：新增渠道（pushplus 用业务码而非 HTTP 码表达失败）时不必再改重试判定，
// 只要错误类型实现 HTTPStatus() 即可复用同一套退避逻辑。
func httpStatusOf(err error) (int, bool) {
	if te, ok := err.(*telegramAPIError); ok {
		return te.statusCode, true
	}
	type statuser interface{ HTTPStatus() int }
	if se, ok := err.(statuser); ok {
		return se.HTTPStatus(), true
	}
	return 0, false
}

// isClientError 判断是否为 4xx 客户端错误（不需要重试）
func isClientError(err error) bool {
	code, ok := httpStatusOf(err)
	return ok && code >= 400 && code < 500
}

// isRateLimitError 判断是否为 429 限流错误（需要重试）
func isRateLimitError(err error) bool {
	code, ok := httpStatusOf(err)
	return ok && code == 429
}

// TelegramNotifier Telegram 通知渠道
type TelegramNotifier struct {
	BotToken string
	ChatID   int64
	client   *http.Client
}

func NewTelegramNotifier(botToken string, chatID int64) *TelegramNotifier {
	return &TelegramNotifier{
		BotToken: botToken,
		ChatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TelegramNotifier) Name() string {
	return "telegram"
}

// telegramAPIError Telegram API 返回的错误（携带状态码，用于重试判断）
type telegramAPIError struct {
	statusCode int
	body       string
}

func (e *telegramAPIError) Error() string {
	return fmt.Sprintf("Telegram API 返回 %d: %s", e.statusCode, e.body)
}

// HTTPStatus 让 Telegram 错误复用 notify 包统一的 4xx/429 判定
func (e *telegramAPIError) HTTPStatus() int {
	return e.statusCode
}

func (t *TelegramNotifier) Send(msg *Message) error {
	// 构建 Telegram 消息
	text := t.formatMessage(msg)
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)

	form := url.Values{}
	form.Set("chat_id", fmt.Sprintf("%d", t.ChatID))
	form.Set("text", text)
	form.Set("parse_mode", "Markdown")

	resp, err := t.client.PostForm(apiURL, form)
	if err != nil {
		return fmt.Errorf("发送 Telegram 消息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &telegramAPIError{
			statusCode: resp.StatusCode,
			body:       strings.TrimSpace(string(body)),
		}
	}
	return nil
}

// formatMessage 构造 Telegram 消息格式
func (t *TelegramNotifier) formatMessage(msg *Message) string {
	// 根据级别选择图标
	icon := "ℹ️"
	switch msg.Level {
	case "warning":
		icon = "⚠️"
	case "critical":
		icon = "🔴"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s *[%s]* %s\n", icon, strings.ToUpper(msg.Level), msg.Title))
	if msg.Body != "" {
		sb.WriteString(fmt.Sprintf("详情: %s\n", msg.Body))
	}
	if msg.AgentID != "" {
		sb.WriteString(fmt.Sprintf("探针: `%s`\n", msg.AgentID))
	}
	return sb.String()
}

// WebhookNotifier Webhook 通知渠道（预留）
type WebhookNotifier struct {
	URL    string
	Secret string
}

func NewWebhookNotifier(url, secret string) *WebhookNotifier {
	return &WebhookNotifier{
		URL:    url,
		Secret: secret,
	}
}

func (w *WebhookNotifier) Name() string { return "webhook" }

func (w *WebhookNotifier) Send(msg *Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 实际发送 HTTP POST 请求到 Webhook URL。
	// 原因：旧代码只打印日志不发送请求，Webhook 通知形同虚设。
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", w.URL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("构建 Webhook 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// 如果配置了 Secret，通过 X-Webhook-Secret 头传递，接收方可用于验签
	if w.Secret != "" {
		req.Header.Set("X-Webhook-Secret", w.Secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送 Webhook 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("Webhook 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	log.Printf("Webhook 通知发送成功: %s", msg.Title)
	return nil
}
