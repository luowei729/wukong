<template>
  <!-- ============ 丢包时间轴：色条 + 时间刻度 + 丢包时段汇总 ============ -->
  <div class="wk-loss" :style="gridStyle">
    <!-- 时间刻度：所有线路共用同一时间轴，所以只画一行，放在色条上方 -->
    <span />
    <div class="wk-loss-axis">
      <span
        v-for="tick in ticks"
        :key="tick.pos"
        class="wk-loss-tick"
        :style="{ left: tick.pos + '%', transform: tick.shift }"
      >
        {{ tick.label }}
      </span>
    </div>
    <span />

    <!-- 每条线路一行色条 -->
    <template v-for="row in rows" :key="row.name">
      <span class="wk-loss-label" :title="row.name">{{ row.name }}</span>
      <div class="wk-strip">
        <span
          v-for="(cell, index) in row.cells"
          :key="index"
          :class="['wk-strip-cell', `is-${cell.kind}`]"
          :title="cellTip(row.name, cell)"
        />
      </div>
      <span class="wk-loss-value wk-num">
        丢 {{ row.loss.toFixed(1) }}%
        <span v-if="row.peak > 0" class="wk-loss-peak">峰 {{ Math.round(row.peak) }}%</span>
      </span>
    </template>

    <!-- 丢包时段汇总：把连续丢包的格子合并成区间，直接回答"什么时候丢的包"，
         不必一格一格去 hover -->
    <div v-if="rows.length" class="wk-loss-ranges">
      <div v-for="row in rows" :key="row.name" class="wk-loss-range-row">
        <span class="wk-loss-label" :title="row.name">{{ row.name }}</span>
        <div class="wk-loss-range-list">
          <span v-if="row.ranges.length === 0" class="wk-sub">无丢包时段</span>
          <span
            v-for="(range, index) in visibleRanges(row)"
            :key="index"
            :class="['wk-loss-range', `is-${range.kind}`]"
            :title="rangeTip(row.name, range)"
          >
            {{ rangeLabel(range) }}
            <span class="wk-loss-range-peak">峰 {{ Math.round(range.peak) }}%</span>
          </span>
          <span v-if="row.ranges.length > maxRanges" class="wk-sub">
            另有 {{ row.ranges.length - maxRanges }} 段
          </span>
        </div>
        <span />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 丢包时间轴组件
// 原因：旧版色条 hover 只显示"第 105 段：部分丢包"，既不知道对应几点，也没法快速定位
// 到底是哪一段时间在丢包。这里补齐三件事：
//   1) 每格 title 给出精确起止时间 + 平均/峰值丢包 + 采样点数
//   2) 色条上方加统一时间刻度（数据分桶已按全局时间域对齐，见 utils/ping.ts）
//   3) 下方列出合并后的丢包时段，一眼看出"什么时候丢的"
import { computed } from 'vue'
import { buildLossRows, type LossCell, type LossRange, type LossRow, type PingPoint } from '@/utils/ping'
import { formatHourMinute } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    /** { 线路名: 探测点[] } */
    series: Record<string, PingPoint[]>
    /** 色条格数，越多越细但每格越窄 */
    cellCount?: number
    /** 每条线路最多列出几个丢包时段 */
    maxRanges?: number
    /** 左侧线路名列宽，需与后台其他区块对齐时由调用方覆盖 */
    labelWidth?: string
    /** 右侧数值列宽 */
    valueWidth?: string
  }>(),
  { cellCount: 120, maxRanges: 6, labelWidth: '108px', valueWidth: '96px' }
)

const rows = computed<LossRow[]>(() => buildLossRows(props.series, props.cellCount))

// 三列共用一个 grid 模板，保证刻度轴与各色条行的中间列严格对齐
const gridStyle = computed(() => ({
  '--wk-loss-label-w': props.labelWidth,
  '--wk-loss-value-w': props.valueWidth,
}))

// 刻度取 0/25/50/75/100% 五个位置；两端分别左右对齐，避免文字溢出容器
const ticks = computed(() => {
  const cells = rows.value.find((row) => row.cells.length > 0)?.cells || []
  if (cells.length === 0) return []
  const start = cells[0].from
  const end = cells[cells.length - 1].to
  return [0, 25, 50, 75, 100].map((pos) => ({
    pos,
    shift: pos === 0 ? 'none' : pos === 100 ? 'translateX(-100%)' : 'translateX(-50%)',
    label: formatHourMinute(start + ((end - start) * pos) / 100),
  }))
})

function cellTip(name: string, cell: LossCell): string {
  const range = `${formatHourMinute(cell.from)}–${formatHourMinute(cell.to)}`
  if (cell.kind === 'none') return `${name} ${range}｜该时段无探测数据`
  return (
    `${name} ${range}｜${kindLabel(cell.kind)}｜` +
    `平均丢包 ${cell.avg.toFixed(1)}% · 峰值 ${Math.round(cell.peak)}%｜${cell.samples} 个采样点`
  )
}

function kindLabel(kind: string): string {
  if (kind === 'ok') return '无丢包'
  if (kind === 'warn') return '部分丢包'
  if (kind === 'bad') return '严重丢包'
  return '无数据'
}

function rangeLabel(range: LossRange): string {
  return `${formatHourMinute(range.from)}–${formatHourMinute(range.to)}`
}

function rangeTip(name: string, range: LossRange): string {
  return (
    `${name} ${rangeLabel(range)}｜${kindLabel(range.kind)}｜` +
    `峰值丢包 ${Math.round(range.peak)}%｜${range.samples} 个采样点`
  )
}

// 时段很多时优先显示最严重的几段（严重丢包优先、其次按峰值降序），
// 而不是简单取前 N 个 —— 用户关心的是"最糟的那几次发生在什么时候"
function visibleRanges(row: LossRow): LossRange[] {
  if (row.ranges.length <= props.maxRanges) return row.ranges
  return row.ranges
    .slice()
    .sort((a, b) => {
      if (a.kind !== b.kind) return a.kind === 'bad' ? -1 : 1
      return b.peak - a.peak
    })
    .slice(0, props.maxRanges)
    .sort((a, b) => a.from - b.from)
}
</script>

<style scoped>
.wk-loss {
  display: grid;
  grid-template-columns: var(--wk-loss-label-w, 108px) minmax(0, 1fr) var(--wk-loss-value-w, 96px);
  align-items: center;
  column-gap: var(--wk-space-3);
  row-gap: 7px;
}

/* 时间刻度轴：留出与色条等高的底部空隙，刻度文字压在色条上方 */
.wk-loss-axis {
  position: relative;
  height: 14px;
}

.wk-loss-tick {
  position: absolute;
  top: 0;
  font-family: var(--wk-mono-family);
  font-variant-numeric: tabular-nums;
  font-size: var(--wk-fs-xs);
  color: var(--wk-text-muted);
  white-space: nowrap;
}

.wk-loss-label {
  min-width: 0;
  font-size: var(--wk-fs-sm);
  color: var(--wk-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 右侧同时给出平均与峰值：平均 0.2% 也可能藏着一次 100% 丢包 */
.wk-loss-value {
  font-size: var(--wk-fs-sm);
  color: var(--wk-text-secondary);
  text-align: right;
  white-space: nowrap;
}

.wk-loss-peak {
  margin-left: 4px;
  color: var(--wk-text-muted);
}

/* 丢包时段汇总区：与上方色条用一条 hairline 分隔 */
.wk-loss-ranges {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-top: var(--wk-space-2);
  padding-top: var(--wk-space-3);
  border-top: 1px solid var(--wk-border);
}

.wk-loss-range-row {
  display: contents;
}

.wk-loss-range-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 6px;
  min-width: 0;
}

.wk-loss-range {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 6px;
  border-radius: var(--wk-radius-xs);
  font-family: var(--wk-mono-family);
  font-variant-numeric: tabular-nums;
  font-size: var(--wk-fs-xs);
  /* 用丢包语气的浅色底，与色条颜色语义一致 */
  background: color-mix(in srgb, var(--wk-warning) 16%, transparent);
  color: var(--wk-warning-text);
}

.wk-loss-range.is-bad {
  background: color-mix(in srgb, var(--wk-danger) 16%, transparent);
  color: var(--wk-danger-text);
}

.wk-loss-range-peak {
  opacity: 0.75;
}

@media (max-width: 640px) {
  /* 窄屏把时段挪到色条下一行，否则三列挤在一起谁都不够宽 */
  .wk-loss {
    grid-template-columns: 72px minmax(0, 1fr) 56px;
    column-gap: var(--wk-space-2);
  }
}
</style>
