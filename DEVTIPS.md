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

### ECharts 与 CSS 变量（重要）

canvas **不解析** `var(--x)`。任何 `axisLabel: { color: 'var(--wk-text-muted)' }` 都会静默退化成默认色。

正确做法：`utils/charts.ts`
- `readChartTokens()`：`getComputedStyle` 读 `--wk-*`，再用隐藏探针元素的 `style.color` 归一化成 `rgb()/rgba()`（自定义属性的计算值是未求值的 token 串，`color-mix()` 必须走这一步）；
- `ensureWukongTheme()`：按令牌注册 `wukong` 主题，替代旧代码写死的 `echarts.init(el, 'dark')`；
- `baseGrid/baseTooltip/baseLegend/baseCategoryAxis/baseValueAxis/baseDataZoom/buildLineSeries/tooltipRow`：统一图表口径。

`WkChart.vue` 是唯一出口：传 `builder`（函数）+ `deps`（数据），它负责 init/dispose/ResizeObserver，并监听 `useTheme().themeVersion` 在切主题/改主色后重画。

细节：`baseGrid()` 默认 `bottom: 28`，是给底部 dataZoom 滑块让位；只用滚轮缩放时传 `{ bottom: 8 }`，否则时间轴与滑块会重叠。

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

### 表格密度（Nodes / Alerts）

- el-table 的 `.cell` 默认 `overflow:hidden; text-overflow:ellipsis`：列的 `min-width` 必须 ≥ 内容实际宽度，否则会截出多余的"…"。
- 窄列（≤96px）里放 `WkProgressBar` 时用 `.wk-cell-meter`（Nodes.vue 内）改成"数值在上、4px 条在下"；横向布局会把进度条压到 0 高度（纵向 flex 里 `track` 必须 `flex: none`）。
- 列总宽（各 `min-width` 之和）控制在 1150px 以内，1440 视口减去侧栏后不需要横向滚动。

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

### 构建产物与嵌入

- `cd web && npm run build` → 产物输出到 `internal/webapi/dist/`（`vite.config.ts` 的 `build.outDir`）。
  该目录**本身不参与发布**：镜像里的前端是 CI 在 `node:22-alpine` 阶段从源码重新构建的。
- Go 侧用 `//go:embed all:dist`，`all:` 前缀不能少（Vite 会生成 `_` 开头的资源）。
- 新增静态资源放 `web/public/`（如 `favicon.svg`），Vite 会原样复制进 `dist/`。
- `internal/webapi/dist/` 已被 `.gitignore` 排除，但仓库里还残留两个早期误提交的跟踪文件
  （`dist/index.html` 与一个旧 js）。改前端后 **不要只提交这两个文件**（会留下指向未跟踪资源的破状态），
  镜像里的前端是 CI 从源码重新构建的；建议后续单独一次 `git rm --cached internal/webapi/dist` 清理。

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

## 生产部署与 CI（2026-10-05 迁移后）

### Cloudflare 与 gRPC 的硬限制（重要）

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
