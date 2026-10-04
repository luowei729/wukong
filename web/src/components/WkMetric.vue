<template>
  <!-- KPI 指标卡：标签 + 大数字 + 次要说明 + 可选迷你趋势 -->
  <div :class="['wk-metric', tone ? `wk-metric--${tone}` : '', hasSpark ? 'has-spark' : '']">
    <div class="wk-metric-label">
      <slot name="label">{{ label }}</slot>
    </div>

    <!-- 加载态复用骨架屏，尺寸与真实数值一致，避免加载完成时布局跳动 -->
    <div v-if="loading" class="wk-metric-value">
      <WkSkeleton width="72px" height="26px" rounded="var(--wk-radius-xs)" />
    </div>
    <div v-else class="wk-metric-value">
      {{ displayValue }}
      <span v-if="unit" class="wk-metric-unit">{{ unit }}</span>
    </div>

    <div v-if="delta || hint" class="wk-metric-hint">
      <span v-if="delta" :class="['wk-badge', deltaToneClass]">{{ delta }}</span>
      <span v-if="hint" class="wk-sub">{{ hint }}</span>
    </div>

    <!-- 迷你趋势固定在右下，不参与文本流，卡片高度保持恒定 -->
    <div v-if="spark && spark.length > 1" class="wk-metric-spark">
      <WkSparkline :points="spark" :color="sparkColor || autoSparkColor" :max="sparkMax" />
    </div>

    <slot name="extra" />
  </div>
</template>

<script setup lang="ts">
// KPI 指标卡组件
// 原因：Dashboard / PublicHome / NodeDetail 都需要同一种"标签+大数+说明"的卡，
//       旧代码是三份结构略异的 HTML（其中 .value/.sub/.label 命名还不一致）；
//       统一后数值全部等宽 tabular-nums，每秒刷新不会左右抖动。
import { computed } from "vue"
import WkSparkline from "./WkSparkline.vue"
import WkSkeleton from "./WkSkeleton.vue"

type Tone = "default" | "success" | "warning" | "danger" | "primary"

const props = withDefaults(
  defineProps<{
    /** 顶部小标签（大写字距样式） */
    label?: string
    /** 主数值，字符串或数字（数字保留原样，由调用方决定精度） */
    value?: string | number | null
    /** 单位，跟在数值后弱化显示 */
    unit?: string
    /** 次要说明 */
    hint?: string
    /** 语气色：仅用于表达状态语义，不做装饰 */
    tone?: Tone
    /** 加载态 */
    loading?: boolean
    /** 迷你趋势数据 */
    spark?: number[]
    /** 迷你趋势颜色，默认跟随 tone */
    sparkColor?: string
    /** 迷你趋势上界（百分比类固定 100） */
    sparkMax?: number
    /** 变化摘要，显示为小徽章 */
    delta?: string
    /** 变化摘要的语气，positive 绿 / negative 红 */
    deltaTone?: "ok" | "fail" | "neutral"
  }>(),
  {
    label: "",
    value: null,
    unit: "",
    hint: "",
    tone: "default",
    loading: false,
    spark: undefined,
    sparkColor: "",
    sparkMax: undefined,
    delta: "",
    deltaTone: "neutral",
  }
)

// 数值为空时统一显示短横线，避免卡片出现空白或 NaN
const displayValue = computed(() => {
  const value = props.value
  if (value === null || value === undefined || value === "") return "-"
  return String(value)
})

// tone → sparkline 默认色，让"内存告警红"这类卡片无需手动传色值
const autoSparkColor = computed(() => {
  if (props.tone === "danger") return "var(--wk-danger)"
  if (props.tone === "warning") return "var(--wk-warning)"
  if (props.tone === "success") return "var(--wk-success)"
  return "var(--wk-primary)"
})

const deltaToneClass = computed(() =>
  props.deltaTone === "ok" ? "ok" : props.deltaTone === "fail" ? "fail" : "neutral"
)

// 是否渲染了迷你曲线（决定下方提示文字要不要给右下角让位）
const hasSpark = computed(() => Array.isArray(props.spark) && props.spark.length > 1)
</script>

<style scoped>
/* 覆盖值容器：骨架屏与真实值共用同一基线，切换时无位移 */
.wk-metric-value {
  min-height: 30px;
}

/* 有 sparkline 时提示文字给右下角让位，避免长文案与曲线重叠 */
.wk-metric.has-spark .wk-metric-hint {
  padding-right: 104px;
}
</style>
