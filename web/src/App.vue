<template>
  <!-- wukong 监控根组件：只承载路由视图，全局样式统一由 styles/index.scss 提供 -->
  <router-view />
</template>

<script setup lang="ts">
// App.vue - 根组件
// 说明：此前这里有一份与 styles/base.scss 重复的全局 reset，且引用了未加载的 'Inter' 字体，
//       导致后台与公开页的字体口径不一致；现在删除，样式单一来源为 styles/ 目录。
</script>
