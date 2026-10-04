<template>
  <!-- 卡片容器：全系统唯一的区块外壳（标题 + 副标题 + 操作区 + 内容 + 页脚） -->
  <section :class="['wk-card', `wk-card--pad-${padClass}`]" :aria-label="title || undefined">
    <!-- 头部：只有在有标题/副标题/操作区时才渲染，避免空头部占位 -->
    <header
      v-if="title || subtitle || $slots.actions"
      class="wk-card-head"
    >
      <div class="wk-card-head-text">
        <h3 v-if="title" class="wk-card-title">{{ title }}</h3>
        <p v-if="subtitle" class="wk-card-sub">{{ subtitle }}</p>
      </div>
      <div v-if="$slots.actions" class="wk-card-actions">
        <slot name="actions" />
      </div>
    </header>

    <div class="wk-card-body">
      <slot />
    </div>

    <footer v-if="$slots.footer" class="wk-card-foot">
      <slot name="footer" />
    </footer>
  </section>
</template>

<script setup lang="ts">
// 卡片组件
// 原因：旧页面里 8 个文件各自用 .wk-card-solid + scoped 覆写 padding/margin，
//       卡片内边距出现 16/18/20/22/24 五种值；抽出本组件后由 padding prop 统一三档。
import { computed } from "vue"

const props = withDefaults(
  defineProps<{
    /** 卡片标题 */
    title?: string
    /** 副标题：一句话说明这块区域的数据来源或口径 */
    subtitle?: string
    /** 内边距档位：none 用于表格/图表贴边，sm 紧凑，md 默认 */
    padding?: "none" | "sm" | "md"
  }>(),
  { title: "", subtitle: "", padding: "md" }
)

// none 档不加 padding，其余映射到全局令牌类
const padClass = computed(() => props.padding)
</script>

<style scoped>
/* 标题区左侧文本块允许收缩，长标题走省略号而不是撑破布局 */
.wk-card-head-text {
  min-width: 0;
}

/* padding=none 时头部与页脚自行补回内边距，保证与内容对齐 */
.wk-card--pad-none .wk-card-head,
.wk-card--pad-none .wk-card-foot {
  padding: var(--wk-space-4);
}

.wk-card--pad-none .wk-card-head + .wk-card-body {
  margin-top: 0;
}
</style>
