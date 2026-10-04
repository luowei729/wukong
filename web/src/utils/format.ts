// =============================================
// 统一格式化工具
// 原因：此前 formatBytes / relativeTime / loadLevel 在 5 个页面各写一份，
//       单位口径不一致（KiB 与 KB 混用）、阈值判断也不统一（70/85 vs 60/85），
//       重构后所有页面共用本文件，保证"同一数值在任何页面显示完全一致"
// =============================================

// 负载分级的统一阈值：≥85% 危险、≥70% 警告，其余正常
export const LOAD_WARN = 70
export const LOAD_DANGER = 85

export type LoadLevel = "normal" | "warning" | "danger"

/** 数值是否可用（排除 null/undefined/NaN） */
function isNum(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value)
}

/** 根据使用率返回负载级别，供进度条与文本色共用同一判定 */
export function loadLevel(value?: number | null): LoadLevel {
  if (!isNum(value)) return "normal"
  if (value >= LOAD_DANGER) return "danger"
  if (value >= LOAD_WARN) return "warning"
  return "normal"
}

/** 负载级别对应的令牌色变量名（组件里用 var() 引用，自动跟随主题与自定义主色） */
export function loadLevelColorVar(level: LoadLevel): string {
  if (level === "danger") return "var(--wk-danger)"
  if (level === "warning") return "var(--wk-warning)"
  return "var(--wk-success)"
}

/** 百分比：固定一位小数，无数据返回短横线 */
export function formatPercent(value?: number | null, digits = 1): string {
  if (!isNum(value)) return "-"
  return `${value.toFixed(digits)}%`
}

/** 数值保留指定位，无数据显示占位 */
export function formatNumber(value?: number | null, digits = 1): string {
  if (!isNum(value)) return "-"
  return value.toFixed(digits)
}

/** 把字节数按 1024 进制折算，全站统一用 KB/MB/GB/TB 口径（不再混用 KiB） */
export function formatBytes(value?: number | null, digits?: number): string {
  if (!isNum(value) || value <= 0) return value === 0 ? "0 B" : "-"
  const units = ["B", "KB", "MB", "GB", "TB", "PB"]
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  // 小于 10 的数值多保留一位小数，保证吞吐类小数值也有可读精度
  const keep = digits ?? (size >= 10 ? 1 : 2)
  return `${size.toFixed(keep)} ${units[index]}`
}

/** 紧凑字节数（用于 KPI 卡与摘要行，长度敏感） */
export function formatBytesShort(value?: number | null): string {
  if (!isNum(value) || value <= 0) return "0B"
  const units = ["B", "K", "M", "G", "T"]
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  return `${size.toFixed(size >= 10 ? 0 : 1)}${units[index]}`
}

/** 速率：字节/秒 → "1.25 MB/s" */
export function formatRate(value?: number | null): string {
  if (!isNum(value)) return "-"
  return `${formatBytes(value)}/s`
}

/** 运行时长：秒 → "12d 3h 5m"（详情页/卡片用同一紧凑格式） */
export function formatDuration(seconds?: number | null): string {
  if (!isNum(seconds) || seconds <= 0) return "-"
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h ${minutes}m`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}

/** 相对时间："3 秒前 / 5 分钟前"，用于"最近上报"这类实时字段 */
export function relativeTime(value?: string | number | Date | null): string {
  if (value == null || value === "") return "暂无上报"
  const time = new Date(value).getTime()
  if (Number.isNaN(time)) return "暂无上报"
  const diff = Date.now() - time
  if (diff < 0) return "刚刚"
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds} 秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  return `${Math.floor(hours / 24)} 天前`
}

/** 本地日期时间（tooltip 与详情页绝对时间用） */
export function formatDateTime(value?: string | number | Date | null): string {
  if (value == null || value === "") return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString()
}

/** Unix 秒时间戳 → 本地日期时间 */
export function formatUnixTime(seconds?: number | null): string {
  if (!isNum(seconds) || seconds <= 0) return "-"
  return formatDateTime(new Date(seconds * 1000))
}

/** 图表 X 轴时间标签：HH:mm:ss（秒级 ping 数据需要到秒） */
export function formatClock(value: string | number | Date): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  const hh = String(date.getHours()).padStart(2, "0")
  const mm = String(date.getMinutes()).padStart(2, "0")
  const ss = String(date.getSeconds()).padStart(2, "0")
  return `${hh}:${mm}:${ss}`
}

/** 图表 X 轴时间标签：HH:mm（长区间趋势用，避免轴标签过密） */
export function formatHourMinute(value: string | number | Date): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  const hh = String(date.getHours()).padStart(2, "0")
  const mm = String(date.getMinutes()).padStart(2, "0")
  return `${hh}:${mm}`
}

/** 状态映射：后端 online/offline/stale/unknown → 中文 */
export function statusText(status?: string | null): string {
  const map: Record<string, string> = {
    online: "在线",
    offline: "离线",
    stale: "数据延迟",
    unknown: "未知",
  }
  return (status && map[status]) || "未知"
}

/** 架构映射：探针上报的 go arch → 运维习惯写法 */
export function archText(arch?: string | null): string {
  if (!arch) return "-"
  if (arch === "amd64") return "x86_64"
  if (arch === "arm64") return "aarch64"
  return arch
}

/** CPU 描述：型号 + 核数 */
export function cpuText(value?: { cpu_model?: string; cpu_cores?: number } | null): string {
  if (!value) return "-"
  const model = value.cpu_model || "未知型号"
  const cores = isNum(value.cpu_cores) && value.cpu_cores > 0 ? ` · ${value.cpu_cores} 核` : ""
  return `${model}${cores}`
}

/** 负载：1/5/15 分钟，保留两位小数 */
export function loadText(value?: { load1?: number; load5?: number; load15?: number } | null): string {
  if (!value) return "-"
  const fmt = (v?: number) => (isNum(v) ? v.toFixed(2) : "-")
  return `${fmt(value.load1)} / ${fmt(value.load5)} / ${fmt(value.load15)}`
}

/** 单值负载格式化 */
export function formatLoad(value?: number | null): string {
  return isNum(value) ? value.toFixed(2) : "-"
}

/** 丢包率：后端返回 0~1 的比例，统一转成百分数值 */
export function lossPercent(lossRate?: number | null): number {
  if (!isNum(lossRate)) return 0
  return lossRate * 100
}

/** 系统名简写："Ubuntu 22.04" → "Ubuntu 22.04"，仅去掉多余空格与过长后缀 */
export function systemText(value?: string | null): string {
  if (!value) return "-"
  return value.trim()
}

/** 数组求平均（忽略非数值），用于集群平均指标计算 */
export function average(values: Array<number | undefined | null>): number | null {
  const nums = values.filter(isNum)
  if (nums.length === 0) return null
  return nums.reduce((sum, item) => sum + item, 0) / nums.length
}

/** 数组求和（忽略非数值），用于全站上下行流量汇总 */
export function sum(values: Array<number | undefined | null>): number {
  return values.filter(isNum).reduce((total, item) => total + item, 0)
}

/** 把值夹到 0~100，供进度条宽度直接使用 */
export function clampPercent(value?: number | null): number {
  if (!isNum(value)) return 0
  return Math.max(0, Math.min(100, value))
}
