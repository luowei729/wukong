// Ping 数据的共享类型与纯计算逻辑
//
// 为什么单独成模块：PingPoint 原先在 WkPingChart.vue 里 `export interface`（但
// <script setup> 导出的类型外部无法 import，等于白写）、在 PublicServerDetail.vue 里
// 又本地声明了一份、NodeDetail 里还用到第三种字段名（bucket_min）。类型与分桶算法
// 散在三处，改一处漏两处。这里作为唯一来源。
import { lossPercent } from './format'

/** 单个探测聚合点：后台 ping-agg 返回 bucket_min，公开接口返回 timestamp */
export interface PingPoint {
  timestamp?: string
  bucket_min?: string
  avg_lat: number
  min_lat: number
  max_lat: number
  loss_rate: number
}

/** 取探测点时间：兼容 bucket_min / timestamp 两种后端字段 */
export function pointTime(point: PingPoint): string {
  return point.timestamp || point.bucket_min || ''
}

/** 取探测点毫秒时间戳；无法解析时返回 NaN，由调用方跳过 */
export function pointMillis(point: PingPoint): number {
  const raw = pointTime(point)
  if (!raw) return NaN
  return new Date(raw).getTime()
}

// 色条格子类型：无丢包 / 部分丢包 / 严重丢包 / 该时段无数据
export type LossKind = 'ok' | 'warn' | 'bad' | 'none'

export interface LossCell {
  kind: LossKind
  /** 该桶起止时间（毫秒），用于显示"哪个时间段丢包" */
  from: number
  to: number
  avg: number
  peak: number
  samples: number
}

/** 连续丢包合并成的一段区间 */
export interface LossRange {
  from: number
  to: number
  peak: number
  kind: Exclude<LossKind, 'ok' | 'none'>
  samples: number
}

export interface LossRow {
  name: string
  cells: LossCell[]
  /** 24 小时平均丢包率 % */
  loss: number
  /** 24 小时峰值丢包率 % */
  peak: number
  ranges: LossRange[]
}

/** 丢包分级阈值：只要有一点丢包就算 warn，达到 50% 视为 bad（与旧版一致） */
const WARN_CEILING = 50

function kindOf(peak: number): LossKind {
  if (peak <= 0) return 'ok'
  if (peak < WARN_CEILING) return 'warn'
  return 'bad'
}

/**
 * 把多条线路的探测点聚合成统一时间轴的丢包色条。
 *
 * 关键：时间域取"所有线路的最早点—最晚点"的全局范围，而不是每条线各自的首尾。
 * 否则某条线路数据有缺口时，它的格子会与其他线路错位，同一列不再是同一时刻，
 * 时间刻度和"第 N 格对应几点"就都是错的。
 */
export function buildLossRows(
  series: Record<string, PingPoint[]>,
  cellCount = 120
): LossRow[] {
  const rows = Object.entries(series || {}).map(([name, points]) => ({
    name,
    points: points.filter((p) => Number.isFinite(pointMillis(p))),
  }))

  let start = Infinity
  let end = -Infinity
  for (const row of rows) {
    for (const point of row.points) {
      const ts = pointMillis(point)
      if (ts < start) start = ts
      if (ts > end) end = ts
    }
  }
  // 没有任何有效时间点时返回空行，调用方据此显示空态
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) {
    return rows.map((row) => ({
      name: row.name,
      cells: [],
      loss: 0,
      peak: 0,
      ranges: [],
    }))
  }

  const span = Math.max(1, end - start)
  const bucketMs = span / cellCount

  return rows.map((row) => {
    // 每个桶收集该时间段内所有采样点的丢包率（0~100）
    const buckets: number[][] = Array.from({ length: cellCount }, () => [] as number[])
    for (const point of row.points) {
      const index = Math.min(
        cellCount - 1,
        Math.floor((pointMillis(point) - start) / bucketMs)
      )
      buckets[index].push(lossPercent(Number(point.loss_rate || 0)))
    }

    const cells: LossCell[] = buckets.map((bucket, index) => {
      const from = start + index * bucketMs
      const to = from + bucketMs
      if (bucket.length === 0) {
        return { kind: 'none' as LossKind, from, to, avg: 0, peak: 0, samples: 0 }
      }
      const peak = Math.max(...bucket)
      const avg = bucket.reduce((sum, v) => sum + v, 0) / bucket.length
      return { kind: kindOf(peak), from, to, avg, peak, samples: bucket.length }
    })

    const all = row.points.map((p) => lossPercent(Number(p.loss_rate || 0)))
    return {
      name: row.name,
      cells,
      loss: all.length ? all.reduce((sum, v) => sum + v, 0) / all.length : 0,
      peak: all.length ? Math.max(...all) : 0,
      ranges: mergeLossRanges(cells),
    }
  })
}

/** 把连续丢包的格子合并成时间段，直接回答"什么时候丢的包" */
export function mergeLossRanges(cells: LossCell[]): LossRange[] {
  const ranges: LossRange[] = []
  let current: LossRange | null = null
  for (const cell of cells) {
    if (cell.kind === 'ok' || cell.kind === 'none') {
      if (current) {
        ranges.push(current)
        current = null
      }
      continue
    }
    if (current) {
      // 相邻丢包格合并：终点顺延，峰值与样本数累加，任一段严重就整体标严重
      current.to = cell.to
      current.peak = Math.max(current.peak, cell.peak)
      current.samples += cell.samples
      if (cell.kind === 'bad') current.kind = 'bad'
    } else {
      current = {
        from: cell.from,
        to: cell.to,
        peak: cell.peak,
        kind: cell.kind as 'warn' | 'bad',
        samples: cell.samples,
      }
    }
  }
  if (current) ranges.push(current)
  return ranges
}
