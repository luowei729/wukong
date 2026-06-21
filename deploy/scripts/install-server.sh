#!/bin/bash
# =============================================
# wukong 监控系统 - 主控安装脚本
# 用法: curl -fsSL https://你的域名/api/install-server.sh | bash
# 或: bash deploy/scripts/install-server.sh
# =============================================
set -euo pipefail

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log_info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ---- 配置 ----
INSTALL_DIR="/opt/wukong"
DATA_DIR="$INSTALL_DIR/data"
SIGNING_DIR="$DATA_DIR/signing"
UPLOAD_DIR="$DATA_DIR/uploads"
CONFIG_FILE="$INSTALL_DIR/wukong.conf"
SERVICE_FILE="/etc/systemd/system/wukong.service"
BINARY_URL="${BINARY_URL:-}" # 由 web API 自动填充

# 检测架构
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) log_error "不支持的架构: $arch (仅支持 amd64/arm64)"; exit 1 ;;
    esac
}

# 检测包管理器
detect_pkg_manager() {
    if command -v apt &>/dev/null; then echo "apt"
    elif command -v yum &>/dev/null; then echo "yum"
    elif command -v dnf &>/dev/null; then echo "dnf"
    elif command -v apk &>/dev/null; then echo "apk"
    else echo "unknown"
    fi
}

# ---- 前置检查 ----
preflight_check() {
    log_info "运行前置检查..."

    # root 检查
    if [[ $EUID -ne 0 ]]; then
        log_error "请以 root 用户运行此脚本 (sudo bash install-server.sh)"
        exit 1
    fi

    # 系统检测
    local os=""
    if [[ -f /etc/os-release ]]; then
        os=$(grep ^ID= /etc/os-release | cut -d= -f2 | tr -d '"')
    fi
    log_info "操作系统: $os $(uname -m)"

    # 检查关键命令
    for cmd in curl systemctl; do
        if ! command -v $cmd &>/dev/null; then
            log_error "缺少命令: $cmd，请先安装"
            exit 1
        fi
    done

    log_ok "前置检查通过"
}

# ---- 安装依赖 ----
install_deps() {
    log_info "安装系统依赖..."

    local pkg_manager
    pkg_manager=$(detect_pkg_manager)

    case "$pkg_manager" in
        apt)
            apt-get update -qq
            apt-get install -y -qq nginx ca-certificates curl 2>/dev/null
            ;;
        yum|dnf)
            $pkg_manager install -y nginx ca-certificates curl 2>/dev/null
            ;;
        apk)
            apk add nginx ca-certificates curl 2>/dev/null
            ;;
        *)
            log_warn "未知包管理器，请手动安装 nginx"
            ;;
    esac

    log_ok "系统依赖安装完成"
}

# ---- 创建目录结构 ----
create_dirs() {
    log_info "创建目录结构..."

    mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$SIGNING_DIR" "$UPLOAD_DIR"

    chmod 755 "$INSTALL_DIR"
    chmod 755 "$DATA_DIR"
    chmod 700 "$SIGNING_DIR"  # 密钥目录，权限收紧
    chmod 755 "$UPLOAD_DIR"

    log_ok "目录结构创建完成"
    log_info "  $INSTALL_DIR/"
    log_info "  ├── wukong          # 主控二进制"
    log_info "  ├── wukong.conf     # 主控配置"
    log_info "  ├── deploy/"
    log_info "  └── data/"
    log_info "      ├── wukong.db   # SQLite 数据库(自动创建)"
    log_info "      ├── signing/    # ed25519 密钥对(权限700)"
    log_info "      └── uploads/    # Logo 等上传文件"
}

# ---- 下载二进制 ----
download_binary() {
    local arch
    arch=$(detect_arch)

    if [[ -n "$BINARY_URL" ]]; then
        # 由 web API 提供下载
        BINARY_DOWNLOAD_URL="${BINARY_URL}/api/server/binary/latest/${arch}"
    else
        # 编译方式：从 build/ 目录复制
        if [[ -f "build/wukong-server" ]]; then
            cp build/wukong-server "$INSTALL_DIR/wukong"
            cp build/wukong-signer "$INSTALL_DIR/wukong-signer"
            log_ok "从本地 build/ 目录复制二进制"
            return
        else
            log_error "未找到二进制文件。请先运行 make build 编译，或设置 BINARY_URL 环境变量"
            exit 1
        fi
    fi

    log_info "下载主控二进制 ($arch)..."
    curl -fsSL "$BINARY_DOWNLOAD_URL" -o "$INSTALL_DIR/wukong"
    chmod 755 "$INSTALL_DIR/wukong"
    log_ok "主控二进制下载完成: $INSTALL_DIR/wukong"
}

# ---- 创建默认配置 ----
create_config() {
    log_info "创建默认配置..."

    if [[ -f "$CONFIG_FILE" ]]; then
        log_warn "配置文件已存在，跳过创建"
        return
    fi

    cat > "$CONFIG_FILE" <<CONFEOF
{
  "listen_addr": "127.0.0.1:64443",
  "data_dir": "$DATA_DIR",
  "db_path": "$DATA_DIR/wukong.db",
  "signer_socket": "$DATA_DIR/signer.sock",
  "log_level": "info",
  "default_collect_interval": 5,
  "default_ping_interval": 60,
  "heartbeat_timeout": 30,
  "alert_suppress_minutes": 30,
  "jwt_secret": "",
  "admin_username": "admin"
}
CONFEOF
    chmod 600 "$CONFIG_FILE"

    log_ok "配置文件创建完成: $CONFIG_FILE"
    log_warn "请在首次启动前设置管理员密码: (见下方第5步)"
}

# ---- 安装 systemd 服务 ----
install_systemd() {
    log_info "安装 systemd 服务..."

    cat > "$SERVICE_FILE" <<UNIT
[Unit]
Description=wukong Server Monitor - Master
Documentation=https://github.com/wukong/monitor
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/wukong --config $CONFIG_FILE
ExecReload=/bin/kill -HUP \$MAINPID
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=root
LimitNOFILE=65536
LimitNPROC=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=wukong

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    log_ok "systemd 服务安装完成: $SERVICE_FILE"
}

# ---- 生成 JWT 密钥和初始管理员密码 ----
generate_credentials() {
    log_info "生成初始凭证..."

    # 如果配置中已有 JWT 密钥，跳过
    if grep -q '"jwt_secret": "[^"]\{16,\}"' "$CONFIG_FILE" 2>/dev/null; then
        log_warn "JWT 密钥已存在，跳过生成"
        return
    fi

    # 生成随机 JWT 密钥
    local jwt_secret
    jwt_secret=$(head -c 32 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 32)

    # 生成随机管理员密码（明文，首次登录后修改）
    local admin_password
    admin_password=$(head -c 12 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 16)

    # 写入配置（jwt_secret 明文存配置文件；密码用环境变量传入）
    sed -i "s/\"jwt_secret\": \"\"/\"jwt_secret\": \"$jwt_secret\"/" "$CONFIG_FILE"

    # 保存密码到临时文件（仅 root 可读），用于首次设置
    echo "$admin_password" > "$DATA_DIR/.admin_password"
    chmod 600 "$DATA_DIR/.admin_password"

    log_warn "========================================================"
    log_warn "  管理员用户名: admin"
    log_warn "  管理员密码:   $admin_password"
    log_warn "  (密码已保存到: $DATA_DIR/.admin_password)"
    log_warn "  请立即登录并在后台修改密码！"
    log_warn "========================================================"
}

# ---- 配置 nginx ----
configure_nginx() {
    log_info "配置 nginx 反代..."

    local domain="${DOMAIN:-}"
    if [[ -z "$domain" ]]; then
        log_warn "未设置 DOMAIN 环境变量，nginx 配置中使用占位符 wukong.example.com"
        log_warn "安装完成后请手动编辑 /etc/nginx/conf.d/wukong.conf 修改 server_name"
        domain="wukong.example.com"
    fi

    local nginx_conf="/etc/nginx/conf.d/wukong.conf"

    # 如果已有配置，备份
    if [[ -f "$nginx_conf" ]]; then
        cp "$nginx_conf" "${nginx_conf}.bak.$(date +%Y%m%d%H%M%S)"
        log_info "已备份现有 nginx 配置"
    fi

    cat > "$nginx_conf" <<NGINXEOF
server {
    listen 80;
    server_name $domain;
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name $domain;

    # !! 请替换为实际 SSL 证书路径 !!
    ssl_certificate     /etc/nginx/ssl/${domain}.crt;
    ssl_certificate_key /etc/nginx/ssl/${domain}.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    client_max_body_size 10M;

    # gRPC 探针通道
    location /wukong.AgentService/ {
        grpc_pass grpc://127.0.0.1:64443;
        grpc_set_header Host \$host;
        grpc_read_timeout 86400s;
        grpc_send_timeout 86400s;
    }

    # Web API + SSE
    location /api/ {
        proxy_pass http://127.0.0.1:64443;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400s;
        proxy_http_version 1.1;
    }

    # 上传文件
    location /uploads/ {
        alias $UPLOAD_DIR/;
        expires 7d;
    }

    # 前端 SPA
    location / {
        proxy_pass http://127.0.0.1:64443;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_buffering on;
        expires 1h;
    }
}
NGINXEOF

    # 测试 nginx 配置
    if nginx -t 2>/dev/null; then
        systemctl enable nginx 2>/dev/null || true
        systemctl restart nginx 2>/dev/null || true
        log_ok "nginx 配置完成并重启"
    else
        log_warn "nginx 配置验证失败，请手动检查 /etc/nginx/conf.d/wukong.conf"
        log_warn "常见问题：SSL 证书路径不正确（先注释掉 ssl_ 行可绕过）"
    fi

    # 复制 nginx 配置到项目目录作为参考
    mkdir -p "$INSTALL_DIR/deploy/nginx"
    cp "$nginx_conf" "$INSTALL_DIR/deploy/nginx/wukong.conf"
    log_ok "nginx 配置已复制到: $INSTALL_DIR/deploy/nginx/wukong.conf"
}

# ---- 启动服务 ----
start_service() {
    log_info "启动 wukong 主控服务..."

    # 检查是否需要先设置管理员密码
    local admin_password_file="$DATA_DIR/.admin_password"
    if [[ -f "$admin_password_file" ]]; then
        local pw
        pw=$(cat "$admin_password_file")

        # 通过环境变量传递密码，首次启动自动 hash
        export WUKONG_ADMIN_PASSWORD="$pw"
        systemctl start wukong
        sleep 2

        # 检查状态
        if systemctl is-active --quiet wukong; then
            log_ok "wukong 主控已启动"
            systemctl enable wukong
            log_ok "已设置为开机自启"

            # 清理密码临时文件
            # rm -f "$admin_password_file"
        else
            log_error "wukong 启动失败，请查看日志: journalctl -u wukong -n 50"
            systemctl status wukong --no-pager
            exit 1
        fi
    else
        # 已有密码配置，直接启动
        systemctl enable --now wukong
        log_ok "wukong 主控已启动"
    fi
}

# ---- 输出完成信息 ----
print_summary() {
    local domain="${DOMAIN:-wukong.example.com}"
    local ip
    ip=$(curl -s ifconfig.me 2>/dev/null || echo "<服务器IP>")

    echo ""
    echo -e "${GREEN}=================================================${NC}"
    echo -e "${GREEN}  wukong 监控系统 - 主控安装完成！${NC}"
    echo -e "${GREEN}=================================================${NC}"
    echo ""
    echo -e "  管理后台: ${CYAN}https://${domain}${NC}"
    echo -e "  或:       ${CYAN}http://${ip}:64443${NC} (未配置 SSL 时)"
    echo ""
    echo -e "  管理员登录:"
    echo -e "    用户名: ${YELLOW}admin${NC}"
    echo -e "    密码:   ${YELLOW}$(cat "$DATA_DIR/.admin_password" 2>/dev/null || echo '<已设置>')${NC}"
    echo ""
    echo -e "  后续步骤:"
    echo -e "  1. 配置 DNS: 将 ${CYAN}${domain}${NC} 指向本机 IP ${ip}"
    echo -e "  2. 配置 SSL: 使用 certbot 申请证书"
    echo -e "     ${YELLOW}apt install -y certbot python3-certbot-nginx${NC}"
    echo -e "     ${YELLOW}certbot --nginx -d ${domain}${NC}"
    echo -e "  3. 登录管理后台，在"系统设置"中生成节点安装命令"
    echo -e "  4. 在被监控节点上运行生成的安装命令"
    echo ""
    echo -e "  服务管理:"
    echo -e "    查看日志: ${YELLOW}journalctl -u wukong -f${NC}"
    echo -e "    重启服务: ${YELLOW}systemctl restart wukong${NC}"
    echo -e "    停止服务: ${YELLOW}systemctl stop wukong${NC}"
    echo ""
    echo -e "  ${YELLOW}⚠ 请立即登录后台修改默认密码！${NC}"
    echo ""
}

# ---- 主流程 ----
main() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║   wukong 监控系统 - 主控安装脚本    ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"
    echo ""

    preflight_check
    install_deps
    create_dirs
    download_binary
    create_config
    install_systemd
    generate_credentials
    configure_nginx
    start_service
    print_summary
}

main "$@"