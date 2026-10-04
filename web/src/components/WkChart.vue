<template>
  <!-- 图表容器：ECharts 在本系统的唯一出口 -->
  <div v-loading="loading" class="wk-chart-wrap" :style="{ height }">
    <div ref="chartEl" class="wk-chart-canvas" />
  </div>
</template>

<script setup lang="ts">
// ECharts 封装组件
// 原因：旧代码每个图表页都要重复写 init/dispose/resize + 手写配色，
//       并且把 CSS 变量直接当颜色传给 canvas（不生效）、主题写死 'dark'（切浅色不变色）。
// 方案：组件内部把 option 的生成延迟到"渲染时刻"，用 builder 函数 + 主题版本号触发重建，
//       保证深浅色切换、自定义主色变更时图表颜色一定跟随，且父组件不必关心生命周期。
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue"
import * as echarts from "echarts"
import { ensureWukongTheme, readChartTokens } from "@/utils/charts"
import { useTheme } from "@/composables/useTheme"

const props = withDefaults(
  defineProps<{
    /** 配置生成函数：每次渲染都会带上当前解析后的主题色重新调用 */
    builder: () => Record<string, any>
    /** 数据依赖：变化时重画（浅比较引用即可，避免每秒深拷贝大数据） */
    deps?: unknown
    /** 容器高度 */
    height?: string
    /** 加载态遮罩 */
    loading?: boolean
    /** 是否以“不合并”方式重建配置（series 数量会变时必须为 true） */
    notMerge?: boolean
  }>(),
  { deps: undefined, height: "320px", loading: false, notMerge: true }
)

const chartEl = ref<HTMLDivElement>()
// shallowRef：ECharts 实例不需要深度响应式，避免 Vue 递归代理带来的开销与异常
const chart = shallowRef<echarts.ECharts | null>(null)

// 主题版本号：preset 或 primary 变化时递增，用于触发图表重建
const { themeVersion } = useTheme()

/** 用当前主题令牌生成并应用配置 */
function render() {
  if (!chart.value) return
  const option = props.builder()
  // notMerge 保证 series 数量变化（例如增删运营商线路）时不残留旧线
  chart.value.setOption(option, { notMerge: props.notMerge, lazyUpdate: true })
}

/** 初始化实例：主题名由令牌注册，不再写死 'dark' */
function init() {
  if (!chartEl.value) return
  const theme = ensureWukongTheme()
  chart.value = echarts.init(chartEl.value, theme, { renderer: "canvas" })
  render()
}

// 容器尺寸变化自适应（侧栏折叠、断点切换、窗口缩放都会触发）
let observer: ResizeObserver | null = null

onMounted(() => {
  init()
  if (chartEl.value && typeof ResizeObserver !== "undefined") {
    observer = new ResizeObserver(() => chart.value?.resize())
    observer.observe(chartEl.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
  // 必须显式 dispose，否则页面切换后 canvas 与事件监听残留导致内存泄漏
  chart.value?.dispose()
  chart.value = null
})

// 数据变化 → 重画
watch(
  () => props.deps,
  () => render(),
  { deep: false }
)

// builder 换函数（例如切换统计口径）→ 重画
watch(() => props.builder, () => render())

// 主题/自定义主色变化 → 重新注册主题并重建实例配置
watch(themeVersion, () => {
  // 令牌已变，先让注册逻辑感知新色值，再整体重设 option
  readChartTokens()
  ensureWukongTheme()
  render()
})

defineExpose({ render, resize: () => chart.value?.resize() })
</script>

<style scoped>
/* canvas 需要显式尺寸，父级高度由 height prop 控制 */
.wk-chart-canvas {
  width: 100%;
  height: 100%;
}
</style>
