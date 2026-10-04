<template>
  <!-- 骨架屏块：加载占位，宽度/高度/圆角都可覆盖，默认填满父容器一行 -->
  <span
    class="wk-skeleton"
    :style="{
      width: width,
      height: height,
      borderRadius: rounded,
      display: block ? 'block' : 'inline-block',
    }"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
// 骨架屏组件
// 原因：旧代码到处写 style="width:80px;height:12px" 的魔法数，
//       统一成组件后各页占位尺寸口径一致，改一次全局即可。
withDefaults(
  defineProps<{
    /** 宽度，默认 100% */
    width?: string
    /** 高度，默认 12px */
    height?: string
    /** 圆角，默认使用小圆角令牌 */
    rounded?: string
    /** 是否块级显示 */
    block?: boolean
  }>(),
  {
    width: "100%",
    height: "12px",
    rounded: "var(--wk-radius-xs)",
    block: true,
  }
)
</script>
