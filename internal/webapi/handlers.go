// Web API 路由处理函数
// 所有请求处理的具体实现
package webapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wukong/internal/auth"
	"wukong/internal/notify"
	"wukong/internal/store"

	"golang.org/x/crypto/bcrypt"
)

// ---- 辅助函数 ----

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func getPathValue(r *http.Request, key string) string {
	// 从 URL 路径参数中取值（Go 1.22 内置）
	return r.PathValue(key)
}

func parseIntParam(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// ---- 健康检查 ----

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "0.1.0"})
}

// ---- 鉴权相关 ----

// loadPersistentAdminPasswordHash 从 SQLite 读取已固化的管理员密码 hash。
// 原因：用户要求配置都写进数据库固化，修改密码后重启容器也必须继续使用新密码。
func (h *Handler) loadPersistentAdminPasswordHash() error {
	hash, err := h.store.GetSetting("admin_password_hash")
	if err != nil {
		return fmt.Errorf("读取管理员密码设置失败: %w", err)
	}
	if strings.TrimSpace(hash) != "" {
		h.cfg.AdminPasswordHash = hash
	}
	return nil
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// 解析管理员登录请求，前端登录和后续安装命令生成都依赖真实 JWT。
	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}

	// 登录前优先加载数据库固化密码，保证修改密码后即使主控重启也仍用最新 hash。
	if err := h.loadPersistentAdminPasswordHash(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员密码失败")
		return
	}

	// 使用 auth.Service 统一校验用户名、密码和可选 TOTP，避免在 Web 层重复鉴权逻辑。
	resp, err := h.authSvc.Authenticate(req.Username, req.Password, req.TOTPCode, getClientIP(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	// 刷新令牌：前端 access token 过期后，使用 refresh token 获取新的 access token。
	// 避免用户频繁重新输入密码，同时保持安全性。
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token 不能为空")
		return
	}

	// 只接受 refresh token 类型，防止攻击者用 access token 当作 refresh token 刷新。
	claims, err := h.authSvc.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, fmt.Sprintf("refresh token 无效: %v", err))
		return
	}

	// 重新生成令牌
	resp, err := h.authSvc.GenerateTokens(claims.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成新令牌失败")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}

	// 修改密码必须同时校验旧密码和新密码强度，避免已登录页面被他人临时操作后直接接管账号。
	req.OldPassword = strings.TrimSpace(req.OldPassword)
	if req.OldPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "当前密码和新密码不能为空")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "新密码至少需要 8 位")
		return
	}
	if req.OldPassword == req.NewPassword {
		writeError(w, http.StatusBadRequest, "新密码不能与当前密码相同")
		return
	}

	// 修改前先加载数据库里的最新 hash，防止用启动时的旧内存值覆盖已经固化的新密码。
	if err := h.loadPersistentAdminPasswordHash(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员密码失败")
		return
	}
	if h.cfg.AdminPasswordHash == "" {
		writeError(w, http.StatusInternalServerError, "管理员密码未设置")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(h.cfg.AdminPasswordHash), []byte(req.OldPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "当前密码错误")
		return
	}

	// bcrypt hash 写入 SQLite settings 表，随后同步内存配置，让新密码无需重启立即生效。
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成密码 hash 失败")
		return
	}
	if err := h.store.SetSetting("admin_password_hash", string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "保存管理员密码失败")
		return
	}
	h.cfg.AdminPasswordHash = string(hash)

	writeJSON(w, http.StatusOK, map[string]string{"message": "密码已修改"})
}

// ---- 中间件 ----

// authMiddleware JWT 鉴权中间件
func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 优先从 Authorization header 读取，SSE 场景回退到 ?token= 查询参数。
		// 原因：浏览器 EventSource API 不支持自定义 header，只能通过 URL 传 token。
		var tokenStr string
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			if tokenStr == authHeader {
				writeError(w, http.StatusUnauthorized, "Authorization 格式应为 Bearer <token>")
				return
			}
		} else {
			// 回退：从查询参数读取（SSE 场景）
			tokenStr = r.URL.Query().Get("token")
		}
		if tokenStr == "" {
			writeError(w, http.StatusUnauthorized, "需要 Authorization 头或 token 参数")
			return
		}

		// 只接受 access token 类型，防止 refresh token 被用于 API 访问
		claims, err := h.authSvc.ValidateAccessToken(tokenStr)
		if err != nil {
			writeError(w, http.StatusUnauthorized, fmt.Sprintf("token 无效: %v", err))
			return
		}

		// 将用户名注入请求上下文
		r.Header.Set("X-Username", claims.Username)
		next(w, r)
	}
}

// ---- 探针管理 ----

func (h *Handler) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询探针列表失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (h *Handler) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	agent, err := h.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("探针 %s 未找到", id))
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *Handler) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	var agent struct {
		Name        *string `json:"name"`
		GroupID     *string `json:"group_id"`
		CollectIntv *int    `json:"collect_intv"`
		PingIntv    *int    `json:"ping_intv"`
	}
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}

	// 获取当前探针
	existing, err := h.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("探针 %s 未找到", id))
		return
	}

	// 更新字段；采集和 Ping 频率会写入 SQLite 固化，新注册/重启探针后通过配置生效。
	if agent.Name != nil {
		name := strings.TrimSpace(*agent.Name)
		if name == "" || len([]rune(name)) > 64 {
			writeError(w, http.StatusBadRequest, "节点名称长度必须为 1-64 个字符")
			return
		}
		existing.Name = name
	}
	existing.GroupID = agent.GroupID
	if agent.CollectIntv != nil {
		if *agent.CollectIntv < 1 || *agent.CollectIntv > 3600 {
			writeError(w, http.StatusBadRequest, "采集频率必须在 1-3600 秒之间")
			return
		}
		existing.CollectIntv = agent.CollectIntv
	}
	if agent.PingIntv != nil {
		if *agent.PingIntv < 5 || *agent.PingIntv > 3600 {
			writeError(w, http.StatusBadRequest, "Ping 频率必须在 5-3600 秒之间")
			return
		}
		existing.PingIntv = agent.PingIntv
	}

	if err := h.store.UpdateAgent(existing); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("更新探针失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	if err := h.store.DeleteAgent(id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("删除探针失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已删除"})
}

func (h *Handler) handleGetAllLatestMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.store.GetAllLatestMetrics()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询最新指标失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// ---- 分组管理 ----

func (h *Handler) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.ListGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询分组失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *Handler) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	group, err := h.store.CreateGroup(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("创建分组失败: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (h *Handler) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	var group struct {
		Name           string `json:"name"`
		CollectIntv    *int   `json:"collect_intv"`
		PingIntv       *int   `json:"ping_intv"`
		TelegramConfID *int64 `json:"telegram_conf_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	existing, err := h.store.GetGroup(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("分组 %s 未找到", id))
		return
	}
	if group.Name != "" {
		existing.Name = group.Name
	}
	existing.CollectIntv = group.CollectIntv
	existing.PingIntv = group.PingIntv
	existing.TelegramConfID = group.TelegramConfID
	if err := h.store.UpdateGroup(existing); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("更新分组失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	if err := h.store.DeleteGroup(id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("删除分组失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已删除"})
}

// ---- ISP 管理 ----

func (h *Handler) handleListISPTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := h.store.ListISPTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询 ISP 目标失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (h *Handler) handleCreateISPTarget(w http.ResponseWriter, r *http.Request) {
	var target struct {
		Name    string `json:"name"`
		IP      string `json:"ip"`
		Port    int    `json:"port"`
		Mode    string `json:"mode"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	t := &store.ISPTarget{
		Name: target.Name, IP: target.IP, Port: target.Port,
		Mode: target.Mode, Enabled: target.Enabled,
	}
	if err := validateISPTarget(t); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := h.store.CreateISPTarget(t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("创建 ISP 目标失败: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *Handler) handleUpdateISPTarget(w http.ResponseWriter, r *http.Request) {
	idStr := getPathValue(r, "id")
	id, err := parseIntParam(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	var target store.ISPTarget
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	target.ID = id
	if err := validateISPTarget(&target); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.UpdateISPTarget(&target); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("更新 ISP 目标失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (h *Handler) handleDeleteISPTarget(w http.ResponseWriter, r *http.Request) {
	idStr := getPathValue(r, "id")
	id, err := parseIntParam(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := h.store.DeleteISPTarget(id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("删除 ISP 目标失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已删除"})
}

func validateISPTarget(target *store.ISPTarget) error {
	// 运营商目标会下发到探针执行网络探测，必须在入库前约束模式和端口范围。
	if target == nil {
		return fmt.Errorf("运营商目标不能为空")
	}
	target.Name = strings.TrimSpace(target.Name)
	target.IP = strings.TrimSpace(target.IP)
	target.Mode = strings.ToLower(strings.TrimSpace(target.Mode))
	if target.Name == "" {
		return fmt.Errorf("运营商名称不能为空")
	}
	if target.IP == "" {
		return fmt.Errorf("目标 IP 或域名不能为空")
	}
	if target.Mode == "" {
		target.Mode = "auto"
	}
	if target.Mode != "auto" && target.Mode != "icmp" && target.Mode != "tcp" {
		return fmt.Errorf("探测模式必须是 auto、icmp 或 tcp")
	}
	if target.Port == 0 {
		target.Port = 80
	}
	if target.Port < 1 || target.Port > 65535 {
		return fmt.Errorf("端口必须在 1-65535 之间")
	}
	return nil
}

// ---- 设置 ----

// allowedSettingKeys 允许通过通用 API 读写的 setting key 白名单。
// 原因：旧代码 handleGetSetting/handleSetSetting 接受任意 key，
// 攻击者可通过 /api/settings/admin_password_hash 读取或覆盖敏感配置。
var allowedSettingKeys = map[string]bool{
	"site_domain":                    true,
	"agent_server_addr":              true,
	"theme_preset":                   true,
	"theme_primary_color":            true,
	"theme_site_title":               true,
	"theme_footer_text":              true,
	"theme_logo_url":                 true,
	"telegram_bot_token":             true,
	"telegram_chat_id":               true,
	"alert_cpu_threshold":            true,
	"alert_mem_threshold":            true,
	"alert_disk_threshold":           true,
	"alert_offline_seconds":          true,
	"alert_ping_latency_threshold":   true,
	"alert_ping_loss_threshold":      true,
	"alert_metric_duration_seconds":  true,
}

func (h *Handler) handleGetSetting(w http.ResponseWriter, r *http.Request) {
	key := getPathValue(r, "key")
	// 白名单校验，拒绝读取未授权的敏感配置
	if !allowedSettingKeys[key] {
		writeError(w, http.StatusForbidden, fmt.Sprintf("不允许读取配置项: %s", key))
		return
	}
	value, err := h.store.GetSetting(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询设置失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

func (h *Handler) handleSetSetting(w http.ResponseWriter, r *http.Request) {
	key := getPathValue(r, "key")
	// 白名单校验，拒绝写入未授权的敏感配置
	if !allowedSettingKeys[key] {
		writeError(w, http.StatusForbidden, fmt.Sprintf("不允许写入配置项: %s", key))
		return
	}
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	if err := h.store.SetSetting(key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("设置失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已设置"})
}

// normalizeSiteBaseURL 将后台填写的站点域名统一为可访问的基础 URL。
// 原因：安装命令需要同时支持正式域名 https://example.com 和本地调试 http://127.0.0.1:64443。
func normalizeSiteBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("站点域名格式无效")
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// requestBaseURL 在未配置站点域名时按当前请求推导本地调试地址。
// 注意：后台生成可复制命令仍要求 site_domain；这里仅用于直接请求脚本时的兜底。
func requestBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

// agentServerAddr 将 HTTP 基础地址转换为探针 gRPC 连接地址。
// 原因：探针注册走 cmux 同端口 gRPC，命令行只需要 host:port，不需要 URL scheme。
func agentServerAddr(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("无法解析主控地址")
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "http" {
			host = net.JoinHostPort(host, "80")
		} else {
			host = net.JoinHostPort(host, "443")
		}
	}
	return host, nil
}

// normalizeAgentServerAddr 规范化探针 gRPC 连接地址。
// 原因：站点域名用于浏览器 HTTPS 访问，生产环境的 gRPC 可能走 64443 直连或单独的 nginx grpc_pass，不能总是从站点域名推导 443。
func normalizeAgentServerAddr(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("探针连接地址格式无效")
		}
		raw = u.Host
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return "", fmt.Errorf("探针连接地址必须是 host:port 格式")
	}
	return net.JoinHostPort(host, port), nil
}

// installAgentServerAddr 返回安装脚本实际写入的探针 gRPC 地址。
// 原因：优先使用数据库固化的 agent_server_addr；未配置时才回退到旧的 site_domain 推导，兼容本地单端口部署。
func (h *Handler) installAgentServerAddr(baseURL string) (string, error) {
	raw, err := h.store.GetSetting("agent_server_addr")
	if err != nil {
		return "", fmt.Errorf("读取探针连接地址失败")
	}
	addr, err := normalizeAgentServerAddr(raw)
	if err != nil {
		return "", err
	}
	if addr != "" {
		return addr, nil
	}
	return agentServerAddr(baseURL)
}

// ---- 安装 Token ----

func (h *Handler) handleCreateInstallToken(w http.ResponseWriter, r *http.Request) {
	// 安装 token 仍然一次性生成，但只有站点域名配置完整时才返回可复制命令。
	token, err := h.store.CreateInstallToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("生成安装 token 失败: %v", err))
		return
	}

	// site_domain 由后台设置页维护；未配置时不生成占位命令，避免用户复制后 token 丢失。
	domain, _ := h.store.GetSetting("site_domain")
	baseURL, err := normalizeSiteBaseURL(domain)
	if domain == "" || err != nil {
		message := "请先在系统设置中配置站点域名后再复制安装命令"
		if err != nil {
			message = "站点域名格式无效，请检查后再生成安装命令"
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"ready":      false,
			"token":      token,
			"script_url": "",
			"message":    message,
			"expires_in": "30分钟",
		})
		return
	}

	// 生成命令前先校验探针 gRPC 地址，避免复制后才发现 443 反代不支持 gRPC。
	serverAddr, err := h.installAgentServerAddr(baseURL)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"ready":      false,
			"token":      token,
			"script_url": "",
			"message":    "探针 gRPC 地址格式无效，请在系统设置中填写 host:port",
			"expires_in": "30分钟",
		})
		return
	}

	// token 必须放在脚本 URL 的查询参数中；curl -k 是 TLS 参数，不能用于传 token。
	scriptURL := fmt.Sprintf("curl -fsSL %q | bash", fmt.Sprintf("%s/api/install-agent.sh?k=%s", baseURL, url.QueryEscape(token)))
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"ready":             true,
		"token":             token,
		"script_url":        scriptURL,
		"agent_server_addr": serverAddr,
		"expires_in":        "30分钟",
	})
}

// ---- 安装脚本 ----

func (h *Handler) handleInstallAgentScript(w http.ResponseWriter, r *http.Request) {
	// 安装 token 必须通过 ?k= 传入，禁止生成空 TOKEN 脚本。
	token := strings.TrimSpace(r.URL.Query().Get("k"))
	if token == "" {
		writeError(w, http.StatusBadRequest, "缺少安装 token，请从后台重新生成安装命令")
		return
	}

	// 优先使用后台配置的站点域名；本地调试时可回退到请求 Host。
	domain, _ := h.store.GetSetting("site_domain")
	baseURL, err := normalizeSiteBaseURL(domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, "站点域名格式无效，请先在后台修正")
		return
	}
	if baseURL == "" {
		baseURL = requestBaseURL(r)
	}

	serverAddr, err := h.installAgentServerAddr(baseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法生成探针连接地址，请检查探针 gRPC 地址")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	script := fmt.Sprintf(`#!/bin/bash
# wukong 探针安装脚本
# 用法: curl -fsSL "%s/api/install-agent.sh?k=<token>" | bash
set -e

TOKEN=%q
BASE_URL=%q
SERVER_ADDR=%q
INSTALL_DIR="/opt/wukong/agent"
DATA_DIR="$INSTALL_DIR/data"

# 检测架构：同时支持 amd64 和 arm64 服务器节点，其他架构直接给出明确错误。
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "不支持的架构: $ARCH（当前仅支持 amd64 / arm64）"; exit 1 ;;
esac

# 创建目录
mkdir -p "$INSTALL_DIR" "$DATA_DIR"

# 下载探针二进制
# 已实现公开二进制下载接口，安装脚本会从 BASE_URL 下载当前镜像内置的探针。
echo "下载 wukong 探针..."
if ! curl -fsSL "$BASE_URL/api/agent/binary/latest/$ARCH" -o "$INSTALL_DIR/wukong-agent"; then
    echo "探针二进制下载失败，请手动复制 wukong-agent 后重新执行注册命令。"
    exit 1
fi
chmod +x "$INSTALL_DIR/wukong-agent"

# 注册探针
echo "注册到主控 $SERVER_ADDR ..."
"$INSTALL_DIR/wukong-agent" --server "$SERVER_ADDR" --token "$TOKEN"

# 安装 systemd 服务
cat > /etc/systemd/system/wukong-agent.service <<EOF
[Unit]
Description=wukong Agent - Server Monitor
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/wukong-agent --config $INSTALL_DIR/agent.conf
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=root
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now wukong-agent

echo "wukong 探针安装完成！"
`, baseURL, token, baseURL, serverAddr)

	w.Write([]byte(script))
}

func (h *Handler) handleInstallServerScript(w http.ResponseWriter, r *http.Request) {
	// 使用后台配置的站点域名替换脚本中的下载地址，避免硬编码占位符。
	// 原因：旧代码写死 https://<域名>/... 占位符，用户复制后无法直接使用。
	domain, _ := h.store.GetSetting("site_domain")
	baseURL, err := normalizeSiteBaseURL(domain)
	if err != nil || baseURL == "" {
		baseURL = requestBaseURL(r)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	script := fmt.Sprintf(`#!/bin/bash
# wukong 主控安装脚本
set -e

INSTALL_DIR="/opt/wukong"
DATA_DIR="$INSTALL_DIR/data"
SIGNING_DIR="$DATA_DIR/signing"

echo "安装 wukong 主控..."

mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$SIGNING_DIR" "$INSTALL_DIR/deploy/scripts"

# 下载主控二进制
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

curl -fsSL "%s/api/server/binary/latest/$ARCH" -o "$INSTALL_DIR/wukong"
chmod +x "$INSTALL_DIR/wukong"

# 创建默认配置
cat > "$INSTALL_DIR/wukong.conf" <<EOF
{
  "listen_addr": "127.0.0.1:64443",
  "data_dir": "$DATA_DIR",
  "db_path": "$DATA_DIR/wukong.db",
  "log_level": "info",
  "default_collect_interval": 5,
  "default_ping_interval": 60,
  "heartbeat_timeout": 30,
  "alert_suppress_minutes": 30
}
EOF

# 安装 systemd 服务
cat > /etc/systemd/system/wukong.service <<UNIT
[Unit]
Description=wukong Server Monitor - Master
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/wukong --config $INSTALL_DIR/wukong.conf
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=root
Environment=WUKONG_ADMIN_PASSWORD=
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now wukong

echo "wukong 主控安装完成！"
echo "请编辑 $INSTALL_DIR/wukong.conf 配置，然后设置管理员密码："
echo "  export WUKONG_ADMIN_PASSWORD=<密码>"
echo "  systemctl restart wukong"
`, baseURL)

	w.Write([]byte(script))
}

// ---- 探针二进制下载 ----

func (h *Handler) handleAgentBinaryDownload(w http.ResponseWriter, r *http.Request) {
	version := strings.TrimSpace(getPathValue(r, "version"))
	arch := strings.TrimSpace(getPathValue(r, "arch"))
	if version == "" || version == "latest" {
		version = "latest"
	}

	// 安装脚本只允许下载随主控发布的固定架构二进制，避免把路径参数变成任意文件读取。
	if arch != "amd64" && arch != "arm64" {
		writeError(w, http.StatusBadRequest, "不支持的探针架构")
		return
	}

	// Docker 镜像会把探针复制到 /opt/wukong/bin/wukong-agent-<arch>；本地开发构建可回退到当前目录。
	candidates := []string{
		filepath.Join("/opt/wukong/bin", "wukong-agent-"+arch),
		filepath.Join("/opt/wukong", "wukong-agent"),
		filepath.Join(filepath.Dir(os.Args[0]), "wukong-agent-"+arch),
		filepath.Join(filepath.Dir(os.Args[0]), "wukong-agent"),
	}
	for _, path := range candidates {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wukong-agent-%s\"", arch))
			w.Header().Set("X-Wukong-Agent-Version", version)
			http.ServeFile(w, r, path)
			return
		}
	}

	writeError(w, http.StatusNotFound, fmt.Sprintf("版本 %s/%s 的探针二进制文件不存在，请重新构建镜像", version, arch))
}

// ---- 告警 ----

func (h *Handler) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.store.ListAlerts(100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询告警失败: %v", err))
		return
	}
	if alerts == nil {
		alerts = []*store.Alert{}
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (h *Handler) handleListActiveAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.store.ListActiveAlerts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询活跃告警失败: %v", err))
		return
	}
	if alerts == nil {
		alerts = []*store.Alert{}
	}
	writeJSON(w, http.StatusOK, alerts)
}

// ---- SSE 实时推送 ----

func (h *Handler) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE 不支持")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 发送初始连接确认
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
	flusher.Flush()

	// 定时推送最新指标数据给前端，而不仅仅是心跳。
	// 原因：旧代码只发心跳从不推送 metrics_update 事件，前端无法收到实时数据更新。
	dataTicker := time.NewTicker(5 * time.Second)
	defer dataTicker.Stop()

	// 保持连接，每 30 秒发送一次心跳
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-dataTicker.C:
			// 推送最新指标数据
			metrics, err := h.store.GetAllLatestMetrics()
			if err != nil {
				log.Printf("SSE: 查询最新指标失败: %v", err)
				continue
			}
			data, err := json.Marshal(map[string]interface{}{
				"type": "metrics_update",
				"data": metrics,
			})
			if err != nil {
				log.Printf("SSE: JSON 序列化失败: %v", err)
				continue
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		case <-heartbeatTicker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// ---- 探针指标查询 ----

func (h *Handler) handleGetAgentMetrics(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	since := r.URL.Query().Get("since")
	until := r.URL.Query().Get("until")

	now := time.Now()
	sinceTime := now.Add(-24 * time.Hour)
	untilTime := now

	if since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			sinceTime = t
		}
	}
	if until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			untilTime = t
		}
	}

	metrics, err := h.store.GetSystemMetrics(id, sinceTime, untilTime)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询指标失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, downsampleSystemMetrics(metrics, 1440))
}

func downsampleSystemMetrics(metrics []*store.RawSystemMetric, maxPoints int) []*store.RawSystemMetric {
	if maxPoints <= 0 || len(metrics) <= maxPoints {
		return metrics
	}
	step := (len(metrics) + maxPoints - 1) / maxPoints
	result := make([]*store.RawSystemMetric, 0, (len(metrics)+step-1)/step)
	for i := 0; i < len(metrics); i += step {
		end := i + step
		if end > len(metrics) {
			end = len(metrics)
		}
		bucket := metrics[i:end]
		point := &store.RawSystemMetric{Timestamp: bucket[len(bucket)-1].Timestamp}
		var netUp, netDown int64
		for _, item := range bucket {
			point.CPU += item.CPU
			point.Mem += item.Mem
			point.Disk += item.Disk
			netUp += item.NetUp
			netDown += item.NetDown
		}
		count := float64(len(bucket))
		point.CPU /= count
		point.Mem /= count
		point.Disk /= count
		point.NetUp = netUp / int64(len(bucket))
		point.NetDown = netDown / int64(len(bucket))
		result = append(result, point)
	}
	return result
}

func (h *Handler) handleGetPingAgg(w http.ResponseWriter, r *http.Request) {
	id := getPathValue(r, "id")
	isp := r.URL.Query().Get("isp")

	now := time.Now()
	since := now.Add(-24 * time.Hour)
	until := now

	results, err := h.store.GetPingAgg(id, isp, since, until)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询 Ping 聚合失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// ---- 主题 ----

func (h *Handler) handleGetTheme(w http.ResponseWriter, r *http.Request) {
	preset, _ := h.store.GetSetting("theme_preset")
	primary, _ := h.store.GetSetting("theme_primary_color")
	title, _ := h.store.GetSetting("theme_site_title")
	footer, _ := h.store.GetSetting("theme_footer_text")
	siteDomain, _ := h.store.GetSetting("site_domain")
	agentServerAddr, _ := h.store.GetSetting("agent_server_addr")

	if preset == "" {
		preset = "dark"
	}
	if title == "" {
		title = "wukong 监控"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"preset":            preset,
		"primary":           primary,
		"title":             title,
		"footer_text":       footer,
		"site_domain":       siteDomain,
		"agent_server_addr": agentServerAddr,
	})
}

func (h *Handler) handleUpdateTheme(w http.ResponseWriter, r *http.Request) {
	var theme struct {
		Preset          string `json:"preset"`
		Primary         string `json:"primary"`
		Title           string `json:"title"`
		Footer          string `json:"footer_text"`
		SiteDomain      string `json:"site_domain"`
		AgentServerAddr string `json:"agent_server_addr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&theme); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}

	if theme.Preset != "" {
		if err := h.store.SetSetting("theme_preset", theme.Preset); err != nil {
			writeError(w, http.StatusInternalServerError, "保存主题预设失败")
			return
		}
	}
	if theme.Primary != "" {
		if err := h.store.SetSetting("theme_primary_color", theme.Primary); err != nil {
			writeError(w, http.StatusInternalServerError, "保存主题色失败")
			return
		}
	}
	if theme.Title != "" {
		if err := h.store.SetSetting("theme_site_title", theme.Title); err != nil {
			writeError(w, http.StatusInternalServerError, "保存站点标题失败")
			return
		}
	}
	if theme.Footer != "" {
		if err := h.store.SetSetting("theme_footer_text", theme.Footer); err != nil {
			writeError(w, http.StatusInternalServerError, "保存页脚文案失败")
			return
		}
	}

	// 站点域名允许保存为空，用于主动关闭安装命令复制；非空时统一规范化为 http(s)://host。
	baseURL, err := normalizeSiteBaseURL(theme.SiteDomain)
	if err != nil {
		writeError(w, http.StatusBadRequest, "站点域名格式无效")
		return
	}
	if err := h.store.SetSetting("site_domain", baseURL); err != nil {
		writeError(w, http.StatusInternalServerError, "保存站点域名失败")
		return
	}

	// 探针 gRPC 地址允许保存为空；为空时安装脚本按站点域名回退推导，非空时必须是 host:port。
	agentAddr, err := normalizeAgentServerAddr(theme.AgentServerAddr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "探针 gRPC 地址格式无效，请填写 host:port")
		return
	}
	if err := h.store.SetSetting("agent_server_addr", agentAddr); err != nil {
		writeError(w, http.StatusInternalServerError, "保存探针 gRPC 地址失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "主题已更新", "site_domain": baseURL, "agent_server_addr": agentAddr})
}

func (h *Handler) handleGetTelegram(w http.ResponseWriter, r *http.Request) {
	botToken, _ := h.store.GetSetting("telegram_bot_token")
	chatID, _ := h.store.GetSetting("telegram_chat_id")

	// 不把已保存的 bot token 回填到前端，避免浏览器密码管理器误填或泄露；前端只显示是否已配置。
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"bot_token":     "",
		"chat_id":       chatID,
		"has_bot_token": strings.TrimSpace(botToken) != "",
	})
}

func (h *Handler) handleUpdateTelegram(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}

	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "Chat ID 不能为空")
		return
	}
	if _, err := strconv.ParseInt(chatID, 10, 64); err != nil {
		writeError(w, http.StatusBadRequest, "Chat ID 必须是数字")
		return
	}

	// bot token 留空表示保留数据库中已有值，避免每次打开设置页都要求重新输入敏感 token。
	if token := strings.TrimSpace(req.BotToken); token != "" {
		if err := h.store.SetSetting("telegram_bot_token", token); err != nil {
			writeError(w, http.StatusInternalServerError, "保存 Telegram Bot Token 失败")
			return
		}
	}
	if err := h.store.SetSetting("telegram_chat_id", chatID); err != nil {
		writeError(w, http.StatusInternalServerError, "保存 Telegram Chat ID 失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Telegram 配置已保存"})
}

func (h *Handler) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	botToken := strings.TrimSpace(req.BotToken)
	if botToken == "" {
		stored, _ := h.store.GetSetting("telegram_bot_token")
		botToken = strings.TrimSpace(stored)
	}
	chatIDRaw := strings.TrimSpace(req.ChatID)
	if chatIDRaw == "" {
		stored, _ := h.store.GetSetting("telegram_chat_id")
		chatIDRaw = strings.TrimSpace(stored)
	}
	if botToken == "" || chatIDRaw == "" {
		writeError(w, http.StatusBadRequest, "请先填写 Bot Token 和 Chat ID")
		return
	}
	chatID, err := strconv.ParseInt(chatIDRaw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Chat ID 必须是数字")
		return
	}

	// 直接使用当前表单值或数据库值发送测试消息，验证 Telegram 网络、token 和 chat_id 是否可用。
	n := notify.NewTelegramNotifier(botToken, chatID)
	if err := n.Send(&notify.Message{Title: "wukong 测试通知", Body: "这是一条来自 wukong 后台的 Telegram 测试消息。", Level: "info"}); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("测试通知发送失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "测试通知已发送"})
}

func intSettingValue(raw string, fallback int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return v
	}
	return fallback
}

func floatSettingValue(raw string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
		return v
	}
	return fallback
}

func (h *Handler) handleGetAlertSettings(w http.ResponseWriter, r *http.Request) {
	cpu, _ := h.store.GetSetting("alert_cpu_threshold")
	mem, _ := h.store.GetSetting("alert_mem_threshold")
	disk, _ := h.store.GetSetting("alert_disk_threshold")
	offline, _ := h.store.GetSetting("alert_offline_seconds")
	duration, _ := h.store.GetSetting("alert_metric_duration_seconds")

	pingLatency, _ := h.store.GetSetting("alert_ping_latency_threshold")
	pingLoss, _ := h.store.GetSetting("alert_ping_loss_threshold")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cpu":                     floatSettingValue(cpu, 90),
		"mem":                     floatSettingValue(mem, 90),
		"disk":                    floatSettingValue(disk, 90),
		"ping_latency":            floatSettingValue(pingLatency, 200),
		"ping_loss":               floatSettingValue(pingLoss, 20),
		"offline_seconds":         intSettingValue(offline, h.cfg.HeartbeatTimeout),
		"metric_duration_seconds": intSettingValue(duration, 60),
		"suppress_minutes":        h.cfg.AlertSuppressMinutes,
	})
}

func (h *Handler) handleUpdateAlertSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CPU                   float64 `json:"cpu"`
		Mem                   float64 `json:"mem"`
		Disk                  float64 `json:"disk"`
		PingLatency           float64 `json:"ping_latency"`
		PingLoss              float64 `json:"ping_loss"`
		OfflineSeconds        int     `json:"offline_seconds"`
		MetricDurationSeconds int     `json:"metric_duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	if req.PingLatency == 0 {
		req.PingLatency = 200
	}
	if req.PingLoss == 0 {
		req.PingLoss = 20
	}
	if req.CPU <= 0 || req.CPU > 100 || req.Mem <= 0 || req.Mem > 100 || req.Disk <= 0 || req.Disk > 100 {
		writeError(w, http.StatusBadRequest, "CPU/内存/磁盘阈值必须在 1-100 之间")
		return
	}
	if req.PingLatency <= 0 || req.PingLatency > 10000 || req.PingLoss <= 0 || req.PingLoss > 100 {
		writeError(w, http.StatusBadRequest, "Ping 延迟阈值必须在 1-10000ms，丢包阈值必须在 1-100% 之间")
		return
	}
	if req.OfflineSeconds < 5 || req.OfflineSeconds > 3600 {
		writeError(w, http.StatusBadRequest, "离线报警阈值必须在 5-3600 秒之间")
		return
	}
	if req.MetricDurationSeconds < 1 || req.MetricDurationSeconds > 3600 {
		writeError(w, http.StatusBadRequest, "资源告警持续时间必须在 1-3600 秒之间")
		return
	}

	settings := map[string]string{
		"alert_cpu_threshold":           fmt.Sprintf("%.1f", req.CPU),
		"alert_mem_threshold":           fmt.Sprintf("%.1f", req.Mem),
		"alert_disk_threshold":          fmt.Sprintf("%.1f", req.Disk),
		"alert_ping_latency_threshold":  fmt.Sprintf("%.1f", req.PingLatency),
		"alert_ping_loss_threshold":     fmt.Sprintf("%.1f", req.PingLoss),
		"alert_offline_seconds":         strconv.Itoa(req.OfflineSeconds),
		"alert_metric_duration_seconds": strconv.Itoa(req.MetricDurationSeconds),
	}
	for key, value := range settings {
		if err := h.store.SetSetting(key, value); err != nil {
			writeError(w, http.StatusInternalServerError, "保存告警阈值失败")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "告警阈值已保存"})
}

// ---- 上传 Logo ----

func (h *Handler) handleUploadLogo(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(500 << 10) // 500KB 限制

	file, header, err := r.FormFile("logo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "需要上传 logo 文件")
		return
	}
	defer file.Close()

	// 验证 MIME 类型
	mime := header.Header.Get("Content-Type")
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/svg+xml" {
		writeError(w, http.StatusBadRequest, "不支持的文件类型，仅支持 PNG/JPEG/SVG")
		return
	}

	// 生成文件名
	ext := ".png"
	if mime == "image/jpeg" {
		ext = ".jpg"
	}
	if mime == "image/svg+xml" {
		ext = ".svg"
	}
	filename := "logo" + ext

	// 创建上传目录并写入文件。
	// 原因：旧代码只打印日志不实际写文件，返回“上传成功”但文件不存在。
	uploadDir := "/opt/wukong/data/uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("创建上传目录失败: %v", err))
		return
	}
	dstPath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("创建文件失败: %v", err))
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("写入文件失败: %v", err))
		return
	}

	// 将 logo URL 写入数据库，前端读取后可在主题设置中展示
	logoURL := "/uploads/" + filename
	_ = h.store.SetSetting("theme_logo_url", logoURL)

	log.Printf("Logo 上传: %s (%s, %d bytes)", filename, mime, written)
	writeJSON(w, http.StatusOK, map[string]string{
		"message":  "上传成功",
		"filename": filename,
		"url":      logoURL,
	})
}

// ---- 2FA ----

func (h *Handler) handleSetup2FA(w http.ResponseWriter, r *http.Request) {
	secret, url, err := h.authSvc.SetupTOTP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("生成 TOTP 密钥失败: %v", err))
		return
	}
	// TOTP 密钥生成后立即写入数据库和内存，用户扫码后即可使用。
	// 原因：旧代码只生成密钥返回给前端，但从未调用 SetTOTPSecret 或写入 SQLite，
	// 导致用户扫码后 TOTP 验证永远失败（AdminTOTPSecret 始终为空）。
	if err := h.store.SetSetting("admin_totp_secret", secret); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("保存 TOTP 密钥失败: %v", err))
		return
	}
	h.authSvc.SetTOTPSecret(secret)
	log.Println("TOTP 2FA 密钥已生成并持久化到数据库")

	writeJSON(w, http.StatusOK, map[string]string{
		"secret":  secret,
		"url":     url,
		"message": "请使用 Authenticator 应用扫描二维码或输入密钥",
	})
}

func newJWTID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getClientIP 从 HTTP 请求中获取客户端真实 IP。
// 优先级：X-Forwarded-For 第一个 > X-Real-IP > RemoteAddr
// 原因：生产环境 nginx 反代后 RemoteAddr 是 127.0.0.1，必须从转发头取真实 IP 用于登录限流。
func getClientIP(r *http.Request) string {
	// X-Forwarded-For 格式: client, proxy1, proxy2，取第一个即原始客户端 IP
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if ip := strings.TrimSpace(ips[0]); ip != "" {
			return ip
		}
	}
	// X-Real-IP 由 nginx proxy_set_header X-Real-IP $remote_addr 设置
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// 兜底：直接连接时的远端地址（去掉端口）
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
