// 生成 nginx 反代配置
// 主控默认监听 127.0.0.1:64443，nginx 从 443 反代
// gRPC 和 Web 共用 443 端口，通过路径区分

package webapi

import (
	"fmt"
	"strings"
)

// GenerateNginxConfig 生成 nginx 反代配置
// domain: 用户配置的域名（如 wukong.example.com）
// backendAddr: 主控监听地址（默认 127.0.0.1:64443）
// certPath: SSL 证书路径占位
// keyPath: SSL 私钥路径占位
func GenerateNginxConfig(domain, backendAddr, certPath, keyPath string) string {
	if backendAddr == "" {
		backendAddr = "127.0.0.1:64443"
	}
	if certPath == "" {
		certPath = "/etc/nginx/ssl/wukong.crt"
	}
	if keyPath == "" {
		keyPath = "/etc/nginx/ssl/wukong.key"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`# ==============================================
# wukong 监控系统 - nginx 反代配置
# 生成时间: 2026-06-21 (北京时间)
# 用法: 将本文件放入 /etc/nginx/conf.d/wukong.conf
#       或 /etc/nginx/sites-enabled/wukong
# 然后: nginx -t && systemctl reload nginx
# ==============================================

# --- HTTP 80 -> HTTPS 443 重定向 ---
server {
    listen 80;
    server_name %s;
    return 301 https://$host$request_uri;
}

# --- HTTPS 443 主入口 ---
server {
    listen 443 ssl http2;
    server_name %s;

    # SSL 证书配置（请替换为你的实际证书路径）
    ssl_certificate     %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # 安全头
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # 最大请求体（Logo 上传需要）
    client_max_body_size 10M;

    # ==========================================
    # gRPC 探针双向流
    # 路径前缀: /wukong.AgentService/ 等 gRPC 方法
    # ==========================================
    location /wukong.AgentService/ {
        grpc_pass grpc://%s;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
        grpc_read_timeout 86400s;  # 长连接超时（24小时）
        grpc_send_timeout 86400s;
    }

    # 签名服务（不对外暴露，仅主控内部 Unix Socket 调用）
    # location /wukong.SignerService/ { 不配在 nginx 中 }

    # ==========================================
    # Web API (REST) + SSE 实时推送
    # ==========================================
    location /api/ {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE 关键配置: 关缓冲、长超时
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;

        # WebSocket/SSE 升级头
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # ==========================================
    # 上传文件访问（Logo 等）
    # ==========================================
    location /uploads/ {
        alias /opt/wukong/data/uploads/;
        expires 7d;
        add_header Cache-Control "public, immutable";
    }

    # ==========================================
    # 前端 SPA（Vue3 单页应用）
    # 所有非 API 路径 fallback 到 index.html
    # ==========================================
    location / {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 前端静态资源可缓存
        proxy_buffering on;
        expires 1h;
    }
}
`, domain, domain, certPath, keyPath, backendAddr, backendAddr, backendAddr))
	return sb.String()
}

// GetDefaultNginxConfig 返回带有注释说明的默认配置
func GetDefaultNginxConfig() string {
	return GenerateNginxConfig(
		"wukong.example.com",  // 替换为你的域名
		"127.0.0.1:64443",
		"/etc/nginx/ssl/wukong.crt",
		"/etc/nginx/ssl/wukong.key",
	)
}