<template>
  <!-- ============ Ping 线路图：分线路 / 叠加对比 双模式 ============ -->
  <div class="wk-ping">
    <!-- 模式切换：默认分线路，因为延时量级不同或彼此接近时，叠加图会让线条互相盖住 -->
    <div class="wk-ping-head">
      <div class="wk-segment" role="group" aria-label="Ping 图表视图">
        <button
          type="button"
          :class="['wk-segment-item', { active: mode === 'split' }]"
          @click="setMode('split')"
        >
          分线路
        </button>
        <button
          type="button"
          :class="['wk-segment-item', { active: mode === 'overlay' }]"
          @click="setMode('overlay')"
        >
          叠加对比
        </button>
      </div>
      <span class="wk-sub">
        {{ mode === 'split' ? '每条线路独立纵轴，几毫秒的差别也看得见' : '同一纵轴便于比较尖峰与量级' }}
      </span>
    </div>

    <!-- ---------------- 分线路：每条独立 Y 轴 ---------------- -->
    <div v-if="mode === 'split'" class="wk-ping-split">
      <div v-for="(line, index) in lines" :key="line.name" class="wk-ping-item">
        <div class="wk-ping-item-head">
          <span class="wk-ping-dot" :style="{ background: colorOf(index) }" />
          <span class="wk-ping-name">{{ line.name }}</span>
          <span class="wk-ping-stats wk-num">
            均 {{ line.avg.toFixed(1) }}ms · 最低 {{ line.min.toFixed(1) }} · 最高
            {{ line.max.toFixed(1) }} · 丢包 {{ line.loss.toFixed(1) }}%
          </span>
        </div>
        <WkChart
          :builder="singleBuilders[index]"
          :deps="line.points"
          :height="singleHeight"
        />
      </div>
    </div>

    <!-- ---------------- 叠加对比：单图多线 ---------------- -->
    <WkChart v-else :builder="overlayBuilder" :deps="lines" height="320px" />
  </div>
</template>

<script setup lang="ts">
// Ping 线路图组件
// 原因：生产上上海电信 5.4ms 与上海联通 5.6ms 只差 0.2ms，在 0~60ms 同一纵轴上
//       只占约 0.4% 高度，两条线必然完全重叠、看不出第二条；对数刻度同样无解
//       （log 下差距仍不到 1%）。唯一可靠做法是给每条线路独立 Y 轴的小图阵列。
import { computed, ref } from 'vue'
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

// 统一入参：每条线路一组按时间升序的探测点
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

interface PingLine {
  name: string
  avg: number
  min: number
  max: number
  loss: number
  points: PingPoint[]
}

const props = withDefaults(
  defineProps<{
    /** 原始数据：{ 线路名: 探测点[] }，空线路由调用方过滤 */
    series: Record<string, PingPoint[]>
    /** 分线路模式下单图高度 */
    singleHeight?: string
  }>(),
  { singleHeight: '108px' }
)

const MODE_KEY = 'wk-ping-mode'
// 默认分线路：先保证"每条都看得见"，需要横向比较时再切叠加
const mode = ref<'split' | 'overlay'>(
  localStorage.getItem(MODE_KEY) === 'overlay' ? 'overlay' : 'split'
)

function setMode(next: 'split' | 'overlay') {
  mode.value = next
  localStorage.setItem(MODE_KEY, next)
}

// 每条线路的统计摘要：把数值直接摊在标题行，不依赖 tooltip 才能读到
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

// builder 必须是稳定引用：只在数据变化时重建，避免 WkChart 每帧无谓重画
const singleBuilders = computed(() =>
  lines.value.map((line, index) => () => buildSingleOption(line, index))
)

const overlayBuilder = computed(() => () => buildOverlayOption(lines.value))

/** 单条线路配置：纵轴按本线路数据自适应（min 取数据下界留 8% 余量），
 *  这样 5.4ms 与 5.6ms 的两条线各自都能看清波动 */
function buildSingleOption(line: PingLine, index: number) {
  const tokens = readChartTokens()
  const color = seriesColor(tokens, index)
  const labels = line.points.map((p) => formatHourMinute(pointTime(p)))
  const data = line.points.map((p) => Number(p.avg_lat || 0))
  const low = line.min > 0 ? line.min : Math.max(0, Math.min(...data, line.avg))
  return {
    animation: false,
    grid: { left: 4, right: 12, top: 8, bottom: 4, containLabel: true },
    tooltip: {
      ...baseTooltip(tokens),
      formatter: (params: any) => {
        const p = Array.isArray(params) ? params[0] : params
        if (!p || p.value === null || p.value === undefined) return ''
        const point = line.points[p.dataIndex]
        const loss = lossPercent(Number(point?.loss_rate || 0))
        return tooltipRow(color, line.name, `${Number(p.value).toFixed(2)} ms`, `${loss.toFixed(1)}% loss`)
      },
    },
    xAxis: { ...baseCategoryAxis(tokens, labels), axisLabel: { ...baseCategoryAxis(tokens, labels).axisLabel, fontSize: 10 } },
    yAxis: {
      ...baseValueAxis(tokens, {}),
      // 关键：不从 0 开始，否则线条永远贴着底部、看不出差异
      min: Math.max(0, Math.floor(low * 0.92 * 10) / 10),
      scale: true,
    },
    series: [buildLineSeries(line.name, data, color, { area: true, width: 1.4 })],
  }
}

/** 叠加对比配置：多线共轴，用于横向比较量级与尖峰 */
function buildOverlayOption(list: PingLine[]) {
  const tokens = readChartTokens()
  // 时间点取并集后排序，保证多条线路在同一 x 位置上对齐
  const allTimes = Array.from(
    new Set(list.flatMap((line) => line.points.map((p) => pointTime(p))))
  ).sort()
  const labels = allTimes.map((time) => formatHourMinute(time))

  const lossIndex = new Map<string, Map<string, number>>()
  const series = list.map((line, index) => {
    const latMap = new Map<string, number>()
    const lossMap = new Map<string, number>()
    for (const point of line.points) {
      latMap.set(pointTime(point), Number(point.avg_lat || 0))
      lossMap.set(pointTime(point), lossPercent(Number(point.loss_rate || 0)))
    }
    lossIndex.set(line.name, lossMap)
    return {
      ...buildLineSeries(
        line.name,
        allTimes.map((time) => (latMap.has(time) ? latMap.get(time)! : null)),
        seriesColor(tokens, index)
      ),
    }
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
    yAxis: baseValueAxis(tokens, { name: 'ms' }),
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

.wk-ping-head {
  display: flex;
  align-items: center;
  gap: var(--wk-space-3);
  flex-wrap: wrap;
}

.wk-ping-split {
  display: flex;
  flex-direction: column;
  gap: var(--wk-space-4);
}

.wk-ping-item-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 2px;
  min-width: 0;
}

.wk-ping-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.wk-ping-name {
  font-size: var(--wk-fs-base);
  font-weight: 600;
  color: var(--wk-text);
  white-space: nowrap;
}

/* 统计摘要靠右，窄屏换行到第二行也不遮挡图 */
.wk-ping-stats {
  margin-left: auto;
  font-size: var(--wk-fs-sm);
  color: var(--wk-text-muted);
  white-space: nowrap;
}

@media (max-width: 860px) {
  .wk-ping-stats {
    margin-left: 16px;
    white-space: normal;
  }
}
</style>
