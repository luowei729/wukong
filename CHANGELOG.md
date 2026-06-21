# wukong 监控系统 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-06-21 15:05] - 完善 Telegram、告警、探针安装与 443 gRPC 连接

### 改动前总结
Telegram Bot Token 输入框容易被浏览器密码管理器误填，且缺少测试通知入口；告警设置页没有展示离线报警阈值，报警中心在空告警或异常响应时可能闪现后消失。探针安装链路虽然能直连 64443，但用户要求生产只能通过 443；旧探针客户端对所有地址都使用明文 gRPC，连接 HTTPS/nginx 443 时会失败。同时静态安装脚本仍可能把 `:443` 重复追加到 `SERVER`，节点注册过程会在前台常驻，用户 Ctrl+C 后才继续安装 systemd。

### 改动后总结
1. Telegram 设置接口新增读取、保存和测试通知；前端不回显 Bot Token，不使用 password 类型输入框，并关闭自动补全，避免密码管理器误填。
2. 告警阈值接口新增 CPU/内存/磁盘、离线秒数和资源持续时间的 SQLite settings 固化，告警引擎每 5 秒检查并读取最新阈值。
3. 告警中心后端空列表兜底为空数组，前端 `Array.isArray` 兜底，避免页面闪现后因 `.length` 报错消失。
4. 探针安装脚本自动识别 `amd64` / `arm64`，注册命令执行成功后退出，由 systemd `enable --now` 后台常驻并设置开机自启。
5. 探针客户端在目标端口为 `443` 时使用 TLS gRPC，满足 `server.lkz.pub:443` 通过 nginx `ssl http2` 反代；其他端口继续使用明文 gRPC，兼容本地和内网直连。
6. 静态安装脚本修正 `SERVER` 处理，`SERVER=host:443` 时下载地址使用 `https://host`，注册地址保持 `host:443`，不再拼成 `host:443:443`。
7. 后台设备页和节点详情页支持自定义服务器节点名称，调用现有 `PUT /api/agents/{id}` 固化保存。
8. nginx 示例关闭前端页面缓存，并保留 `/wukong.AgentService/` 的 443 gRPC 反代示例。

### 验证结果
- `go test ./...` 通过。
- `npm --prefix /root/wukong/web run build` 通过，Vite chunk size warning 不影响本次功能。
- 曾在项目根目录误执行一次 `npm run build`，因根目录无 `package.json` 失败；随后已用 `npm --prefix /root/wukong/web run build` 正确验证。
- 构建产生的 `internal/webapi/dist` hash 文件已还原，避免提交旧 dist 删除。

### 涉及文件
- `cmd/agent/main.go`
- `internal/agentcore/agent.go`
- `internal/alert/engine.go`
- `internal/config/config.go`
- `internal/notify/notify.go`
- `internal/webapi/handler.go`
- `internal/webapi/handlers.go`
- `deploy/nginx/wukong.conf`
- `deploy/scripts/install-agent.sh`
- `web/src/views/Settings.vue`
- `web/src/views/Alerts.vue`
- `web/src/views/Nodes.vue`
- `web/src/views/NodeDetail.vue`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`
- `DEPLOY_CREDENTIALS.md`

---

## [2026-06-21 14:43] - 每秒刷新、禁用缓存并新增密码固化修改

### 改动前总结
公开首页、公开详情页和后台设备页存在浏览器或反代缓存旧页面/API 的风险，手动刷新后可能仍看不到新版本或最新设备指标；后台设备列表只展示基础节点信息，CPU/内存/磁盘没有合并最新指标。管理员密码只能依赖启动配置或环境变量，缺少后台修改入口，也没有按“配置写入数据库固化”的要求保存到 SQLite。

### 改动后总结
1. `ServeHTTP` 全站写入 `no-store/no-cache` 响应头，避免 HTML、静态资源和 API 被浏览器或反代缓存。
2. 公开首页、公开详情页、后台总览、后台设备页改为每秒静默刷新，请求追加 `?_=${Date.now()}`，后台节点页合并 `/api/agents/latest` 显示实时 CPU/内存/磁盘。
3. 主控默认下发采集间隔和探针默认采集间隔从 5 秒调整为 1 秒，新注册探针默认按秒级上报。
4. 新增 `PUT /api/auth/password`，JWT 鉴权后校验当前密码，新密码用 bcrypt 生成 hash 并写入 SQLite `settings.admin_password_hash`。
5. 登录前优先读取数据库固化的 `admin_password_hash` 并同步到内存配置，确保修改密码后立即生效且重启容器后仍使用新密码。
6. 后台设置页新增“修改密码”页签，提供当前密码、新密码、确认新密码表单。

### 验证计划
- `curl -I` 检查页面和 API 响应包含 `Cache-Control: no-store, no-cache, must-revalidate, max-age=0`。
- 无头 Chrome 打开首页、详情页和后台设备页，确认不是白屏且请求会每秒刷新。
- 修改密码后旧密码登录失败，新密码登录成功；重启主控/容器后新密码仍可登录。
- `go test ./...` 通过。
- `cd web && npm run build` 通过。

### 涉及文件
- `internal/webapi/handler.go`
- `internal/webapi/handlers.go`
- `internal/config/config.go`
- `web/src/views/public/PublicHome.vue`
- `web/src/views/public/PublicServerDetail.vue`
- `web/src/views/Dashboard.vue`
- `web/src/views/Nodes.vue`
- `web/src/views/Settings.vue`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`
- `DEPLOY_CREDENTIALS.md`

---

## [2026-06-21 14:10] - 修复探针 gRPC 地址配置并完成远程本机节点验证

### 改动前总结
`site_domain=https://server.lkz.pub` 已能让 Web/API、安装脚本和探针二进制下载正常工作，但安装脚本会把探针 gRPC 地址推导为 `server.lkz.pub:443`。实测该地址注册超时；同一主控的 `server.lkz.pub:64443` 直连 gRPC 注册和上报正常。

### 改动后总结
1. 新增 `agent_server_addr` 设置项并写入 SQLite `settings` 表，后台可独立配置探针注册/上报地址。
2. 安装脚本继续用 `site_domain` 作为 `BASE_URL` 下载脚本和二进制，但 `SERVER_ADDR` 优先使用 `agent_server_addr`，未配置时才按站点域名回退推导。
3. 安装 token 接口会预先校验 `agent_server_addr`，格式错误时返回 `ready=false`，避免复制后才注册失败。
4. 后台设置页新增“探针 gRPC 地址”输入框，读取 `/api/theme` 时与 `site_domain` 一起回填。

### 验证结果
- `server.lkz.pub:443` gRPC 注册超时，`server.lkz.pub:64443` 注册成功。
- 本机临时探针已注册为 `home-pc-server-lkz-e2e` 并向远程主控上报指标。
- 公开 API `/api/public/servers` 与后台 `/api/agents`、`/api/agents/latest` 均能看到本机节点和指标。
- `go test ./...` 通过。
- `cd web && npm run build` 通过。

### 涉及文件
- `internal/webapi/handlers.go`
- `web/src/views/Settings.vue`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`

---

## [2026-06-21 13:45] - 修复站点域名保存与配置数据库固化

### 改动前总结
后台“站点域名 / 访问地址”保存时，后端只在字段非空时才写入 `settings` 表，清空或覆盖失败时前端仍可能显示保存成功；前端读取主题时也只在 `site_domain` 非空时覆盖表单，导致用户感觉配置不能保存或清空后仍显示旧值。

### 改动后总结
1. `handleUpdateTheme` 对主题、标题、页脚和 `site_domain` 的 `SetSetting` 结果逐项检查，数据库写入失败会返回明确错误。
2. `site_domain` 现在每次保存都会写入 SQLite `settings` 表，允许保存为空，用于明确关闭安装命令复制。
3. 前端读取 `/api/theme` 时总是用后端返回值覆盖 `site_domain` 输入框，确保页面显示与数据库固化值一致。

### 验证计划
- 保存 `https://server.lkz.pub` 后重新读取 `/api/theme`，应返回同一值。
- 重启容器并保留 `/opt/wukong/data` volume 后再次读取，配置仍应存在。
- 配置存在时生成安装命令不应再提示未配置站点域名。

### 涉及文件
- `internal/webapi/handlers.go`
- `web/src/views/Settings.vue`
- `CHANGELOG.md`

---

## [2026-06-21 13:32] - 修复在线安装探针二进制下载 401

### 改动前总结
安装命令中的 `?k=token-...` 已能正确传入安装脚本，但脚本下载 `/api/agent/binary/latest/$ARCH` 时，该路由仍被 JWT `authMiddleware` 包裹，远程服务器执行在线安装会返回 401，提示“探针二进制下载失败”。

### 改动后总结
1. `/api/agent/binary/{version}/{arch}` 改为无需 JWT 的只读二进制下载接口，仅允许 `amd64` / `arm64`，并只从固定发布目录返回随主控镜像内置的 `wukong-agent`。
2. Docker 镜像构建阶段同时编译 `wukong-agent-amd64` 和 `wukong-agent-arm64`，运行镜像复制到 `/opt/wukong/bin/`，供安装脚本下载。
3. 管理接口仍保持 JWT 鉴权；公开二进制接口不返回数据库、token、secret 或任何管理配置。

### 验证结果
- `go test ./...` 通过。
- 本地 `/api/agent/binary/latest/amd64` 返回 `HTTP/1.1 200 OK`，响应体为 ELF 二进制，不再返回 401。

### 涉及文件
- `internal/webapi/handler.go`
- `internal/webapi/handlers.go`
- `deploy/Dockerfile`
- `CHANGELOG.md`

---

## [2026-06-21 12:39] - 新增公开状态首页与服务器详情，修复安装命令 token 传递

### 改动前总结
首页默认进入后台登录流程，未登录用户不能查看服务器状态；服务器详情也只有后台节点页。安装命令在未配置域名时仍生成可复制的 `<你的域名>` 占位命令，并把安装 token 放在 `curl -k token` 位置，实际不会传给 `/api/install-agent.sh?k=`，导致脚本 TOKEN 为空，后续注册/下载链路可能把安装 token 误当 JWT 解析并报 `token is malformed`。

### 改动后总结
1. 新增 `/api/public/servers`、`/api/public/servers/{id}`、`/api/public/servers/{id}/metrics`、`/api/public/servers/{id}/ping-agg` 脱敏只读接口，公开页面不复用后台 `/api/agents` 管理接口。
2. 修复 `GetLatestMetrics()` / `GetAllLatestMetrics()` 最新指标时间读取，公开首页可正确判断在线、离线、数据延迟和最近更新时间。
3. `/` 改为公开状态首页，新增 `/server/:id` 公开服务器详情页；后台 `/dashboard`、`/nodes`、`/alerts`、`/settings` 继续要求登录。
4. 登录接口接入真实 `auth.Service.Authenticate`，登录成功后支持 `redirect` 回跳后台目标。
5. 安装命令生成改为必须先设置 `site_domain`，未设置时 `ready=false` 且前端禁止复制；正确命令格式为 `curl -fsSL "https://域名/api/install-agent.sh?k=<token>" | bash`。
6. 安装脚本缺少 `?k=` 时直接返回错误，不再生成空 TOKEN 脚本；脚本中的探针连接地址由站点地址推导，不再使用监听地址冒充公网域名。
7. 由于当前 proto 的 `MetricsReport` 暂无 agent_secret 字段，注册后的流连接先按已注册 agent_id 校验，避免本机探针注册成功后立即被空 secret 校验拒绝；后续应在协议中补充凭证或签名字段。

### 涉及文件
- `internal/webapi/handler.go`
- `internal/webapi/handlers.go`
- `internal/webapi/public.go`
- `internal/store/sqlite.go`
- `internal/grpcapi/agent_server.go`
- `web/src/router/index.ts`
- `web/src/views/Login.vue`
- `web/src/views/Settings.vue`
- `web/src/views/public/PublicHome.vue`
- `web/src/views/public/PublicServerDetail.vue`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`
- `DEPLOY_CREDENTIALS.md`

### 验证结果
- `go test ./...` 通过。
- `cd web && npm run build` 通过，chunk size warning 不影响本次功能。
- 本地主控 `127.0.0.1:18080` 启动成功，真实登录接口能签发 JWT。
- 未配置 `site_domain` 时，安装 token 接口返回 `ready=false` 且 `script_url` 为空，不能复制占位命令。
- 配置 `site_domain=http://127.0.0.1:18080` 后，安装命令包含 `/api/install-agent.sh?k=token-...`，没有 `curl -k token`。
- 安装脚本中 `TOKEN` 非空、`SERVER_ADDR` 正确。
- 本机探针已使用生成 token 注册成功，节点 `home-pc` 在线；日志未出现 `token is malformed`。
- 无头 Chrome 验证 `/`、`/server/:id`、`/dashboard` 都不是白屏；公开页未登录可访问，后台页未登录展示登录页。
- 公开 API 不包含 `secret`、`token`，管理接口未登录仍返回 401。

---

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