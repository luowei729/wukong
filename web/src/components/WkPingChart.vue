<template>
  <!-- ============ Ping 线路图：叠加对比 + 每线路统计摘要 ============ -->
  <div class="wk-ping">
    <!-- 摘要行：颜色与图例一致，数值直接摊出来，不必悬停 tooltip 才能读到 -->
    <div v-if="lines.length" class="wk-ping-stats">
      <div v-for="(line, index) in lines" :key="line.name" class="wk-ping-stat">
        <span class="wk-ping-dot" :style="{ background: colorOf(index) }" />
        <span class="wk-ping-name">{{ line.name }}</span>
        <span class="wk-num wk-ping-val">
          均 {{ line.avg.toFixed(1) }}ms
          <span class="wk-sub">· 最低 {{ line.min.toFixed(1) }} · 最高 {{ line.max.toFixed(1) }} · 丢包
            {{ line.loss.toFixed(1) }}%</span>
        </span>
      </div>
    </div>

    <WkChart :builder="overlayBuilder" :deps="lines" :height="height" :loading="loading" />
  </div>
</template>

<script setup lang="ts">
// Ping 线路图（后台节点详情与公开详情页共用）
// 只保留"叠加对比"一种视图：多线共轴便于横向比较量级与尖峰，
// 每条线路的 均/最低/最高/丢包 用摘要行直接给出，避免必须悬停才能看到数值。
import { computed } from 'vue'
import WkChart from './WkChart.vue'
import {
  baseCategoryAxis,
  baseDataZoom,
  baseGrid,
  baseLegend,
  baseTooltip,
  baseValueAxis,
  buildLineSeries,
  readChartTokens,
  seriesColor,
  tooltipRow,
} from '@/utils/charts'
import { formatHourMinute, lossPercent } from '@/utils/format'

export interface PingPoint {
  // 后台 ping-agg 返回 bucket_min，公开接口返回 timestamp，两种字段都兼容
  timestamp?: string
  bucket_min?: string
  avg_lat: number
  min_lat: number
  max_lat: number
  loss_rate: number
}

/** 取探测点时间：兼容 bucket_min / timestamp 两种后端字段 */
function pointTime(point: PingPoint): string {
  return point.timestamp || point.bucket_min || ''
}

const props = withDefaults(
  defineProps<{
    /** 原始数据：{ 线路名: 探测点[] }，空线路由调用方过滤 */
    series: Record<string, PingPoint[]>
    height?: string
    loading?: boolean
  }>(),
  { height: '320px', loading: false }
)

interface PingLine {
  name: string
  avg: number
  min: number
  max: number
  loss: number
  points: PingPoint[]
}

// 每条线路的统计摘要：min 只统计有效值（0 表示该分钟无样本，不参与最小值）
const lines = computed<PingLine[]>(() => {
  return Object.entries(props.series || {}).map(([name, points]) => {
    const avgs = points.map((p) => Number(p.avg_lat || 0))
    const mins = points.map((p) => Number(p.min_lat || 0)).filter((v) => v > 0)
    const maxs = points.map((p) => Number(p.max_lat || 0))
    const losses = points.map((p) => lossPercent(Number(p.loss_rate || 0)))
    const avg = avgs.length ? avgs.reduce((s, v) => s + v, 0) / avgs.length : 0
    return {
      name,
      avg,
      min: mins.length ? Math.min(...mins) : 0,
      max: maxs.length ? Math.max(...maxs) : 0,
      loss: losses.length ? losses.reduce((s, v) => s + v, 0) / losses.length : 0,
      points,
    }
  })
})

function colorOf(index: number): string {
  return seriesColor(readChartTokens(), index)
}

// builder 用 computed 包一层：只在数据变化时换引用，避免 WkChart 每帧无谓重画
const overlayBuilder = computed(() => () => buildOverlayOption(lines.value))

/** 叠加对比配置：多线共轴；时间点取并集对齐，缺失点填 null 让线自然断开 */
function buildOverlayOption(list: PingLine[]) {
  const tokens = readChartTokens()
  const allTimes = Array.from(
    new Set(list.flatMap((line) => line.points.map((p) => pointTime(p))))
  ).sort()
  const labels = allTimes.map((time) => formatHourMinute(time))

  // 每条线路建立"时间 → 丢包率"索引，tooltip 才能同时给出延时和丢包两个指标
  const lossIndex = new Map<string, Map<string, number>>()
  const series = list.map((line, index) => {
    const latMap = new Map<string, number>()
    const lossMap = new Map<string, number>()
    for (const point of line.points) {
      latMap.set(pointTime(point), Number(point.avg_lat || 0))
      lossMap.set(pointTime(point), lossPercent(Number(point.loss_rate || 0)))
    }
    lossIndex.set(line.name, lossMap)
    return buildLineSeries(
      line.name,
      allTimes.map((time) => (latMap.has(time) ? latMap.get(time)! : null)),
      seriesColor(tokens, index)
    )
  })

  return {
    animation: false,
    grid: baseGrid(),
    tooltip: {
      ...baseTooltip(tokens),
      axisPointer: {
        type: 'cross',
        lineStyle: { color: tokens.axis },
        crossStyle: { color: tokens.axis },
      },
      formatter: (params: any) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        let html = `<div style="font-size:11px;opacity:.7;margin-bottom:6px;font-weight:600">${params[0].axisValue}</div>`
        params.forEach((param: any, order: number) => {
          if (param.value === null || param.value === undefined) return
          const lossMap = lossIndex.get(param.seriesName)
          const lossPct = lossMap?.get(allTimes[param.dataIndex])?.toFixed(1) ?? '0.0'
          html += tooltipRow(
            seriesColor(tokens, order),
            param.seriesName,
            `${Number(param.value).toFixed(2)} ms`,
            `${lossPct}% loss`
          )
        })
        return html
      },
    },
    legend: baseLegend(tokens),
    xAxis: baseCategoryAxis(tokens, labels),
    // 不写 yAxis.name：ECharts 把轴名画在轴顶端（左上），会和同一行的图例（如“上海电信”）重叠遮挡。
    // 单位改由卡片副标题和上方统计摘要行（均 6.1ms）承载，画面更干净也不会互挡。
    yAxis: baseValueAxis(tokens, {}),
    dataZoom: baseDataZoom(tokens),
    series,
  }
}
</script>

<style scoped>
.wk-ping {
  display: flex;
  flex-direction: column;
  gap: var(--wk-space-3);
}

.wk-ping-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 6px var(--wk-space-5);
}

.wk-ping-stat {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wk-fs-sm);
  color: var(--wk-text-secondary);
}

.wk-ping-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.wk-ping-name {
  font-weight: 600;
  color: var(--wk-text);
  white-space: nowrap;
}

.wk-ping-val {
  color: var(--wk-text-secondary);
  white-space: nowrap;
}
</style>
