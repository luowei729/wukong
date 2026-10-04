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

- `cd web && npm run build` → 产物直接输出到 `internal/webapi/dist/`（`vite.config.ts` 的 `build.outDir`），该目录**在仓库里是被跟踪的**，改完前端要一并提交。
- Go 侧用 `//go:embed all:dist`，`all:` 前缀不能少（Vite 会生成 `_` 开头的资源）。
- 新增静态资源放 `web/public/`（如 `favicon.svg`），Vite 会原样复制进 `dist/`。
