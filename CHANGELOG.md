# wukong 监控系统 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-06-25 07:40] - Ping 1 秒配置热更新与出口 IPv4/IPv6 上报显示闭环

### 改动前总结
1. 图表是 1 分钟 K 线聚合点，容易误以为 Ping/TCP 探测也是 1 分钟。
2. 已安装探针本地 `agent.conf` 仍可能保留 `ping_interval=60`，默认值改为 1 秒不会自动覆盖旧配置。
3. 节点出口 IPv4/IPv6 只在注册时上报，旧节点不会自动补齐，后台节点列表也未显示。

### 改动后总结
1. 主控连接后先下发 `COMMAND_UPDATE_CONFIG`，同步 `collect_interval`、`ping_interval=1` 和启用运营商目标；探针收到后立即保存配置并重建采集器，Ping/TCP 原始探测按 1 秒执行。
2. `MetricsReport` 新增 `ip_v4` / `ip_v6`；探针启动后立即并每 10 分钟自测公网出口 IPv4/IPv6，随指标上报主控，主控写回 agents 表。
3. 后台节点列表新增“出口 IP”列，显示 IPv4 和 IPv6；有 IPv6 才显示 IPv6，没有则不显示。
4. 说明：网络延时 K 线图继续读取 `ping_agg_1min`，展示粒度仍是 1 分钟聚合，不等于原始探测间隔。

### 涉及文件
- `proto/wukong.proto`、`proto/gen/wukong.pb.go` — MetricsReport 新增 ip_v4/ip_v6
- `internal/agentcore/agent.go` — 出口 IP 周期自测、配置热更新立即重建采集器、上报 IP
- `internal/grpcapi/agent_server.go` — 下发配置更新指令、写回上报 IP
- `web/src/views/Nodes.vue` — 后台节点列表显示出口 IP

## [2026-06-25 07:15] - 修复自动升级架构误判、并发崩溃和 ff1 Ping 聚合漏数

### 改动前总结
1. 自动升级目标版本开启后，主控因 `onlineAgents` map 并发写入崩溃，容器反复重启。
2. 4 台 arm64 节点因旧探针注册时把架构写成 amd64，收到错误架构升级包后离线。
3. ff1 原始 Ping 表中有数据，但 `ping_agg_1min` 无聚合数据，前端 Ping 图为空。
4. GitHub Actions 构建未传 Docker build args，线上版本显示 `dev`。

### 改动后总结
1. `AgentServer.onlineAgents` 增加 `sync.RWMutex`，修复 `fatal error: concurrent map writes`。
2. `MetricsReport` 新增 `agent_version` / `arch` 字段，探针每次上报当前版本和架构，主控实时更新 agents 表，避免下次自动升级继续按旧架构下发。
3. `AggregatePingMin` 改为滚动聚合最近 10 分钟并按 `(ts/60)*60` 计算分钟桶，避免低频/延迟上报节点漏聚合；生产已回填 ff1 历史聚合数据。
4. `.github/workflows/docker.yml` 给 Docker 构建传入 `VERSION/COMMIT/BUILD_TIME`，线上版本显示 commit SHA。
5. 生产手动更新 4 台 arm64 探针：129.150.44.117、146.56.173.198、64.110.72.71、134.185.89.93；随后设置 `agent_target_version=a35fe13...`，其余 amd64 探针已自动升级。当前 13/13 节点在线，ff1 Cloudflare Ping 24h 已有 96 个聚合点。
6. 管理员密码已统一为 `782094Abc`。

### 涉及文件
- `proto/wukong.proto`、`proto/gen/wukong.pb.go` — MetricsReport 新增 agent_version/arch
- `internal/agentcore/agent.go` — 上报版本和架构
- `internal/grpcapi/agent_server.go` — onlineAgents 加锁；同步探针版本/架构
- `internal/store/sqlite.go` — Ping 分钟聚合滚动补偿
- `.github/workflows/docker.yml` — Docker build args 注入版本信息

## [2026-06-25 15:30] - 探针版本自增/自动升级、全屏布局、架构修复、登录过期调整

### 改动前总结
1. 节点列表不显示探针版本
2. 探针版本号硬编码 "0.1.0"，无自动递增机制
3. 探针升级需要管理员在系统设置手动配置目标版本
4. 后端管理页面使用固定侧边栏，内容区只占左侧部分，不够全屏
5. `getArch()` 硬编码返回 "amd64"，arm64 节点被错误标识
6. 管理员 access token 仅 15 分钟就过期，且前端没有自动刷新机制，用户频繁被登出

### 改动后总结
1. **节点列表新增探针版本列**：显示 `agent_ver` 字段
2. **探针版本号自增**：Makefile 使用 `git describe --tags` 自动生成版本号；cmd/agent/main.go 和 cmd/server/main.go 新增 `var version/commit/buildTime` 接收 ldflags 注入；Dockerfile 同步添加 `ARG VERSION/COMMIT/BUILD_TIME` 注入
3. **探针每 5 分钟自动检查升级**：新增 `autoUpgradeCheck` 和 `checkAndUpgrade` 函数，发现新版本自动下载替换重启；系统设置"探针升级"改为只读信息展示
4. **后端页面全屏铺满**：侧边栏改为顶部导航栏，`.wk-main` 不再有 `margin-left: 240px`，内容区占满全屏
5. **修复架构检测**：`getArch()` 从硬编码 "amd64" 改为 `runtime.GOARCH` 动态获取
6. **登录过期调整**：access token 15min→2h，refresh token 7d→30d；前端 `http.ts` 新增 401 自动用 refresh token 续期逻辑

### 涉及文件
- `cmd/agent/main.go` — 新增 version/commit/buildTime 变量，使用 NewAgentWithVersion
- `cmd/server/main.go` — 新增 version/commit/buildTime 变量
- `internal/agentcore/agent.go` — NewAgentWithVersion、getArch 改用 runtime.GOARCH、autoUpgradeCheck/checkAndUpgrade
- `internal/config/config.go` — JWT 过期时间调整
- `Makefile` — 版本号自动生成
- `deploy/Dockerfile` — 版本号 ARG 注入
- `web/src/views/Nodes.vue` — 新增探针版本列
- `web/src/views/Settings.vue` — 探针升级改为只读
- `web/src/layouts/MainLayout.vue` — 侧边栏改顶部导航
- `web/src/styles/index.scss` — 全屏布局样式
- `web/src/utils/http.ts` — 401 自动刷新 token

## [2026-06-25 13:30] - IPv4/IPv6 存储、Ping IPv6、Ping 频率 1s、丢包率显示

### 改动前总结
1. 节点不存储/显示 IP 地址，无法区分 IPv4/IPv6
2. Ping ICMP 模式不支持 IPv6 目标（未加 `-6` 标志）
3. Ping 默认频率 60 秒，采集间隔 1 秒但 Ping 太慢
4. 延时 K 线图只显示延迟，不显示丢包率（后端已有 loss_rate 数据但前端未使用）

### 改动后总结
1. **节点 IPv4/IPv6 存储**：proto RegisterRequest 新增 `ip_v4`/`ip_v6` 字段，探针注册时通过 ipify.org 获取公网 IP 并上报，主控存入 agents 表新列 `ip_v4`/`ip_v6`，**前端不显示 IP 避免暴露**；已有数据库通过 ALTER TABLE 迁移自动添加新列
2. **Ping IPv6 支持**：ICMP 模式自动检测 IPv6 目标，使用 `ping6` 或 `ping -6` 探测；TCP 模式天然支持 IPv6
3. **Ping 默认频率 60→1 秒**：ServerConfig/AgentConfig/PingCollector/agent_server 兜底值全部改为 1
4. **延时 K 线图显示丢包百分比**：图例名追加丢包率（如"上海电信 2%loss"），tooltip 同时显示延时和丢包率

### 涉及文件
- `proto/wukong.proto` — RegisterRequest 新增 ip_v4/ip_v6
- `proto/gen/wukong.pb.go` — 重新生成
- `proto/gen/wukong_grpc.pb.go` — 重新生成
- `internal/store/store.go` — Agent 结构体和接口新增 IPv4/IPv6
- `internal/store/sqlite.go` — 表结构、RegisterAgent、UpdateAgent、GetAgent、ListAgents、迁移
- `internal/grpcapi/agent_server.go` — Register 传递 IPv4/IPv6，PingInterval 兜底 1
- `internal/agentcore/agent.go` — 注册时获取并上报公网 IP
- `internal/agentcore/ping_collector.go` — IPv6 ICMP 支持，默认频率 1s
- `internal/config/config.go` — DefaultPingInterval/PingInterval 60→1
- `web/src/views/NodeDetail.vue` — 丢包率显示、Ping 频率默认 1s
- `web/src/views/public/PublicServerDetail.vue` — 丢包率显示

## [2026-06-24 17:08] - 生产部署、恢复旧数据卷并更新本机探针

### 改动前总结
最新镜像部署时一度使用 `/opt/wukong/data` 新目录挂载，导致生产页面临时显示 0 台服务器；旧生产 SQLite 实际仍在 Docker 匿名卷中。`us4` 节点系统版本显示为 `linux 22.04`，因为旧探针二进制使用 `hostInfo.OS + PlatformVersion` 上报。后台节点页缺少删除按钮，公开首页仍有手动刷新按钮，且公开首页没有读取后台设置的站点标题。

### 改动后总结
1. 生产容器已拉取 `ghcr.io/luowei729/wukong:latest` 并重建，继续只绑定 `127.0.0.1:64443:64443`。
2. 已定位旧生产数据库在 Docker 匿名卷 `_data/wukong.db`，复制恢复到固定目录 `/opt/wukong/data/`，并备份误建空库到 `/opt/wukong/backups/`，后续部署使用固定挂载避免再次丢数据。
3. 已从最新容器复制 `wukong-agent-amd64` 替换生产本机 `/opt/wukong/agent/wukong-agent`，并重启 `wukong-agent.service`。
4. `us4` 最新公开 API 已显示 `os_version=Ubuntu 22.04`、`platform=ubuntu`，修复 `linux 22.04` 展示问题。
5. 新增公开主题接口 `/api/public/theme`，公开首页读取后台设置的站点标题和页脚。
6. 后台节点列表新增“删除”按钮和二次确认；公开首页去掉“刷新状态”按钮，保留 1 秒自动刷新。
7. 探针自动升级链路已实现：后台“探针升级”设置目标版本/下载 URL，主控按心跳下发升级指令，探针下载新二进制、备份、替换并退出由 systemd 拉起。
8. Telegram Bot Token 已验证可用（`getMe` 成功返回 `@lkz_nezha_bot`），但尚未拿到 Chat ID；需要先给 bot 发消息后才能发送测试通知。

### 验证结果
- GHCR 最新镜像构建成功并已拉取到生产。
- `https://server.lkz.pub/api/health` 返回 `{"status":"ok","version":"0.1.0"}`。
- `https://server.lkz.pub/api/public/servers` 已恢复 3 台服务器，其中 `us4` 在线。
- `us4` 已上报 `Ubuntu 22.04`。
- 容器端口仍为 `127.0.0.1:64443->64443/tcp`。

### 涉及文件
- `internal/agentcore/collector.go`
- `internal/agentcore/agent.go`
- `internal/grpcapi/agent_server.go`
- `internal/webapi/handler.go`
- `internal/webapi/public.go`
- `proto/wukong.proto`
- `proto/gen/wukong.pb.go`
- `web/src/views/Nodes.vue`
- `web/src/views/Settings.vue`
- `web/src/views/public/PublicHome.vue`
- `CHANGELOG.md`

---

## [2026-06-24 16:00] - 优化公开首页 qio.ng 风格统计摘要

### 改动前总结
公开首页只有服务器卡片列表，缺少整体统计概览；进度条颜色无区分；状态指示器缺少在线呼吸灯效果；服务器元信息显示过于简略。

### 改动后总结
1. 新增 6 格统计摘要区域：服务器总数/在线/离线、平均 CPU/内存、网络流量总量。
2. 进度条颜色根据使用率分级：<50% 绿色、50-80% 蓝色、80-90% 黄色、>90% 红色。
3. 在线状态徽章增加呼吸灯圆点，视觉更接近 qio.ng。
4. 服务器卡片 meta 信息改为 `平台 · 区域 · 架构` 组合显示。
5. 卡片悬停效果增强：4px 上移 + 24px 阴影。
6. 增加移动端响应式断点：980px 3列→1列，640px 摘要→2列。

### 涉及文件
- `web/src/views/public/PublicHome.vue`
- `CHANGELOG.md`

---

## [2026-06-24 15:50] - 修复登录问题并完善鉴权流程

### 改动前总结
登录后前端各页面需要手动在每个 axios 请求中添加 `authHeaders()`，容易遗漏且无法统一处理 401 过期跳转。`WUKONG_ADMIN_PASSWORD` 环境变量传入明文密码时，直接将明文赋值给 `AdminPasswordHash` 字段，而 `Authenticate` 使用 `bcrypt.CompareHashAndPassword` 比较，导致 Docker 环境变量设置密码后永远无法登录。`POST /api/auth/refresh` 返回 501 未实现，access token 过期后无法续期。

### 改动后总结
1. 新增 `web/src/utils/http.ts` 全局 axios 拦截器：请求自动附加 JWT Token；响应 401 时清除 Token 并跳转登录页（携带 redirect 参数）；非 401 错误统一 ElMessage 提示。
2. 所有 Vue 组件（Login/Dashboard/Nodes/NodeDetail/Alerts/Settings/PublicHome/PublicServerDetail）改用全局 http 实例，移除手动 `authHeaders()` 调用。
3. 修复 `WUKONG_ADMIN_PASSWORD` 明文密码 bug：环境变量值以 `$2a$`/`$2b$` 开头时直接用作 bcrypt hash，否则自动 `bcrypt.GenerateFromPassword` 转换后赋值。
4. 实现 `POST /api/auth/refresh` 刷新令牌端点：验证 refresh token 有效性后重新签发 access + refresh token。
5. `auth.Service.generateTokens` 改为公开方法 `GenerateTokens`，供 webapi handler 调用。

### 验证结果
- `go build ./cmd/server` 编译通过。
- `npm --prefix web run build` 构建通过。
- 本地启动主控，`POST /api/auth/login` 返回有效 JWT。
- 带 Token 访问 `/api/agents` 返回正常数据，无 Token 返回 401。
- `POST /api/auth/refresh` 使用 refresh token 成功获取新 access token。

### 涉及文件
- `web/src/utils/http.ts`（新增）
- `web/src/views/Login.vue`
- `web/src/views/Dashboard.vue`
- `web/src/views/Nodes.vue`
- `web/src/views/NodeDetail.vue`
- `web/src/views/Alerts.vue`
- `web/src/views/Settings.vue`
- `web/src/views/public/PublicHome.vue`
- `web/src/views/public/PublicServerDetail.vue`
- `internal/config/config.go`
- `internal/auth/auth.go`
- `internal/webapi/handlers.go`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`

---

## [2026-06-21 18:39] - 生产部署并验证 443 探针与公开详情

### 改动前总结
Ping 运营商配置和 qio.ng 风格详情字段已完成代码实现、提交并推送，GHCR 镜像构建成功，但生产服务器尚未拉取最新镜像；远程本机探针没有运行，也没有 `agent.conf`，因此生产公开详情暂时还缺少新增系统规格字段和 Ping 聚合数据。

### 改动后总结
1. 远程服务器已拉取 `ghcr.io/luowei729/wukong:latest` 最新镜像并重建 `wukong` 容器，端口继续保持 `127.0.0.1:64443->64443/tcp`，不对公网暴露 64443。
2. 已确认生产 SQLite 设置仍为 `site_domain=https://server.lkz.pub`、`agent_server_addr=server.lkz.pub:443`。
3. 生产 SQLite 已写入一个启用的 Cloudflare TCP Ping 目标用于验证链路；公开 API 只暴露 ISP 名称和聚合延迟，不暴露目标 IP/端口。
4. 远程本机探针已通过在线安装脚本注册，`agent.conf` 固化 `server_addr=server.lkz.pub:443`、`collect_interval=1`、`ping_interval=60` 和 1 个 Ping 目标，并由 systemd 常驻运行。
5. 公开详情 API 已返回 Uptime、Boot time、Mem/Disk total、CPU 型号/核心、Load、累计流量、Platform 等新增字段，并能读取 Cloudflare Ping 聚合点。
6. 无头 Chrome 已打开生产首页和公开详情页，DOM 非空，详情页包含 Status/Uptime/Arch/Mem/Disk/System/CPU/Load/Upload/Download/Boot time/Last active time/网络延迟/Cloudflare 等内容。

### 验证结果
- 远程 Docker：`wukong ghcr.io/luowei729/wukong:latest 127.0.0.1:64443->64443/tcp`。
- 健康接口：`https://server.lkz.pub/api/health` 返回 `{"status":"ok","version":"0.1.0"}`。
- 生产探针日志显示已连接到 `server.lkz.pub:443`。
- 公开 API 脱敏检查未发现 `agent_secret`、`secret`、`token`、`jwt`、`totp`、`telegram` 或 Ping 目标地址。
- 无头 Chrome 截图已生成到 `/tmp/wukong-chrome-final/home.png` 和 `/tmp/wukong-chrome-final/detail.png`。

### 涉及文件
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`
- `DEPLOY_CREDENTIALS.md`

---

## [2026-06-21 18:23] - 补齐 Ping 运营商配置、服务器配置与详情字段

### 改动前总结
Ping 运营商目标只有数据库表和部分 API，探针没有真实 PingCollector，注册响应也不会下发运营商目标；后台设置页不能维护 Ping 线路，节点详情页 Ping 图使用随机模拟数据。公开服务器详情字段仍偏简陋，缺少 Uptime、Boot time、Region、CPU 型号/核心、Load、内存/磁盘总量和累计流量等 qio.ng 风格字段。

### 改动后总结
1. `SystemMetric` 追加 uptime、boot time、mem/disk total、CPU 型号/核心、load、累计上下行、region、platform 等字段，并重新生成 Go proto。
2. 探针系统采集器接入 gopsutil 的 host/load/cpu/mem/disk/net 数据；区域只读取本地配置或环境变量，不自动访问第三方定位服务。
3. 新增探针 `PingCollector`，按 `ping_interval` 独立限流，支持 ICMP(auto 回退 TCP) 和 TCP 探测，失败按丢包上报。
4. 注册响应下发启用的 Ping 运营商目标和 Ping 频率，探针注册后写入本地配置，重启后仍可持续探测。
5. SQLite 系统指标小时表新增详情列并带旧表兼容；Ping 原始数据每分钟聚合到 `ping_agg_1min`，主控后台循环负责聚合和历史清理。
6. 后台“Ping 运营商”页支持新增、编辑、删除、启停线路；后端校验名称、目标、模式和端口范围，并写入 SQLite 固化。
7. 节点详情页新增服务器配置表单，可保存节点名称、采集频率和 Ping 频率；Ping 图删除随机数据，改为读取真实 `/api/agents/{id}/ping-agg`。
8. 公开详情 API 返回脱敏后的新增规格字段和启用 ISP 名称；公开详情页展示 Status/Uptime/Arch/Mem/Disk/Region/System/CPU/Load/Upload/Download/Boot time/Last active time，并显示真实 Ping 聚合图。

### 验证结果
- `PATH="$(go env GOPATH)/bin:$PATH" make proto` 已重新生成 proto。
- `go test ./...` 已通过。
- `npm --prefix /root/wukong/web run build` 已通过，Vite chunk size warning 不影响构建；构建产物已按当前仓库策略还原，避免提交 dist hash 删除。
- 无头 Chrome、远程部署验证将在本次后续步骤继续执行。

### 涉及文件
- `proto/wukong.proto`
- `proto/gen/wukong.pb.go`
- `internal/config/config.go`
- `internal/agentcore/agent.go`
- `internal/agentcore/collector.go`
- `internal/agentcore/ping_collector.go`
- `internal/grpcapi/agent_server.go`
- `internal/store/store.go`
- `internal/store/sqlite.go`
- `internal/webapi/handlers.go`
- `internal/webapi/public.go`
- `cmd/server/main.go`
- `web/src/views/Settings.vue`
- `web/src/views/NodeDetail.vue`
- `web/src/views/public/PublicServerDetail.vue`
- `AGENTS.md`
- `CHANGELOG.md`
- `PROJECT_PLAN.md`
- `DEPLOY_CREDENTIALS.md`

---

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