# wukong 开发提示（DEVTIPS）

> 记录跨会话必须记住的实现细节与踩坑教训。新增条目请带北京时间，并按主题分节。

## 前端 UI 设计系统（2026-10-05 04:13 重构后）

### 目录与职责划分

```
web/src/
├── components/     # 只负责结构与 props 契约的展示组件（Wk*）
├── composables/    # 跨页共享状态：useTheme / usePolling / useOverview
├── utils/          # http.ts（axios 拦截器）/ format.ts / charts.ts
└── styles/         # variables → base → layout → components → element（index.scss 只做 @use 汇总）
```

- 视觉定义的**唯一来源**是 `styles/`。`Wk*` 组件的 scoped 样式只允许写"该组件特有的结构"（如 order、尺寸变体），不允许写颜色/圆角/字号字面值。
- 新页面需要卡片/指标/进度条/空态时，**先用现有 `Wk*` 组件**；确实缺组件再往 `components/` 加，不要在页面里复制一份 HTML + scoped 样式。

### 设计令牌（`styles/variables.scss`）

| 令牌族 | 说明 |
|---|---|
| `--wk-bg / bg-soft / bg-recess / panel / elevated` | 画布→内凹→卡片→浮层四级表面，中性灰阶（暗 #0a0a0b 起，浅 #f7f7f8 起） |
| `--wk-border / border-strong` | hairline 边框承担主要分层，阴影只用于浮层 |
| `--wk-primary` | 全站唯一强调色（暗 #5e6ad2 / 浅 #4f5bd5），**派生色一律 `color-mix()` 推导** |
| `--wk-success / warning / danger / info` + `-soft` + `-text` | 颜色只表达状态，不做装饰；浅色主题下取文本安全色（对比度 ≥ 4.5:1） |
| `--wk-space-1..10 / radius-xs..xl / fs-xs..3xl / fw-* / ctl-sm..lg` | 尺度令牌，禁止在组件里写魔法数 |
| `--wk-chart-grid / chart-axis / chart-text / chart-tooltip-* / chart-c1..c7` | 图表专用令牌 |

新增令牌规则：只增不改名。旧名（`--wk-shadow-sm/md`、`--wk-border-light`、`--wk-panel-solid`）保留为别名，避免历史页面回归。

### 主色注入链路（重要）

`设置页 theme.primary` → `useTheme().setPrimary()` → 只在 `<html>` 上写一个内联 `--wk-primary`。
`--wk-primary-hover/soft/glow` 与 `--el-color-primary*` 都写成 `color-mix(in srgb, var(--wk-primary) N%, …)`，浏览器在**计算值阶段**求值，所以内联覆盖会自动让全部派生色与 Element Plus 联动。

踩坑：早期版本 `applyTheme()` 只写 `data-theme`，导致"自定义主色"存进了 SQLite 却从未生效。改主题相关代码时，务必确认链路末端真的有 `documentElement.style.setProperty`。

### 覆盖 Element Plus 浮层（重要）

覆盖 `.el-popper` 时，**背景、文字色、箭头三者必须一起接管**。EP 默认 `effect="dark"` 的 tooltip 是
“深底 + 浅字”一对：只改 background 会把文字留在新背景上，浅色主题下就是**白底白字**，
用户看到的正是“一个空白悬窗”（箭头也会仍是深色，变成白框黑箭头）。

另外 `.el-popper.is-dark` 的特异度高于 `.el-popper`，必须把 `.is-dark` / `.is-light` 一起写进选择器，
并用 `!important` 覆盖 `.el-popper__arrow::before`。侧栏导航这类“只是提示菜单名”的场景优先用原生
`title`（展开态传空串即不显示），不要包 `el-tooltip`，否则折叠/展开两种状态还要额外管弹层。

### ECharts 与 CSS 变量（重要）

canvas **不解析** `var(--x)`。任何 `axisLabel: { color: 'var(--wk-text-muted)' }` 都会静默退化成默认色。

正确做法：`utils/charts.ts`
- `readChartTokens()`：`getComputedStyle` 读 `--wk-*`，再用隐藏探针元素的 `style.color` 归一化成 `rgb()/rgba()`（自定义属性的计算值是未求值的 token 串，`color-mix()` 必须走这一步）；
- `ensureWukongTheme()`：按令牌注册 `wukong` 主题，替代旧代码写死的 `echarts.init(el, 'dark')`；
- `baseGrid/baseTooltip/baseLegend/baseCategoryAxis/baseValueAxis/baseDataZoom/buildLineSeries/tooltipRow`：统一图表口径。

`WkChart.vue` 是唯一出口：传 `builder`（函数）+ `deps`（数据），它负责 init/dispose/ResizeObserver，并监听 `useTheme().themeVersion` 在切主题/改主色后重画。

细节：`baseGrid()` 默认 `bottom: 28`，是给底部 dataZoom 滑块让位；只用滚轮缩放时传 `{ bottom: 8 }`，否则时间轴与滑块会重叠。

**量级差异大的多系列不要共轴**：Ping 延时就是典型——上海电信 5.40ms 与上海联通 5.57ms 在
0~60ms 轴上只差 0.3% 高度，完全重叠；**对数刻度也解决不了**（问题不是量级而是绝对差值太小）。
曾试过 small multiples（每线一个小图、Y 轴 `min` 按本线数据下界自适应），能彻底避开重叠，
但用户反馈“叠加对比已足够”并要求取消，所以 `WkPingChart.vue` 最终只保留叠加对比 +
一条每线路的 均/最低/最高/丢包 摘要行（数值不靠 tooltip 也能读到）。新增多系列图表时先问一句：
这些系列的取值范围是否吻合？差一个量级就该拆图而不是调颜色。

**图例放在左上时不要写 `yAxis.name`**：ECharts 把轴名画在轴顶端（也是左上），两者会直接重叠，
而且 `grid.containLabel` 不会为轴名预留空间（实测“ms”盖住了图例“上海电信”）。单位改放到
卡片副标题和统计摘要里；确实需要轴名时，把图例改到右上或给 `grid.top` 留足高度。

### 丢包时间轴与 Ping 数据类型（2026-10-10 04:09）

- `PingPoint` / `pointTime` / 分桶合并算法的**唯一来源是 `web/src/utils/ping.ts`**。
  注意：`<script setup>` 里 `export interface` 导出的类型**外部 import 不到**（等于白写），
  要跨文件共享的类型必须放 `.ts` 模块。
- **多行色条/时间轴必须按全局时间域分桶**（所有序列的最早点—最晚点），不能每条各自首尾分桶 ——
  否则某条数据有缺口就整行错位，同一列不是同一时刻，加时间刻度反而是错的。
- "哪一段时间丢包"要**直接给答案**：连续丢包格合并成区间列出来（`mergeLossRanges`），
  段数超上限时按严重度优先（bad 优先、再按峰值降序）而不是取前 N 个。
- 平均丢包会掩盖尖峰：右侧数值必须**同时给平均与峰值**（平均 0.2% 可能藏着一次 100% 丢包）。
- 色条本体 `.wk-strip` / `.wk-strip-cell` 留在 `components.scss`（跨组件复用），
  行布局与标签样式放组件 scoped。

### 实时数据与轮询

- 统一走 `composables/useOverview.ts`：模块级单例 + 引用计数，多个组件共用**一个** 1s 定时器；`document.visibilityState !== 'visible'` 时暂停（`usePolling.ts` 同理）。
- 不要再在组件里写 `setInterval(fetch, 1000)`。历史事故（2026-07-23）：SQLite 写锁被大表拖死后，前端多路重复轮询会显著放大阻塞面。
- 滚动采样：`useOverview` 在内存里保留最近 600 个采样点（1s × 10min），供 KPI sparkline 与"集群负载趋势"使用，**不需要新增后端聚合接口**。注意：数组必须整体重新赋值（`history.value = next`），否则浅比较的 `watch` 与 `WkChart` 的 `deps` 感知不到新增。
- 需要"强制刷新"（改名/删除节点后）调 `refreshOverview()`，不要各页自己重新拉。

### 格式化与语义阈值

`utils/format.ts` 是唯一实现：`formatBytes`（统一 KB/MB/GB，不再混用 KiB）、`formatBytesShort`、`formatRate`、`formatDuration`、`relativeTime`、`formatClock`（HH:mm:ss）、`formatHourMinute`、`statusText`、`archText`、`cpuText`、`loadText`、`average`、`clampPercent`。

- 负载分级全局统一：`LOAD_WARN = 70`、`LOAD_DANGER = 85`（`loadLevel()`）。旧代码里 60/85 与 70/90 两套阈值已收敛为一套。
- **丢包率不能套用资源阈值**：任何丢包都应可见，用 `lossTone()`（>0 warn、≥5% fail）这类独立语义。
- 数字一律 `.wk-num`（等宽 + `font-variant-numeric: tabular-nums`），否则每秒刷新的数值会左右抖动。
- **节点状态只有一个入口：`nodeState()`（四态 online/stale/offline/unknown）**，阈值 `STALE_SECONDS=300`
  与后端 `publicStatus` 一致。历史教训：后台只看 `agents.online`（gRPC 流在就绿），公开页看指标新鲜度，
  探针自升级后采集挂掉时会出现“后台在线 / 公开页离线”。新增展示状态的页面必须用 `nodeState`，
  不要再写 `row.online ? 'online' : 'offline'`；公开接口的 `status` 字段直接沿用，不在前端重算。
- **列表默认按名称排序**（`localeCompare(..., 'zh-Hans-CN')`）：按 CPU 等实时值排序会让卡片每秒重排，
  人眼无法定位；需要排序时用户自己切换。

### 表格密度（Nodes / Alerts）

- el-table 的 `.cell` 默认 `overflow:hidden; text-overflow:ellipsis`：列的 `min-width` 必须 ≥ 内容实际宽度，否则会截出多余的"…"。
- 窄列（≤96px）里放 `WkProgressBar` 时用 `.wk-cell-meter`（Nodes.vue 内）改成"数值在上、4px 条在下"；横向布局会把进度条压到 0 高度（纵向 flex 里 `track` 必须 `flex: none`）。
- 列总宽（各 `min-width` 之和）控制在 1150px 以内，1440 视口减去侧栏后不需要横向滚动。

### 响应式断点与手机适配（2026-10-06）

断点统一用三档，**不要再发明新数值**：`≤1080`（隐藏顶栏指标）、`≤860`（侧栏转抽屉、网格降列）、
`≤640`（手机：卡片收紧、单列、顶栏收缩）。手机规则集中在 `styles/components.scss` 末尾一段。

- **顶栏必须与正文套同一个容器**。`.wk-public-inner`（`width: min(1240px, 100%-32px); margin:0 auto`）
  是内容宽度约束；header 想背景通铺就用“外层通铺 + 内层套 `.wk-public-inner`”的结构，
  否则 `space-between` 会贴视口边缘，与正文左对不齐（用户反馈的“太靠边”就是这个）。
- **窄屏下“不裁信息”与“不跳动”必须同时解决**：`nowrap + overflow:hidden` 会直接裁掉内容，
  改 `wrap` 又会被每秒变化的文本宽度带着在 1↔2 行之间跳。正确做法是**固定折行结果 + 锁 `min-height`**
  （节点卡底行：`flex-wrap:wrap` + `row-gap:4px` + `min-height:34px`，时间行 `width:100%`）。
- sticky 顶栏一定要给半透背景 + `backdrop-filter`，否则卡片滚过时会穿帮。
- `<style scoped>` 是**纯 CSS**，写 `//` 注释会直接让 `vite build` 失败（报
  `Unexpected '/'. Escaping special characters with \ may help.`）；只有 `lang="scss"` 块才能用 `//`。
  改完样式先 `npx vite build` 跑一次再提交。

### 接口字段名与 CSS 覆盖顺序（两个静默失效的坑）

- **同一类数据在两套接口里字段名不同**：历史趋势点在管理接口 `store.RawSystemMetric` 里是
  `json:"ts"`，在公开接口 `webapi/public.go` 里是 `json:"timestamp"`。前端读错就整条轴变 `undefined`，
  而且只有其中一个页面出错、另一个页面正常，极易误判成"某个节点数据坏了"。
  写图表取时间一律用 `item.ts ?? item.timestamp`，并依赖 `formatClock`/`formatHourMinute` 的
  `-` 兜底（非法值不再 `String(value)` 输出 "undefined"）。
- **`index.scss` 的 @use 顺序决定覆盖结果**：variables → base → layout → components → element。
  后引入文件里的同特异度规则会覆盖先引入文件里的**媒体查询**（媒体查询不增加特异度）。
  所以覆盖 Element Plus 组件的手机端规则必须写在 `element.scss` 内，放 `components.scss` 会静默失效。

### 可访问性与动效

- 图标按钮必须有 `aria-label`；`:focus-visible` 统一用 `--wk-ring`。
- `prefers-reduced-motion: reduce` 时全站关闭呼吸光晕与骨架 shimmer（`styles/base.scss` 末尾）。
- 空态禁止 emoji（旧公开首页出现过 📭），统一用 `WkEmptyState` 的内联 SVG。

### 本地无 Go 工具链时怎么验 UI

`build/mock-api.mjs`（`build/` 已 gitignore）复刻了 `internal/webapi` 的只读响应结构，监听 `127.0.0.1:64443`，与 `vite.config.ts` 的 proxy target 一致：

```bash
node build/mock-api.mjs &        # 提供模拟数据（8 节点 / 4 运营商 / 12 条告警）
cd web && npx vite               # http://127.0.0.1:5173，登录接受任意账号密码
```

改后端接口字段时，记得同步这个 mock，否则 UI 验证会失真。

**Go 侧现在也能本地验了**：本机没装 Go 但**有 docker**，用共享 module 缓存跑 vet/build 即可，
不必再等 GHCR 构建才发现编译错误（一次浪费 ~6 分钟）：

```bash
docker run --rm -v /root/wukong:/src -v wukong-gomod:/go/pkg/mod -w /src \
  -e CGO_ENABLED=1 golang:1.25 sh -c 'go vet ./... ; echo VET_EXIT=$?'
```

注意 `mattn/go-sqlite3` 需要 CGO，跑 `go build` 时不要随手加 `CGO_ENABLED=0`（vet 不链接，无所谓）。

### 构建产物与嵌入

- `cd web && npm run build` → 产物输出到 `internal/webapi/dist/`（`vite.config.ts` 的 `build.outDir`）。
  该目录**本身不参与发布**：镜像里的前端是 CI 在 `node:22-alpine` 阶段从源码重新构建的。
- Go 侧用 `//go:embed all:dist`，`all:` 前缀不能少（Vite 会生成 `_` 开头的资源）。
- 新增静态资源放 `web/public/`（如 `favicon.svg`），Vite 会原样复制进 `dist/`。
- `internal/webapi/dist/` **不再跟踪构建产物**（2026-10-06 已清理，规则见下一小节），
  改前端后不要提交 dist 里的任何文件。


### 构建产物不入库，但要留 embed 占位（2026-10-06）

- `internal/webapi/dist/` 是**构建产物**，仓库里只保留 `.gitkeep`。CI 走
  `Dockerfile` 的 `COPY --from=frontend-builder`，与仓库里的 dist 无关；本地用
  `make build-frontend` / `make all` / `make dev` 生成。
- **`//go:embed all:dist` 要求目录非空**：整个 dist 不存在时 `go build` 直接报
  `pattern all:dist: no matching files found`（连 `go vet ./...` 都会红）。所以必须跟踪一个
  占位文件，而不是把目录彻底清空。
- **Git 忽略规则的关键坑**：`.gitignore` 里写 `internal/webapi/dist/`（目录级）会让
  `!internal/webapi/dist/.gitkeep` **完全失效**，因为 Git 不会进入已被忽略的目录。
  正确写法是排除内容 `internal/webapi/dist/*` 再取反 `!.../.gitkeep`。
- 已跟踪的文件不受 `.gitignore` 影响：`git check-ignore <已跟踪文件>` 会告诉你"没被忽略"，
  这不是规则没写对，而是 Git 的行为。要解除跟踪必须 `git rm --cached`（本地文件保留）。
- 缺前端产物时不会白屏：`webapi/embed.go` 的 `init()` 检测 `dist/index.html`，缺失时
  `PlaceholderHandler` 会显示"请运行 make build-frontend"的引导页。

## 运营商 Ping 目标的作用域（2026-10-05）

- `isp_targets.scope` 三态：`all` / `include` / `exclude`，配套 `agent_ids`（逗号串，存库前 `joinIDs`、读取后 `splitIDs`）。
  不用关联表是因为目标与节点量级都很小，且只在配置下发时整体读取。
- 判定入口只有一个：`store.ISPTarget.AppliesTo(agentID)`。**所有下发路径都必须走它**：
  `AgentServer.enabledPingTargetsFor(agentID)`（注册响应 + `buildConfigFrame` 配置热更新）。
  新增下发渠道时如果直连 `ListISPTargets()`，作用域会被绕过。
- 过滤故意只在主控做：探针不需要知道作用域概念，也不需要为了这个能力全量升级。
- 旧库兼容：`InitSchema` 的 ALTER TABLE 列表会补 `scope`/`agent_ids` 列，默认 `all`，行为与升级前一致；
  `ScopeText()` 对非法值也回退 `all`。
- 为什么需要它：IPv6 目标（上海移动 `2409:8088::a`）在无公网 IPv6 出口的节点上必然全部失败，
  表现为该线路 100% 丢包、拉高告警。现网 hk2香港、sh1上海 就没有 IPv6 出口。
  节点能否走 v6 直接看探针自报的 `agents.ip_v6`（设置页一键选择用的就是它）。
- 公开详情页 `publicPingISPs(agentID)` 同样要过滤，否则会出现“有线路名、永远没数据”的空行。

### 告警抑制期与配置热下发（两个易错点）

- **抑制期只能压制“重复触发”，绝不能 `return` 掉整个检查**。
  旧实现 `if 抑制期 { return }` 写在阈值判断之前，结果是指标恢复后告警最长 30 分钟不转 resolved，
  用户看到的就是一条“早就好了但还在报警”的记录。正确写法：抑制期只拦住 `shouldFire` 分支，
  `value <= recovery` 的恢复分支必须照走（`resolveAlert` 会顺带清掉抑制标记）。
- **改完影响探针行为的配置（ISP 目标、节点采集/Ping 频率）必须主动重发一次**。
  探针只在建连时收到一次 `COMMAND_UPDATE_CONFIG`，不重发就会一直用本地旧配置探测，
  表现为“改了没生效”+ 告警数据源本身就是错的。
  机制：`AgentServer.InvalidateAgentConfigs()` 给在线探针打 `configDirty`，
  stream 心跳循环（≤15s）取出后重发。**gRPC stream 的 Send 必须在处理该 stream 的 goroutine 里做**，
  所以只能“标记 + 由 stream 自己发”，不能从 webapi 直接往 stream 里 Send。
- `webapi` 不直接依赖 `grpcapi`：用 `ConfigInvalidator` 接口在 `cmd/server/main.go` 里注入，
  未注入时仅影响下发时效，不会报错。
- `ping_intv` 下限曾是 5 秒，与“默认 1 秒”的决策矛盾，会让节点详情页保存 1 直接 400；
  改默认值时要回头检查校验区间是否跟着改了。

### 告警规则模型（2026-10-07 00:53）

- 六项告警（offline/cpu/mem/disk/ping_latency/ping_loss）**各自一条规则**，存
  `settings.alert_rule_<metric>` 的 JSON：`enabled / warning / duration / recovery / suppress_min`。
- **所有默认值、范围、单位、"哪一项有持续时间/滞回"只写在 `alert.ruleSpecs` 一处**，
  引擎兜底、API 校验、前端渲染都从 `GET /api/alert-rules` 的 specs 取。
  加新告警项只需往 `ruleSpecs` 加一条 + 引擎里加一次 `checkMetric`，前端零改动。
- **离线项特殊**：`warning` 的含义是"无心跳秒数"，`has_duration=false`、`has_recovery=false`
  （恢复=重新上线，没有滞回可言），`normalize` 会把这两个字段强制置 0，前端按 spec 不渲染。
- **滞回必须低于阈值**：`normalize` 里 `recovery >= warning` 会回退到默认值，`ValidateRule` 也会拒绝，
  否则 `value <= recovery` 永远不成立、告警无法自动恢复。
- **关闭某项要清理遗留 firing**：`Engine.ruleEnabled` 记录上一轮开关，只在"启用→关闭"这一次
  调 `resolveMetricAlerts` 静默 resolve（不发通知，否则十几个节点一起推会打爆微信配额）。
  关掉后不处理的话，告警中心会永远挂着这条 firing。
- 旧接口 `/api/alert-settings` 保留兼容：GET 从规则派生，PUT 只改 warning/duration
  并回写旧扁平 key。**新前端一律用 `/api/alert-rules`**。

## 通知渠道与微信推送（2026-10-05）

### 告警分发出口

`Engine.notifyChannels(msg)` 是唯一出口（fire / resolve 两处都调它）：
- **Telegram 逐条即时**（不走合并器）；
- **pushplus（微信 ClawBot 等）走 `AlertAggregator` 合并节流**。
加新渠道只改 `notifyChannels`，不要去改调用点。

### 为什么微信必须做合并节流（长期约束）

- 微信 ClawBot 官方限制：**每下发 10 条、或每隔 24 小时，需用户在微信里主动发一条消息激活**，否则直接失败；
- pushplus 自身红线（与渠道无关）：实名用户 **1 分钟 5 次**、**相同内容 1 小时 3 条**、
  单日超 1000 次**封号 7 天**（返回码 900，官方明确说继续请求会加重限制）；
- 本项目 1 秒采集 + 5 秒一轮告警检查，一次网络抖动就能几十个节点同时超阈，
  逐条推必然打满配额，结果是“最关键的告警反而发不出去”。
所以 `AlertAggregator` 语义对齐 Alertmanager：空闲时第一条立即发，然后进窗口（默认 5 分钟，
后台可改 1~60），窗口结束时把缓存合成一条汇总并续开窗口。要避开 10 条限制就**换渠道而不是做轮换**
（`wechat` 公众号渠道无条数激活限制，渠道已是可配下拉，改配置即可，不用改代码）。
多 token/多账号轮换、脚本模拟“用户主动对话”属于规避风控，会触发 900 封号，**禁止实现**。

### pushplus 接入三个坑

1. **接口是异步的**：`code=200` 只代表“已受理排队”，不代表微信送达；必须看业务码而不是 HTTP 状态码。
   已实测：假令牌返回 `HTTP=200` + `{"code":903,"msg":"用户令牌不正确"}`。成功时 `data` 是消息流水号，
   失败时 `data` 是错误描述，所以日志里要留流水号供事后查投递结果（自动查需开放接口 AccessKey，未做）。
2. **部分错误绝不能重试**：900/903/905/888 都是账号或配置问题，为此 `notify` 包加了 `retryableError`
   接口，`pushplusError.Retryable()` 只对 500/600 返回 true；新渠道遇到同类情况请实现这个接口而不是改重试函数。
3. **只用 `template=txt`**：ClawBot 只支持纯文本，其他模板会被压成摘要，详情要用户点链接才看得到。
   默认地址用 `https://www.pushplus.plus`（文档写的是 http，不要把令牌明文发上公网）。

### 敏感令牌存储约定

`pushplus_token` 与 `telegram_bot_token` 一样：**不回显、留空表示保留原值**，并且**故意不加入
`allowedSettingKeys` 白名单**（否则通用 `GET /api/setting/{key}` 能把令牌读走），只能走专用接口。

### IPv6 出口可用性判定（2026-10-08 14:48）

- **有公网 IPv6 地址 ≠ IPv6 可用**。外部 API 看到的客户端地址可能是运营商 **NAT64 合成地址**
  （`64:ff9b::/96`，RFC 6052），只能 v6→v4，探测真 IPv6 目标必然 100% 失败。
  同类要一起排除的还有：`64:ff9b:1::/48`（NAT64 本地）、`2001::/32`（Teredo）、
  `2002::/16`（6to4）、`100:64::/10`（DS-Lite）；IPv4 侧另排 CGNAT `100.64.0.0/10`。
- **判定只在 `internal/netutil` 一处**：探针上报、主控落库清洗、目标下发过滤、公开页线路列表
  全部调它。历史上 agentcore 和 grpcapi 各有一份 `isPublicIP`，口径分裂就是这么来的。
- **主控必须是权威清洗方**：`effectiveIPv4/effectiveIPv6` 每次上报都重新校验**库里已有值**，
  不合法就置空。只改探针判定没用 —— 旧探针会持续上报坏地址，而"非空才覆盖"的写法
  让一次误存永远清不掉（也不用为此改 proto/加字段）。
- **作用域是静态列表，节点能力会变**：`ISPTarget.AppliesToAgent(agent)` = 人工 scope + 能力判定，
  gRPC 下发与 `publicPingISPs` 共用。只按 scope 列表下发会让失去 v6 能力的节点长期误告警。
- **网卡回退路径不上报 IPv6**：`getLocalIPs` 拿不到任何连通性证据，误报代价（长期 100% 丢包 +
  误告警）远高于漏报。`setPublicIPs` 的规则是"v4 成功但 v6 为空 = 确实没有可用 v6，清空"，
  "两者都空 = 网络整体不可达，保留旧值"。
- 判定函数会被多 goroutine 并发调用，**前缀必须 init 预计算**，不要写"无锁 map 缓存"。

## 生产部署与 CI（2026-10-05 迁移后）

### Cloudflare 与 gRPC 的硬限制（重要）

**源站绝对不要用 502/503/504 作为业务错误状态码**：CF 会把这三种响应的 body 替换成它自己的错误页
（实测前端只能拿到 `error code: 502` 纯文本），后端精心拼的失败原因全部丢失。
需要表达“上游依赖（第三方 API）调用失败”时用 **424 Failed Dependency**（或其他 4xx），
CF 原样透传，axios 仍归为错误分支，前端不用改。受影响的 `POST /api/telegram/test` 与
`POST /api/pushplus/test` 已统一改为 424；以后新增“代理调用外部服务”的接口都要遵守这条。

- 橙云（Proxied）记录 **不能把 gRPC 代理到明文源站**：探针连 `server.lkz.pub:443` 会拿到
  `403 Forbidden` + `content-type: text/html`（gRPC 报 `PermissionDenied`），因为 Flexible 回源只会用 HTTP/1.1，
  h2/gRPC 被降级。因此 `agent_server_addr` 必须给 **直连源站的 `IP:64443`**（非 443 端口时代码走明文 gRPC），
  网页继续走 CDN。要统一走 TLS 就得在源站给 64443 前置 `listen ssl http2` + `grpc_pass`，并把 CF SSL 改成 Full。
- 回源端口非标准（64443）靠 CF 的 Origin Rules 实现；`521` = 源站 TCP 连不上（没服务/端口不通），
  `525` = TCP 通了但 TLS 握手失败（源站没上 TLS），`403+html` = 被 CF 边缘拦下，三种错误码能直接定位问题层。

### 多架构镜像（CI）

- 生产机是 arm64，拉镜像报 `no matching manifest for linux/arm64/v8` 就是镜像只有 amd64：
  `docker/build-push-action` **不写 `platforms` 时只构宿主架构**；`Dockerfile` 里也不能写死 `GOARCH`，
  要用 buildx 注入的 `TARGETARCH`（本地单平台构建时为空，用 `${TARGETARCH:-amd64}` 回退）。
- 提速：用原生 `ubuntu-24.04-arm` runner 每架构一个 job（公仓免费），按 digest 推送
  （`outputs: type=image,...,push-by-digest=true,name-canonical=true,push=true`，**push 要写在 outputs 里**），
  再由 merge job `docker buildx imagetools create -t <tag> <digest...>` 统一打 tag；
  QEMU 模拟编译 Go+CGO 要 10~25 分钟，原生并行只要 ~2.5 分钟。
- `actions/download-artifact` **必须带 `pattern: digests-*`**：不带 name 时会把 buildx 顺带产生的
  `*.dockerbuild` 元数据 artifact 一起下载，实测会重试 5 次后失败，直接把 merge job 带崩。
- `gha` 缓存要按架构分 `scope`，否则 Go 构建产物会跨架构污染。

### 覆盖已安装二进制必须用 rename

- `curl -o /path/to/wukong-agent` 覆盖 **正在运行** 的文件会被内核拒绝（ETXTBSY），
  curl 只报 `(23) Failure writing output to destination`，完全看不出真实原因。
- 正确做法（安装脚本与探针自升级都已采用）：先 `systemctl stop`，下到 `xxx.new`，`chmod +x` 后 `mv -f`；
  同目录 rename 不受 ETXTBSY 影响，且下载中断不会写坏原本可用的二进制。

### 鉴权失败日志

- `ValidateAgent` 在“查不到节点”与“密钥不对”两种情况都返回 false；已改为先 `GetAgent` 再区分，
  否则在后台删过节点后会满屏刷“secret 不匹配”，把人往密钥方向带偏。
- 后台删节点后，目标机上的探针不会自动停，会带着旧 `agent_secret` 每秒重试；
  要么重新执行安装命令，要么 `systemctl stop wukong-agent`。

### 本机运维入口

- 部署参数、容器重建命令、env 文件位置、遗留安全风险均记在根目录 `DEPLOY_CREDENTIALS.md`（已 gitignore，勿提交）。
- 本机（开发机）未安装 Go 工具链（`go: command not found`），Go 侧改动验证依赖 GHCR 镜像或 `docker run golang:*-alpine` 编译。
