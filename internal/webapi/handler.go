// Web API 路由配置
// 提供 REST API + SSE 实时推送 + Embed 前端静态资源
package webapi

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"

	"wukong/internal/alert"
	"wukong/internal/auth"
	"wukong/internal/config"
	"wukong/internal/notify"
	"wukong/internal/store"
)

//go:embed all:dist
var distFS embed.FS

type Handler struct {
	store       store.MetricsStore
	authSvc     *auth.Service
	alertEngine *alert.Engine
	notifier    *notify.Manager
	cfg         *config.ServerConfig
	mux         *http.ServeMux
}

func NewHandler(s store.MetricsStore, a *auth.Service, ae *alert.Engine, n *notify.Manager, cfg *config.ServerConfig) *Handler {
	h := &Handler{
		store:       s,
		authSvc:     a,
		alertEngine: ae,
		notifier:    n,
		cfg:         cfg,
	}
	h.mux = http.NewServeMux()
	h.RegisterRoutes(h.mux)
	return h
}

// ServeHTTP 实现 http.Handler 接口
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// RegisterRoutes 注册所有 API 路由和静态文件处理
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// 健康检查
	mux.HandleFunc("GET /api/health", h.handleHealth)

	// 鉴权
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/auth/refresh", h.handleRefreshToken)

	// 探针管理
	mux.HandleFunc("GET /api/agents", h.authMiddleware(h.handleListAgents))
	mux.HandleFunc("GET /api/agents/{id}", h.authMiddleware(h.handleGetAgent))
	mux.HandleFunc("PUT /api/agents/{id}", h.authMiddleware(h.handleUpdateAgent))
	mux.HandleFunc("DELETE /api/agents/{id}", h.authMiddleware(h.handleDeleteAgent))

	// 探针最新指标（SSE 推送源）
	mux.HandleFunc("GET /api/agents/latest", h.authMiddleware(h.handleGetAllLatestMetrics))

	// 分组管理
	mux.HandleFunc("GET /api/groups", h.authMiddleware(h.handleListGroups))
	mux.HandleFunc("POST /api/groups", h.authMiddleware(h.handleCreateGroup))
	mux.HandleFunc("PUT /api/groups/{id}", h.authMiddleware(h.handleUpdateGroup))
	mux.HandleFunc("DELETE /api/groups/{id}", h.authMiddleware(h.handleDeleteGroup))

	// 运营商 Ping 目标
	mux.HandleFunc("GET /api/isp-targets", h.authMiddleware(h.handleListISPTargets))
	mux.HandleFunc("POST /api/isp-targets", h.authMiddleware(h.handleCreateISPTarget))
	mux.HandleFunc("PUT /api/isp-targets/{id}", h.authMiddleware(h.handleUpdateISPTarget))
	mux.HandleFunc("DELETE /api/isp-targets/{id}", h.authMiddleware(h.handleDeleteISPTarget))

	// 设置
	mux.HandleFunc("GET /api/settings/{key}", h.authMiddleware(h.handleGetSetting))
	mux.HandleFunc("PUT /api/settings/{key}", h.authMiddleware(h.handleSetSetting))

	// 安装 Token
	mux.HandleFunc("POST /api/install-tokens", h.authMiddleware(h.handleCreateInstallToken))

	// 安装脚本（无需鉴权，供 curl 使用）
	mux.HandleFunc("GET /api/install-agent.sh", h.handleInstallAgentScript)
	mux.HandleFunc("GET /api/install-server.sh", h.handleInstallServerScript)

	// 探针二进制下载（无需鉴权，供探针升级使用）
	mux.HandleFunc("GET /api/agent/binary/{version}/{arch}", h.authMiddleware(h.handleAgentBinaryDownload))

	// 告警
	mux.HandleFunc("GET /api/alerts", h.authMiddleware(h.handleListAlerts))
	mux.HandleFunc("GET /api/alerts/active", h.authMiddleware(h.handleListActiveAlerts))

	// SSE 实时推送
	mux.HandleFunc("GET /api/events", h.authMiddleware(h.handleSSE))

	// 探针指标查询
	mux.HandleFunc("GET /api/agents/{id}/metrics", h.authMiddleware(h.handleGetAgentMetrics))
	mux.HandleFunc("GET /api/agents/{id}/ping-agg", h.authMiddleware(h.handleGetPingAgg))

	// 主题配置
	mux.HandleFunc("GET /api/theme", h.authMiddleware(h.handleGetTheme))
	mux.HandleFunc("PUT /api/theme", h.authMiddleware(h.handleUpdateTheme))

	// 上传 Logo
	mux.HandleFunc("POST /api/upload/logo", h.authMiddleware(h.handleUploadLogo))

	// 登录 2FA
	mux.HandleFunc("POST /api/auth/2fa/setup", h.authMiddleware(h.handleSetup2FA))
	mux.HandleFunc("POST /api/auth/2fa/verify", h.handleLogin) // 合并到 login

	// 安装新节点（生成一次性 token）
	mux.HandleFunc("POST /api/agents/install-token", h.authMiddleware(h.handleCreateInstallToken))

	// 静态资源（前端 SPA）
	staticFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Printf("无法加载 embedded 前端资源: %v", err)
		return
	}
	mux.Handle("GET /", spaFileServer(staticFS))
}

// spaFileServer 返回前端静态资源处理器。
// 原因：Vue Router 使用 history 模式，用户刷新 /dashboard 等前端路由时，
// 后端静态文件系统里没有同名文件；此时必须回退到 index.html，让前端路由接管。
// 同时 Vite 可能生成以下划线开头的资源文件，所以 distFS 必须用 all:dist 嵌入。
func spaFileServer(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			cleanPath = "index.html"
		}

		// 如果请求的静态资源真实存在，直接交给 FileServer 输出，保留正确的缓存和 Content-Type 行为。
		if file, err := staticFS.Open(cleanPath); err == nil {
			_ = file.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// API 或带扩展名的静态资源缺失时返回 404，避免把真正的资源错误伪装成 index.html。
		if strings.HasPrefix(cleanPath, "api/") || path.Ext(cleanPath) != "" {
			http.NotFound(w, r)
			return
		}

		// 其余未知路径按 SPA 路由处理，防止浏览器刷新 /dashboard 等前端页面出现 404/白屏。
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
