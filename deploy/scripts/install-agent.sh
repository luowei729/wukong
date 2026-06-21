#!/bin/bash
# =============================================
# wukong 监控系统 - 探针安装脚本
# 用法: curl -fsSL "https://你的域名/api/install-agent.sh?k=<TOKEN>" | bash
# =============================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ---- 参数解析 ----
# 支持两种传参方式:
#   1. export TOKEN=xxx SERVER=monitor.example.com:443; bash install-agent.sh
#   2. bash install-agent.sh -k <TOKEN> -s monitor.example.com:443
TOKEN="${TOKEN:-}"
SERVER="${SERVER:-}"
# 从命令行参数读取 (curl -k <token> 时 token 在末尾)
while [[ $# -gt 0 ]]; do
    case "$1" in
        -k|--token) TOKEN="$2"; shift 2 ;;
        -s|--server) SERVER="$2"; shift 2 ;;
        *) TOKEN="$1"; shift ;;
    esac
done

# 如果环境变量未设置，尝试从文件读取上次保存的服务器地址
if [[ -z "$SERVER" ]] && [[ -f /opt/wukong/agent/server.txt ]]; then
    SERVER=$(cat /opt/wukong/agent/server.txt)
fi

# 如果仍然没有 TOKEN，检查是否已有配置文件（已注册过）
if [[ -z "$TOKEN" ]] && [[ -f /opt/wukong/agent/agent.conf ]]; then
    log_info "检测到已有探针配置，跳过注册"
    # 直接启动
fi

# ---- 配置 ----
INSTALL_DIR="/opt/wukong/agent"
DATA_DIR="$INSTALL_DIR/data"
CONFIG_FILE="$INSTALL_DIR/agent.conf"
SERVICE_FILE="/etc/systemd/system/wukong-agent.service"
ARCH=""

# 检测架构：同时支持 amd64 和 arm64 节点服务器，其他架构直接提示不可安装。
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) log_error "不支持的架构: $arch（当前仅支持 amd64 / arm64）"; exit 1 ;;
    esac
    log_info "系统架构: $ARCH"
}

# ---- 前置检查 ----
preflight_check() {
    if [[ $EUID -ne 0 ]]; then
        log_error "请以 root 用户运行此脚本"
        exit 1
    fi

    for cmd in curl systemctl; do
        if ! command -v $cmd &>/dev/null; then
            log_error "缺少命令: $cmd"
            exit 1
        fi
    done

    log_ok "前置检查通过"
}

# ---- 创建目录 ----
create_dirs() {
    mkdir -p "$INSTALL_DIR" "$DATA_DIR"
    chmod 755 "$INSTALL_DIR"
    chmod 755 "$DATA_DIR"
    log_info "目录已创建: $INSTALL_DIR"
}

# ---- 下载探针二进制 ----
download_agent() {
    detect_arch

    local base_url="https://${SERVER}"
    # SERVER 可以是 host:port；下载 Web API 统一走 HTTPS 站点地址，常见生产值为 server.lkz.pub:443。
    if [[ "$SERVER" == *":443" ]]; then
        base_url="https://${SERVER%:443}"
    fi
    local binary_url="${base_url}/api/agent/binary/latest/${ARCH}"

    log_info "下载探针二进制 ($ARCH)..."
    if ! curl -fsSL "$binary_url" -o "$INSTALL_DIR/wukong-agent"; then
        log_warn "从网络下载失败，尝试从本地 build/ 复制..."
        SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
        PROJECT_DIR="$(cd "$SCRIPT_DIR/../../" && pwd)"
        if [[ -f "$PROJECT_DIR/build/wukong-agent" ]]; then
            cp "$PROJECT_DIR/build/wukong-agent" "$INSTALL_DIR/wukong-agent"
            log_ok "从本地 build/ 复制成功"
        else
            log_error "下载和本地复制均失败"
            exit 1
        fi
    fi

    chmod 755 "$INSTALL_DIR/wukong-agent"
    log_ok "探针二进制下载完成"
}

# ---- 注册探针 ----
register_agent() {
    if [[ -f "$CONFIG_FILE" ]]; then
        log_info "配置文件已存在，跳过注册"
        return
    fi

    if [[ -z "$TOKEN" ]]; then
        log_error "需要安装 token"
        log_error "请在管理后台"系统设置→安装节点"中生成安装命令"
        exit 1
    fi

    if [[ -z "$SERVER" ]]; then
        log_error "未指定主控服务器地址"
        log_error "请设置 SERVER 环境变量: export SERVER=your-domain.com"
        exit 1
    fi

    # 保存服务器地址
    echo "$SERVER" > "$INSTALL_DIR/server.txt"

    log_info "正在注册到主控 $SERVER ..."

    # 直接运行探针进行注册（注册完成后退出），SERVER 已包含端口时不要再追加 :443。
    if ! "$INSTALL_DIR/wukong-agent" --server "$SERVER" --token "$TOKEN"; then
        log_error "注册失败，请检查："
        log_error "  1. TOKEN 是否有效（30 分钟过期）"
        log_error "  2. 主控地址 $SERVER 是否正确"
        log_error "  3. 主控是否已启动并配置了 nginx"
        exit 1
    fi

    log_ok "注册成功，个体凭证已保存到 $CONFIG_FILE"
}

# ---- 安装 systemd 服务 ----
install_systemd() {
    cat > "$SERVICE_FILE" <<UNIT
[Unit]
Description=wukong Agent - Server Monitor
Documentation=https://github.com/wukong/monitor
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/wukong-agent --config $CONFIG_FILE
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=root
LimitNOFILE=65536
AmbientCapabilities=CAP_NET_RAW
StandardOutput=journal
StandardError=journal
SyslogIdentifier=wukong-agent

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    log_ok "systemd 服务安装完成"
}

# ---- 启动探针 ----
start_agent() {
    log_info "启动探针..."
    systemctl enable --now wukong-agent
    sleep 2

    if systemctl is-active --quiet wukong-agent; then
        log_ok "探针已启动并设置为开机自启"
    else
        log_error "探针启动失败，查看日志: journalctl -u wukong-agent -n 50"
        systemctl status wukong-agent --no-pager
        exit 1
    fi
}

# ---- 输出 ----
print_summary() {
    echo ""
    echo -e "${GREEN}================================================${NC}"
    echo -e "${GREEN}  wukong 探针安装完成！${NC}"
    echo -e "${GREEN}================================================${NC}"
    echo ""
    echo -e "  安装目录: ${CYAN}$INSTALL_DIR${NC}"
    echo -e "  主控地址: ${CYAN}$SERVER${NC}"
    echo ""
    echo -e "  服务管理:"
    echo -e "    查看日志: ${YELLOW}journalctl -u wukong-agent -f${NC}"
    echo -e "    重启探针: ${YELLOW}systemctl restart wukong-agent${NC}"
    echo -e "    停止探针: ${YELLOW}systemctl stop wukong-agent${NC}"
    echo ""
    local display_url="https://${SERVER}"
    if [[ "$SERVER" == *":443" ]]; then
        display_url="https://${SERVER%:443}"
    fi
    echo -e "  现在可以登录管理后台 ${CYAN}${display_url}${NC} 查看节点状态"
    echo ""
}

# ---- 主流程 ----
main() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║   wukong 监控系统 - 探针安装脚本    ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"
    echo ""

    preflight_check
    create_dirs

    # 如果没有配置文件，执行完整安装流程
    if [[ ! -f "$CONFIG_FILE" ]]; then
        download_agent
        register_agent
        install_systemd
    fi

    start_agent
    print_summary
}

main "$@"