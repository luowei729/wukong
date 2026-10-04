<template>
  <!-- 空状态：图标 + 标题 + 说明 + 可选操作 -->
  <div class="wk-empty">
    <div :class="['wk-empty-icon', okState ? 'is-ok' : '']">
      <!-- 内联 SVG 图标：不引第三方图标库，也不使用 emoji，保证公开页观感专业 -->
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <template v-if="icon === 'shield'">
          <path d="M12 2L4 6v6c0 5 3.5 9.5 8 10 4.5-.5 8-5 8-10V6l-8-4z" />
          <path d="M9 12l2 2 4-4" />
        </template>
        <template v-else-if="icon === 'bell'">
          <path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9" />
          <path d="M10 21a2 2 0 0 0 4 0" />
        </template>
        <template v-else-if="icon === 'search'">
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-3.5-3.5" />
        </template>
        <template v-else-if="icon === 'server'">
          <rect x="2" y="3" width="20" height="8" rx="2" />
          <rect x="2" y="13" width="20" height="8" rx="2" />
          <path d="M6 7h.01M6 17h.01" />
        </template>
        <template v-else>
          <path d="M21 8l-9-5-9 5 9 5 9-5z" />
          <path d="M3 8v8l9 5 9-5V8" />
          <path d="M12 13v8" />
        </template>
      </svg>
    </div>

    <div class="wk-empty-title">{{ title }}</div>
    <div v-if="description" class="wk-empty-desc">{{ description }}</div>

    <!-- 操作区：由调用方决定是否给出跳转/新建按钮 -->
    <div v-if="$slots.action" style="margin-top: 12px">
      <slot name="action" />
    </div>
  </div>
</template>

<script setup lang="ts">
// 空状态组件
// 原因：旧代码混用 el-empty 与手写 emoji 空态（公开首页出现过 📭），
//       统一成本组件后空态有语义图标与一致排版，"全部正常"还能显示绿色盾牌。
import { computed } from "vue"

type EmptyIcon = "box" | "shield" | "bell" | "search" | "server"

const props = withDefaults(
  defineProps<{
    /** 标题（必填） */
    title: string
    /** 补充说明 */
    description?: string
    /** 图标语义 */
    icon?: EmptyIcon
    /** 是否"一切正常"的积极空态（如告警中心无告警），图标转绿色 */
    ok?: boolean
  }>(),
  { description: "", icon: "box", ok: false }
)

// 图标与积极态由 props 直接映射，保留 computed 以便后续按需扩展默认语义
const icon = computed<EmptyIcon>(() => props.icon)
const okState = computed(() => props.ok)
</script>
