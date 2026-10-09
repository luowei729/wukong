# wukong 监控系统 部署方案

> 版本: v0.1.0
> 创建日期: 2026-06-21 09:49 (北京时间)
> 最后更新: 2026-06-21 18:39 (北京时间)
> 状态: **公开首页、安装 token、在线二进制下载、站点域名保存固化已修复；已加入秒级刷新、全站禁用缓存、数据库固化修改密码、Telegram 测试、离线阈值、节点改名、amd64/arm64 探针安装、443 gRPC 连接、Ping 运营商配置、服务器配置和 qio.ng 风格详情字段；生产已部署并通过无头 Chrome 验证**

## 一、项目概述

wukong 监控是一个类似哪吒探针的服务器探针系统，实时探测服务器状态，gRPC 双向流通信，单二进制极简部署。整个系统包括：

- **主控（server）**：Go 单二进制，embed Vue3 前端，cmux 单端口同时服务 gRPC（探针通道）和 HTTP（Web API + SSE + 前端静态资源）
- **探针（agent）**：Go 单二进制，采集 CPU/内存/磁盘/网络/Ping，gRPC 上报，本地 10min 缓冲
- **签名服务（signer）**：ed25519 独立进程，私钥与 web 后端物理隔离，Unix Socket 通信

## 二、技术栈

| 层面 | 技术 | 说明 |
|------|------|------|
| 后端 | Go 1.22+ | 单二进制，cmux 双协议 |
| 前端 | Vue3 + Element Plus + ECharts | Go embed 进单二进制，暗黑科技风双主题 |
| 存储 | SQLite | WAL 模式，按小时分表，1 分钟预聚合 |
| 探针 | Go | gopsutil 采集，gRPC 上报 |
| 通信 | gRPC 双向流 | 探针个体凭证认证，指令 ed25519 签名 |
| 部署 | systemd + nginx | 裸跑反代 |

## 三、部署架构

```
浏览器 ──HTTPS 443──→ nginx ──┬→ proxy_pass http://127.0.0.1:64443 (Web REST/SSE)
                              └→ grpc_pass  127.0.0.1:64443 (gRPC双向流)
                                    │ cmux 同端口区分 HTTP/1.1 与 HTTP/2
                                    ▼
                              主控 wukong-server
                              ├── SQLite (/opt/wukong/data/wukong.db)
                              ├── 内存 agents_latest map (SSE源)
                              └── Unix Socket → 签名服务 signer
                                    ▲ gRPC over TLS (个体凭证认证, 指令需签名验签)
                              探针 N 台 (各 /opt/wukong/agent/)
```

## 四、核心安全架构

1. **双层鉴权**：Web 后台走 JWT+TOTP（管理员），探针走个体凭证（agent_id + agent_secret）+ 指令签名
2. **签名私钥隔离**：ed25519 私钥在 signer 进程中，web 后端只能通过 Unix Socket 请求签名，无法直接拿私钥
3. **指令白名单**：探针只接受白名单内的签名指令（更新配置、重启探针），不执行任意 shell
4. **一次性安装 token**：30 分钟有效，注册即作废，防范未授权注册
5. **二进制签名**：安装脚本和探针二进制用 B2 私钥签名，防中间人攻击

## 五、测试验证

```bash
# 启动主控
export WUKONG_ADMIN_PASSWORD='<bcrypt hash>'
./build/wukong-server --config /opt/wukong/wukong.conf

# 验证 API
curl http://127.0.0.1:64443/api/health
# → {"status":"ok","version":"0.1.0"}

# 启动签名服务
./build/wukong-signer --socket /opt/wukong/data/signer.sock

# 启动探针
./build/wukong-agent --config /opt/wukong/agent/agent.conf
```

## 七、2026-06-21 11:57（北京时间）首页白屏修复记录

### 改动前总结
部署后首页返回 HTML，但浏览器显示空白。无头 Chrome 复现显示 Vue 已挂载但 `#app` 内容为空；网络检查发现 Vite 生成的 `_plugin-vue_export-helper-*.js` 下划线资源返回 404。

### 改动后总结
Go embed 从 `dist/*` 调整为 `all:dist`，保证下划线开头资源进入单二进制；新增 SPA 静态处理器，刷新前端 history 路由时回退到 `index.html`，但缺失的 JS/CSS 仍返回 404，方便后续排查真实资源问题。

### 验证要求
- `/assets/_plugin-vue_export-helper-*.js` 返回 200。
- 无头 Chrome 打开首页后 DOM 出现“管理员登录 / 用户名 / 密码”。
- `/dashboard` 刷新返回前端入口，交给 Vue Router 鉴权跳转登录。

## 八、2026-06-21 12:39（北京时间）公开首页与安装命令修复记录

### 改动前总结
系统首页默认进入后台登录页，未登录无法看到服务器状态；安装命令在没有配置域名时仍可复制占位命令，并错误使用 `curl -k token`，导致 token 没有传到安装脚本。

### 改动后总结
- `/` 改为公开服务器状态首页，未登录可访问。
- `/server/:id` 新增公开服务器详情页，可从首页服务器卡片点击进入。
- 新增 `/api/public/servers*` 脱敏只读接口，管理接口仍保持 JWT 鉴权。
- 修复最新指标查询的 `UpdatedAt`，公开页面可展示最近更新时间和数据延迟状态。
- 安装命令必须先配置 `site_domain`；正确格式为 `curl -fsSL "https://域名/api/install-agent.sh?k=<token>" | bash`。
- 安装脚本缺少 `?k=` 时直接报错，避免生成空 TOKEN 脚本。

### 验证要求
- 未登录打开 `/` 能看到公开首页，不跳 `/login`。
- 未登录打开 `/server/:id` 能看到公开详情或友好空态。
- 未登录打开 `/dashboard` 必须跳 `/login?redirect=/dashboard`。
- 未设置 `site_domain` 时后台不能复制安装命令。
- 设置站点地址后生成的安装脚本中 `TOKEN` 非空。
- 本机探针使用生成的 token 能注册进系统，不能再出现 `token is malformed`。

### 验证结果
- `go test ./...` 已通过。
- `cd web && npm run build` 已通过，Vite chunk size warning 不影响构建。
- 本地主控 `127.0.0.1:18080` 启动成功，真实登录接口可签发 JWT。
- 未配置 `site_domain` 时，安装 token 接口返回 `ready=false`、`script_url=""`，前端无可复制命令。
- 设置 `site_domain=http://127.0.0.1:18080` 后，安装命令为 `curl -fsSL "http://127.0.0.1:18080/api/install-agent.sh?k=token-..." | bash`，不再使用错误的 `curl -k token`。
- 请求安装脚本后确认 `TOKEN="token-..."` 非空，`SERVER_ADDR="127.0.0.1:18080"` 正确。
- 本机探针使用生成的 token 注册成功，节点 `home-pc` 已加入系统并在线；主控和探针日志未出现 `token is malformed`。
- 无头 Chrome 打开 `/`、`/server/:id`、`/dashboard` 均非白屏；`/dashboard` 未登录会显示登录页，公开首页和公开详情未登录可访问。
- 公开 API `/api/public/servers` 返回 1 台本机节点，不包含 `secret`、`token` 等敏感字段；未登录访问 `/api/agents` 仍返回 401。

## 九、2026-06-21 13:32（北京时间）在线安装探针二进制下载 401 修复记录

### 改动前总结
安装脚本已经能通过 `/api/install-agent.sh?k=<token>` 拿到非空 TOKEN，但脚本继续下载 `/api/agent/binary/latest/$ARCH` 时，该二进制下载路由仍被 JWT 鉴权中间件拦截，远程服务器会返回 401 并提示“探针二进制下载失败”。

### 改动后总结
- `/api/agent/binary/{version}/{arch}` 改为无需 JWT 的只读二进制下载接口，仅允许 `amd64` 和 `arm64`。
- Docker 镜像构建时同时编译 `wukong-agent-amd64` 与 `wukong-agent-arm64`，运行镜像内置到 `/opt/wukong/bin/`。
- 下载接口只从固定发布目录读取 agent 二进制，不读取任意路径，不返回管理配置、token 或 secret。

### 验证结果
- `go test ./...` 已通过。
- 本地 `GET /api/agent/binary/latest/amd64` 返回 `HTTP/1.1 200 OK`，响应体为 ELF 二进制，不再返回 401。


```
/opt/wukong/
├── wukong                # 主控二进制
├── wukong-signer         # 签名服务二进制
├── wukong.conf           # 主控配置
├── deploy/
│   ├── nginx/wukong.conf # nginx 反代配置
│   └── scripts/
│       ├── install-server.sh  # 主控安装脚本
│       └── install-agent.sh   # 探针安装脚本
├── data/
│   ├── wukong.db         # SQLite 数据库
│   ├── uploads/          # Logo 等上传文件
│   ├── signing/          # ed25519 密钥对（权限 400）
│   ├── signer.sock       # 签名服务 Unix Socket
│   └── .admin_password   # 初始密码（首次登录后删除）
└── agent/                # （仅探针节点）
    ├── wukong-agent      # 探针二进制
    ├── agent.conf        # 探针配置（权限 600）
    ├── data/             # 探针本地数据
    └── server.txt        # 主控地址
```

## 十、2026-06-21 14:10（北京时间）远程探针 gRPC 地址与本机节点加入验证

### 改动前总结
`site_domain=https://server.lkz.pub` 能让 Web/API 与探针二进制下载正常工作，但安装脚本会把探针 gRPC 地址自动推导为 `server.lkz.pub:443`。实测该地址当前 gRPC 注册超时，说明 443 反代尚未支持探针 gRPC；而 Docker 主控已暴露 `64443`，直连 `server.lkz.pub:64443` 可以完成注册和上报。

### 改动后总结
- 新增 SQLite 设置项 `agent_server_addr`，后台设置页可填写“探针 gRPC 地址”，格式必须为 `host:port`。
- `site_domain` 继续负责安装脚本 URL 和二进制下载 BASE_URL；`agent_server_addr` 负责脚本内 `SERVER_ADDR`，未配置时才回退按站点域名推导。
- 安装 token 接口会预先校验探针 gRPC 地址，格式错误时返回 `ready=false`，避免用户复制不可用命令。
- 本机临时探针已使用 `server.lkz.pub:64443` 注册到远程主控并上报指标，公开 API 与后台 API 均可看到该节点。

### 验证结果
- `server.lkz.pub:443` TCP 可达但 gRPC 注册超时。
- `server.lkz.pub:64443` gRPC 注册成功，节点名 `home-pc-server-lkz-e2e`。
- `https://server.lkz.pub/api/public/servers` 返回 1 台在线节点，包含 CPU/内存/磁盘/流量指标且不含敏感字段。
- 后台 `/api/agents` 和 `/api/agents/latest` 均可看到本机节点和最新指标。
- `go test ./...` 通过。
- `cd web && npm run build` 通过。

## 十一、2026-06-21 15:05（北京时间）Telegram、告警、节点安装与 443 gRPC 修复记录

### 改动前总结
Telegram 通知配置缺少测试按钮，Bot Token 输入框容易被浏览器当密码填充；告警阈值页缺少离线阈值设置，报警中心空数据时可能闪现后消失。探针安装虽能通过 `server.lkz.pub:64443` 直连成功，但用户要求生产必须通过 `server.lkz.pub:443`，而旧探针客户端连接 443 时仍使用明文 gRPC，不适配 nginx 的 HTTPS/HTTP2 入口。

### 改动后总结
- Telegram 配置改为不回显 token、普通输入框并关闭自动补全，新增 `POST /api/telegram/test` 测试通知。
- 告警设置新增离线秒数和资源持续时间，写入 SQLite `settings` 固化；告警引擎每 5 秒读取并应用最新阈值。
- 报警中心接口和前端都兜底空数组，避免无告警时页面闪退。
- 探针安装支持 amd64 / arm64，注册成功后退出，由 systemd 后台常驻并开机自启。
- 探针客户端按端口选择传输层：`443` 使用 TLS gRPC，适配 nginx `listen 443 ssl http2`；其他端口保持明文 gRPC，兼容本地 64443。
- 静态安装脚本修复 `SERVER=host:443` 处理，下载走 `https://host`，注册地址保持 `host:443`。
- 后台设备页和节点详情页支持自定义节点名称。
- nginx 示例关闭页面缓存，避免发布新版本后浏览器继续读取旧资源。

### 验证结果
- `go test ./...` 已通过。
- `npm --prefix /root/wukong/web run build` 已通过，chunk size warning 不影响构建。
- 生产数据库部署时需要把 `settings.agent_server_addr` 固化为 `server.lkz.pub:443`，并确认 nginx 443 的 `/wukong.AgentService/` gRPC 反代生效。

## 十二、2026-06-21 18:23（北京时间）Ping 运营商配置、服务器配置与详情字段补齐

### 改动前总结
规划文档中要求 Ping 多运营商探测和 qio.ng 风格服务器详情，但当前只有 `isp_targets` 表和部分聚合接口，探针没有真实 PingCollector，设置页不能维护运营商目标，节点详情使用随机 Ping 数据，公开详情页缺 Uptime、Boot time、Region、CPU 型号、Load、累计流量等关键字段。

### 改动后总结
- `SystemMetric` 协议追加公开详情需要的系统规格字段，SQLite 小时表新增列并兼容旧表缺列查询。
- 探针采集 CPU 型号/核心、内存/磁盘总量、系统启动时间、负载、累计流量和手动区域。
- `PingCollector` 按 `ping_interval` 探测运营商目标，支持 `auto/icmp/tcp`，auto 模式 ICMP 失败后回退 TCP。
- 注册响应下发启用的运营商目标，探针写入本地配置；主控每分钟聚合 Ping 数据。
- 设置页新增“Ping 运营商”页签，节点详情新增服务器配置表单并接入真实 Ping 聚合图。
- 公开详情页展示 qio.ng 风格规格字段和真实 Ping 图，公开 API 仅返回脱敏字段与启用 ISP 名称，不返回目标 IP、端口、secret、token 或管理配置。

### 验证要求
- 新增/编辑/删除 Ping 运营商目标刷新后仍从 SQLite 读回。
- 新注册或重启探针后 `agent.conf` 包含 `ping_interval` 和目标列表，并能上报真实 Ping。
- `/api/agents/{id}/ping-agg?isp=<name>` 和 `/api/public/servers/{id}/ping-agg?isp=<name>` 返回聚合数据或明确空数组。
- 公开详情页显示 Status、Uptime、Arch、Mem、Disk、Region、System、CPU、Load、Upload、Download、Boot time、Last active time。
- 生产部署继续保持探针走 `server.lkz.pub:443`，Docker 64443 仅本机绑定。

## 十三、2026-06-21 18:39（北京时间）生产部署与无头 Chrome 验证

### 改动前总结
代码已补齐 Ping 运营商配置、服务器配置和 qio.ng 风格详情字段，并已提交推送触发 GHCR 构建；但生产环境还停留在旧容器，远程本机探针未安装运行，公开详情页无法展示本阶段新增的系统规格字段和 Ping 聚合结果。

### 改动后总结
- GHCR 最新镜像已拉取到远程服务器并重建 `wukong` 容器，64443 继续仅绑定 `127.0.0.1`。
- 生产 SQLite 设置确认保留 `site_domain=https://server.lkz.pub` 与 `agent_server_addr=server.lkz.pub:443`。
- 远程本机探针已通过在线安装脚本注册并由 systemd 常驻运行，日志确认连接 `server.lkz.pub:443`。
- 已新增一个启用的 Cloudflare TCP Ping 目标用于生产链路验证，探针配置中已固化 `ping_interval` 和目标列表。
- 公开详情 API 已返回新增系统字段，Ping 聚合接口已返回 Cloudflare 聚合点；公开响应未泄露目标 IP、token、secret、JWT/TOTP 或 Telegram 配置。
- 无头 Chrome 打开 `https://server.lkz.pub/` 和 `https://server.lkz.pub/server/<id>` 均不是白屏，详情页展示 qio.ng 风格字段和网络延迟区。

### 验证结果
- Docker 端口：`127.0.0.1:64443->64443/tcp`。
- 健康接口：`{"status":"ok","version":"0.1.0"}`。
- 公开服务器数量：3，其中生产本机节点 `us4` 在线。
- 公开详情字段包含：Status、Uptime、Arch、Mem、Disk、System、CPU、Load、Upload、Download、Boot time、Last active time。
- Ping 聚合：Cloudflare 已返回至少 1 个分钟聚合点。
- 无头 Chrome 截图：`/tmp/wukong-chrome-final/home.png`、`/tmp/wukong-chrome-final/detail.png`。

## 十四、2026-10-05 04:13（北京时间）Web UI 重构为现代 SaaS 控制台

### 改动前总结
前端已完成上一轮换色改造，但仍有结构性欠债：自定义主色只存库不注入（决策 #11 无效）、ECharts 把 CSS 变量当颜色传给 canvas（轴/图例颜色失效且主题写死 dark）、后台节点详情页只用了后端 20 个实时字段中的 3 个、格式化工具函数在 5 个文件重复且口径不一、顶栏与总览/节点页重复轮同一份数据。

### 改动后总结
- **视觉方向**：现代 SaaS 控制台（Linear 近黑画布 + hairline 边框 + 单一强调色；Stripe table-first、颜色只表达状态；Vercel 中性灰阶与排版层次）。暗色画布 #0a0a0b、主色 #5e6ad2；浅色画布 #f7f7f8、主色 #4f5bd5。
- **新增层次**：`web/src/components/`（9 个 Wk* 展示组件）、`web/src/composables/`（useTheme/usePolling/useOverview）、`web/src/utils/`（format.ts/charts.ts）；样式拆为 variables/base/layout/components/element。接口、路由、功能分区完全没动，**未改 `internal/` 任何 Go 代码，未新增 npm 依赖**。
- **功能补齐**：后台节点详情页从 3 字段扩充到全部后端已有字段（CPU 型号/核数、负载 1/5/15、总内存/总磁盘、累计上下行、启动时间、区域、出口 IP）+ 1h/6h/24h 资源趋势；总览页新增卡片/列表双视图与集群趋势；告警中心新增筛选与超阈程度条；公开首页新增在线率圆环并改用后端 `summary`；公开详情页新增 24h 丢包色条。
- **修复**：自定义主色注入链路打通；ECharts 颜色改为 getComputedStyle 解析真实色值并随主题重建；负载阈值全局统一 70/85；重复轮询收敛为共享单例并支持后台标签页暂停；补 `web/public/favicon.svg` 与 `theme-color`；删除 `App.vue` 重复 reset。
- **文档**：新增根目录 `DEVTIPS.md`，收录设计令牌契约、主色注入链路、ECharts 与 CSS 变量坑、轮询约定、表格密度约定、本地 mock 验 UI 方法。

### 验证要求
- 部署前必须在真实主控 + 真实探针环境再跑一轮：本次因本机未安装 Go 工具链，只用了与 `vite.config.ts` proxy 对齐的只读 Mock API（`build/mock-api.mjs`）完成无头浏览器逐页核验（console error 0、4xx/5xx 0、双主题与 6 种主色切换、1440/820 两档宽度）。
- 重点回归项：探针每秒上报下页面稳定性、`agent_server_addr`/`site_domain` 保存与安装命令生成、告警阈值保存、Telegram 测试发送。

### 验证结果
- `npx vue-tsc --noEmit` 零错误；`npx vite build` 成功，产物含 `favicon.svg`（输出到 `internal/webapi/dist/`，
  该目录已被 gitignore，镜像里的前端由 CI 从源码重新构建，不需提交产物）。
- 无头浏览器三轮逐页截图：公开首页/公开详情/登录/总览/节点列表/节点详情/告警/设置 均非白屏，图表轴与图例颜色随主题正确变化，自定义主色在侧栏/按钮/图表首色上生效。

## 十五、2026-10-05 05:10（北京时间）新 UI 上生产：arm64 机 + Cloudflare CDN

### 改动前总结
要求把重构后的前端部署到生产 `146.56.173.198`（Cloudflare 已配到 64443，域名 `https://server.lkz.pub`）。
实际发现三个阻塞：① 该机是 arm64 且从未跑过主控（全盘无 `wukong.db`，只有 agent），而 GHCR 只有 amd64 镜像；
② QEMU 多架构构建超 10 分钟未完；③ 安装脚本在已装探针的机器上因 ETXTBSY 必然失败（`curl: (23)`）。

### 改动后总结
- CI 改为原生 ARM runner 并行 + digest/manifest 合并（~2.5 分钟），`Dockerfile` 主控改用 `TARGETARCH`。
- 安装脚本改为先停服务 + 临时文件 `mv -f` 原子替换；`agent_server.go` 鉴权日志区分“节点未注册”与“密钥不匹配”。
- 容器部署：`-p 64443:64443`（对公网，CF 需外部回源）+ `-v /opt/wukong/data:/opt/wukong/data` + `--env-file /opt/wukong/wukong.env`（JWT 与管理员密码固化）。
- 关键结论：**Cloudflare 橙云不能代理 gRPC 到明文源站**（实测 403+html），`agent_server_addr` 改为直连 `146.56.173.198:64443`，网页仍走 CDN 443。
- 补回 Ping 运营商目标 3 条；部署参数与遗留风险记入 `DEPLOY_CREDENTIALS.md`（gitignore）。

### 验证结果
- `https://server.lkz.pub/api/health` 经 CDN 200；`index.html` 资源哈希与本地构建一致（确认是新 UI）。
- 浏览器核验线上 6 个页面：console error 0、非 200 请求 0；在线率圆环、KPI sparkline、双 Y 轴趋势图、
  系统信息定义列表（含出口 IPv4/IPv6、CPU 型号、探针版本）、Ping 引导空态均正常。
- 节点：`ubuntu`(arm64) / `sel4`(arm64) / `tk3`(amd64) 共 3 台在线每秒上报；主控已收到真实 Ping 聚合（上海电信 29.0ms / 0 丢包）。
- 待办：旧主控剩余节点需在每台重新执行安装命令；建议轮换管理员密码（已泄露在公开仓库的 AGENTS.md）；
  若要给探针链路加 TLS，需在源站给 64443 前置 `ssl http2 + grpc_pass` 并把 CF SSL 改为 Full。

## 十六、2026-10-05 05:52（北京时间）运营商 Ping 目标按节点作用域

### 改动前总结
用户要新增上海移动 IPv6 目标 `2409:8088::a`，但 `isp_targets` 无节点维度配置，启用目标无条件
下发全部探针；而现网 8 台中 hk2香港、sh1上海 无公网 IPv6 出口，这两个节点上该线路会永远 100% 丢包。

### 改动后总结
`isp_targets` 新增 `scope`（all/include/exclude）与 `agent_ids` 两列（含 ALTER TABLE 迁移，旧库默认 all）；
统一由 `ISPTarget.AppliesTo(agentID)` 判定，在 `enabledPingTargetsFor(agentID)` 处按节点过滤下发
（注册响应与配置热更新两处），**探针无需改动也无需升级**；`validateISPTarget` 强制 include/exclude 至少选一个节点；
公开详情 `publicPingISPs(agentID)` 同步过滤；设置页新增作用域下拉、节点多选（带 IPv6/IPv4 标记）、
IPv6 目标一键选节点与列表作用域列。

### 验证结果
- 前端 `vue-tsc` 零错误、`vite build` 成功；Go 编译由 GHCR 多架构镜像构建把关（本机无 Go 工具链）。
- 线上验收项：新增“上海移动IPv6”并排除无 IPv6 节点后，有 v6 出口的节点日志 `Ping目标数` 为 4，
  hk2/sh1 仍为 3；公开详情页不再出现无数据线路行。

## 十七、2026-10-05 08:20（北京时间）新增微信推送渠道（pushplus 中转）

### 改动前总结
告警通知只有 Telegram 一条路（`Engine.sendTelegramNotification` 是唯一硬出口），国内环境访问不稳定。
用户要求对接微信 ClawBot。调研后确认两条路径差异很大：直连微信官方 iLink ClawBot 协议需要主控自己
维护扫码凭证与 `context_token`，还要处理"每下发 10 条 / 每 24 小时需用户主动在微信发一条消息"的激活
续期，状态脆弱；改走 pushplus 中转后主控侧只是一次 HTTP POST，且渠道可随时在微信 ClawBot / 公众号 /
企业微信应用之间切换。用户确认：pushplus 中转 + 只推关键告警并合并 + 纯文本带详情链接 + 配置入口放设置页。

### 改动后总结
- `internal/notify/pushplus.go`：`PushplusNotifier`（`POST https://www.pushplus.plus/send/{token}`，
  `template=txt`，正文含 `site_domain` 拼出的节点详情链接）；业务码非 200 视为失败，成功日志记录消息流水号。
- `internal/notify/aggregator.go`：`AlertAggregator` 合并节流器（第一条立即发 → 5 分钟窗口 → 窗口结束合成
  一条汇总并续窗；汇总最多列 10 条，缓存上限 200，退出时同步 flush）。
- `internal/notify/notify.go`：`Message.Kind`（firing/resolved/summary）、`retryableError` 接口、
  `SendWithRetry` 导出、4xx/429 判定改为基于 `HTTPStatus()` 接口，不再只认 `*telegramAPIError`。
- `internal/alert/engine.go`：新增 `notifyChannels` 统一出口（Telegram 逐条即时、pushplus 走合并器）、
  `sendPushplus`（发送时读最新设置，热生效）、`pushplusWindow`、`pushplusAccept`、`settingString`/`settingBool`。
- `internal/webapi`：`GET/PUT /api/pushplus`、`POST /api/pushplus/test`；设置项 `pushplus_enabled`/
  `pushplus_token`/`pushplus_channel`/`pushplus_merge_minutes` 固化进 SQLite；渠道白名单校验；
  令牌不进通用 settings 白名单也不回显。
- `web/src/views/Settings.vue`：新增"微信推送"节（启用开关、令牌、渠道下拉、合并窗口、测试按钮），
  页面上写明 ClawBot 的 10 条激活限制与"改用公众号渠道可避开"。

### 验证结果
- `go vet ./...` 本机通过（改用 `docker run -v wukong-gomod:/go/pkg/mod golang:1.25`，本机无 Go 也能把关）；
  `vue-tsc` 零错误、`vite build` 成功。
- 真实探测 pushplus 接口（假令牌）：`HTTP=200` + `{"code":903,"msg":"用户令牌不正确"}`，
  证实 https 与路径可用，并证实"只看 HTTP 状态码会把失败误判成成功"。
- 待用户在后台填入 pushplus 令牌并点"发送测试通知"完成端到端确认。

### 后续可选
- 接 pushplus 开放接口（AccessKey）自动查投递状态（`sendStatus` 3=发送失败），把"未激活"明确暴露到 UI。
- 若告警量继续增长，可增加"每日配额闸门"与按分组绑定不同渠道（决策 #17 的分组路由目前仍未真正生效）。

## 十八、2026-10-06 06:53（北京时间）公开页顶栏对齐与手机端布局体系

### 改动前总结
用户反馈两点：① 公开首页的站点名、"公开服务器状态"与"管理登录"按钮太贴屏幕边缘，与下方正文不对齐；
② 手机屏幕上节点信息需要适配，要求整体排查布局。定位到顶栏根因是模板层面缺少容器：
`<header class="wk-public-nav">` 直接 space-between，而 `<main>` 套了 `.wk-public-inner`
（`width: min(1240px, calc(100% - 32px)); margin: 0 auto`），两个公开页都有同样问题。

### 改动后总结
- 顶栏改为"外层通铺 + 内层对齐"结构：`header.wk-public-nav`（sticky、hairline 下边、半透模糊背景）
  内部包 `.wk-public-inner.wk-public-nav-row`，品牌名与登录按钮与正文左边缘一致。
- 新增统一的 ≤640 手机断点段（`styles/components.scss`）：节点卡单列铺满、卡片与 KPI 卡留白收紧、
  数值字号降档、hint 省略号、丢包色条线路名 108→72px、el-table 单元格留白 12→8px。
- 节点卡底行改为"固定折两行 + 锁 min-height"：既不再被 `overflow:hidden` 裁掉信息，
  也不会因每秒文本宽度变化产生 1↔2 行跳动。
- 公开首页 Hero 手机上标题与 64px 在线率环并排（原先上下堆叠会让首屏只剩标题）；
  工具栏右侧操作提示手机上隐藏；公开详情页 ≤640 收紧 hero 让指标卡进入首屏。

### 验证结果
- `vue-tsc` 零错误；`vite build` 首次失败（`//` 注释写进了 `<style scoped>` 纯 CSS 块），
  改 `/* */` 并全库扫描后通过。
- 待部署后以 390px 视口实测：顶栏与正文对齐、卡片信息不被裁、无高度跳动。

## 十九、2026-10-06 07:21（北京时间）X 轴 undefined 与板块间距修复

### 改动前总结
用户补报两个 bug：后台节点详情「资源趋势」X 轴整条显示 `undefined`；公开详情页各板块之间没有
间隔、卡片紧贴。390px 视口实测另发现上一轮写在 `components.scss` 的手机端表格规则未生效。

### 改动后总结
- X 轴根因是字段名：管理接口 `RawSystemMetric` 序列化为 `ts`，前端读 `timestamp` → undefined；
  公开接口用的是 `timestamp`，所以只有后台页面出错。改为 `item.ts ?? item.timestamp`。
- `formatClock` / `formatHourMinute` 对 null/空/Invalid Date 统一返回 `-`，不再 `String(value)`
  把 "undefined" 画上图表，避免同类问题继续伪装成数据异常。
- 公开详情页 main 新增 `.wk-public-sections`（flex column + `--wk-gap-section`），与后台
  `.wk-container-inner` 同令牌；`.wk-detail-hero` 去掉自身上下 padding 防止与 gap 叠加。
- 手机端 el-table 密度规则从 `components.scss` 移到 `element.scss` 末尾：`index.scss` 的 @use
  顺序让 element 的同特异度规则靠后覆盖媒体查询，写在前面会静默失效。

### 验证结果
`vue-tsc` 零错误、`vite build` 成功；产物 CSS 实测 640 覆盖规则位于基础 12px 规则之后（生效），
`.wk-public-sections` 已进入产物。待部署后核对 ff1 趋势轴显示真实时间与详情页板块间距。

## 二十、2026-10-06 07:43（北京时间）停止跟踪前端构建产物

### 改动前总结
`internal/webapi/dist/` 里残留早期误提交的构建产物，且状态自相矛盾：上一次提交删掉唯一被跟踪的
`assets/index-73vta-8R.js`，而 `index.html` 引用的是未入库的 `index-B5UCCBZT.js` /
`index-BWd7ThXf.css`。干净检出后 `go build` 出的二进制必然白屏；CI 一直正常所以无人察觉。

### 改动后总结
- `git rm -r --cached internal/webapi/dist` 解除跟踪（本地文件保留）。
- `.gitignore` 改为 `internal/webapi/dist/*` + `!internal/webapi/dist/.gitkeep`：
  目录级排除会让子文件取反失效，必须排除内容。
- 新增 `dist/.gitkeep`：`//go:embed all:dist` 要求目录非空，否则 `go build` 直接失败；
  缺 `index.html` 时由 `PlaceholderHandler` 显示引导页而不是白屏。

### 验证结果
`git clone file://` 模拟干净检出：`dist/` 内只有 `.gitkeep`，`go build ./cmd/server` 与
`go vet ./...` 均 exit 0；`git check-ignore` 确认其余产物仍被忽略。CI 不依赖仓库内 dist。

## 二十一、告警规则化改造（每项独立开关与独立参数）

### 改动前总结
用户要求每种告警都能单独开关、单独调参数。核对发现：每项只有一个阈值 key、持续时间全局共用一个、
恢复滞回在引擎里硬编码 85 且无界面入口、抑制期只有配置文件一个值、`ThresholdConfig.Enabled`
定义了但引擎从未读取（所以任何一项都关不掉），而卡片副标题写的"探针/分组/全局三级回退"从未实现。
与用户确认：只做全局一份规则表，每项 5 个参数，设置页 6 张独立卡片。

### 改动后总结
- 新增 `internal/alert/rules.go`：`Rule` + `RuleSpec` 定义表（默认值/范围/单位/旧 key/是否含持续与滞回），
  存储为 `settings.alert_rule_<metric>` JSON，首次读取自动从旧扁平 key 迁移并落库。
- 引擎 `checkAlerts` 每轮读一次规则，按 `Enabled` 决定是否检查；`checkMetric` 改为接收 `Rule`；
  离线与 Ping 两项同步改造；关闭某项时 `resolveMetricAlerts` 静默清理遗留 firing（不发通知）。
- API 新增 `GET/PUT /api/alert-rules`（GET 同时下发 specs+rules，PUT 先全量校验再落库）；
  旧 `/api/alert-settings` 保留兼容并回写旧扁平 key。
- 设置页告警节改为 6 张独立卡片（开关在标题行、关掉整卡置灰、离线卡按 spec 不显示持续/滞回），
  侧栏与相关文案统一改为"告警规则"。

### 验证结果
`go vet ./...` 与 `gofmt` 本地通过（docker golang:1.25），`vue-tsc` 零错误、`vite build` 成功。
部署后需核对：规则值等于迁移前生产配置、关闭某项后不再产生新告警且日志有静默清理记录。

### 后续可选
节点级/分组级覆盖（真正落实三级回退）、critical 二级阈值分级、按项恢复通知开关。

## 二十二、IPv6 出口可用性判定与节点列表双栈显示

### 改动前总结
用户反馈 net1上海 的 IPv6 地址不可用，要求增加有效性判断，并要求后台节点列表的出口 IP 同时显示 v4 与 v6。
取生产数据定位：net1上海 上报 `64:ff9b::aff:fb01`，对「上海移动IPV6」100% 丢包（12 点），
对同家 v4 线路 0% 丢包 51.9ms。`64:ff9b::/96` 是 IANA NAT64 well-known 前缀（RFC 6052），
由运营商 NAT64 网关合成、只能 v6→v4，连不上真 IPv6 目标。原 `isPublicIP` 只排除
ULA/link-local/loopback，未排除 NAT64/Teredo/6to4/DS-Lite；且探针与主控各有一份重复判定。

另有两个会让修复失效的问题：主控"非空才覆盖"导致坏地址永远清不掉；ISP 的 IPv6 作用域是
人工勾选的静态列表（含 net1），光修判定该节点仍会收到 IPv6 目标并持续误告警。

### 改动后总结
- 新增 `internal/netutil/ip.go`：`IsUsablePublicIPv6`（排除 NAT64 `64:ff9b::/96`、`64:ff9b:1::/48`、
  Teredo `2001::/32`、6to4 `2002::/16`、DS-Lite `100:64::/10`）与 `IsPublicIPv4`（另排 CGNAT），
  前缀 init 预计算避免并发写 map。
- 新增 `store.ISPTarget.AppliesToAgent(agent)`：人工作用域 + 节点自身能力双重判定，
  gRPC 下发与公开详情页线路列表共用，消除口径分裂。
- 主控改为权威清洗方：`effectiveIPv4/effectiveIPv6` 每次上报重校验库中已有值，不合法即置空；
  注册路径同样先过判定。不需要改 proto 也不需要等探针升级即可清掉存量坏地址。
- 探针侧删除重复 `isPublicIP` 改调 netutil；网卡回退不再上报 IPv6；`setPublicIPs` 区分
  "v4 成功但 v6 空（确实没有→清空）"与"两者都空（网络抖动→保留旧值）"。
- `Nodes.vue` 出口 IP 列改为 v4/v6 各一行同时显示，列宽 124→208，v6 用 break-all 折行完整可读。

### 验证结果
`go build ./...`、`go vet ./...` 通过；新增 `internal/netutil/ip_test.go` 三个用例全部 PASS
（含 net1 真实坏地址被拒、三家真实 v6 通过）；`vue-tsc` 零错误、`vite build` 成功。
部署后核对：net1上海 ip_v6 被清空、下发 targets 由 4 回到 3、IPv6 线路不再产生 100% 丢包点。

## 二十三、丢包色条升级为丢包时间轴

### 改动前总结
用户反馈公开详情页色条 hover 只显示"第 105 段：部分丢包"，无法判断对应的时间段。
排查还发现两个结构问题：`PingPoint` 类型被声明三遍（`<script setup>` 里 export 的 interface
外部无法 import，等于白写），分桶算法散在页面里；更关键的是旧 `buildLossStrip` 按**每条线路
各自的首尾点**分桶，任一线路数据有缺口就会与其他线路错位，同一列不是同一时刻。

### 改动后总结
- 新增 `web/src/utils/ping.ts`：`PingPoint`/`pointTime`/`pointMillis` 唯一来源，
  `buildLossRows()` 改为按**全局时间域**（所有序列最早点—最晚点）分桶，`mergeLossRanges()` 合并连续丢包。
- 新增 `web/src/components/WkLossStrip.vue`：色条上方统一时间刻度（0/25/50/75/100% 五标记，
  三列共用 grid 保证对齐）、每格 title 给出精确起止时间与平均/峰值丢包及采样数、
  下方列出合并后的丢包时段（超上限按 bad 优先与峰值降序挑选）、右侧同时显示平均与峰值。
- `PublicServerDetail.vue` 接入组件并删除本地重复实现（净减 70+ 行）；`components.scss`
  删除随组件重写的死样式，仅保留 `.wk-strip`/`.wk-strip-cell` 供复用。

### 验证结果
`vue-tsc` 零错误、`vite build` 成功。用生产真实数据（sel2首尔 24h）离线复算：全局时间域跨
24.0 小时、每格 12 分钟，上海电信 3 段 / 上海移动 2 段 / 上海联通 0 段，与色条观感一致。
