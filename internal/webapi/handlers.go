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
	"net/http"
	"strconv"
	"strings"
	"time"
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

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// 解析请求体
	// ... 后续实现完整登录逻辑
	writeJSON(w, http.StatusOK, map[string]string{"message": "login endpoint"})
}

func (h *Handler) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "refresh endpoint"})
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
		Name string  `json:"name"`
		CollectIntv *int `json:"collect_intv"`
		PingIntv    *int `json:"ping_intv"`
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
	if group.Name != "" { existing.Name = group.Name }
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

// ---- 安装 Token ----

func (h *Handler) handleCreateInstallToken(w http.ResponseWriter, r *http.Request) {
	token, err := h.store.CreateInstallToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("生成安装 token 失败: %v", err))
		return
	}
	// 获取配置的域名
	domain, _ := h.store.GetSetting("site_domain")
	scriptURL := fmt.Sprintf("curl -fsSL https://%s/api/install-agent.sh -k %s", domain, token)
	if domain == "" {
		scriptURL = fmt.Sprintf("curl -fsSL https://<你的域名>/api/install-agent.sh -k %s", token)
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"token":       token,
		"script_url":  scriptURL,
		"expires_in":  "30分钟",
	})
}

// ---- 安装脚本 ----

func (h *Handler) handleInstallAgentScript(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("k")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	script := fmt.Sprintf(`#!/bin/bash
# wukong 探针安装脚本
# 用法: curl -fsSL https://<域名>/api/install-agent.sh -k <token> | bash
set -e

TOKEN="%s"
SERVER="%s"
INSTALL_DIR="/opt/wukong/agent"
DATA_DIR="$INSTALL_DIR/data"

# 检测架构
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

# 创建目录
mkdir -p "$INSTALL_DIR" "$DATA_DIR"

# 下载探针二进制
echo "下载 wukong 探针..."
curl -fsSL "https://$SERVER/api/agent/binary/latest/$ARCH" -o "$INSTALL_DIR/wukong-agent"
chmod +x "$INSTALL_DIR/wukong-agent"

# 注册探针
echo "注册到主控 $SERVER ..."
"$INSTALL_DIR/wukong-agent" --server "$SERVER:443" --token "$TOKEN"

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
`, token, h.cfg.ListenAddr)

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
	version := getPathValue(r, "version")
	arch := getPathValue(r, "arch")
	writeError(w, http.StatusNotFound, fmt.Sprintf("版本 %s/%s 的二进制文件尚未上传", version, arch))
}

// ---- 告警 ----

func (h *Handler) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.store.ListActiveAlerts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询告警失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (h *Handler) handleListActiveAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.store.ListActiveAlerts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("查询活跃告警失败: %v", err))
		return
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

	if preset == "" { preset = "dark" }
	if title == "" { title = "wukong 监控" }

	writeJSON(w, http.StatusOK, map[string]string{
		"preset":      preset,
		"primary":     primary,
		"title":       title,
		"footer_text": footer,
	})
}

func (h *Handler) handleUpdateTheme(w http.ResponseWriter, r *http.Request) {
	var theme struct {
		Preset  string `json:"preset"`
		Primary string `json:"primary"`
		Title   string `json:"title"`
		Footer  string `json:"footer_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&theme); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败")
		return
	}

	if theme.Preset != "" { h.store.SetSetting("theme_preset", theme.Preset) }
	if theme.Primary != "" { h.store.SetSetting("theme_primary_color", theme.Primary) }
	if theme.Title != "" { h.store.SetSetting("theme_site_title", theme.Title) }
	if theme.Footer != "" { h.store.SetSetting("theme_footer_text", theme.Footer) }

	writeJSON(w, http.StatusOK, map[string]string{"message": "主题已更新"})
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
	if mime == "image/jpeg" { ext = ".jpg" }
	if mime == "image/svg+xml" { ext = ".svg" }
	filename := "logo" + ext

	// 保存文件（后续实现完整路径）
	// 简单实现：写到项目目录
	dst := fmt.Sprintf("/opt/wukong/data/uploads/%s", filename)
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
		"secret":          secret,
		"url":             url,
		"message":         "请使用 Authenticator 应用扫描二维码或输入密钥",
	})
}

func newJWTID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}