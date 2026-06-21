# wukong 监控系统 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-06-21 09:49] - 项目初始化：完整骨架搭建 + 可编译单二进制

### 改动前总结
wukong 监控是一个全新项目，仓库内仅包含另一个量化交易项目的演示文档模板（PROJECT_PLAN.md / CHANGELOG.md / DEPLOY_CREDENTIALS.md），README 只有一行，无实际代码。项目定位为类似哪吒探针的服务器监控系统。

### 改动后总结
**1. 21 项架构决策已完成（grill-me 方式逐项确认）**
- 通信：gRPC 双向流（方案B）+ B2 指令白名单公钥验签 + 窄档（仅更新配置/重启探针）
- 后端：Go 单二进制（cmux 单端口 64443 双协议）
- 前端：Vue3 + Element Plus + ECharts，Go embed 进单二进制，暗黑科技风双主题
- 存储：SQLite（WAL + 按小时分表 + ping_agg_1min 预聚合）+ 内存 latest map，不设硬限
- 部署：/opt/wukong，systemd 裸跑 + nginx 反代（`deploy/nginx/wukong.conf`）
- 探针：单二进制，gopsutil 采集，gRPC 上报，本地 10min 缓冲
- 鉴权：JWT + TOTP 2FA + 登录限流
- 告警：6 指标 + 阈值 + 持续判定 + 三级回退 + 滞回防抖 + 抑制期 + 恢复通知 + 静默
- Telegram：多机器人按分组路由 + Notifier 接口预留多渠道
- 注册：一次性 token（30 分钟过期），注册后发个体 agent_secret
- 升级：主控 web 一键升级 + 探针目标版本自检 + 自动回滚
- 签名服务：ed25519 独立进程，私钥与 web 后端物理隔离

**2. 项目骨架已搭建完毕，全部编译通过**
```
wukong/
├── cmd/
│   ├── server/main.go       # 主控入口 (Go embed Vue3)
│   ├── agent/main.go        # 探针入口
│   └── signer/main.go       # 签名服务入口
├── internal/
│   ├── config/config.go     # 配置加载
│   ├── store/
│   │   ├── store.go         # MetricsStore 接口
│   │   └── sqlite.go        # SQLite 实现 (715行)
│   ├── grpcapi/agent_server.go  # gRPC 探针通道
│   ├── webapi/
│   │   ├── handler.go       # Web 路由 (40+ 端点)
│   │   ├── handlers.go      # 请求处理 (676行)
│   │   ├── embed.go         # Go embed 占位
│   │   ├── nginx.go         # nginx 配置生成
│   │   └── dist/            # Vue3 构建产物
│   ├── auth/auth.go         # JWT+2FA 鉴权
│   ├── signer/service.go    # ed25519 签名
│   ├── alert/engine.go      # 告警引擎
│   ├── notify/notify.go     # Telegram+接口
│   └── agentcore/
│       ├── agent.go         # 探针核心 (347行)
│       └── collector.go     # 系统采集器
├── proto/
│   ├── wukong.proto         # gRPC 协议定义
│   └── gen/                 # 生成的 pb Go 代码
├── web/                     # Vue3 源码 (6页面)
├── deploy/
│   ├── nginx/wukong.conf    # nginx 反代配置
│   ├── systemd/             # systemd unit 文件
│   └── scripts/             # 安装脚本生成逻辑
└── Makefile                 # 构建 + 交叉编译
```

**3. 验证结果**
- `wukong-server` 24MB：cmux 监听 64443，REST API 返回正常，前端页面可访问，auth 中间件正确拦截未授权请求
- `wukong-agent` 15MB：gRPC client + 采集器 + 10min 缓冲，可独立编译
- `wukong-signer` 15MB：ed25519 签名服务，Unix Socket 通信

**4. nginx 配置已生成**
- `deploy/nginx/wukong.conf`：443→64443 反代，gRPC `grpc_pass`，SSE `proxy_buffering off`
- `internal/webapi/nginx.go`：编程化生成 nginx 配置

### 涉及文件
全部新增文件，约 4000+ 行代码。

### 待完善
- Web API 登录/2FA/主题/上传等部分路由端点需完善请求体解析
- 探针本地缓冲 flush、Ping 多运营商探测、自动升级逻辑需补充
- 告警引擎需要集成到 gRPC 心跳判定中
- 前端需要接入真实 API 数据替换模拟数据

## [2026-06-21 10:05] - 补全部署脚本 + 完整部署文档

### 改动前总结
项目骨架已完成可编译，但缺少可运行的安装脚本和完整部署指南。DEPLOY_CREDENTIALS.md 还是旧演示项目的凭证文档，完全不可用。

### 改动后总结
**1. 新增安装脚本（2 个可执行脚本）**
- `deploy/scripts/install-server.sh`：主控一键安装脚本
  - 前置检查（root/架构/包管理器）
  - 自动安装 nginx 等系统依赖
  - 创建 /opt/wukong 目录结构（data/signing/uploads）
  - 下载或复制主控二进制
  - 生成默认 wukong.conf 配置（权限 600）
  - 自动生成 JWT 密钥和随机管理员密码
  - 安装 systemd 服务（wukong.service）
  - 生成 nginx 反代配置（443→64443，gRPC+SSE+SPA）
  - 注意：签名服务内嵌在主控中，无需单独启动
- `deploy/scripts/install-agent.sh`：探针一键安装脚本
  - 支持两种传参方式（-k token 或环境变量）
  - 检测已有配置跳过注册
  - 线上下载或本地复制探针二进制
  - 自动注册到主控（token 验证）
  - 安装 systemd 服务（wukong-agent.service，含 CAP_NET_RAW）
  - 注册成功后保存 agent.conf 个体凭证

**2. 新增 Docker 可选部署**
- `deploy/docker-compose.yml`：Docker 版部署（nginx + wukong 双容器）
- 注：默认使用 systemd 裸跑更简单，Docker 版本供参考

**3. 重写 DEPLOY_CREDENTIALS.md**（完整部署指南）
- 快速部署：主控 4 步 + 探针 3 种方式
- 安全配置：防火墙/SSL/密码/签名私钥
- 目录结构详解（/opt/wukong/ 树）
- 配置文件样例（wukong.conf + agent.conf）
- 服务管理命令汇总
- 升级流程（主控替换/探针自动升级/手动）
- 自动备份 systemd timer（每 6h + 保留 7 份）
- 详细排错指南（主控启动/探针注册/网络/白屏）
- 开发环境快速启动
- 部署检查清单（12 项）
- 实际部署凭证表格（待填）

**4. 更新 PROJECT_PLAN.md 进展状态**
- 标注安装脚本与部署文档已完成
- 更新 Phase 1 完成项

**5. 更新 AGENTS.md**
- 添加部署长期提示段落
- 添加 session 记录到开发提示

### 涉及文件
- `deploy/scripts/install-server.sh`（新增，可执行，162 行）
- `deploy/scripts/install-agent.sh`（新增，可执行，180 行）
- `deploy/docker-compose.yml`（新增，46 行）
- `DEPLOY_CREDENTIALS.md`（重写，完整部署指南，310 行）
- `AGENTS.md`（更新最后更新时间和部署提示）
- `CHANGELOG.md`（当前记录）
- `PROJECT_PLAN.md`（更新 Phase 1 进展）

### 待完善
- 安装脚本中 web API 下载二进制端点需要后端配合实现 `/api/agent/binary/latest/{arch}`
- 探针注册逻辑需要后端 `/api/agents/register` 端点（目前只有 gRPC 注册）
- 自动备份 timer 文件可独立为 `deploy/systemd/wukong-backup.service` 和 `.timer`

## [2026-06-21 10:20] - Docker 编译 + GHCR 自动构建 + 环境变量修复

### 改动前总结
之前部署方式只有 systemd 裸跑 + nginx 反代。缺少 Docker 一键运行方式，也没有 CI 自动构建镜像。

### 改动后总结
**1. Docker multistage 全量编译（deploy/Dockerfile）**
- Stage 1: node:22-alpine 编译 Vue3 前端（vite 构建）
- Stage 2: golang:1.25-alpine + CGO 编译 Go 主控（嵌入前端产物）
- Stage 3: alpine:3.20 运行（仅 10MB + 24MB = ~34MB 镜像）
- 直接暴露 64443，不依赖 nginx
- `.dockerignore` 排除 git/构建产物等

**2. GitHub Actions 自动构建（.github/workflows/docker.yml）**
- 触发条件：push main / tag v* / 手动 workflow_dispatch
- 登录 GHCR → 提取标签（latest/semver/commit-sha）→ build-push-action
- BuildKit 缓存加速
- 推送到 `ghcr.io/wukong-monitor/wukong-server`

**3. 修复环境变量覆盖 Bug**
- 问题：`LoadServerConfig` 在配置文件不存在时提前 `return cfg, nil`，跳过 `os.Getenv` 覆盖
- 修正：配置文件不存在时继续执行环境变量覆盖代码块
- 效果：Docker 内无配置文件也能通过 `-e WUKONG_LISTEN_ADDR=0.0.0.0:64443` 生效

**4. 简化 docker-compose.yml**
- 去掉 nginx 容器，直接暴露 64443
- 两个必须环境变量：WUKONG_ADMIN_PASSWORD / WUKONG_JWT_SECRET

**5. Docker 快速运行命令**
```bash
# 不配 nginx，直接运行
docker run -d --name wukong \
  -p 64443:64443 \
  -e WUKONG_ADMIN_PASSWORD=你的密码 \
  -e WUKONG_JWT_SECRET=32位随机密钥 \
  ghcr.io/wukong-monitor/wukong-server:latest

# 或 docker compose
WUKONG_ADMIN_PASSWORD=xxx WUKONG_JWT_SECRET=xxx \
  docker compose -f deploy/docker-compose.yml up -d

# 验证
curl http://127.0.0.1:64443/api/health
curl http://127.0.0.1:64443/  # 前端 SPA
```

### 涉及文件
- `deploy/Dockerfile`（重写，multistage，63 行）
- `.github/workflows/docker.yml`（新增，56 行）
- `.dockerignore`（新增，10 行）
- `deploy/docker-compose.yml`（简化）
- `internal/config/config.go`（修复环境变量覆盖 Bug）
- `AGENTS.md` / `CHANGELOG.md`（当前记录）

### 待完善
- WUKONG_ADMIN_PASSWORD 纯文本传递不够安全，后续可加 Docker secret 支持
- Docker 版探针镜像待后续补全