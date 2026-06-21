<template>
  <!-- wukong 监控根组件 -->
  <router-view />
</template>

<script setup lang="ts">
// App.vue - 根组件，仅承载路由视图
</script>

<style>
/* 全局基础样式 */
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html {
  font-family: 'Inter', 'PingFang SC', 'Microsoft YaHei', -apple-system, sans-serif;
  -webkit-font-smoothing: antialiased;
}

body {
  background: var(--wk-bg);
  color: var(--wk-text);
  min-height: 100vh;
}

a {
  color: var(--wk-primary);
  text-decoration: none;
}
</style>