# wukong 监控系统 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-10-08 14:48] - IPv6 出口可用性判定（排除 NAT64 等合成地址）+ 节点列表同时显示 v4/v6

### 改动前总结
用户反馈 net1上海的 IPv6 地址不可用，要求增加有效性判断。取生产数据定位到根因：

| 节点 | 上报的 IPv6 | 对 IPv6 线路丢包 | 对同家 v4 线路 |
|---|---|---|---|
| net1上海 | `64:ff9b::aff:fb01` | **100%**（12 点） | 0% / 51.9ms |

`64:ff9b::/96` 是 **IANA NAT64 well-known 前缀**（RFC 6052）—— 运营商 NAT64 网关合成的地址，
只能 v6→v4，访问真正的 IPv6 目标（中国移动 `2409:8088::a`）必然不通。
原 `isPublicIP` 只排除 ULA/link-local/loopback，**没有排除 NAT64、Teredo、6to4、DS-Lite**
这类"看起来是全局单播、实际不可直连"的地址。

还有两个会让修复失效的隐藏问题：
1. 探针与主控都是"**非空才覆盖**"，net1 库里那个坏地址永远不会被清掉；
2. ISP 的 IPv6 目标作用域是**人工勾选后存下来的节点列表**（当时 include 9 个含 net1），
   光修判定 net1 仍会收到 IPv6 目标、继续 100% 丢包、继续误触告警。
3. 判定逻辑在 `agentcore` 和 `grpcapi` 各有一份重复的 `isPublicIP` —— 口径分裂的根源。

### 改动后总结
- **新增 `internal/netutil/ip.go`**：统一判定。`IsUsablePublicIPv6` 在原有过滤之外排除
  `64:ff9b::/96`、`64:ff9b:1::/48`（NAT64）、`2001::/32`（Teredo）、`2002::/16`（6to4）、
  `100:64::/10`（DS-Lite）；`IsPublicIPv4` 另排除 CGNAT `100.64.0.0/10`。
  前缀在 `init()` 预计算，**不用无锁 map 缓存**（并发写 map 会 fatal error，项目历史上栽过）。
- **`store.ISPTarget.AppliesToAgent(agent)`**：人工作用域 + 节点自身能力双重判定，
  IPv6 目标必须有可用 v6 出口。gRPC 下发（`enabledPingTargetsFor`）与公开详情页
  （`publicPingISPs`）**共用这一个方法**，避免"公开页列了线路但探针根本不测"。
  查不到节点时退回只看作用域，不因一次临时失败把所有线路停掉。
- **主控改为权威清洗方**（`effectiveIPv4`/`effectiveIPv6`）：上报值不合法一律置空，
  并且**每次上报都重新校验库里已有的值** —— 这样 net1 的坏地址在下一轮上报就被清掉，
  不需要改 proto、不需要等探针升级。注册路径同样先过判定，避免一开始就存进坏值。
- **探针侧**：删除重复的 `isPublicIP`，改调 netutil；`getLocalIPs` 网卡回退**不再上报 IPv6**
  （拿不到任何连通性证据，误报代价远高于漏报）；`setPublicIPs` 改为"v4 拿到了但 v6 没拿到"
  时清空 v6（有证据说明确实没有），两者都空则保留旧值（避免网络抖动误清）。
- **`Nodes.vue` 出口 IP 列**：v4/v6 各占一行同时显示（旧版只显示 v4、v6 藏在 hover title 里），
  列宽 124→208，v6 用 `break-all` 折行保证完整可读；地址是静态数据不会造成行高跳动。
- 新增 `internal/netutil/ip_test.go` 单测钉死判定口径，含 net1 的真实坏地址与三家真实 v6 正例。

### 验证
- `go build ./...`、`go vet ./...` 通过；`go test ./internal/netutil` **3 个用例全 PASS**
  （`64:ff9b::aff:fb01` 被拒、`2409:8088::a`/Oracle/KDDI 真实地址通过）；`vue-tsc` 零错误、`vite build` 成功。
- 待部署后核对：net1上海 的 `ip_v6` 被清空、不再收到 IPv6 目标（下发 targets 数从 4 回到 3）、
  上海移动IPV6 线路不再产生 100% 丢包点；其余真实 v6 节点不受影响。


## [2026-10-07 00:53] - 告警规则化改造：6 项告警各自独立开关 + 独立参数

### 改动前总结
用户要求"每种报警都要支持单独开关、单独调节每项的详细参数"。现状核对结论：
- 每项只有一个阈值 key，**持续时间全局共用** `alert_metric_duration_seconds`（只有磁盘单独，且是引擎里硬编码的 key）
- **恢复滞回硬编码 85**（`checkMetric(..., 85, duration)`），界面没有入口
- **抑制期只有配置文件一个全局值**，界面显示但不落库
- `ThresholdConfig.Enabled` 字段定义了但**引擎从未读取** → 实际上任何一项告警都关不掉
- 卡片副标题写"支持探针/分组/全局三级回退"，但 store 里没有任何节点/分组级告警字段，实际只有全局一份（描述与实现不符）

经确认采用：只做全局一份规则表 + 每项 5 个参数（开关/阈值/持续时间/恢复滞回/抑制期）+ 设置页 6 张独立卡片。

### 改动后总结
- **新增 `internal/alert/rules.go`**：`Rule`（enabled/warning/duration/recovery/suppress_min）+
  `RuleSpec` 定义表。六项的默认值、取值范围、单位、"哪一项有持续时间/滞回"、旧 key 全部集中在
  `ruleSpecs` 一处，引擎兜底、API 校验、前端渲染共用同一份，不再两边写死数字然后漂移。
  存储为 `settings.alert_rule_<metric>` 一条 JSON（一项规则字段天然成组，整体读写不会出现"改了阈值没改滞回"的半更新）。
- **自动迁移**：`LoadRules` 发现某项没有新 key 时，从旧扁平 key 取值（CPU/内存/磁盘/Ping 阈值、
  `alert_offline_seconds`、磁盘专属 `alert_disk_duration_seconds`、共用的 `alert_metric_duration_seconds`），
  `enabled=true`、滞回用默认值，并立即落库 —— **用户已配好的阈值不会被改版抹掉**。
- **引擎 `engine.go`**：`checkAlerts` 每轮开头读一次规则；每项按 `Enabled` 决定是否检查；
  `checkMetric` 签名改为接收 `Rule`（阈值/滞回/持续/抑制期都来自该项自己），
  `handleOffline(agent, rule)`、`checkPingMetrics(agent, rules)` 同步改造；
  两项 Ping 都关掉时直接 return，连 ISP 列表和聚合查询都不发。
- **关掉某项时静默清理遗留 firing**：`resolveMetricAlerts(metric)` 只在"启用→关闭"这一次执行
  （用 `Engine.ruleEnabled` 记录上一轮状态），把该项活跃告警 resolve 掉并清内存抑制标记。
  **不发恢复通知**：一次开关可能涉及十几个节点，全推会瞬间打爆微信配额，而且用户自己关的规则不需要通知。
- **API**：新增 `GET/PUT /api/alert-rules`（GET 同时下发 specs+rules；PUT 先全量校验再落库，避免存一半）。
  旧 `GET/PUT /api/alert-settings` **保留兼容**：GET 从新模型派生，PUT 只改对应规则的 warning/duration
  并保留该项开关/滞回/抑制期，同时回写旧扁平 key，外部脚本与手工 SQL 查询都不会拿到陈值。
- **前端 `Settings.vue`**：告警阈值节改为 6 张独立卡片（开关在标题行，关掉整卡置灰而不是隐藏，
  参数仍可读）；离线卡按后端 `has_duration/has_recovery` 自动不显示这两个字段；
  保存后用后端规范化返回值回填。侧栏标签、告警中心提示、设置页副标题统一改为"告警规则"。

### 验证
- `go vet ./...` 通过、`gofmt` 干净（docker golang:1.25 本地把关）；`vue-tsc` 零错误、`vite build` 成功。
- 待部署后核对：`GET /api/alert-rules` 返回 6 条且值等于迁移前的生产配置；关掉某项后该项不再产生新告警且
  日志出现"已关闭 X 告警，静默清理 N 条活跃记录"。

### 遗留
- 节点级/分组级覆盖未做（本次确认只做全局）；卡片副标题原先承诺的"三级回退"仍未实现，已把描述改为实际行为。
- `critical` 二级阈值与"按项恢复通知开关"未做（用户选的是 5 参数方案）。


## [2026-10-06 07:43] - 仓库卫生：停止跟踪前端构建产物，dist 只留 .gitkeep 占位

### 改动前总结
`internal/webapi/dist/` 本应由构建生成，但仓库里残留了早期误提交的文件，而且已经**自相矛盾**：
上一次提交删掉了唯一被跟踪的 `assets/index-73vta-8R.js`，`index.html` 却引用未入库的
`index-B5UCCBZT.js` / `index-BWd7ThXf.css`。后果是干净检出后 `go build` 出的二进制必然白屏，
而 CI 一直正常（Dockerfile 用 `COPY --from=frontend-builder` 取现场构建的 dist），所以没人察觉。

### 改动后总结
- `git rm -r --cached internal/webapi/dist`：解除跟踪，**本地文件不动**，当前开发不受影响。
- `.gitignore` 把 `internal/webapi/dist/` 改成 `internal/webapi/dist/*` + `!internal/webapi/dist/.gitkeep`。
  **必须排除内容而不是目录**：Git 不会进入被忽略的目录，目录级排除会让子文件例外完全失效。
- 新增 `dist/.gitkeep`：`//go:embed all:dist` 要求目录至少有一个文件。实测删掉整个 dist 后
  `go build` 直接失败（`pattern all:dist: no matching files found`）；有占位文件则编译通过，
  且 `embed.go` 的 `init()` 检测不到 `index.html` 时会走 `PlaceholderHandler` 引导页
  （提示运行 `make build-frontend`），**不再是白屏**。

### 验证
- 真实干净检出验证：`git clone file://` 到临时目录 → `dist/` 内只有 `.gitkeep` →
  `go build ./cmd/server` 与 `go vet ./...` 均 exit 0。
- `git check-ignore` 确认 `.gitkeep` 命中取反规则可入库，其余产物（index.html / assets/* / favicon.svg）仍被忽略。
- CI 构建结果见下一次检查（Dockerfile 不依赖仓库里的 dist，预期不受影响）。


## [2026-10-06 07:21] - 修复趋势图 X 轴 undefined 与公开详情页板块无间距

### 改动前总结
用户补报两个 bug：① 后台节点详情页「资源趋势」整条 X 轴画成 `undefined`（ff1 截图）；
② 公开详情页各板块之间没有间隔、卡片边框紧贴叠在一起（tk3 截图）。
另外 390px 视口实测还发现：我上一轮写在 `components.scss` 的手机端表格规则**完全没生效**
（`.el-table .cell` 的 padding 仍是 12px）。

### 改动后总结
- **X 轴 undefined 根因是字段名不一致**：后端 `store.RawSystemMetric` 的序列化 tag 是
  `json:"ts"`，而管理端前端读的是 `item.timestamp` → 永远 undefined；公开接口
  （`public.go`）用的又是 `json:"timestamp"`，所以公开详情页正常、只有后台错。
  修复：`NodeDetail.vue` 改为 `item.ts ?? item.timestamp`（两套字段都兼容，注释写明来源）。
- **兜底防复发**：`formatClock` / `formatHourMinute` 旧代码在非法时间时 `return String(value)`，
  正是它把字面量 `"undefined"` 画上轴的；现改为 null/undefined/空串/Invalid Date 一律返回 `-`。
  以后任何字段名错都会显示成一片 `-`（一眼可见），不会再伪装成"数据内容"。
- **板块间距**：公开详情页 `main` 是普通 block，各 section/WkCard 之间没有任何 gap。
  新增 `.wk-public-sections { display:flex; column; gap: var(--wk-gap-section) }` 挂到该页 main，
  与后台 `.wk-container-inner` 用同一令牌，两个入口分区节奏一致；同时把 `.wk-detail-hero` 原有的
  上下 padding 收掉，避免 gap + padding 叠加。
- **表格规则不生效的根因是 @use 顺序**：`index.scss` 里 element.scss 排在 components.scss 之后，
  element 的 `.el-table .cell{padding:0 12px}` 与媒体查询同特异度且靠后 → 覆盖了我的 640 规则。
  已把手机端表格规则移到 `element.scss` 末尾，并在两处都留下说明注释。

### 验证
- `vue-tsc` 零错误、`vite build` 成功；产物 CSS 实测确认：640 覆盖规则位置(375359) 在基础
  12px 规则(370577) **之后** → 生效；`.wk-public-sections` 已进产物。
- 本轮又踩一次 `<style scoped>` 里写 `//` 注释导致 build 失败，已改 `/* */`（该教训上一轮刚记进 DEVTIPS）。
- 待部署后核对：后台 ff1 节点详情 X 轴显示真实时间、公开详情页板块之间有明显间隔。


## [2026-10-06 06:53] - 公开页顶栏对齐正文 + 手机端（≤640）布局体系

### 改动前总结
用户两项反馈：① 首页“刘刻舟的服务器 / 公开服务器状态 / 管理登录”太靠边、布局不合理；
② 手机屏上节点信息需要适配，要求整体排查。
顶栏根因很具体：`<header class="wk-public-nav">` **没套 `.wk-public-inner`**，
而下方 `<main>` 套了（`width: min(1240px, 100%-32px); margin: 0 auto`），
所以顶栏 `space-between` 直接贴视口左右边缘，与正文左对不齐；两个公开页同样问题。

### 改动后总结
- **顶栏结构**（`PublicHome.vue` / `PublicServerDetail.vue`）：header 内部新增
  `.wk-public-inner .wk-public-nav-row`，背景通铺、内容对齐正文；
  `layout.scss` 里顶栏改为 `position: sticky` + hairline 下边 + 半透模糊底。
- **新增 ≤640 手机断点（`components.scss` 统一一段）**：
  - 节点卡底行（↑速率 ↓速率 uptime 时间）之前是 `nowrap + overflow:hidden`，
    手机上直接**裁掉信息**；放开折行又会每秒 1↔2 行跳。改为**固定两行 + 锁 `min-height`**，
    高度恒定后网格行高一致，既不裁信息也不跳。
  - 节点卡单列铺满、卡片与 KPI 卡留白收紧、数值字号降一档、hint 省略号；
    丢包色条线路名 108px→72px；`el-table` 单元格左右留白 12px→8px。
- **公开首页 Hero**：手机上不再上下堆叠（堆叠后首屏只剩标题），改为标题 + 64px 在线率环并排；
  工具栏右侧“点击卡片查看详情”提示手机上隐藏（同时把 inline `margin-left:auto` 收进 CSS）。
- **公开详情页**：≤640 收紧 hero 留白与节点名字号，让实时指标卡能进入首屏。

### 验证
- `vue-tsc` 零错误；`vite build` 首次失败——**`//` 注释写进了 `<style scoped>` 纯 CSS 块**
  （只有 `lang="scss"` 的块才能用 `//`），已改 `/* */` 并全库扫描确认无同类问题，重构建通过。
- 待部署后以 390px 宽实测顶栏对齐、卡片不裁信、无 1↔2 行跳动。

## [2026-10-05 08:55] - 重跑 CI + 测试接口状态码改用 424（Cloudflare 会吞掉源站 5xx）

### 改动前总结
部署后做端到端验证：路由、鉴权、参数校验、令牌不回显全部通过，但两个“发送测试通知”接口
返回的是 `{"error": ...}` 而不是预期的业务码，实际拿到的是 `error code: 502` 纯文本。
排查：容器无 panic、重启次数 0；绕开 CF 直接请求路由返回正常 401 JSON；
在生产主机与容器内分别 POST `https://www.pushplus.plus/send/BOGUS` 均 1 秒返回
`{"code":903,"msg":"用户令牌不正确"}`，证明出站链路和 CA 证书都没问题。

**真正原因**：源站返回 502/503/504 时，**Cloudflare 会用它自己的错误页替换响应体**，
我们写的“pushplus 返回 903：用户令牌不正确”根本到不了浏览器，用户只会看到一个没有原因的 502。
这是本次新代码里的设计错误，而且 **Telegram 测试接口早就有同一个 bug**（同样用 502）。

### 改动后总结
`handleTestPushplus` 与 `handleTestTelegram` 的发送失败状态码由 `502 Bad Gateway` 改为
**`424 Failed Dependency`**：4xx 不会被 CF 拦截，语义上就是“上游依赖调用失败”，
且 axios 仍归入错误分支，前端一行代码都不用改就能展示真实原因。

### 验证
- `go vet ./...` 通过；待部署后复测：假令牌请求测试接口应返回 424 + 含“903”的 JSON。

## [2026-10-05 08:20] - 新增微信推送渠道（pushplus 中转，支持微信 ClawBot）

### 改动前总结
告警只能推 Telegram（`Engine.sendTelegramNotification` 是唯一硬出口），国内环境访问 Telegram 不可靠。
用户要求对接微信 ClawBot。调研结论：微信官方 ClawBot 直连需自己维护扫码凭证与 `context_token`，
并且“每下发 10 条 / 每 24 小时”必须用户在微信里主动发一条消息才能继续下发；改走 pushplus 中转后
主控侧只是一次 HTTP POST，风险面小得多（已经用户确认选择 pushplus + 只推关键告警合并 + 纯文本带链接 + 设置页入口）。

### 改动后总结
- **新增 `internal/notify/pushplus.go`**：`PushplusNotifier`，`POST {base}/send/{token}`，
  body `{token,title,content,channel,template:txt,topic?}`，默认 `https://www.pushplus.plus`（不用文档里的 http，
  避免令牌明文过公网）。content 为纯文本 + 节点详情链接（依赖 `site_domain`，未配置则不输出链接）。
  **关键：接口是异步的，`code=200` 只代表已受理**，所以成功日志里带上返回的消息流水号（shortCode）供事后查投递结果。
- **新增 `internal/notify/aggregator.go`（AlertAggregator）**：节流+合并，语义对齐 Alertmanager：
  空闲时第一条立即发（保证 critical 即时），随后进入合并窗口（默认 5 分钟，后台可改 1~60），
  窗口内新告警只缓存，窗口结束时合成一条汇总（最多列 10 条，多余折叠为“另有 N 条”，
  超出 200 条缓存上限的部分也如实告知）；`Run(ctx)` 退出时 `Close()` 同步 flush，不丢告警。
- **`internal/notify/notify.go`**：`Message` 新增 `Kind`（firing/resolved/summary），因为恢复通知是 info 级
  但必须送达，不能只按 Level 过滤；新增 `retryableError` 接口与 `SendWithRetry` 导出，
  并把 4xx/429 判定改成基于 `HTTPStatus()` 接口，不再硬写 `*telegramAPIError` 类型断言。
  **为什么需要 `Retryable()`**：pushplus 的 900（账号受限）/903（无效令牌）/905（未实名）重试无用，
  而且官方明确说继续请求会加重限制（单日超 1000 次封 7 天），必须不重试。
- **`internal/alert/engine.go`**：`sendTelegramNotification` 调用点收敛为 `notifyChannels`（Telegram 逐条即时 +
  pushplus 走合并器）；新增 `sendPushplus`（每次发送时读最新设置，改配置热生效）、`pushplusWindow`、
  `pushplusAccept`（只推 warning/critical 触发 + 全部恢复）、`settingString`/`settingBool`。
- **`internal/webapi`**：新增 `GET/PUT /api/pushplus` 与 `POST /api/pushplus/test`（JWT 鉴权）；
  设置项 `pushplus_enabled`/`pushplus_token`/`pushplus_channel`/`pushplus_merge_minutes` 写入 SQLite 固化；
  **`pushplus_token` 故意不加入 `allowedSettingKeys` 白名单**，只能走专用接口且永不回显；
  渠道白名单 `clawbot/wechat/cp/qq/cmcc/mail`（排除需额外 option 的 webhook 与付费的 sms/voice）；
  测试接口不走合并器，失败时把业务码原样回传（用户对照文档就能分清令牌错还是账号受限）。
- **`web/src/views/Settings.vue`**：新增“微信推送”节（启用开关 + 令牌不回显 + 渠道下拉 + 合并窗口 + 测试），
  并把 ClawBot 的 10 条激活限制、使用步骤、以及“改成微信公众号可避开该限制”写在页面上。

### 验证
- `go vet ./...` 本地通过（无 Go 工具链，用 `docker run golang:1.25` + 共享 module 缓存跑）；`vue-tsc` 零错误、`vite build` 成功。
- **已真实探测 pushplus 接口**（假令牌）：返回 `{"code":903,"data":"无效的用户token","msg":"用户令牌不正确"}` 与 `HTTP=200`，
  证明 https 可用、路径正确，并证实“只看 HTTP 状态码会误判失败”这个设计判断。
- 待用户提供 pushplus 令牌后在后台填写并点“发送测试通知”做端到端确认。

### 遗留
- 未做自动“查投递结果”（需 pushplus 开放接口 AccessKey，额外凭证），目前靠日志流水号 + 微信侧人工确认。
- 告警引擎当前只有 `alertLevel()` 产出的 warning（资源类）/critical（离线），所以“只推关键”过滤实际上
  主要靠合并窗口节流；若以后引入 info 级事件，过滤器会直接生效。

## [2026-10-05 08:02] - 去掉 Ping 图纵轴单位名，修复与图例文字重叠

### 改动前总结
用户反馈细节：图例“— 上海电信”左侧被纵轴单位“ms”遮挡。原因：ECharts 把 `yAxis.name` 画在轴顶端
（左上角），而 Ping 图把图例也放在左上（`legend: { top: 0, left: 0 }`），两者同位置重叠；
且 `grid.containLabel` 不会为轴名预留空间。

### 改动后总结
- `WkPingChart.vue`：去掉 `yAxis.name: 'ms'`，单位改由卡片副标题与上方统计摘要行（均 6.1ms）承载。
- `NodeDetail.vue` / `PublicServerDetail.vue`：网络质量卡片副标题补上“（ms）”，信息不丢失。
- 其他图表不受影响：资源趋势图的图例默认隐藏，纵轴 `%` 不会与任何东西重叠。

### 验证
- `vue-tsc` 零错误、`vite build` 成功；待部署后核对图例与轴区域不再互挡。

## [2026-10-05 07:32] - 取消分线路视图、修复白底白字的空白悬窗、节点详情按作用域取 Ping

### 改动前总结
用户三项反馈：① 分线路视图不必保留，叠加对比已足够；② 鼠标悬停侧栏图标时弹出一个**空白悬窗**（附图：
白框 + 黑箭头）；③ 后台节点详情仍会向被作用域排除的线路（上海移动IPV6）白跑一次 ping-agg 请求。

空白悬窗根因（不是没内容，是内容看不见）：`element.scss` 把 `.el-popper` 背景强制成
`--wk-elevated`（浅色主题下是白色），但 EP 默认 `effect="dark"` 的**文字色**仍是
`--el-bg-color-overlay`（也是白色）→ **白底白字**；箭头没被覆盖，仍是深色 → 白框黑箭头。

### 改动后总结
- `WkPingChart.vue`：删除分线路模式与模式切换按钮，只保留叠加对比；把每条线路的
  均/最低/最高/丢包 做成图表上方的**摘要行**（颜色与图例一致），补回节点详情页被删掉的数值可见性。
- `MainLayout.vue`：侧栏导航项不再包 `el-tooltip`，改为条件原生 `title`（仅折叠态给提示，
  展开态传空串不显示），彻底消除重复弹层。
- `element.scss`：`.el-popper` / `.is-dark` / `.is-light` 统一接管背景、**文字色**、**箭头色**，
  修复全站 tooltip/弹层在浅色主题下的白底白字与黑箭头。
- `NodeDetail.vue`：`loadPingAgg` 先按 `scope`/`agent_ids` 过滤当前节点适用的运营商线路再发请求，
  不再给被排除线路白跑查询。

### 验证
- `vue-tsc` 零错误、`vite build` 成功；待部署后核对：无分线路开关、悬停侧栏图标能看到菜单名、
  节点详情只请求适用线路。

## [2026-10-05 06:48] - Ping 线路图改为分线路/叠加双模式（修复线路重叠看不见）

### 改动前总结
用户反馈公开详情页“上海电信和上海联通重叠后看不到”。实测上海电信 5.40ms、上海联通 5.57ms、
上海移动 51.96ms 共轴：两条线只差 0.17ms，在 0~60ms 轴上占 0.3% 高度，必然完全重叠；
**对数刻度也救不了**（log10 下差距仍不足 1% 高度），因为问题不是量级而是绝对差值太小。

### 改动后总结
新增 `web/src/components/WkPingChart.vue`（两个页面共用，避免再出现两套 Ping 图）：
- **分线路（默认）**：每条线路一个小图，各自独立 Y 轴且 `min` 按本线路数据下界自适应（不从 0 开始），
  几毫秒的差别也能看清波动；标题行直接给出 均/最低/最高/丢包，不依赖 tooltip。
- **叠加对比**：保留原来的共轴多线 + dataZoom，用于横向比较尖峰与量级；选择结果存 localStorage。
- 兼容两种后端时间字段（后台 ping-agg 返 `bucket_min`，公开接口返 `timestamp`）。
- `NodeDetail.vue`：改用 `WkPingChart`，删除与之重复的 ISP 摘要行与本地 `buildPingOption/ispSummary/lossTone`
  （净减 130 多行）；`PublicServerDetail.vue`：改用 `WkPingChart`，色条行只保留丢包百分比，不再重复写延时统计。

### 验证
- `vue-tsc` 零错误、`vite build` 成功；待部署后核对上海电信与上海联通各自成图、线条清晰可辨。

## [2026-10-05 06:30] - 节点新鲜度改用主控时钟（修正 ff1 误判根因）

### 改动前总结
上一条只是把两端判定统一到一个函数，但没解决数据从哪来：`metric.UpdatedAt` 来自探针自报的
`sys.Timestamp`。实测 ff1 最新点 22:17:06 而主控当时 22:22:43（hk2 只差 4 秒），
即 **ff1 机器时钟比主控慢约 5.6 分钟**，刚好越过 5 分钟阈值 → 两端统一后反而会一起错。

### 改动后总结
- `internal/webapi/public.go` `publicStatus()`：新鲜度改为优先用 `agents.last_seen_at`
  （主控收到上报时用服务器时间写入），无该字段时才回退到指标时间；探针时钟偏差不再影响在线判定。
- `web/src/utils/format.ts` `nodeState()`：同样改为 `last_seen_at || updated_at`，与后端保持一致。

### 验证
- 部署后 ff1 在公开页与后台应均为“在线”（它的 `last_seen_at` 一直是秒级新鲜的）。
- 待用户在 ff1 上处理：`timedatectl` 看 NTP 是否同步（大概率 System clock not synchronized），
  校时后 `systemctl restart wukong-agent`；否则它的小时表分桶、启动时间、趋势图 X 轴仍会偏 5~6 分钟。

## [2026-10-05 06:22] - 列表默认按名称排序 + 统一节点状态口径

### 改动前总结
1. 公开首页与后台节点列表默认按 CPU 降序，每秒刷新时卡片位置会随负载重排，找机器靠名字反而更难；
2. ff1法兰克福 在公开页显示离线、后台显示在线。实测：`agents.online=true`、`last_seen_at` 每秒在动（gRPC 流未断），
   但内存/小时表里它的最新指标停在 22:06（自升级后系统采集没再产出数据）。
   根因是两端用了两套判定：后台只看 `online`，公开页看指标新鲜度（>5min → stale）。

### 改动后总结
- `utils/format.ts` 新增 `nodeState()`（四态 online/stale/offline/unknown，阈值 `STALE_SECONDS=300` 与后端
  `publicStatus` 对齐）、`nodeStateText()`、`nodeStateTone()`；公开接口已返的 `status` 直接沿用，不再前端重算。
- 后台三处改用统一判定：Dashboard 卡片/列表、Nodes 状态列与“在线/离线”筛选与计数、
  `useOverview.onlineCount`（顶栏与 KPI 共用），数据延迟的节点不再被当作在线。
- 默认按名称排序：Nodes 页、公开首页、Dashboard（新增 `sortedNodes`，卡片与列表同序），
  并用 `localeCompare(..., 'zh-Hans-CN')` 保证中文城市名按本地顺序而非码位。
- 公开首页：`is-offline` 置灰样式只给 offline/unknown，stale 保留亮度 + 琥珀色灯（之前被归成离线观感）。

### 验证
- `vue-tsc` 零错误、`vite build` 成功；待部署后核对 ff1 在两端都显示“数据延迟”。
- ff1 本身需要处理：它自升级后只发空帧（`System=nil`），需在 ff1 上 `systemctl restart wukong-agent` 并看日志（本机无该机器 SSH 权限）。

## [2026-10-05 06:05] - 修复：改完运营商目标后告警不恢复、新配置不生效

### 改动前总结
用户反馈“修改上海移动目标 IP 后，ping 丢包告警还在继续”。实测两个叠加缺陷：
1. `alert.checkMetric` 在 30 分钟抑制期内**直接 return**，连恢复判断一起跳过 →
   hk2香港 三条线路 21:42–21:47 全部 0% 丢包，告警 id=15 仍挂 firing；
2. 主控仅在**建连时**下发一次 `COMMAND_UPDATE_CONFIG` → 改完目标 IP 后已连接的探针
   仍按本地旧配置探测（sh1上海 21:47 仍为 100% 丢包），告警数据源本身就是错的。

### 改动后总结
- `internal/alert/engine.go`：抑制期语义收敛为“只压制重复触发”——超阈值且处于抑制期时不告警也不累计持续时间；
  低于恢复阈值的分支照常执行，`resolveAlert` 会顺便清除抑制标记（抑制期与恢复通知不再冲突）。
- `internal/grpcapi/agent_server.go`：新增 `configDirty` 标记表与 `InvalidateAgentConfigs()`；
  stream 心跳循环（≤15s）用 `takeConfigDirty` 取出标记后重发一次配置。
  **发送仍在 stream goroutine 内完成**，避免 gRPC 并发 SendMsg；探针断线时清理标记防 map 增长。
- `internal/webapi/handler.go`：新增 `ConfigInvalidator` 接口 + `SetConfigInvalidator`（不直接依赖 grpcapi），
  `handlers.go` 在 ISP 目标新增/修改/删除与节点采集配置更新四处调用；`cmd/server/main.go` 把
  `RegisterService` 移出 goroutine 并注入实例。
- 顺带修：`PUT /api/agents/{id}` 的 `ping_intv` 下限 5 秒与默认 1 秒的既定决策矛盾，
  节点详情页填 1 会被 400 拒掉，改为 1-3600。

### 验证（生产 13 节点）
- 部署后 70 秒：`firing=0`，之前卡住的 ping_loss id=13/15 均变 resolved。
- 启用“上海移动IPV6 = 2409:8088::a”（`scope=include`，7 个有 v6 出口的节点）后：
  主控日志 `已标记 13 个在线探针待重新下发配置`，同一轮心跳内 v6 节点 `targets=4`、
  无 v6 节点 `targets=3`；本机探针无需重连就在 22:02:49 收到 `Ping目标数: 4`。
- 数据侧：sel2/sel3/sel4 新线路 35 个点、真实延时 201~331ms、0% 丢包；
  hk2香港/server1上海 的脏数据停在 22:00（作用域生效前），之后不再产生 100% 丢包点。

## [2026-10-05 05:52] - 运营商 Ping 目标支持按节点作用域（include/exclude）

### 改动前总结
`isp_targets` 只有 `name/ip/port/mode/enabled`，启用目标无条件下发给全部探针。用户要加
上海移动 IPv6 目标 `2409:8088::a`，但现网 8 台里 **hk2香港、sh1上海 没有公网 IPv6 出口**，
这类节点探测 IPv6 会全部失败，在图表上表现为该线路长期 100% 丢包并可能误触告警。

### 改动后总结（三态作用域，仅主控侧过滤，探针不改不升级）
- `internal/store/store.go`：`ISPTarget` 新增 `Scope`（`all`/`include`/`exclude`）与 `AgentIDs`；
  新增 `ScopeText()`（非法值回退 all，旧调用方行为不变）与 `AppliesTo(agentID)`。
- `internal/store/sqlite.go`：`isp_targets` 建表新增 `scope`/`agent_ids` 两列，并加入 ALTER TABLE 迁移列表
  （旧库自动升级，存量数据默认 `all`）；读写均进过 `splitIDs/joinIDs` 序列化。
- `internal/grpcapi/agent_server.go`：`enabledPingTargets()` → `enabledPingTargetsFor(agentID)`，
  注册响应与配置热更新两处下发都按节点过滤。
- `internal/webapi/handlers.go`：`validateISPTarget` 校验 scope，**include/exclude 时必须至少选一个节点**
  （否则“仅选中”会退化成“全部”，难发现）；创建接口接受 `scope`/`agent_ids`。
- `internal/webapi/public.go`：`publicPingISPs(agentID)` 只列出该节点实际会测的线路，
  避免公开详情页出现“有线路名但永远没数据”的空行。
- `web/src/views/Settings.vue`：Ping 运营商区新增“作用域”下拉 + 节点多选（带 IPv6/IPv4 标记）；
  目标地址含冒号时出现“选中有 IPv6 的 / 选中无 IPv6 的”一键按钮；列表新增“作用域”列。

### 验证
- `npx vue-tsc --noEmit` 零错误、`vite build` 成功；Go 编译由 GHCR 镜像构建验证（本机无 Go 工具链）。
- 线上：新增“上海移动IPv6 = 2409:8088::a”并排除无 IPv6 的节点后，核对各探针日志的
  `Ping目标数`：有 IPv6 出口的节点应为 4，hk2/sh1 仍为 3。

## [2026-10-05 05:30] - 修复节点卡每秒 1 行↔2 行跳动

### 改动前总结
用户在线上发现：公开首页与总览的节点卡每秒在 1 行与 2 行之间反复跳动。根因是页脚一行内要装
上行/下行/运行时长/最近上报 四段每秒变化的文本，而 `formatRate` 输出 `745.0 B/s`、
`relativeTime` 输出 `0 秒前`/`刚刚`、`formatDuration` 输出 `22d 18h 32m` 全带空格，
浏览器会在空格处折行；叠加“刚刚”(2字) 与 “0 秒前”(4字) 宽度不同，高度就会反复变。

### 改动后总结
- `utils/format.ts`：新增 `formatRateShort`（无空格 `745B/s`）；`relativeTime` 去掉全部空格并将
  “刚刚”归一为“0秒前”；`formatDuration` 改为 `22d18h32m`。
- `styles/components.scss`：`.wk-node-card-foot` 强制 `flex-wrap/white-space: nowrap` + `overflow: hidden`，
  时间段 `min-width: 0` + 省略号（空间不足只裁时间，不撑高卡片）；`.wk-metric-hint` 与
  `.wk-metric-value` 同步 nowrap + 省略号，防止 KPI 卡随数值位数抖动。
- `Dashboard.vue` / `PublicHome.vue`：页脚与表格速率列改用 `formatRateShort`。

### 验证（生产 8 节点真实数据）
- 公开首页与后台总览各做 8 轮（间隔 1.2s）DOM 高度采样：8 张卡均 **恒定 198px**、页脚 **31px**、
  `flexWrap=nowrap`，无任何卡出现两个高度交替；`scrollWidth > clientWidth` 截断元素 **0**。
- KPI 卡 hint 实测高度 18px = 行高 → 均单行未换行；两页 console error/warn 均为 0。

## [2026-10-05 05:10] - 生产部署到 arm64 机 + CI 多架构提速 + 安装脚本 ETXTBSY 修复

### 改动前总结
- 要求把新 UI 部署到生产 `146.56.173.198`（Cloudflare CDN → 回源 64443，域名 `https://server.lkz.pub`）；
- 但该机器是 **arm64**，而 GHCR 只有 amd64 镜像（`no matching manifest for linux/arm64/v8`）：
  `docker.yml` 未声明 `platforms`，且 `Dockerfile` 把主控二进制写死 `GOARCH=amd64`；
- 改成单 job + QEMU 多平台后构建超过 10 分钟未完（arm64 层在 x86 上模拟编译 Go+CGO）；
- 安装脚本在已装过探针的机器上必然失败：`curl -o` 直接覆盖运行中的二进制 → ETXTBSY → `curl: (23)`。

### 改动后总结
1. **CI 多架构提速**（`.github/workflows/docker.yml`）：改为 amd64/arm64 各跑一个**原生 runner**
   （`ubuntu-24.04` / `ubuntu-24.04-arm`，公开仓库免费），按 digest 推送后由 merge job 用
   `docker buildx imagetools create` 合并 manifest 并打 tag；`gha` 缓存按架构分 scope。
   实测整体 **~2.5 分钟**（对比 QEMU 方案 10 分钟未完）。中途踩到：`download-artifact` 不带 pattern
   会把 buildx 产出的 `*.dockerbuild` 元数据 artifact 一起下载并重试失败，已限定 `pattern: digests-*`。
2. **Dockerfile**：主控二进制改用 buildx 注入的 `TARGETARCH`（本地单平台构建为空时回退 amd64，行为不变）。
3. **安装脚本**（`internal/webapi/handlers.go` 的 `handleInstallAgentScript`）：先 `systemctl stop wukong-agent`，
   再下载到 `wukong-agent.new` 后 `mv -f` 原子替换（rename 不受 ETXTBSY 影响，下载中断也不会写坏原二进制），
   失败提示补充排查方向。探针自升级路径本来就是临时文件+rename，无需改。
4. **鉴权日志**（`internal/grpcapi/agent_server.go`）：`ValidateAgent` 在“节点不存在”时也返回 false，
   旧日志一律打“secret 不匹配”会误导排查；现在先查 `GetAgent` 区分“节点未注册（已在后台删除）”与“密钥不匹配”。

### 部署结果（生产已验证）
- 容器：`wukong` on `146.56.173.198`，`-p 64443:64443`（对公网，CF 要外部回源）+ `-v /opt/wukong/data:/opt/wukong/data`
  + `--env-file /opt/wukong/wukong.env`（JWT 密钥与管理员密码已固化，重启不失效）。
- 链路：`https://server.lkz.pub/api/health` 经 Cloudflare 200；`index.html` 引用的资源哈希与本地构建一致，确认新 UI 已上线；
  静态资源与 favicon 均 200；浏览器核验 6 个页面 console error 0、非 200 请求 0。
- 节点：`ubuntu`(arm64, 主控本机) / `sel4`(arm64) / `tk3`(amd64) 共 3 台在线每秒上报；
  Ping 运营商目标已补回 3 条，主控侧已收到真实聚合（上海电信 29.0ms / 0 丢包）。
- 重要结论：**Cloudflare 橙云不能把 gRPC 代理到明文源站**（实测 403 + text/html），
  因此 `agent_server_addr` 改为直连源站 `146.56.173.198:64443`，网页继续走 CDN 443。

### 遗留风险（已记入 DEPLOY_CREDENTIALS.md）
- `AGENTS.md` 把管理员密码明文写进了公开仓库，建议立即改密并清理文档。
- 探针上报为公网明文 gRPC，每帧携带 `agent_secret`；要加密需在源站给 64443 前置 TLS 或走灰云。
- 旧主控上的其他节点仍连 `server.lkz.pub:443`，会被 CF 403 拒绝，需在每台重新执行安装命令。

## [2026-10-05 04:13] - Web UI 重构为现代 SaaS 控制台（设计令牌 + 组件层 + 页面信息架构）

### 改动前总结
上一轮（2026-08-06）只做了换色层面的现代化，仍存在结构性问题：
1. 设置页保存的 `theme.primary`（自定义主色）从未写进 CSS 变量，架构决策 #11 的自定义主题实际不生效；
2. ECharts 配置里直接写 `axisLabel:{color:'var(--wk-text-muted)'}`，canvas 不解析 CSS 变量，轴/图例颜色失效，且 `echarts.init(el,'dark')` 写死深色，切浅色主题图表仍是深色底；
3. 后台节点详情页只用了后端已有的 20 个 `LatestMetric` 字段中的 3 个（名称/采集频率/Ping 频率），信息量反而少于公开详情页；
4. `formatBytes/relativeTime/loadLevel` 在 5 个文件各写一份，单位口径不一（KiB 与 KB 混用）、负载阈值不一（60/85 与 70/90）；
5. 顶栏每 5s 拉 3 接口 + 总览每 1s 拉 2 接口 + 节点页每 1s 拉 2 接口，同一份数据重复请求，后台标签页也不停；
6. `index.html` 引用 `/favicon.svg` 但仓库无此文件（404）；`App.vue` 与 `index.scss` 重复一份全局 reset 并引用未加载的 Inter 字体。

### 改动后总结
视觉方向改为现代 SaaS 控制台（参考 Linear 近黑画布 + hairline 边框 + 单一克制强调色、Stripe 的 table-first 与“颜色只表达状态”、Vercel 的中性灰阶与排版层次）。**纯前端改动，未修改 `internal/` 任何 Go 代码，未新增 npm 依赖。**

1. **设计令牌重写**（`web/src/styles/variables.scss`）：中性灰阶画布（暗 #0a0a0b / 浅 #f7f7f8）+ 新主色 indigo（暗 #5e6ad2 / 浅 #4f5bd5）；新增圆角/间距/字号/字重/控件高度/动效尺度令牌；派生色（hover/soft/glow）全部改用 `color-mix()` 从 `--wk-primary` 推导，因此改一个变量即可全站联动；`--el-*` 全部指向 `--wk-*`，Element Plus 自动跟随主题与自定义主色；保留 `--wk-shadow-sm/md`、`--wk-border-light` 等旧名作别名，避免中途回归。
2. **样式分层**：`index.scss` 拆为 `variables → base → layout → components → element` 五个文件，入口路径与 `main.ts` 导入不变；卡片 hover 不再 translateY 上浮，改为边框 + 阴影反馈。
3. **新增组件层**（`web/src/components/`，9 个）：`WkCard`、`WkMetric`、`WkProgressBar`、`WkStatusDot`、`WkBadge`、`WkSparkline`（纯 SVG 迷你曲线）、`WkChart`（ECharts 唯一出口，自动 init/dispose/resize + 主题重建）、`WkEmptyState`、`WkSkeleton`。
4. **新增工具与组合层**：`utils/format.ts`（统一格式化与负载阈值 70/85）、`utils/charts.ts`（`getComputedStyle` 解析 CSS 变量为真实色值 + 注册 wukong 主题 + 统一 axis/tooltip/dataZoom 工厂）、`composables/useTheme.ts`（主题单例，**真正注入 `--wk-primary`**）、`composables/usePolling.ts`（页面不可见自动暂停）、`composables/useOverview.ts`（多页共享一个 1s 轮询 + 内存滚动采样）。
5. **逐页重构**：顶栏改为页面标题 + 副标题 + 实时药丸 + 告警铃带角标 + 头像菜单；总览新增 KPI sparkline、卡片/列表双视图、集群负载趋势、最近告警；节点列表改为 table-first + 搜索/状态 chips/排序；节点详情页补齐后端已有字段（CPU 型号/核数/负载/总内存/总磁盘/累计流量/启动时间/出口 IP）+ 资源趋势 1h/6h/24h + ISP 摘要 + 系统信息定义列表；告警中心新增 4 计数 + 状态/指标/节点筛选 + severity 色条 + 超阈程度条；设置页改为左侧竖排分区导航，主题页改预设卡 + 主色色板 + 即时预览；登录页改为左品牌右表单分栏；公开首页新增可用性圆环 + 直接复用后端 `summary`；公开详情页新增 Statuspage 风格 24h 丢包色条。
6. **顺带修复**：删除 `App.vue` 重复 reset；新增 `web/public/favicon.svg` 与 `<meta name="theme-color">`；`prefers-reduced-motion` 下关闭呼吸动画与骨架流光；icon-only 按钮补 `aria-label`。

### 验证
- `npx vue-tsc --noEmit` 零错误；`npx vite build` 产物输出 `internal/webapi/dist/`（含 favicon.svg）成功。
- 本机未安装 Go 工具链（`go: command not found`），因此未跑真实主控；改用与 `vite.config.ts` proxy 完全一致的只读 Mock API（`build/mock-api.mjs`，8 节点/4 运营商/12 条告警）+ Vite dev server，用无头浏览器三轮逐页核验：所有页面 console error 0、4xx/5xx 0、无白屏；双主题与 6 种主色切换、1440/820 两档宽度均正常；修复了核验中发现的 `wk-grid-*` 缺 `display:grid`、表格列宽截断、dataZoom 压住时间轴、设置页强制回滚主题、丢包误用资源阈值、内存线与主色同色相等问题。

### 已知限制
- `/api/public/theme` 不返回 `primary`，未登录访客看到的仍是默认主色（登录态走 `/api/theme` 可完整生效）；要覆盖匿名场景需后端补 1 个字段，本次按“不改后端”约束未做。
- KPI sparkline 与集群趋势基于前端内存滚动采样，刷新页面后历史清空。

## [2026-08-06 05:12] - Web UI 全面重构：参考 QuantKing 设计系统现代化改造

### 改动前总结
wukong 前端 UI 使用 neon cyan (#38bdf8) 暗黑科技风，顶部导航栏布局，卡片简单无侧栏，视觉风格偏旧。

### 改动后总结
参考 QuantKing 项目设计系统，全面重构 wukong Web UI，使其美观现代化：
- **设计令牌系统**：蓝色主色调 #3b82f6，charcoal 深色 + 明亮浅色双主题，CSS 变量统一管理
- **布局重构**：顶栏 + 可折叠侧栏 + 主内容区（替代纯顶部导航），参考 QuantKing AppShell 布局
- **顶栏**：品牌 Logo + 实时指标条（在线/CPU/内存/告警）+ 时钟 + 主题切换 + 退出按钮
- **侧栏**：分组导航 + 状态指示灯 + 折叠/展开 + 移动端响应式
- **卡片系统**：圆角卡片 + 微阴影 + hover 浮起效果 + 骨架屏加载
- **登录页**：径向渐变背景 + 品牌标志 + 主题切换
- **仪表盘**：统计条 + 节点卡片网格 + 进度条指标
- **ECharts 配色**：统一使用新主题配色 (#3b82f6/#34d399/#fbbf24/#f87171)
- **公开页面**：简洁渐变背景 + 服务器卡片网格 + Hero 区域

### 修改文件（16 个）
1. `web/src/styles/variables.scss` - 设计令牌系统重写（双主题 CSS 变量 + Element Plus 覆盖）
2. `web/src/styles/index.scss` - 全局样式重写（AppShell + 侧栏 + 卡片 + 骨架屏 + 响应式）
3. `web/src/layouts/MainLayout.vue` - 顶栏+可折叠侧栏布局重写
4. `web/src/views/Login.vue` - 现代登录页重写
5. `web/src/views/Dashboard.vue` - 仪表盘重写（统计条+进度条+骨架屏）
6. `web/src/views/Nodes.vue` - 节点列表页重写
7. `web/src/views/NodeDetail.vue` - 节点详情页重写
8. `web/src/views/Alerts.vue` - 告警中心重写
9. `web/src/views/Settings.vue` - 系统设置页重写
10. `web/src/views/public/PublicHome.vue` - 公开首页重写
11. `web/src/views/public/PublicServerDetail.vue` - 公开详情页重写
12. `internal/webapi/dist/*` - 构建产物更新

### 构建验证
- `npm run build` ✅ 通过（44.98s，0 错误）
- TypeScript 类型检查 ✅ 通过
- 所有功能逻辑保持不变（API 调用、SSE、定时刷新、路由守卫等）

## [2026-07-23 15:55] - 修复 DropOldHourlyTables 死锁致主控卡死

### 改动前总结
24 小时保留版部署后约 1 小时（清理定时器首次触发时），主控再次静默卡死：API 全部超时（000），WAL 飙至 94MB，日志停在清理触发时刻。根因为 `DropOldHourlyTables` 在 `rows.Next()` 循环内直接执行 `s.db.Exec("DROP TABLE")`（schema 修改需独占锁），但 `defer rows.Close()` 延迟关闭使读事务一直持有，schema 修改无法获取独占锁导致死锁，阻塞全部写操作 goroutine。30 天保留版因几乎无过期表未触发；24 小时保留首次清理需 DROP 约 360 张表，密集 schema 修改 + 读事务未关闭 = 死锁。

### 改动后总结
1. **死锁修复**：`DropOldHourlyTables` 改为先收集过期表名到 slice、显式 `rows.Close()` 关闭读事务释放锁、再循环执行 DROP TABLE，彻底消除 schema 修改与读事务的锁冲突。
2. **WAL 膨胀防护**：批量 DROP 完成后执行 `PRAGMA wal_checkpoint(TRUNCATE)`，主动合并 WAL 到主库并截断 WAL 文件，防止 DROP 产生的大量 WAL 堆积。

### 涉及文件
- `internal/store/sqlite.go` - `DropOldHourlyTables` 读事务关闭前置 + WAL checkpoint

## [2026-07-23 14:15] - 主站故障修复：数据库膨胀致 API 卡死 + 清理保留期修复

### 改动前总结
主控服务器（72.249.203.145）生产故障：wukong.db 膨胀至 9.9GB（1455 张按小时分表，含 30 天未清理的系统指标与 Ping 原始数据），SQLite 单写者锁在高频写入（13 节点 × 1s 采集）下产生严重锁竞争，导致全部 HTTP API goroutine 阻塞，主控于 7 月 21 日 09:19 后静默卡死（进程存活但无响应，docker stop 需 SIGKILL）。前端静态页经 openresty 仍可加载，但全部 API 请求超时（HTTP 000），探针 gRPC 通道同时中断。根因为 `runMaintenanceLoop` 中数据保留期硬编码为 30 天（`DropOldHourlyTables(24*30)`、`CleanOldAggData(24*30)`），1 秒级采集 × 13 节点 × 30 天 ≈ 1.3 亿行撑至 10GB。

### 改动后总结
1. **紧急数据清理**：停容器后用 python3 批量 DROP 1086 张 7 月 14 日前的历史小时表（metrics_sys_ 544 张 + metrics_ping_ 542 张），`VACUUM INTO` 将数据库从 9.9GB 压缩至 2.4GB，回收 7.5GB 空间，磁盘占用 67% → 46%。
2. **保留期修复**：`cmd/server/main.go` 历史数据保留期从 30 天（`24*30`）缩短为 24 小时（`24`），系统指标/Ping 原始小时表与 Ping 分钟聚合统一使用 `retentionHours` 常量，控制单库体积在百 MB 量级。
3. **清理频率修复**：`cleanupTicker` 从每 6 小时改为每 1 小时执行一次，更及时回收过期表，降低峰值膨胀风险。

### 涉及文件
- `cmd/server/main.go` - `runMaintenanceLoop` 保留期 30天→7天、清理频率 6h→1h

## [2026-06-30 14:00] - 16 项 bug 与安全修复（P0-P3 全面修复）

### 改动前总结
全面代码审查发现 6 个严重 bug、4 个安全漏洞、2 个并发问题和 10 个功能缺陷，涵盖探针 ticker 泄漏、JWT 类型不区分、TOTP 密钥未持久化、SSE 鉴权不匹配、Logo 上传未写文件、Settings API 无白名单、Webhook 未实现、告警引擎无退出等。

### 改动后总结
**P0 严重修复：**
1. **ticker 泄漏**：`reportLoop` 的 ticker 从循环内移到循环外创建，重连时 `Reset` 而非新建，避免每次重连旧 ticker 泄漏。
2. **refresh token 类型区分**：Claims 新增 `token_type` 字段，新增 `ValidateAccessToken`/`ValidateRefreshToken`，authMiddleware 只接受 access token，refresh 接口只接受 refresh token，防止攻击者混用 token 类型。
3. **TOTP 2FA 持久化**：`handleSetup2FA` 生成密钥后立即写入 SQLite 和内存，新增 `SetTOTPSecret` 方法，用户扫码后 TOTP 验证不再永远失败。

**P1 重要修复：**
4. **SSE 鉴权**：authMiddleware 支持 `?token=` 查询参数回退，解决 EventSource API 不支持自定义 header 的问题。
5. **SSE 数据推送**：SSE 每 5s 推送 `metrics_update` 事件，不再只发心跳。
6. **Logo 上传**：`handleUploadLogo` 实际写入 `/opt/wukong/data/uploads/` 并将 URL 写入数据库。
7. **Settings 白名单**：`handleGetSetting`/`handleSetSetting` 加 `allowedSettingKeys` 白名单，防止访问/覆盖敏感配置。
8. **PingTargets 清空**：`handleUpdateConfig` 始终用主控下发列表覆盖（含空列表），允许后台清空所有 Ping 目标。

**P2 并发与质量：**
9. **client 竞态**：`Agent.client` 加 `clientMu` 读写锁保护，避免 reportLoop 与 autoUpgradeCheck 并发读写。
10. **Webhook 发送**：`WebhookNotifier.Send` 实际发送 HTTP POST + X-Webhook-Secret 头。
11. **告警引擎退出**：`Run(ctx)` 支持 context 取消，主控可优雅停止告警引擎。
12. **DNS 超时**：`isIPv6` 的 `LookupHost` 加 3s 超时，防止 DNS 问题卡死 Ping 采集器。
13. **strings.Title 弃用**：替换为自定义 `titleCase` 函数。

**P3 体验与清理：**
14. **登出按钮**：MainLayout 用户图标改为 SwitchButton，点击清除 JWT 并跳转登录页。
15. **退出清理**：main.go 先 cancel 维护/告警 context 再停服务，避免 SQLite "database is closed" 错误。
16. **安装脚本占位符**：主控安装脚本动态替换站点域名，不再硬编码 `https://<域名>/...`。

### 涉及文件
- `internal/agentcore/agent.go` — ticker 泄漏、client 竞态、PingTargets 清空
- `internal/auth/auth.go` — token 类型区分、TOTP 持久化接口
- `internal/webapi/handlers.go` — SSE 鉴权/数据推送、Logo 上传、Settings 白名单、TOTP 持久化、安装脚本占位符
- `internal/alert/engine.go` — context 退出
- `internal/notify/notify.go` — Webhook HTTP 发送
- `internal/agentcore/ping_collector.go` — DNS 超时
- `internal/agentcore/collector.go` — strings.Title 替换
- `cmd/server/main.go` — 退出清理顺序、alert engine context
- `web/src/layouts/MainLayout.vue` — 登出按钮
- `AGENTS.md` / `CHANGELOG.md` — 文档更新

## [2026-06-25 08:25] - 告警中心显示历史告警，补齐 Ping 告警，修复首页最近上报时间

### 改动前总结
1. 告警中心只请求 `/api/alerts/active`，已恢复告警不会显示，看起来像没起作用。
2. 告警引擎只检查离线、CPU、内存、磁盘，没有检查 Ping 延迟和 Ping 丢包。
3. 首页卡片优先使用 `updated_at` 显示最近上报时间，个别节点如 ff1 可能显示“3分钟前”但实际 `last_seen_at` 每秒更新。

### 改动后总结
1. `/api/alerts` 改为返回最近 100 条历史告警，包含 `firing/resolved`；告警中心使用该接口并新增恢复时间列。
2. store 新增 `ListAlerts(limit)`。
3. 告警引擎新增 Ping 延迟/丢包检查：默认阈值 200ms / 20%，持续时间复用资源告警持续时间；系统设置页新增 Ping 延迟/Ping 丢包阈值。
4. 首页最近上报时间优先使用 `last_seen_at`，避免 `updated_at` 偶发滞后造成误显示。

### 涉及文件
- `internal/store/store.go`、`internal/store/sqlite.go` — 历史告警查询
- `internal/webapi/handlers.go` — `/api/alerts` 返回历史告警；告警设置新增 Ping 阈值
- `internal/alert/engine.go` — Ping 延迟/丢包告警检查
- `web/src/views/Alerts.vue` — 显示历史告警和恢复时间
- `web/src/views/Settings.vue` — Ping 告警阈值配置
- `web/src/views/public/PublicHome.vue` — 最近上报时间优先 last_seen_at

## [2026-06-25 08:05] - 并发单包 Ping 与 live2 出口 IP 兜底修复

### 改动前总结
1. 虽然 `ping_interval=1`，但 PingCollector 串行探测 4 个运营商目标，且 ICMP 使用 `ping -c 3`，导致单线路实际无法稳定每秒产生点。
2. live2 公网 IP 查询失败时会回退到云内网地址，出现 `172.31.*` / `fe80::*` 异常显示。

### 改动后总结
1. PingCollector 对所有运营商目标并发探测，ICMP 改为 `ping -c 1 -W 1`，配合 1 秒配置尽量保证每条线路每秒一个原始点。
2. 公网 IP 查询使用多服务商兜底：api4/api6.ipify、icanhazip、ifconfig.me；仍严格过滤非公网地址，获取不到公网就留空。

### 涉及文件
- `internal/agentcore/ping_collector.go` — 并发探测 + 单包 ICMP
- `internal/agentcore/agent.go` — 多服务商公网 IP 查询与过滤

## [2026-06-25 07:55] - 过滤非公网出口 IP，Ping K 线改为秒级颗粒度

### 改动前总结
1. 出口 IP 回退逻辑会把本机网卡私网地址当作出口 IP 上报，例如 `172.31.*` 和 IPv6 link-local `fe80::*`。
2. Ping K 线接口读取 1 分钟聚合表，前端时间标签只显示 `HH:mm`，不是每秒颗粒度。

### 改动后总结
1. 探针和主控双侧过滤非公网 IP：私网 IPv4、loopback、link-local、IPv6 ULA/link-local 均不会作为出口 IP 入库或显示。
2. Ping K 线接口改为直接读取原始 `metrics_ping_YYYYMMDDHH` 小时表，按 `ts` 秒级聚合返回；前端时间标签显示 `HH:mm:ss`。
3. `ping_agg_1min` 仍保留用于历史维护/兜底，但 K 线展示使用秒级原始数据。

### 涉及文件
- `internal/agentcore/agent.go` — 过滤非公网出口 IP
- `internal/grpcapi/agent_server.go` — 入库前再次过滤非公网 IP
- `internal/store/sqlite.go` — Ping K 线查询改为秒级原始数据
- `web/src/views/NodeDetail.vue`、`web/src/views/public/PublicServerDetail.vue` — 时间标签显示秒

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