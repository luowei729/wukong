<template>
  <!-- 迷你曲线：纯 SVG 实现，每秒刷新也只重绘一条 path，不进 ECharts 避免 canvas 开销 -->
  <svg
    v-if="path.length > 0"
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    class="wk-sparkline"
    aria-hidden="true"
    preserveAspectRatio="none"
  >
    <!-- 面积填充：折线下方闭合到基线，透明度极低只做衬托 -->
    <path
      v-if="area"
      :d="areaPath"
      :fill="strokeColor"
      fill-opacity="0.14"
      stroke="none"
    />
    <!-- 折线本体：用 CSS 变量描边，自动跟随主题与自定义主色 -->
    <path
      :d="path"
      fill="none"
      :stroke="strokeColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
      vector-effect="non-scaling-stroke"
    />
    <!-- 末点强调：让"当前值"在视觉上与曲线末端对齐 -->
    <circle v-if="lastPoint" :cx="lastPoint.x" :cy="lastPoint.y" r="1.8" :fill="strokeColor" />
  </svg>
</template>

<script setup lang="ts">
// 迷你曲线组件
// 原因：KPI 卡需要"最近趋势"的直观表达，但为每张卡建 ECharts 实例会明显拖慢每秒刷新；
//       SVG path 计算量极小，且能直接用 CSS 变量跟随主题，无需解析色值。
import { computed } from "vue"

const props = withDefaults(
  defineProps<{
    /** 数据点序列（按时间正序） */
    points: number[]
    /** 画布宽度 */
    width?: number
    /** 画布高度 */
    height?: number
    /** 手动指定上界（例如百分比固定 100），不传则按数据自适应 */
    max?: number
    /** 手动指定下界，默认按数据最小值 */
    min?: number
    /** 线色，默认主色令牌 */
    color?: string
    /** 是否绘制面积填充 */
    area?: boolean
  }>(),
  {
    width: 96,
    height: 28,
    max: undefined,
    min: undefined,
    color: "var(--wk-primary)",
    area: true,
  }
)

const strokeColor = computed(() => props.color)

// 计算映射后的坐标：上下各留 2px 余量，避免线贴边被裁切
const coords = computed(() => {
  const pts = props.points || []
  if (pts.length < 2) return []
  const dataMax = Math.max(...pts)
  const dataMin = Math.min(...pts)
  const max = props.max ?? dataMax
  const min = props.min ?? dataMin
  // 全程等值时用一个中性跨度，避免除零导致所有点堆在一条线上
  const span = max - min === 0 ? Math.max(1, Math.abs(max) * 0.1) : max - min
  const innerHeight = props.height - 4
  const stepX = props.width / (pts.length - 1)
  return pts.map((value, index) => ({
    x: index * stepX,
    y: 2 + innerHeight - ((value - min) / span) * innerHeight,
  }))
})

// 折线路径
const path = computed(() =>
  coords.value
    .map((point, index) => `${index === 0 ? "M" : "L"}${point.x.toFixed(2)},${point.y.toFixed(2)}`)
    .join(" ")
)

// 闭合到基线的面积路径
const areaPath = computed(() => {
  if (path.value.length === 0) return ""
  const last = coords.value[coords.value.length - 1]
  const first = coords.value[0]
  return `${path.value} L${last.x.toFixed(2)},${props.height} L${first.x.toFixed(2)},${props.height} Z`
})

// 末点坐标（用于高亮圆点）
const lastPoint = computed(() =>
  coords.value.length > 0 ? coords.value[coords.value.length - 1] : null
)
</script>

<style scoped>
.wk-sparkline {
  display: block;
  overflow: visible;
}
</style>
