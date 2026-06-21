// Web API 路由处理函数
// 所有请求处理的具体实现
package webapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	resp, err := h.authSvc.Authenticate(req.Username, req.Password, req.TOTPCode)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	// 刷新令牌端点预留，当前前端主要使用登录签发的 access token。
	writeJSON(w, http.StatusNotImplemented, map[string]string{"message": "refresh endpoint 待实现"})
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
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "需要 Authorization 头")
			return
		}

		// 提取 Bearer token
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			writeError(w, http.StatusUnauthorized, "Authorization 格式应为 Bearer <token>")
			return
		}

		// 验证 token
		claims, err := h.authSvc.ValidateToken(tokenStr)
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
		Name    string  `json:"name"`
		GroupID *string `json:"group_id"`
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

	// 更新字段
	if agent.Name != "" {
		existing.Name = agent.Name
	}
	existing.GroupID = agent.GroupID

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

// ---- 设置 ----

func (h *Handler) handleGetSetting(w http.ResponseWriter, r *http.Request) {
	key := getPathValue(r, "key")
	value, err := h.store.GetSetting(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询设置失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

func (h *Handler) handleSetSetting(w http.ResponseWriter, r *http.Request) {
	key := getPathValue(r, "key")
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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	script := `#!/bin/bash
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

curl -fsSL "https://<域名>/api/server/binary/latest/$ARCH" -o "$INSTALL_DIR/wukong"
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
`

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
	alerts, err := h.store.ListActiveAlerts()
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

	// 保持连接，每 30 秒发送一次心跳
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
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
	writeJSON(w, http.StatusOK, metrics)
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cpu":                     floatSettingValue(cpu, 90),
		"mem":                     floatSettingValue(mem, 90),
		"disk":                    floatSettingValue(disk, 90),
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
		OfflineSeconds        int     `json:"offline_seconds"`
		MetricDurationSeconds int     `json:"metric_duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}
	if req.CPU <= 0 || req.CPU > 100 || req.Mem <= 0 || req.Mem > 100 || req.Disk <= 0 || req.Disk > 100 {
		writeError(w, http.StatusBadRequest, "CPU/内存/磁盘阈值必须在 1-100 之间")
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

	// 保存文件（后续实现完整路径）
	// 简单实现：写到项目目录
	_ = fmt.Sprintf("/opt/wukong/data/uploads/%s", filename) // 占位，后续实现文件写入
	log.Printf("Logo 上传: %s (%s, %d bytes)", filename, mime, header.Size)
	writeJSON(w, http.StatusOK, map[string]string{
		"message":  "上传成功",
		"filename": filename,
		"url":      "/uploads/" + filename,
	})
}

// ---- 2FA ----

func (h *Handler) handleSetup2FA(w http.ResponseWriter, r *http.Request) {
	secret, url, err := h.authSvc.SetupTOTP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("生成 TOTP 密钥失败: %v", err))
		return
	}
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
