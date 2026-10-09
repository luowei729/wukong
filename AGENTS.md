# wukong 监控系统 - 开发规范与提示

> 最后更新: 2026-10-10 04:28 (北京时间)

## 开发原则

1. **使用 codegraph MCP 检索和 semantic_search 向量索引**来检索代码库
2. **调用智能体和 Worktree 并行工作**，确保阅读过项目所有 md
3. **维护项目的所有 md 文档**，有些文档内容可能过时要分辨
4. **要求代码里每步都要中文注释**，说明功能的实现和实现的原因，为后期排查问题和开发打好基础
5. **按照项目代码功能结构、功能区域划分规则**进行开发修改，不要擅自改变代码架构和功能划分结构
6. **先规划架构，不明白的细节先提问确认**再写代码，开发阶段可以打开无头 Chrome 访问主站页面调试验证
7. **每次写代码前先给"改动前总结"**，写完后给"改动后总结"
8. **每次更改变动要按照格式中文写入**项目根目录下的 AGENTS.md / CHANGELOG.md / DEPLOY_CREDENTIALS.md / PROJECT_PLAN.md，记录北京时间和日期
9. **把在本项目需要长期记住的开发提示**写到本文件的下方

## 目录结构

```
wukong/
├── cmd/
│   ├── server/        # 主控入口（Go embed Vue3）
│   ├── agent/         # 探针入口
│   └── signer/        # 签名服务入口
├── internal/
│   ├── config/        # 配置加载
│   ├── store/         # 存储层（MetricsStore 接口 + SQLite 实现）
│   ├── grpcapi/       # gRPC server（探针通道）
│   ├── webapi/        # Web API（REST + SSE + nginx 配置生成 + embed）
│   ├── signer/        # 签名客户端
│   ├── alert/         # 告警引擎
│   ├── notify/        # 通知渠道（Telegram + 接口）
│   ├── auth/          # JWT + 2FA 鉴权
│   └── agentcore/     # 探针核心（采集 + gRPC client + 缓冲）
├── proto/             # gRPC proto 定义 + 生成 Go 代码
├── web/               # Vue3 前端源码
│   └── src/
│       ├── components/    # Wk* 展示组件（卡片/指标/图表/空态…）
│       ├── composables/   # useTheme / usePolling / useOverview（单例状态）
│       ├── utils/         # http.ts / format.ts / charts.ts
│       └── styles/        # variables / base / layout / components / element
├── deploy/
│   ├── nginx/         # nginx 反代配置
│   ├── systemd/       # systemd unit 文件
│   └── scripts/       # 安装脚本
├── build/             # 构建产物（.gitignore）
└── Makefile           # 构建 + 交叉编译
```

## 已确认的 21 项架构决策

| # | 分支 | 决策 |
|---|------|------|
| 1 | 通信架构 | **方案B gRPC 双向流**：探针主动连主控建长连接 |
| 2 | 节点安全 | **B2 指令白名单 + 预置公钥验签**：签名私钥与 web 后端物理隔离，web 被打穿拿不到私钥无法伪造指令 |
| 3 | 指令白名单 | **窄档（修订）**：仅①更新配置 ②重启探针进程。升级由探针主动自检完成 |
| 4 | 后端栈 | **Go 单二进制**（主控+探针同语言，共享 proto） |
| 5 | 前端栈 | **Vue3 + Element Plus + ECharts**，Go embed 进单二进制 |
| 6 | 存储 | **SQLite**（WAL + 按小时分表 + ping_agg_1min 预聚合 + DROP 清理）+ 内存 latest map |
| 7 | 部署目录 | **/opt/wukong**，主控单二进制 + 探针单二进制 |
| 8 | 规模 | **不设硬限**，全套优化 + 背压 + 预留 MetricsStore 接口 |
| 9 | 采集频率 | **默认 1s，后端可改**，D4 三级回退（探针 > 分组 > 全局）；公开首页和后台设备页也按 1s 轮询刷新 |
| 10 | 运营商 Ping | **E1 全局 IP 池 + 双模式**（默认 ICMP，回退 TCP） |
| 11 | 自定义主题 | **F3 预设 + CSS 变量微调 + Logo/站名/页脚**，全局一份，改后刷新 |
| 12 | 实时推送 | **SSE 增量帧**（浏览器自动重连，不通回退轮询） |
| 13 | 管理员鉴权 | **H2 单管理员 + bcrypt + TOTP 2FA + JWT**（access 15min/refresh 7d 可主动失效）+ 可选 IP 白名单 + 登录限流 |
| 14 | 安装 key | **一次性 token**：后台生成，30 分钟过期，用后作废。注册后发个体 agent_secret，gRPC 用个体凭证。不绑分组，后台手动命名分组 |
| 15 | 告警阈值 | **J3 固定 6 指标**（离线/CPU/内存/磁盘/Ping延时/Ping丢包）+ 阈值 + 持续 + 三级回退 + 滞回防抖 |
| 16 | 告警去重 | **抑制期 30min + 恢复通知 + 静默窗口 + alerts 表记录** |
| 17 | Telegram | **L2 多机器人按分组路由 + Notifier 接口抽象**预留多渠道，bot_token 加密存储 |
| 18 | 升级机制 | **N3 主控 web 一键升级**（确认→备份→验签→替换）+ **P3 探针目标版本自检**（主控设目标，探针 10min 自检下载验签替换）+ **自动回滚** |
| 19 | 高可用 | **Q1 单主控 + Q2 定时备份**（systemd timer 每 6h 在线备份 + 探针 10min 缓冲补传） |
| 20 | 端口与反代 | **M-1 主控单端口 64443 cmux 双协议 + nginx 443 统一反代**，TLS 全交 nginx |
| 21 | 前端 UI | **U3 双主题**（默认暗黑科技风 + 浅色可切），Element Plus 深度定制 |

## 开发提示

- **2026-06-21 09:30（北京时间）**：推 GitHub 前排除 DEPLOY_CREDENTIALS.md 和 .codegraph/。DEPLOY_CREDENTIALS.md 是本项目敏感凭证文档，不允许提交公网仓库；.codegraph/ 是本地索引，可重建。
- **2026-06-21 10:20（北京时间）**：补全 Docker 部署、GHCR 自动构建（GitHub Actions）。GitHub Actions 在 push main 或 tag v* 时自动构建并推送到 `ghcr.io/luowei729/wukong`。已实测拉取 GHCR 镜像并 docker run 验证成功。tldr: `docker run -p 64443:64443 -e WUKONG_ADMIN_PASSWORD=xxx -e WUKONG_JWT_SECRET=xxx ghcr.io/luowei729/wukong:latest`
- **2026-06-21 11:30（北京时间）**：`WUKONG_ADMIN_PASSWORD` 和 `WUKONG_JWT_SECRET` 不设时自动生成随机值并打印到日志。`randomHex` 改用 `crypto/rand`。管理员默认用户名为 `admin`。Docker 不加环境变量也能启动。
- **2026-06-21 11:57（北京时间）**：部署后首页白屏根因是 Go `//go:embed dist/*` 未嵌入 Vite 生成的 `_plugin-vue_export-helper-*.js` 下划线资源，浏览器动态加载登录页 404。静态资源嵌入必须用 `//go:embed all:dist`，并用 SPA fallback 支持 history 路由刷新；同次修复 `log.Println` 格式化占位符导致的 go vet 失败。
- **2026-06-21 12:39（北京时间）**：公开首页改为未登录可访问的服务器状态展示，公开详情路径为 `/server/:id`，后端新增 `/api/public/servers*` 脱敏只读接口；安装命令必须先配置 `site_domain`，token 必须通过 `?k=` 传给 `/api/install-agent.sh`，禁止再用 `curl -k token` 这种错误格式。
- **2026-06-21 12:55（北京时间）**：本机端到端验证通过：未配置 `site_domain` 时安装命令 `ready=false` 且 `script_url` 为空；配置 `http://127.0.0.1:18080` 后脚本内 `TOKEN` 非空、`SERVER_ADDR` 正确；本机探针 `home-pc` 使用生成 token 注册上线，日志无 `token is malformed`；无头 Chrome 验证 `/`、`/server/:id`、`/dashboard` 均非白屏，公开 API 不含 `secret`/`token`，管理 API 未登录仍 401。
- **2026-06-21 13:32（北京时间）**：在线安装脚本下载探针失败 401 的根因是 `/api/agent/binary/{version}/{arch}` 路由注释写无需鉴权但实际包了 JWT `authMiddleware`。已改成只读公开二进制下载接口，仅允许 `amd64`/`arm64`，并由 Docker 镜像内置 `/opt/wukong/bin/wukong-agent-amd64` 与 `wukong-agent-arm64`；本地验证 `/api/agent/binary/latest/amd64` 返回 200 ELF，不再 401。
- **2026-06-21 13:45（北京时间）**：配置必须写入 SQLite `settings` 表固化，不能只保存在前端或内存；`site_domain` 每次保存都要写库（允许保存为空来关闭安装命令复制），并检查 `SetSetting` 错误。前端读取 `/api/theme` 时必须用后端返回值覆盖输入框，避免清空或保存失败后显示旧值。
- **2026-06-21 14:10（北京时间）**：生产环境 `server.lkz.pub:443` 当前只验证 HTTP/API 正常，gRPC 注册会超时；`server.lkz.pub:64443` 直连 gRPC 注册和上报已验证成功。因此安装脚本的 Web 下载地址继续使用 `site_domain`，探针注册/上报地址改由 SQLite `agent_server_addr` 固化（格式必须 `host:port`），未配置时才回退按站点域名推导。
- **2026-06-21 14:43（北京时间）**：页面和 API 响应必须全站带 `Cache-Control: no-store, no-cache, must-revalidate, max-age=0`、`Pragma: no-cache`、`Expires: 0`，公开首页、公开详情页、后台总览、后台设备页每秒静默轮询并给请求加 `?_=${Date.now()}`；默认采集间隔改为 1 秒。管理员修改密码接口为 `PUT /api/auth/password`，必须 JWT 鉴权、校验当前密码，新密码 bcrypt hash 写入 SQLite `settings.admin_password_hash` 固化，登录前优先读取该设置。
- **2026-06-21 15:05（北京时间）**：生产要求探针只能通过 `server.lkz.pub:443` 连接，不再使用 `64443` 对外直连；探针客户端在目标端口为 443 时使用 TLS gRPC，经 nginx `listen 443 ssl http2` 的 `/wukong.AgentService/` 反代转发到本机 64443，其他端口仍保持明文 gRPC。后台 `agent_server_addr` 生产值应固化为 `server.lkz.pub:443`，安装脚本必须输出 `SERVER_ADDR="server.lkz.pub:443"`。Telegram 设置页不回显 token、不使用 password 类型并提供测试通知；告警阈值页必须显示离线阈值；告警中心空列表要返回/兜底成数组；agent 安装需支持 amd64/arm64，注册后退出并由 systemd 常驻和开机自启；服务器节点名称支持后台自定义修改。
- **2026-06-21 18:23（北京时间）**：Ping 运营商配置已形成第一阶段闭环：后台“Ping 运营商”页写入 SQLite `isp_targets`，注册响应向探针下发启用目标和 `ping_interval`，探针将目标持久化到 `agent.conf` 并按独立频率执行 ICMP(auto 回退 TCP)/TCP 探测，上报后主控写入小时表并每分钟聚合到 `ping_agg_1min`。公开详情页只暴露启用 ISP 名称和聚合延迟，不泄露目标 IP/端口；服务器详情字段扩展为 Uptime/Boot time/Region/CPU 型号/Load/累计流量等 qio.ng 风格展示。
- **2026-06-21 18:39（北京时间）**：生产已部署 commit `61f033a` 对应的 GHCR 最新镜像，远程 Docker 容器继续保持 `127.0.0.1:64443->64443/tcp`，SQLite `site_domain=https://server.lkz.pub` 与 `agent_server_addr=server.lkz.pub:443` 已确认。生产本机探针已通过在线安装脚本注册并由 systemd 常驻，日志显示连接 `server.lkz.pub:443`；公开详情已拿到 qio.ng 风格系统字段和 Cloudflare Ping 聚合数据，无头 Chrome 验证首页与详情页均不是白屏。
- **开发后续优先级**：① 签名配置热更新闭环 ② Web API 端点完整实现 ③ 告警引擎集成 gRPC 心跳 ④ 前端接入真实 API 数据 ⑤ 安装脚本与升级流程端到端原型
- **2026-06-24 15:50（北京时间）**：修复登录问题并完善鉴权流程。新增 `web/src/utils/http.ts` 全局 axios 拦截器，自动在请求头附加 JWT Token、401 时清除 Token 并跳转登录页，所有 Vue 组件不再手动添加 `authHeaders()`。修复 `WUKONG_ADMIN_PASSWORD` 环境变量传入明文密码时直接赋值给 `AdminPasswordHash` 导致 bcrypt 比对失败的严重 bug，现自动检测 `$2a$`/`$2b$` 前缀区分明文和 hash。实现 `POST /api/auth/refresh` 刷新令牌端点，前端 access token 过期后可无感续期。`auth.Service.generateTokens` 改为公开方法 `GenerateTokens`。
- **2026-06-24 17:08（北京时间）**：生产部署最新 GHCR 镜像时必须保持数据卷。旧生产数据库位于 Docker 匿名卷 `/var/lib/docker/volumes/5e192c.../_data/wukong.db`，已复制恢复到固定目录 `/opt/wukong/data/`，后续 `docker run` 必须使用 `-v /opt/wukong/data:/opt/wukong/data`。本次已修复 `us4` 系统版本显示为 `Ubuntu 22.04`，原因是探针改用 `hostInfo.Platform + PlatformVersion`；主控更新后还必须同步替换生产本机 `/opt/wukong/agent/wukong-agent` 并重启 systemd 探针。公开首页标题通过 `/api/public/theme` 读取后台设置；节点页有删除按钮；首页无手动刷新按钮。Telegram bot `@lkz_nezha_bot` token 可用，但需用户先给 bot 发消息才能拿到 Chat ID 测试发送。
- **2026-06-25 13:30（北京时间）**：四项功能改进。① 节点 IPv4/IPv6 存储：proto RegisterRequest 新增 `ip_v4`/`ip_v6` 字段，探针注册时通过外部 API（ipify.org）获取公网 IP 并上报，主控存入 agents 表新列 `ip_v4`/`ip_v6`，**前端不显示 IP 避免暴露**；已有数据库通过 ALTER TABLE 迁移自动添加新列。② Ping IPv6 支持：ICMP 模式自动检测 IPv6 目标，使用 `ping6` 或 `ping -6` 探测；TCP 模式天然支持 IPv6。③ Ping 默认频率从 60 秒改为 1 秒：ServerConfig/AgentConfig/PingCollector/agent_server 兜底值全部改为 1。④ 延时 K 线图显示丢包百分比：图例名追加丢包率（如"上海电信 2%loss"），tooltip 同时显示延时和丢包率。
- **2026-06-25 15:30（北京时间）**：五项功能改进。① 节点列表新增"探针版本"列，显示 `agent_ver` 字段。② 探针版本号自增：Makefile 使用 `git describe --tags` 自动生成版本号，cmd/agent/main.go 和 cmd/server/main.go 新增 `var version/commit/buildTime` 接收 ldflags 注入，Dockerfile 同步添加 `ARG VERSION` 注入。探针每 5 分钟自动检查版本，发现新版本自动升级（`autoUpgradeCheck`）。③ 系统设置"探针升级"改为只读信息展示（自动升级机制说明），不再提供手动配置目标版本。④ 后端管理页面重构为全屏铺满布局：侧边栏改为顶部导航栏，`.wk-main` 不再有 `margin-left: 240px`。⑤ 修复 `getArch()` 硬编码 `amd64` 问题，改为 `runtime.GOARCH` 动态获取。⑥ 管理员登录过期机制调整：access token 从 15 分钟延长到 2 小时，refresh token 从 7 天延长到 30 天；前端 `http.ts` 新增 401 自动刷新 token 逻辑，避免用户频繁被登出。
- **2026-06-25 07:15（北京时间）**：修复自动升级与 Ping 聚合生产问题。① 主控 `AgentServer.onlineAgents` 增加 `sync.RWMutex`，修复多探针并发连接时 `fatal error: concurrent map writes` 导致容器反复重启。② `MetricsReport` 新增 `agent_version` / `arch` 字段，探针每次上报当前版本和架构，主控实时写回 agents 表，避免旧注册值把 arm64 节点误判为 amd64 并下发错误升级包。③ `AggregatePingMin` 改为滚动聚合最近 10 分钟，并按 `(ts/60)*60` 写入分钟桶，修复 ff1 原始 Ping 有数据但 `ping_agg_1min` 漏聚合导致前端无 Ping 图的问题；生产已回填 ff1 历史聚合。④ GitHub Actions Docker 构建添加 `VERSION/COMMIT/BUILD_TIME` build args，线上主控和探针版本不再显示 `dev`。⑤ 生产已手动更新 4 台 arm64 探针（129.150.44.117、146.56.173.198、64.110.72.71、134.185.89.93）并设置 `agent_target_version=a35fe13...`，其余 amd64 探针已自动升级；当前 13/13 节点在线，管理员密码为 `782094Abc`。
- **2026-06-25 07:40（北京时间）**：补齐 Ping 1 秒与出口 IP 闭环。① 明确图表仍读取 `ping_agg_1min`，K 线展示粒度是 1 分钟聚合点，不代表原始 Ping/TCP 探测间隔。② 主控连接后先下发 `COMMAND_UPDATE_CONFIG`，强制同步 `collect_interval`、`ping_interval=1` 和启用的运营商目标；探针收到后立即保存 `agent.conf` 并重建采集器，不再等重启。③ `MetricsReport` 新增 `ip_v4` / `ip_v6`，探针启动后立即并每 10 分钟自测公网 IPv4/IPv6 出口 IP，随指标上报；主控写回 agents 表。④ 后台节点列表新增“出口 IP”列，显示 IPv4 和 IPv6（有 v6 显示，没有则不显示）。
- **2026-06-25 07:55（北京时间）**：修正出口 IP 和 K 线展示。① 出口 IP 只允许公网地址：探针和主控双侧过滤 `10/172.16-31/192.168`、loopback、link-local（如 `fe80::/10`）、ULA（`fc00::/7`）等非公网地址，避免把 `172.31.*` 或 `fe80::*` 显示为出口 IP。② Ping K 线查询改为从原始 `metrics_ping_YYYYMMDDHH` 小时表按 `ts` 秒级聚合，前端时间标签改为 `HH:mm:ss`，实现每秒颗粒度；`ping_agg_1min` 继续保留用于历史兜底和维护。
- **2026-06-25 08:05（北京时间）**：补强秒级 Ping 与 live2 出口 IP。① PingCollector 改为对所有运营商目标并发探测，不再串行等待 4 条线路；ICMP 从 `ping -c 3` 改为 `ping -c 1 -W 1`，配合 `ping_interval=1` 让每条线路尽量每秒产生一个原始点。② 公网 IP 获取改为多服务商兜底（api4/api6.ipify、icanhazip、ifconfig.me），并继续严格过滤非公网地址；live2 这类云内网 `172.31.*` / `fe80::*` 不再显示，若公网接口获取失败则留空而不是显示内网。
- **2026-06-25 08:25（北京时间）**：修复告警中心和首页时间显示。① 告警中心改为读取 `/api/alerts` 最近 100 条历史告警，包含 `firing/resolved`，不再只显示 `/api/alerts/active` 活跃告警；新增恢复时间列。② store 新增 `ListAlerts(limit)` 查询历史告警。③ 告警引擎补齐 Ping 延迟和 Ping 丢包检查，默认阈值为 200ms / 20%，复用资源告警持续时间；系统设置告警阈值页新增 Ping 延迟/Ping 丢包阈值。④ 首页卡片最近上报时间优先使用 `last_seen_at`，避免 `updated_at` 偶发滞后导致 ff1 显示”3分钟前”但实际每秒心跳。
- **2026-06-25 18:00（北京时间）**：三项安全与可靠性修复。① gRPC ReportStream 认证漏洞修复：proto `MetricsReport` 新增 `agent_secret` 字段（field 10），探针每次上报携带个体凭证密钥，主控 `ReportStream` 首条消息验证改为调用 `ValidateAgent(agentID, secret)` bcrypt 校验，不再只查 agent_id 是否存在；探针端 `collectAndReport` 自动填入 `a.cfg.AgentSecret`；对旧探针（无 agent_secret）退回兼容模式只检查 ID，允许连上接收升级指令。② 登录限流 IP 获取修复：`auth.Authenticate` 新增 `ip` 参数，`handleLogin` 调用时传入 `getClientIP(r)`（优先 X-Forwarded-For → X-Real-IP → RemoteAddr），不再写死 `”global”` 导致限流完全失效。③ Telegram 通知重试机制：`notify.go` 增加 `sendWithRetry` 指数退避重试（最多 3 次，延迟 2s→4s→8s），`telegramAPIError` 结构体区分 4xx（不重试）和 429/5xx（重试），避免网络抖动或 Telegram 限流导致关键告警丢失。④ nginx 反代配置模板 `nginx.go` gRPC 块新增 `X-Forwarded-For` 和 `X-Forwarded-Proto` 头传递。
- **2026-06-25 18:30（北京时间）**：修复 Ping 数据批量丢包 Bug。根因：`SystemCollector.Collect()` 内 `cpu.Percent(time.Second)` 同步阻塞 1 秒，后续 `PingCollector` 被延迟执行，加上 `probeTCP` 超时 2 秒，每轮 Ping 实际耗时 3 秒远超 1 秒采集周期，导致数据间隔不规律、图表表现为”每隔几十秒一起丢包”。修复：① `cpu.Percent` 改为独立 goroutine 异步采样循环 `StartCPULoop`，`Collect()` 只读缓存值不再阻塞；② `collectAndReport` 改为所有采集器并发执行，互不等待；③ `probeTCP` 超时从 2 秒降到 1 秒（与 `ping -W 1` 对齐）。
- **2026-06-25 19:00（北京时间）**：修复探针升级死循环导致节点频繁掉线。根因：手动设置 `agent_target_version` 用的是本地 git commit hash，而 GHCR 镜像中探针的版本是 Actions 构建时的 hash，两者完整值不同但前缀相同，精确匹配 `==` 永远不等 → 主控反复下发升级 → 探针无限重启。修复：① 主控启动时自动将自己的 `commit` 写入 `agent_target_version`（同一 Docker 镜像内主控和探针 commit 完全一致，永不会不匹配）；② `buildUpgradeFrame` 和探针侧 `handleUpgradeAgent` / `checkAndUpgrade` 版本比较改为前缀匹配（`versionMatch` / `agentVersionMatch`，前 7 位一致即视为同版本）。
- **2026-06-25 19:30（北京时间）**：修复首页 CPU 指标归零。根因：`handleUpdateConfig` 调用 `initCollectors()` 重建采集器时，每次都 `new SystemCollector` 创建新实例（`cpuReady=false, cpuPercent=0`），但新实例没有启动 `StartCPULoop`，CPU 异步采样循环仍在旧实例上运行，`Collect()` 从新实例读到 0。修复：`initCollectors()` 检测已有 `sysCollector` 时复用原实例，仅首次创建新的，保留 CPU 异步采样的缓存状态。
- **2026-06-30 14:00（北京时间）**：16 项 bug 与安全修复。① P0 ticker 泄漏：`reportLoop` 的 ticker 从循环内移到循环外创建，重连时 `Reset` 而非新建。② P0 refresh token 类型区分：Claims 新增 `token_type` 字段，新增 `ValidateAccessToken`/`ValidateRefreshToken`，authMiddleware 只接受 access token，refresh 接口只接受 refresh token。③ P0 TOTP 2FA 持久化：`handleSetup2FA` 生成密钥后立即写入 SQLite 和内存，新增 `SetTOTPSecret` 方法。④ P1 SSE 鉴权：authMiddleware 支持 `?token=` 查询参数回退（EventSource 不支持自定义 header）。⑤ P1 SSE 数据推送：SSE 每 5s 推送 `metrics_update` 事件，不再只发心跳。⑥ P1 Logo 上传：`handleUploadLogo` 实际写入 `/opt/wukong/data/uploads/` 并持久化 URL。⑦ P1 Settings 白名单：`handleGetSetting`/`handleSetSetting` 加 `allowedSettingKeys` 白名单，拒绝访问敏感 key。⑧ P1 PingTargets 清空：`handleUpdateConfig` 始终用主控下发列表覆盖（含空列表）。⑨ P2 client 竞态：`Agent.client` 加 `clientMu` 读写锁保护。⑩ P2 Webhook 发送：`WebhookNotifier.Send` 实际发送 HTTP POST。⑪ P2 告警引擎退出：`Run(ctx)` 支持 context 取消。⑫ P2 DNS 超时：`isIPv6` 的 `LookupHost` 加 3s 超时。⑬ P2 strings.Title 弃用：替换为自定义 `titleCase`。⑭ P3 登出按钮：MainLayout 用户图标改为 SwitchButton，清除 JWT 并跳转登录页。⑮ P3 退出清理：main.go 先 cancel 维护/告警 context 再停服务。⑯ P3 安装脚本占位符：主控安装脚本动态替换站点域名。

- **2026-07-23 14:15（北京时间）**：主站故障修复。根因：`runMaintenanceLoop` 数据保留期硬编码 30 天（`DropOldHourlyTables(24*30)`），13 节点 × 1s 采集 × 30 天 ≈ 1.3 亿行撑至 wukong.db 9.9GB，SQLite 写锁竞争导致全部 HTTP API goroutine 阻塞，主控 7 月 21 日静默卡死（进程存活无响应，docker stop 需 SIGKILL），前端经 openresty 能加载但 API 全部超时（000）。紧急处理：停容器批量 DROP 1086 张 7 天前历史小时表 + `VACUUM INTO` 压缩 9.9GB→2.4GB。代码修复：保留期 30 天→24 小时（`retentionHours=24`），清理频率 6h→1h。注意：主控容器为 alpine(musl) 镜像，本机 glibc 编译的二进制 docker cp 进去会报 `exec format: no such file`，必须走 GHCR Actions 重新构建镜像或用 alpine 容器编译 musl 二进制；重建容器时不传 `--config`（纯环境变量启动），原容器 Cmd `[--config ]` 带尾随空格会触发 `flag needs an argument`。

- **2026-07-23 15:55（北京时间）**：修复 DropOldHourlyTables 死锁 bug。根因：`DropOldHourlyTables` 在 `rows.Next()` 循环内执行 `DROP TABLE`（schema 修改需独占锁），但 `defer rows.Close()` 使读事务一直持有，独占锁获取不到导致死锁，阻塞全部写 goroutine 致主控卡死（WAL 飙至 94MB）。30 天保留版因无过期表未触发；24 小时保留首次清理 DROP 约 360 张表即死锁。修复：先收集表名到 slice、显式 `rows.Close()` 释放读事务后再循环 DROP，末尾 `PRAGMA wal_checkpoint(TRUNCATE)` 防 WAL 膨胀。教训：SQLite schema 修改（DROP/ALTER TABLE）不能在未关闭的 rows 读事务中执行，必须先读完关闭再改 schema。本机 glibc 编译的二进制不可用（alpine musl），紧急修复用 `docker run golang:1.25-alpine` 编译 musl 二进制 + docker cp 部署，后续走 GHCR Actions 持久化。

- **2026-10-05 04:13（北京时间）**：Web UI 重构为现代 SaaS 控制台（纯前端，未改 `internal/` Go 代码、未加 npm 依赖）。① 设计令牌单一来源在 `web/src/styles/variables.scss`，暗 #0a0a0b / 浅 #f7f7f8 中性灰阶 + indigo 主色（暗 #5e6ad2 / 浅 #4f5bd5）；新增圆角/间距/字号/控件高度尺度令牌，**新代码禁止魔法数，一律用 var(--wk-space-*/radius-*/fs-*)**；样式已拆为 variables → base → layout → components → element，入口仍是 `styles/index.scss`。② 新增 `web/src/components/` 9 个展示组件（WkCard/WkMetric/WkProgressBar/WkStatusDot/WkBadge/WkSparkline/WkChart/WkEmptyState/WkSkeleton）与 `web/src/composables/`（useTheme/usePolling/useOverview）、`web/src/utils/`（format.ts/charts.ts）。**改页面先查有没有现成 Wk* 组件与 format 函数可用，不要在页面里重写卡片/进度条/字节格式化**。③ 主色注入链路：设置页 `theme.primary` → `useTheme().setPrimary()` → 只写 `--wk-primary`，派生色与 `--el-color-primary` 靠 `color-mix()` 自动联动（旧版只存库不注入，自定义主色一直无效）。④ **ECharts 永远不能用 CSS 变量当颜色**：canvas 不解析 var()，必须经 `utils/charts.ts` 的 `readChartTokens()` 用 getComputedStyle 取真实色值；图表统一用 `WkChart` + `builder` 函数，它监听 `themeVersion` 保证切主题/改主色时重建。⑤ 实时数据统一走 `useOverview()` 单例（引用计数共用一个 1s 定时器 + 页面不可见暂停），**禁止再在组件里手写 `setInterval(fetch, 1000)`**；内存滞后的 600 个采样点给 sparkline 与集群趋势用。⑥ 负载分级全局统一为 `LOAD_WARN=70 / LOAD_DANGER=85`（`utils/format.ts`）；丢包不能套用资源阈值，用 `lossTone()` 类独立语义。⑦ 已知限制：`/api/public/theme` 不返回 `primary`，未登录访客看不到自定义主色；要覆盖需后端补 1 个字段（本次按“不改后端”未做）。

- **2026-10-05 05:10（北京时间）**：生产已迁到 **arm64 机 `146.56.173.198`**（容器 `wukong`，`-p 64443:64443` 对公网 + `-v /opt/wukong/data:/opt/wukong/data` + `--env-file /opt/wukong/wukong.env`），入口 `https://server.lkz.pub`（Cloudflare 橙云→回源 64443 明文）。三条必须知道的结论：① **Cloudflare 橙云不能代理 gRPC 到明文源站**（实测 403 + text/html），所以 SQLite `agent_server_addr` 定为 `146.56.173.198:64443` 直连源站，只有网页走 CDN；② CI 已改为 **amd64/arm64 各跑一个原生 runner（`ubuntu-24.04-arm`）按 digest 推送 + merge job 合并 manifest**，约 2.5 分钟；不要再回到 QEMU 方案（超 10 分钟），也不要在 `Dockerfile` 里写死 `GOARCH`（用 `TARGETARCH`）；`download-artifact` 必须限定 `pattern: digests-*`，否则会把 buildx 的 `*.dockerbuild` artifact 一起下载并失败；③ 安装脚本已改为“先 `systemctl stop wukong-agent` + 下载到 `.new` 再 `mv -f`”；**直接 `curl -o` 覆盖运行中的二进制会被内核拒（ETXTBSY，curl 报 error 23）**，写任何“覆盖已安装二进制”的脚本都要用 rename 而不是原地写。另：`AGENTS.md` 历史上把管理员密码明文写进了公开仓库，已记入 `DEPLOY_CREDENTIALS.md` 待处理，新节点接入统一用一次性 token 的安装命令。

- **2026-10-05 05:52（北京时间）**：运营商 Ping 目标改为**可按节点设作用域**。`isp_targets` 新增 `scope`（`all`/`include`/`exclude`）与 `agent_ids`（逗号串，已加 ALTER TABLE 迁移，旧库自动升级且默认 `all`）；判定入口统一用 `store.ISPTarget.AppliesTo(agentID)`，**过滤只在主控下发时做**（`enabledPingTargetsFor(agentID)`，包括注册响应与 `buildConfigFrame` 两处），探针不改也不升级。约束：`validateISPTarget` 要求 include/exclude 至少选一个节点，否则“仅选中”会静默退化成“全部节点”。公开详情页的 `publicPingISPs(agentID)` 也只列该节点实际会测的线路，避免空行。典型用途：IPv6 目标（如上海移动 `2409:8088::a`）必须排除无公网 IPv6 出口的节点（现网 hk2香港、sh1上海），否则这些节点上该线路永远 100% 丢包并误告警；设置页目标地址含冒号时会提供“选中有/无 IPv6 出口节点”一键选择（依据探针自报的 `ip_v6`）。

- **2026-10-05 06:30（北京时间）**：一轮 UI/告警细节修复，留下六条长期约束：① 卡片/指标区的文本**一律不留空格**（`relativeTime` 输出“5分钟前”、`formatRateShort` 输出“745B/s”、`formatDuration` 输出“22d18h32m”）+ `white-space:nowrap`，否则带空格文本在窄卡里折行、每秒文本宽度变化会造成 1↔2 行跳动。② **节点新鲜度一律用主控时钟 `last_seen_at`**，不要用 `updated_at`：后者是探针自报的 `sys.Timestamp`，实测 ff1 法兰克福机器时钟慢 5.6 分钟，导致“后台在线、公开页离线”。③ 告警抑制期只能压制重复触发，**绝不能 `return` 掉恢复判定**（否则改完目标后旧告警最长 30 分钟不恢复）；改完影响探针的配置要 `AgentServer.InvalidateAgentConfigs()` 标脏、由 stream 自己的 goroutine 重发（gRPC stream 不能并发 SendMsg）。④ **多系列取值差一个量级就拆图而不是共轴**（Ping 5.4ms vs 5.6ms 在 0~60 轴上只差 0.3% 高度，对数刻度也无效）；最终按用户要求只保留叠加对比 + 每线路统计摘要行（`WkPingChart.vue`，两页共用）。⑤ 覆盖 Element Plus 浮层必须**背景、文字色、箭头三者一起接管**且选择器含 `.is-dark/.is-light`，否则浅色主题下白底白字看起来像一个“空白悬窗”；ECharts 图例在左上时**不要写 `yAxis.name`**（轴名也画在左上，会盖住图例）。⑥ 列表默认**按名称排序**（`localeCompare` 中文序），前后端口径统一用 `utils/format.ts` 的 `nodeState()`。另：本机无 Go 但有 docker，用 `docker run -v wukong-gomod:/go/pkg/mod golang:1.25 go vet ./...` 就能本地把关编译，不必等 CI。

- **2026-10-05 08:20（北京时间）**：新增**微信推送渠道（pushplus 中转）**，渠道覆盖微信 ClawBot / 公众号 / 企业微信应用 / QQ / 邮件。关键结论：① **不走直连 iLink 协议**（要自己维护扫码凭证和 `context_token`）；② 告警出口已收敛为 `Engine.notifyChannels(msg)`，Telegram 逐条即时、pushplus 走 `notify.AlertAggregator` 合并节流（空闲第一条立即发 → 进 5 分钟窗口→窗口结束合成一条），因为 **ClawBot 每 10 条/每 24h 需用户在微信里主动发消息激活**，且 pushplus 自身有 1 分钟 5 次 / 相同内容 1 小时 3 条 / 单日超 1000 次封号 7 天的硬红线；**想避开 10 条限制就换渠道（`wechat`）而不是做多账号轮换**，后者违反风控会封号，禁止实现。③ `pushplus` 接口是**异步**的，`code=200` 只代表已受理（实测假令牌也是 `HTTP=200` + `code:903`），必须看业务码；900/903/905/888 类错误**不得重试**（`notify.retryableError` 接口）。④ 设置项 `pushplus_*` 写入 SQLite 固化，`pushplus_token` **不进 `allowedSettingKeys` 白名单**且永不回显（与 `telegram_bot_token` 同标准）。⑤ 详情链接依赖 `site_domain`；未配置则不输出链接。

- **2026-10-06 06:53（北京时间）**：公开页顶栏对齐 + 手机端布局体系。① 顶栏“太靠边”的根因是 `<header class="wk-public-nav">` 没套 `.wk-public-inner`（正文套了 `width:min(1240px,100%-32px); margin:0 auto`），space-between 直接贴视口边缘；现在结构是“外层通铺 sticky 背景 + 内层 `.wk-public-inner .wk-public-nav-row`”，两个公开页一致。**新增任何全宽 header/nav 都要套同一个 inner 容器**。② 断点固定三档：≤1080 / ≤860 / ≤640，手机规则集中在 `styles/components.scss` 末尾，**不要再发明新断点数值**。③ 窄屏下“不裁信息”和“不跳动”必须一起解决：节点卡底行原来是 `nowrap+overflow:hidden`（手机直接裁掉），改成 `flex-wrap:wrap` 又会每秒 1↔2 行跳；最终方案是**固定折两行 + 锁 `min-height:34px`**，高度恒定后网格行高一致。凡“每秒变化的文本”在窄容器里换行都要用这个套路。④ `<style scoped>` 是纯 CSS，写 `//` 注释会让 `vite build` 直接失败（`Unexpected '/'`），只有 `lang="scss"` 块能用 `//`；改完样式必须先本地 `npx vite build`。

- **2026-10-06 07:21（北京时间）**：修 X 轴 `undefined` 与公开详情页板块无间距。① **趋势点字段名两套接口不一样**：管理接口 `store.RawSystemMetric` 是 `json:"ts"`，公开接口 `public.go` 是 `json:"timestamp"`，读错就整条 X 轴变 `undefined`（且只有一个页面出错，容易误判为数据问题）；前端取时间统一 `item.ts ?? item.timestamp`。② `formatClock`/`formatHourMinute` 对非法值**必须返回 `-`**，旧代码 `String(value)` 会把 "undefined" 画上图表轴——所有"看起来像数据其实是字段名错"的问题都源于此。③ 公开详情页 `main` 是普通 block，板块之间必须挂 `.wk-public-sections`（flex column + `--wk-gap-section`），与后台 `.wk-container-inner` 同令牌。④ **媒体查询不增加特异度**：`index.scss` 顺序是 variables→base→layout→components→element，覆盖 Element Plus 的手机端规则只能写在 `element.scss`，放 `components.scss` 会被靠后的同特异度规则静默覆盖。

- **2026-10-06 07:43（北京时间）**：仓库卫生——`internal/webapi/dist/` **不再跟踪构建产物**，只留 `.gitkeep`。三条必须知道的规则：① 已跟踪文件不受 `.gitignore` 影响（`git check-ignore` 对它们报"未忽略"是 Git 行为不是规则错），解除跟踪必须 `git rm -r --cached`（本地文件不会被删）。② `.gitignore` 里**目录级排除会让子文件取反失效**（Git 不进入已忽略目录），必须写 `internal/webapi/dist/*` + `!internal/webapi/dist/.gitkeep`。③ **`//go:embed all:dist` 要求目录非空**：整个 dist 不在时 `go build` 直接报 `pattern all:dist: no matching files found`，所以不能彻底清空，要留占位；缺 `index.html` 时 `embed.go` 的 `init()` + `PlaceholderHandler` 会显示"请运行 make build-frontend"引导页而不是白屏。CI 不受影响（Dockerfile 用 `COPY --from=frontend-builder` 取现场构建产物）。本地构建走 `make all` / `make dev`（都已串 `build-frontend`）。

- **2026-10-07 00:53（北京时间）**：告警从"扁平阈值"升级为"**每项一条独立规则**"。六项（offline/cpu/mem/disk/ping_latency/ping_loss）各自有 开关/阈值/持续时间/恢复滞回/抑制期，存 `settings.alert_rule_<metric>` JSON。关键约束：① **默认值、范围、单位、"哪一项有持续时间/滞回"只定义在 `internal/alert/rules.go` 的 `ruleSpecs` 一处**，引擎兜底、API 校验、前端渲染都从这里取（`GET /api/alert-rules` 同时下发 specs+rules），前端不许再写死数字；新增告警项只加一条 spec + 一次 `checkMetric` 调用。② 首次读取会从旧扁平 key **自动迁移**并落库，不会抹掉用户已配的阈值；改模型时必须保留这条迁移路径。③ 离线项 `has_duration/has_recovery` 都是 false（warning 语义就是"无心跳秒数"），`normalize` 强制置零。④ **滞回必须低于阈值**，否则 `value <= recovery` 永不成立、告警无法恢复，`normalize` 与 `ValidateRule` 双侧拦截。⑤ **关闭某项会静默清理它遗留的 firing 记录**（只在"启用→关闭"那次执行，不发通知，避免十几个节点一起推打爆微信配额）。⑥ `/api/alert-settings` 保留向后兼容（PUT 只改 warning/duration 并回写旧 key），新前端一律用 `/api/alert-rules`。⑦ 卡片副标题原先写的"探针/分组/全局三级回退"从未实现（store 里没有节点/分组级告警字段），已按实际行为改描述。

- **2026-10-08 14:48（北京时间）**：IPv6 出口有效性判定。net1上海 上报的 `64:ff9b::aff:fb01` 是**运营商 NAT64 合成地址**（`64:ff9b::/96`，RFC 6052，只能 v6→v4），对真 IPv6 目标 100% 丢包、对同家 v4 线路 0% 丢包 —— 证明"有公网 IPv6 地址 ≠ IPv6 可用"。规则：① **判定只在 `internal/netutil` 一处**（`IsUsablePublicIPv6` 排除 NAT64/Teredo `2001::/32`/6to4 `2002::/16`/DS-Lite `100:64::/10`，`IsPublicIPv4` 另排 CGNAT），探针上报、主控清洗、下发过滤、公开页四处共用，**禁止再复制第二份 isPublicIP**。② **主控是权威清洗方**：`effectiveIPv4/effectiveIPv6` 每次上报重校验库里已有值，不合法即置空 —— 只改探针判定无效，因为"非空才覆盖"会让一次误存永远清不掉。③ **作用域 + 能力双判定**统一走 `ISPTarget.AppliesToAgent(agent)`（IPv6 目标必须有可用 v6 出口），gRPC 下发与 `publicPingISPs` 共用；只按人工勾选的静态列表下发会让失去 v6 能力的节点长期误告警。④ 网卡回退路径**不上报 IPv6**（无连通性证据）；`setPublicIPs` 区分"v4 成功+v6 空=确实没有→清空"与"两者都空=网络抖动→保留旧值"。⑤ 判定前缀在 `init()` 预计算，不要写无锁 map 缓存（并发写 fatal error）。⑥ 后台节点列表出口 IP 列 v4/v6 各一行同时显示（旧版 v6 只藏在 hover title）。

- **2026-10-10 04:09（北京时间）**：公开详情页丢包色条升级为**丢包时间轴**。用户反馈旧版 hover 只有"第 105 段：部分丢包"、看不出对应几点。改动：① 新增 `web/src/utils/ping.ts` 作为 `PingPoint`/`pointTime`/分桶合并算法的唯一来源（原先 `WkPingChart.vue` 里 `export interface` 在 `<script setup>` 下外部根本 import 不到、页面里又各写一份），**跨文件共享类型必须放 .ts 模块**。② **多行色条必须按全局时间域分桶**（所有序列的最早点—最晚点），旧版每条线路各自首尾分桶，一旦某条数据有缺口整行错位、同一列不是同一时刻 —— 加时间刻度前必须先修这个。③ 新增 `WkLossStrip.vue`：时间刻度轴 + 每格 title 精确区间与峰值 + **连续丢包合并成时段列表直接给出答案**（段数超上限按 bad 优先/峰值降序挑，不是取前 N 个）+ 右侧同时显示平均与峰值（平均 0.2% 可能藏着一次 100% 丢包）。④ 色条本体 `.wk-strip`/`.wk-strip-cell` 留 components.scss 供复用，行布局样式进组件 scoped。

- **2026-10-10 04:28（北京时间）**：后台节点详情页也接入 `WkLossStrip` 丢包时间轴（与公开页同一组件，ping 数据字段差异 `bucket_min`/`timestamp` 已在 utils 内兼容）。两点长期约束：① **`v-if`/`v-else` 必须是相邻兄弟节点**，中间插入任何元素都会让 `v-else` 失去关联 —— 要在"有数据"分支放多个块就用 `<template v-else>` 包住（本次踩过）。② 色条格数与"每格多少分钟"统一由 `utils/ping.ts` 的 `DEFAULT_LOSS_CELLS` / `stripBucketMinutes()` 提供，页面副标题不许再自己写 120。

## 部署相关长期提示

- **部署目录**: `/opt/wukong/`，主控 wukong.conf 权限 600，signing/ 权限 400
- **nginx**: 443 → 64443 反代，gRPC 用 `grpc_pass`，SSE 必须 `proxy_buffering off`
- **密钥安全**: ed25519 私钥 /opt/wukong/data/signing/ed25519.key 权限 400，首次登录后删除 .admin_password
- **探针注册**: 一次性 token 30 分钟过期，注册即作废；注册后服务器下发 agent_secret 个体凭证
- **签名校验**: 安装脚本 web 后端不接触私钥，签名请求通过 Unix Socket 发送到 signer 进程
- **更新记录**: 2026-06-21 09:49 骨架完成；2026-06-21 10:05 完成安装脚本+部署文档+DEPLOY_CREDENTIALS.md；2026-06-21 10:20 补Docker/GHCR/GitHub Actions
- **Docker 部署**: `deploy/Dockerfile` multistage 全量编译（Vue3→Go→alpine 运行），直接暴露 64443，不依赖 nginx
- **GHCR 自动构建**: `.github/workflows/docker.yml` 在 push main 或 tag v* 时自动构建推 ghcr.io
- **快速运行**: `docker run -p 64443:64443 -e WUKONG_ADMIN_PASSWORD=xxx ghcr.io/wukong-monitor/wukong-server:latest`
- **docker compose**: `WUKONG_ADMIN_PASSWORD=xxx WUKONG_JWT_SECRET=xxx docker compose -f deploy/docker-compose.yml up -d`