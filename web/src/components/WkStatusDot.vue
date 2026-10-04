<template>
  <!-- 状态灯：唯一的状态可视化出口，尺寸/颜色/是否呼吸都由 props 决定 -->
  <span
    :class="['wk-status-dot', `wk-size-${size}`, pulse ? 'is-pulse' : '', statusClass]"
    :title="title"
    :aria-label="ariaLabel || status"
    role="img"
  />
</template>

<script setup lang="ts">
// 状态灯组件
// 原因：原来每个页面各自写 <span class="wk-status-dot online">，尺寸与语义不统一，
//       且离线/数据延迟两种状态被混用成灰色，看不出区别。
import { computed } from "vue"

type DotStatus = "online" | "offline" | "stale" | "alert" | "muted"

const props = withDefaults(
  defineProps<{
    /** 状态语义：在线 / 离线 / 数据延迟 / 告警 / 中性 */
    status?: DotStatus
    /** 是否显示呼吸光晕（只建议用在在线与告警，避免满屏闪烁） */
    pulse?: boolean
    /** 直径像素：6 用于密集表格行，8 默认，10 用于 Hero 区 */
    size?: 6 | 8 | 10
    /** 悬浮提示文本 */
    title?: string
    /** 无障碍标签，默认按状态生成 */
    ariaLabel?: string
  }>(),
  {
    status: "muted",
    pulse: false,
    size: 8,
    title: "",
    ariaLabel: "",
  }
)

// 状态到样式类的映射（online/alert 等类名在 components.scss 已定义）
const statusClass = computed(() => props.status)
</script>

<style scoped>
/* 尺寸变体：只用局部覆盖，颜色仍由全局令牌类提供 */
.wk-size-6 {
  width: 6px;
  height: 6px;
}
.wk-size-8 {
  width: 8px;
  height: 8px;
}
.wk-size-10 {
  width: 10px;
  height: 10px;
}
</style>
