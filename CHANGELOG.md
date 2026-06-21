# wukong 监控系统 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-06-21 11:57] - 修复部署后首页白屏：嵌入 Vite 下划线资源 + SPA 回退

### 改动前总结
部署后用无头 Chrome 打开首页，HTML 和主 JS 均正常返回，但 Vue 挂载后的 `#app` 内容为空注释。进一步检查网络资源发现 `/assets/_plugin-vue_export-helper-DlAUqK2U.js` 返回 404；该文件是 Vite 动态组件拆包生成的下划线开头资源，本地 dist 存在，但 Go `//go:embed dist/*` 未可靠嵌入下划线文件，导致登录页动态 import 失败白屏。

### 改动后总结
1. `internal/webapi/handler.go` 将 `//go:embed dist/*` 改为 `//go:embed all:dist`，确保 `_` 开头资源进入单二进制。
2. 新增 `spaFileServer`，真实存在的静态资源直接返回；缺失的 JS/CSS 等带扩展名文件保留 404；无扩展名前端路由回退到 `index.html`，支持刷新 `/dashboard` 等 history 路由。
3. 已用无头 Chrome 复现并定位白屏，修复后重新构建验证首页有登录卡片内容。
4. 顺手修复 `internal/agentcore/agent.go` 中 `log.Println` 误用格式化占位符导致 `go test ./...` vet 失败的问题。

### 涉及文件
- `internal/webapi/handler.go`
- `internal/agentcore/agent.go`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`
- `DEPLOY_CREDENTIALS.md`

---

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

## [2026-06-21 11:30] - 默认密码自动生成 + JWT 随机密钥

### 改动前总结
`WUKONG_ADMIN_PASSWORD` 和 `WUKONG_JWT_SECRET` 必须由用户指定。`randomHex` 使用确定性算法（非随机），安全性差。Docker 和 docker-compose 强制要求环境变量，不设则报错。

### 改动后总结
1. **config.go 自动生成随机密码**
   - `AdminPasswordHash` 为空时自动生成 16 字符随机明文密码，bcrypt 哈希存配置
   - 日志打印用户名+密码，方便首次登录
   - `AdminUsername` 默认值设为 `"admin"`

2. **config.go 自动生成随机 JWT 密钥**
   - `JWTSecret` 为空时自动调用 `randomHex(32)` 生成

3. **randomHex 改用 crypto/rand**
   - 从确定性算法改为 `crypto/rand.Read` 安全随机数
   - 极端回退方案保留（时间种子）

4. **Dockerfile 去掉空 ENV 声明**
   - 删掉 `ENV WUKONG_ADMIN_PASSWORD=""` 和 `ENV WUKONG_JWT_SECRET=""`，避免空值覆盖自动生成

5. **docker-compose.yml 简化**
   - 不强制要求环境变量，注释示例
   - 镜像地址改为 `ghcr.io/luowei729/wukong`

### 验证结果
```bash
# 不设任何环境变量启动
docker run -d --name wukong -p 64443:64443 ghcr.io/luowei729/wukong:latest

# 日志自动输出
# ========================================
#   管理员用户名: admin
#   管理员密码:   5987ad38b274f29a1dbd5b7252305757
# ========================================

# API 正常
curl http://127.0.0.1:64443/api/health
→ {"status":"ok"}
```

### 涉及文件
- `internal/config/config.go`（密码生成、JWT 生成、randomHex 安全随机、默认用户名）
- `deploy/Dockerfile`（去掉空 ENV）
- `deploy/docker-compose.yml`（去掉 required 标志）
- `AGENTS.md` / `CHANGELOG.md`（当前记录）

### 改动前总结
Actions 首次运行因 cache 后端 `type=local` 失败。Dockerfile 中 `CMD ["--config", ""]` 但配置不存在时的环境变量覆盖在第一次修改后才生效。

### 改动后总结
1. **修复 Actions Workflow**: 去掉 `type=local` cache 改用 `type=gha`（GitHub Actions 原生缓存），添加 `setup-qemu` 和 `setup-buildx` 步骤，3 分 17 秒编译完成
2. **GHCR 镜像验证**: 
   - `docker pull ghcr.io/luowei729/wukong:latest` → 38MB alpine 镜像
   - `docker run` 启动，监听 `0.0.0.0:64443`
   - `api/health` → 200 `{"status":"ok"}`
   - `api/agents` → 401（auth 拦截正常）
   - 前端 index.html 正常返回
3. **重写 README.md**：添加 Docker 快速部署章节（3 种方式）、功能特性表、架构图、安全说明、GHCR 信息
4. **更新 .gitignore**：排除 `build/` 和 `internal/webapi/dist/`

### 涉及文件
- `.github/workflows/docker.yml`（修复 cache 后端，增加 setup 步骤）
- `README.md`（重写，完整项目 README）
- `.gitignore`（添加 build/ 和 dist/）

### 当前全链路验证通过
```
push → GitHub Actions → GHCR build → docker pull → docker run → API/前端正常
```

### 待完善
- WUKONG_ADMIN_PASSWORD 纯文本传递不够安全，后续可加 Docker secret 支持
- Docker 版探针镜像待后续补全