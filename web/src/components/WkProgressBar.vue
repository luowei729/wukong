<template>
  <!-- 指标条：标签 + 极细进度条 + 数值，CPU/内存/磁盘等使用率统一用它 -->
  <div class="wk-meter" :class="`wk-meter--thick-${thick}`">
    <span v-if="label" class="wk-meter-label">{{ label }}</span>
    <div
      class="wk-meter-track"
      role="progressbar"
      :aria-valuenow="clamped"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-label="label || '使用率'"
    >
      <div :class="['wk-meter-fill', levelClass]" :style="{ width: `${clamped}%` }" />
    </div>
    <span v-if="showValue" :class="['wk-meter-value', `is-${grade}`]">{{ displayValue }}</span>
    <span v-else-if="hint" class="wk-sub">{{ hint }}</span>
  </div>
</template>

<script setup lang="ts">
// 指标条组件
// 原因：旧代码 Dashboard 用手写 progress-bar、PublicHome 用 el-progress，
//       同样的"使用率"出现两种粗细与配色口径；这里统一成 4px 极细条 + 分级色。
import { computed } from "vue"
import { clampPercent, loadLevel, formatPercent, type LoadLevel } from "@/utils/format"

const props = withDefaults(
  defineProps<{
    /** 使用率数值（0~100），非数字显示为 0 宽 */
    value?: number | null
    /** 左侧标签，例如 CPU / 内存 / 磁盘 */
    label?: string
    /** 右侧提示（show-value=false 时生效） */
    hint?: string
    /** 分级策略：auto 按负载阈值变色，none 恒为主色 */
    level?: "auto" | "none"
    /** 进度条高度 */
    thick?: 4 | 6 | 8
    /** 是否显示右侧百分比数值 */
    showValue?: boolean
  }>(),
  {
    value: null,
    label: "",
    hint: "",
    level: "auto",
    thick: 4,
    showValue: true,
  }
)

const clamped = computed(() => clampPercent(props.value))

// auto 模式下按统一阈值（≥85 危险 / ≥70 警告）着色，全局口径一致
// 变量名避开 props.level，否则模板作用域内会相互覆盖
const grade = computed<LoadLevel>(() => {
  if (props.level === "none") return "normal"
  return loadLevel(props.value)
})

const levelClass = computed(() =>
  props.level === "none" ? "" : `is-${grade.value}`
)

const displayValue = computed(() => formatPercent(props.value))
</script>
