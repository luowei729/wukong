<template>
  <!-- 徽章：状态药丸 / 版本标签 / ISP 标签的统一外观 -->
  <span :class="['wk-badge', toneClass]">
    <WkStatusDot v-if="dot" :status="dotStatus" :size="6" />
    <slot>{{ text }}</slot>
  </span>
</template>

<script setup lang="ts">
// 徽章组件
// 原因：旧代码在页面里手写 <span class="wk-badge ok"> 且要自己配状态灯，
//       这里把"语气色 + 可选状态灯"收敛成一个 props 契约，避免各页配色不一致。
import { computed } from "vue"
import WkStatusDot from "./WkStatusDot.vue"

type Tone = "ok" | "warn" | "fail" | "info" | "neutral" | "primary"
type DotStatus = "online" | "offline" | "stale" | "alert" | "muted"

const props = withDefaults(
  defineProps<{
    /** 语气色，仅表达状态语义 */
    tone?: Tone
    /** 文本（也可用默认插槽传富文本） */
    text?: string
    /** 是否在前面显示状态灯 */
    dot?: boolean
    /** 状态灯语义，未传时按 tone 推导 */
    dotStatus?: DotStatus
  }>(),
  { tone: "neutral", text: "", dot: false, dotStatus: undefined }
)

const toneClass = computed(() => props.tone)

// tone → 状态灯的默认语义映射，调用方只写 tone 也能得到正确的灯
const dotStatus = computed<DotStatus>(() => {
  if (props.dotStatus) return props.dotStatus
  if (props.tone === "ok") return "online"
  if (props.tone === "fail") return "alert"
  if (props.tone === "warn") return "stale"
  return "muted"
})
</script>

<style scoped>
/* 徽章内的状态灯不需要额外右边距，由 gap 统一控制 */
:deep(.wk-status-dot) {
  margin-right: 0;
}
</style>
