# wukong 监控系统

> 类似哪吒探针的服务器监控系统：gRPC 双向流通信，单二进制极简部署，ed25519 签名安全保障。
>
> 架构与 21 项决策详见 [PROJECT_PLAN.md](./PROJECT_PLAN.md)，部署说明详见 [DEPLOY_CREDENTIALS.md](./DEPLOY_CREDENTIALS.md)。

---

## 🚀 快速部署

### 方式一：Docker 一键运行（推荐）

无需编译、无需 nginx，直接运行：

```bash
# 1. 从 GHCR 拉取并启动
docker run -d --name wukong \
  -p 64443:64443 \
  -e WUKONG_ADMIN_PASSWORD="你的密码" \
  -e WUKONG_JWT_SECRET="32位随机密钥" \
  ghcr.io/luowei729/wukong:latest

# 2. 验证
curl http://127.0.0.1:64443/api/health

# 3. 浏览器打开 http://你的IP:64443
# 用户名: admin / 密码: 你设置的密码
```

或使用 docker compose：

```bash
WUKONG_ADMIN_PASSWORD="你的密码" \
WUKONG_JWT_SECRET="32位随机密钥" \
docker compose -f deploy/docker-compose.yml up -d
```

### 方式二：systemd 裸跑（传统部署）

```bash
# 编译
make build-frontend && make build

# 安装
DOMAIN="monitor.your.com" bash deploy/scripts/install-server.sh

# 在被监控节点上：
curl -fsSL https://monitor.your.com/api/install-agent.sh -k token-xxx | bash
```

详见 [DEPLOY_CREDENTIALS.md](./DEPLOY_CREDENTIALS.md#一快速部署5-分钟搞定)。

### 方式三：本地开发

```bash
# 先编译前端
make build-frontend && make build

# 启动后端（监听 127.0.0.1:64443）
export WUKONG_ADMIN_PASSWORD="dev123"
export WUKONG_JWT_SECRET="dev-jwt-secret-32bytes!!!!!"
./build/wukong-server

# 或启动前端热更新 dev server（需要先启动后端）
make dev-frontend
# 前端监听 http://localhost:5173，API 代理到 64443
```

## ✨ 功能特性

| 功能 | 实现 |
|------|------|
| 通信 | gRPC 双向流，探针主动连主控建长连接 |
| 安全 | 指令白名单 + ed25519 签名验签，签名私钥与 web 后端物理隔离 |
| 监控指标 | CPU / 内存 / 磁盘 / 网络 / Ping（运营商多节点） |
| 实时推送 | SSE 增量帧，浏览器自动重连 |
| 告警 | 6 类固定指标阈值 + 三级回退 + 滞回防抖 + 抑制期去重 |
| 通知 | Telegram 多机器人按分组路由（预留多渠道接口） |
| 存储 | SQLite（WAL + 按小时分表 + 1 分钟预聚合） |
| 鉴权 | JWT（access 15min / refresh 7d）+ TOTP 2FA + 登录限流 |
| 注册 | 一次性安装 token（30 分钟过期），注册后个体凭证认证 |
| 前端 | Vue3 + Element Plus + ECharts，暗黑科技风 + 浅色双主题 |
| 部署 | 单二进制（Go embed 前端），systemd 裸跑 或 Docker 4MB 镜像 |

## 🏗 架构

```
浏览器 ──64443──→ wukong-server (cmux) ─→ REST/SSE/前端 SPA
探针 ──gRPC──→   wukong-server (cmux) ─→ gRPC 双向流
                 ├── SQLite (WAL 按小时分表)
                 ├── 内存 latest map (SSE 源)
                 └── Unix Socket → signer (ed25519)
```

## 📁 目录结构

```
wukong/
├── cmd/
│   ├── server/           # 主控入口（Go embed Vue3）
│   ├── agent/            # 探针入口
│   └── signer/           # 签名服务入口
├── internal/
│   ├── config/           # 配置加载
│   ├── store/            # 存储层（MetricsStore 接口 + SQLite 实现）
│   ├── grpcapi/          # gRPC server（探针通道）
│   ├── webapi/           # Web API（REST + SSE + nginx 生成 + embed）
│   ├── signer/           # 签名客户端
│   ├── alert/            # 告警引擎
│   ├── notify/           # 通知渠道（Telegram + 接口）
│   ├── auth/             # JWT + 2FA 鉴权
│   └── agentcore/        # 探针核心（采集 + gRPC client + 缓冲）
├── proto/                # gRPC proto 定义 + 生成 Go 代码
├── web/                  # Vue3 前端源码
├── deploy/
│   ├── nginx/            # nginx 反代配置
│   ├── systemd/          # systemd unit 文件
│   ├── scripts/          # 安装脚本
│   ├── Dockerfile        # multistage 编译
│   └── docker-compose.yml
├── .github/workflows/docker.yml  # CI → GHCR 自动构建
└── Makefile              # 构建 + 交叉编译
```

## 🔧 构建

```bash
make deps           # 安装 Go 和前端依赖
make build-frontend # 构建 Vue3 前端
make build          # 编译三个二进制
make proto          # 重新生成 gRPC proto 代码
make cross          # 交叉编译 amd64/arm64
make test           # 运行测试
make clean          # 清理构建产物
```

## 🐳 GHCR 镜像

| 用途 | 地址 |
|------|------|
| 下载 | `ghcr.io/luowei729/wukong:latest` |
| CI | push main / tag v* 自动构建 |
| 大小 | ~38MB（alpine 运行） |

## 🔐 安全

- 签名私钥与 web 后端物理隔离：signer 独立进程，web 端通过 Unix Socket 请求签名
- 指令白名单：探针仅接受更新配置 / 重启探针两类签名指令
- 一次性注册 token：30 分钟过期，注册即作废
- 探针升级采用目标版本自检（主动下载验签），主控不下发任意执行命令

## 📄 文档

| 文档 | 内容 |
|------|------|
| [PROJECT_PLAN.md](./PROJECT_PLAN.md) | 21 项架构决策、部署方案、安全架构 |
| [DEPLOY_CREDENTIALS.md](./DEPLOY_CREDENTIALS.md) | 完整部署指南、排错、备份、凭证管理 |
| [CHANGELOG.md](./CHANGELOG.md) | 变更记录 |
| [AGENTS.md](./AGENTS.md) | 开发规范与提示 |