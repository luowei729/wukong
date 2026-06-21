// 前端静态资源占位（Go embed）
// 构建后将 Vue3 打包产物嵌入主控二进制
package webapi

import (
	"io/fs"
	"log"
	"net/http"
)

// init 检查 embed 目录是否存在
func init() {
	if _, err := fs.Stat(distFS, "dist/index.html"); err != nil {
		log.Printf("警告: 前端构建产物不存在 (dist/index.html)，将显示占位页面")
	}
}

// PlaceholderHandler 占位页面处理器（当 embed 前端不存在时使用）
func PlaceholderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>wukong 监控</title></head>
<body style="background:#0f172a;color:#e2e8f0;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;">
<div style="text-align:center;">
<h1 style="color:#38bdf8;">🐒 wukong 监控</h1>
<p style="color:#64748b;">主控已启动，等待前端页面构建...</p>
<p style="font-size:12px;color:#475569;">
  请运行 <code style="background:#1e293b;padding:2px 6px;border-radius:4px;">make build-frontend</code> 构建前端页面
</p>
</div>
</body>
</html>`))
}