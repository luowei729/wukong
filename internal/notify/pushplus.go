// pushplus（推送加）通知渠道
// 通过 pushplus 中转把告警推送到微信 ClawBot / 微信公众号 / 企业微信应用 / QQ 等渠道
//
// 为什么走中转而不是直连微信 iLink ClawBot 协议：
// 直连需要自己维护扫码凭证与 context_token，并且要处理"每下发 10 条 / 每 24 小时
// 必须用户在微信里主动发一条消息"的激活续期，主控里要常驻一份脆弱的登录态。
// pushplus 已经实现了该协议，我们只需一次 HTTP POST，失败面清晰、渠道还能随时切换。
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

// pushplus 默认服务地址（官方文档写的是 http，这里用 https 避免令牌在公网明文传输）
const defaultPushplusBase = "https://www.pushplus.plus"

// pushplus 业务返回码（官方 /doc/guide/code.html）
const (
	pushplusCodeOK          = 200 // 请求受理成功（注意：只代表服务端收到，不代表微信侧送达）
	pushplusCodeSystemError = 500 // 系统异常，稍后可重试
	pushplusCodeDataError   = 600 // 数据异常，操作失败，可能是服务端瞬时问题
)

// PushplusNotifier pushplus 中转渠道实现
type PushplusNotifier struct {
	Token    string // pushplus 用户令牌（官网"我的凭证"里那一串）
	Channel  string // 发送渠道：clawbot / wechat / cp / qq / cmcc，空则 wechat
	Template string // 消息模板，ClawBot 只支持纯文本，固定用 txt
	Topic    string // 群组编码，留空只发给自己
	BaseURL  string // 接口地址，默认 https://www.pushplus.plus
	SiteURL  string // 主控站点地址，用于在消息里拼节点详情链接
	client   *http.Client
}

// NewPushplusNotifier 构造 pushplus 渠道，并对空字段填默认值。
// 原因：设置页允许用户只填 token 就能用，渠道和模板这些细节给合理默认，减少配置面。
func NewPushplusNotifier(token, channel string) *PushplusNotifier {
	ch := strings.TrimSpace(channel)
	if ch == "" {
		ch = "clawbot"
	}
	return &PushplusNotifier{
		Token:    strings.TrimSpace(token),
		Channel:  ch,
		Template: "txt",
		BaseURL:  defaultPushplusBase,
		// 10 秒超时：告警引擎每 5 秒 tick 一次，推送不能长时间占用发送协程
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 渠道名带上具体渠道，便于日志区分"pushplus 的 clawbot"和"pushplus 的公众号"
func (p *PushplusNotifier) Name() string {
	return "pushplus:" + p.Channel
}

// pushplusError pushplus 返回的业务错误
type pushplusError struct {
	Code int    // 业务返回码
	Body string // 原始响应片段，便于排查官方未文档化的码值
}

func (e *pushplusError) Error() string {
	return fmt.Sprintf("pushplus 返回 %d: %s", e.Code, e.Body)
}

// Retryable 只有服务端瞬时错误才值得重试。
// 原因：900（账号使用受限）、903（无效令牌）、905（未实名）、888（积分不足）都是账号/配置问题，
// 官方明确说明"请求次数过多时继续请求会加重限制"（单日超 1000 次封 7 天），
// 对这类错误重试不仅无用，还会把账号推向更长的封禁期。
func (e *pushplusError) Retryable() bool {
	return e.Code == pushplusCodeSystemError || e.Code == pushplusCodeDataError
}

// HTTPStatus 把业务码映射成 HTTP 语义，复用 notify 包统一的 4xx/429 重试判定
func (e *pushplusError) HTTPStatus() int {
	if e.Retryable() {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

// Send 调用 pushplus 发送接口。
// 注意：接口是异步的，code=200 只代表"服务端已受理并排队"，最终是否送达微信侧
// 要用返回的消息流水号（shortCode）去查，或在 pushplus 公众号里确认。
// 因此这里把流水号打进日志，方便事后排查"日志说发成功了但微信没收到"。
func (p *PushplusNotifier) Send(msg *Message) error {
	if p.Token == "" {
		return fmt.Errorf("pushplus 令牌未配置")
	}

	payload := map[string]string{
		"token":    p.Token,
		"title":    msg.Title,
		"content":  p.buildContent(msg),
		"channel":  p.Channel,
		"template": p.Template,
	}
	// topic 只在配了群组编码时传：不填仅发给自己，填了会发给群组全部成员
	if strings.TrimSpace(p.Topic) != "" {
		payload["topic"] = strings.TrimSpace(p.Topic)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 pushplus 请求失败: %w", err)
	}
	endpoint := fmt.Sprintf("%s/send/%s", p.baseURL(), url.PathEscape(p.Token))

	resp, err := p.client.Post(endpoint, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("请求 pushplus 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		// HTTP 层错误：4xx 视为请求问题不重试，5xx/429 交给统一退避逻辑重试
		return &httpStatusError{statusCode: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}

	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data string `json:"data"` // 成功时为消息流水号 shortCode
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("解析 pushplus 响应失败: %v, 原始响应: %s", err, strings.TrimSpace(string(raw)))
	}
	if parsed.Code != pushplusCodeOK {
		return &pushplusError{Code: parsed.Code, Body: strings.TrimSpace(parsed.Msg)}
	}

	log.Printf("pushplus 通知已受理 渠道=%s 流水号=%s 标题=%s", p.Channel, parsed.Data, msg.Title)
	return nil
}

// baseURL 去掉尾部斜杠，避免拼出 //send/xxx
func (p *PushplusNotifier) baseURL() string {
	base := strings.TrimSpace(p.BaseURL)
	if base == "" {
		base = defaultPushplusBase
	}
	return strings.TrimRight(base, "/")
}

// buildContent 组装纯文本正文。
// 原因：微信 ClawBot 渠道只支持文字，官方也建议 template=txt（其他模板会被压成摘要，
// 详情要点链接看），所以这里刻意不用 HTML/Markdown 标记，保证在微信里完整可读。
func (p *PushplusNotifier) buildContent(msg *Message) string {
	var sb strings.Builder
	if strings.TrimSpace(msg.Body) != "" {
		sb.WriteString(strings.TrimSpace(msg.Body))
	}
	// 详情链接：ClawBot 里链接可点，是"看到告警→打开监控页"的唯一入口
	if link := p.detailLink(msg.AgentID); link != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("详情 ")
		sb.WriteString(link)
	}
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	sb.WriteString("时间 ")
	sb.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	return sb.String()
}

// detailLink 拼节点详情页地址；未配置 site_domain 时返回空串（不输出裸的 /server/xxx）
func (p *PushplusNotifier) detailLink(agentID string) string {
	site := strings.TrimRight(strings.TrimSpace(p.SiteURL), "/")
	if site == "" || strings.TrimSpace(agentID) == "" {
		return ""
	}
	return site + "/server/" + strings.TrimSpace(agentID)
}

// httpStatusError HTTP 层错误（pushplus 返回非 200 状态码时使用）
type httpStatusError struct {
	statusCode int
	body       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("pushplus HTTP %d: %s", e.statusCode, e.body)
}

// HTTPStatus 供 notify 包统一判定：5xx/429 重试，其余 4xx 不重试
func (e *httpStatusError) HTTPStatus() int {
	return e.statusCode
}
