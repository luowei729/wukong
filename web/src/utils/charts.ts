// =============================================
// ECharts 主题工具
// 根因修复：ECharts 渲染在 canvas 上，canvas 不解析 CSS 变量，
//           旧代码写成 axisLabel:{color:'var(--wk-text-muted)'} 会被当作非法颜色，
//           轴标签/图例掉回默认色，浅色主题下几乎看不见；
//           并且旧代码写死 echarts.init(el,'dark')，切到浅色主题图表仍是深色底。
// 方案：统一用 getComputedStyle 把 --wk-* 令牌解析成真实色值后再喂给 ECharts，
//       主题或自定义主色变化时由 WkChart 重新构建 option，实现图表跟随全站主题。
// =============================================
import * as echarts from "echarts"

// 颜色探针：把 color-mix()/var() 之类的非法 canvas 颜色，借助浏览器自身解析成 rgb()
let probeEl: HTMLElement | null = null

function getProbe(): HTMLElement {
  if (!probeEl) {
    probeEl = document.createElement("span")
    // visibility:hidden 不影响 color 计算值解析，同时保证不占位不可见
    probeEl.style.cssText =
      "position:absolute;width:0;height:0;overflow:hidden;visibility:hidden"
    document.body.appendChild(probeEl)
  }
  return probeEl
}

/** 把任意合法 CSS 颜色写法归一化为 rgb()/rgba() 字符串 */
export function resolveColor(value: string): string {
  const probe = getProbe()
  probe.style.color = ""
  probe.style.color = value
  const computed = getComputedStyle(probe).color
  return computed || value
}

/** 读取根元素上的 CSS 变量真实值（自定义属性计算值是未求值的 token 串，需要再走一次颜色解析） */
export function cssVar(name: string, fallback = "#888888"): string {
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
  if (!raw) return fallback
  const resolved = resolveColor(raw)
  // 解析失败（例如非颜色变量）时回退，保证图表永远能画出来
  return resolved || fallback
}

/** 从 rgb()/rgba() 字符串换指定透明度的颜色，用于面积填充与 hover 底色 */
export function withAlpha(color: string, alpha: number): string {
  const match = color.match(/rgba?\(([^)]+)\)/)
  if (!match) return color
  const parts = match[1].split(/[\s,/]+/).filter(Boolean).map(Number)
  const [r, g, b] = parts
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** 图表用的一组真实色值（每次渲染重新读取，跟随主题切换） */
export interface ChartTokens {
  text: string
  muted: string
  grid: string
  axis: string
  tooltipBg: string
  tooltipBorder: string
  primary: string
  success: string
  warning: string
  danger: string
  palette: string[]
}

export function readChartTokens(): ChartTokens {
  const primary = cssVar("--wk-primary")
  const muted = cssVar("--wk-text-muted")
  return {
    text: cssVar("--wk-text"),
    muted,
    grid: cssVar("--wk-chart-grid", "rgba(128,128,128,0.12)"),
    axis: cssVar("--wk-chart-axis", "rgba(128,128,128,0.2)"),
    tooltipBg: cssVar("--wk-chart-tooltip-bg", "rgba(20,20,24,0.94)"),
    tooltipBorder: cssVar("--wk-chart-tooltip-border", "rgba(255,255,255,0.14)"),
    primary,
    success: cssVar("--wk-success"),
    warning: cssVar("--wk-warning"),
    danger: cssVar("--wk-danger"),
    // 7 色循环调色板：第 1 支跟随主色，其余来自辅助色令牌
    palette: [
      primary,
      cssVar("--wk-chart-c2"),
      cssVar("--wk-chart-c3"),
      cssVar("--wk-chart-c4"),
      cssVar("--wk-chart-c5"),
      cssVar("--wk-chart-c6"),
      cssVar("--wk-chart-c7"),
    ],
  }
}

// 已注册主题的签名：主题或主色变化时才重新注册，避免每次渲染都改全局主题
let themeSignature = ""

/** 依据当前令牌注册名为 wukong 的 ECharts 主题，替代旧代码里写死的 'dark' */
export function ensureWukongTheme(): string {
  const t = readChartTokens()
  const signature = [
    t.text,
    t.muted,
    t.grid,
    t.tooltipBg,
    t.primary,
    t.palette.join(","),
  ].join("|")
  if (themeSignature !== signature) {
    echarts.registerTheme("wukong", {
      color: t.palette,
      backgroundColor: "transparent",
      textStyle: { color: t.muted },
      title: { textStyle: { color: t.text } },
      legend: { textStyle: { color: t.muted } },
      tooltip: {
        backgroundColor: t.tooltipBg,
        borderColor: t.tooltipBorder,
        textStyle: { color: t.text },
      },
    })
    themeSignature = signature
  }
  return "wukong"
}

/** 取调色板第 index 个颜色（超出循环） */
export function seriesColor(tokens: ChartTokens, index: number): string {
  return tokens.palette[index % tokens.palette.length]
}

/**
 * 统一 grid：留足坐标轴标签空间，右侧不裁切
 * bottom 默认 28px 是为了给底部 dataZoom 滑块让位，
 * 否则滑块会压住 X 轴时间标签（无头浏览器实测重叠）；只用滚轮缩放的图传 8
 */
export function baseGrid(options: { bottom?: number; top?: number } = {}) {
  return {
    left: 8,
    right: 16,
    top: options.top ?? 36,
    bottom: options.bottom ?? 28,
    containLabel: true,
  }
}

/** 统一 tooltip：跟随主题的深/浅卡片，禁用动画避免每秒刷新时抖动 */
export function baseTooltip(tokens: ChartTokens) {
  return {
    trigger: "axis",
    confine: true,
    transitionDuration: 0,
    backgroundColor: tokens.tooltipBg,
    borderColor: tokens.tooltipBorder,
    borderWidth: 1,
    padding: [8, 10] as [number, number],
    textStyle: { color: tokens.text, fontSize: 12 },
    axisPointer: {
      type: "line",
      lineStyle: { color: tokens.axis, width: 1 },
    },
  }
}

/** 统一图例：可滚动，字号与次级文本色一致 */
export function baseLegend(tokens: ChartTokens) {
  return {
    type: "scroll" as const,
    top: 0,
    left: 0,
    right: 0,
    itemWidth: 10,
    itemHeight: 2,
    itemGap: 14,
    icon: "roundRect",
    textStyle: { color: tokens.muted, fontSize: 11 },
  }
}

/** 统一类目 X 轴（时间轴） */
export function baseCategoryAxis(tokens: ChartTokens, data: string[]) {
  return {
    type: "category" as const,
    boundaryGap: false,
    data,
    axisLine: { lineStyle: { color: tokens.axis } },
    axisTick: { show: false },
    axisLabel: { color: tokens.muted, fontSize: 11, hideOverlap: true },
    splitLine: { show: false },
  }
}

/** 统一数值 Y 轴（formatter 支持字符串模板与函数两种写法） */
export function baseValueAxis(
  tokens: ChartTokens,
  options: {
    name?: string
    max?: number
    formatter?: string | ((value: number) => string)
  } = {}
) {
  return {
    type: "value" as const,
    name: options.name,
    max: options.max,
    nameTextStyle: { color: tokens.muted, fontSize: 11, padding: [0, 0, 0, 8] },
    axisLine: { show: false },
    axisTick: { show: false },
    axisLabel: { color: tokens.muted, fontSize: 11, formatter: options.formatter },
    splitLine: { lineStyle: { color: tokens.grid } },
  }
}

/** 统一 dataZoom：内置滚轮缩放 + 底部细滑块 */
export function baseDataZoom(tokens: ChartTokens) {
  return [
    { type: "inside" as const, throttle: 80 },
    {
      type: "slider" as const,
      height: 18,
      bottom: 2,
      brushSelect: false,
      borderColor: "transparent",
      backgroundColor: withAlpha(tokens.grid, 0.35),
      fillerColor: withAlpha(tokens.primary, 0.12),
      handleStyle: { color: tokens.primary, borderColor: tokens.primary },
      moveHandleStyle: { color: tokens.axis },
      dataBackground: {
        lineStyle: { color: tokens.axis },
        areaStyle: { color: withAlpha(tokens.axis, 0.4) },
      },
      selectedDataBackground: {
        lineStyle: { color: tokens.primary },
        areaStyle: { color: withAlpha(tokens.primary, 0.2) },
      },
      textStyle: { color: tokens.muted, fontSize: 10 },
    },
  ]
}

/** 折线/面积 series 工厂：全站曲线口径一致（1.6px 线宽、无 symbol、LTTB 抽样） */
export function buildLineSeries(
  name: string,
  data: Array<number | null>,
  color: string,
  options: { area?: boolean; width?: number; yAxisIndex?: number; smooth?: boolean } = {}
) {
  return {
    name,
    type: "line" as const,
    data,
    smooth: options.smooth ?? false,
    sampling: "lttb" as const,
    large: true,
    symbol: "none",
    connectNulls: true,
    yAxisIndex: options.yAxisIndex ?? 0,
    lineStyle: { color, width: options.width ?? 1.6 },
    itemStyle: { color },
    // 面积用固定透明度，比旧代码的 hex + "22" 拼接更可控（色值非 hex 时旧写法会失效）
    areaStyle: options.area
      ? { color: `${withAlpha(color, 0.16)}`, origin: "start" as const }
      : undefined,
    emphasis: { focus: "series" as const },
  }
}

/** 生成 tooltip HTML 行：圆点 + 名称 + 右对齐数值，多处共用同一视觉 */
export function tooltipRow(
  color: string,
  label: string,
  value: string,
  extra?: string
): string {
  return `<div style="display:flex;align-items:center;gap:8px;font-size:12px;line-height:20px">
    <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};flex-shrink:0"></span>
    <span style="opacity:.85;min-width:92px">${label}</span>
    <span style="font-variant-numeric:tabular-nums;font-weight:600;margin-left:auto">${value}</span>
    ${
      extra
        ? `<span style="opacity:.7;font-size:11px;min-width:56px;text-align:right">${extra}</span>`
        : ""
    }
  </div>`
}
