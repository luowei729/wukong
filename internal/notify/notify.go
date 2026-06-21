// 通知管理器
// Notifier 接口 + Telegram 实现 + 预留多渠道扩展
package notify

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// Send 遍历所有渠道发送
func (m *Manager) Send(msg *Message) {
	for _, n := range m.notifiers {
		if err := n.Send(msg); err != nil {
			log.Printf("通知渠道 %s 发送失败: %v", n.Name(), err)
		}
	}
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
		client:   &http.Client{Timeout: 10},
	}
}

func (t *TelegramNotifier) Name() string {
	return "telegram"
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
		return fmt.Errorf("Telegram API 返回非 200: %d", resp.StatusCode)
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
	// 使用 http.Post 发送
	log.Printf("Webhook 通知: %s", body)
	return nil
}