<template>
  <!-- 主布局：侧边栏 + 顶部栏 + 内容区 -->
  <div class="wk-layout">
    <!-- 侧边栏 -->
    <aside class="wk-sidebar">
      <div class="wk-sidebar-logo">
        <span class="logo-text">🐒 wukong</span>
      </div>
      <ul class="wk-sidebar-menu">
        <li
          v-for="item in menuItems"
          :key="item.path"
          :class="{ active: currentPath === item.path }"
          @click="navigate(item.path)"
        >
          <el-icon>
            <component :is="item.icon" />
          </el-icon>
          <span>{{ item.label }}</span>
        </li>
      </ul>
    </aside>

    <!-- 主内容 -->
    <div class="wk-main">
      <!-- 顶部栏 -->
      <header class="wk-header">
        <div class="wk-header-left">
          <span class="wk-header-title">{{ currentTitle }}</span>
        </div>
        <div class="wk-header-right">
          <el-button
            :icon="themeIcon"
            circle
            size="small"
            @click="toggleTheme"
          />
          <el-button text>
            <el-icon><User /></el-icon>
          </el-button>
        </div>
      </header>

      <!-- 内容区 -->
      <main class="wk-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  Odometer, Monitor, WarningFilled, Setting, Moon, Sunny, User,
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

// 菜单项
const menuItems = [
  { path: '/dashboard', label: '总览', icon: 'Odometer' },
  { path: '/nodes', label: '节点列表', icon: 'Monitor' },
  { path: '/alerts', label: '告警中心', icon: 'WarningFilled' },
  { path: '/settings', label: '系统设置', icon: 'Setting' },
]

const currentPath = computed(() => route.path)
const currentTitle = computed(() => route.meta.title as string || 'wukong')

// 主题切换
const isDark = computed(() => document.documentElement.dataset.theme === 'dark')
const themeIcon = computed(() => isDark.value ? Sunny : Moon)

function toggleTheme() {
  const newTheme = isDark.value ? 'light' : 'dark'
  document.documentElement.dataset.theme = newTheme
  document.documentElement.classList.toggle('dark', newTheme === 'dark')
  localStorage.setItem('theme', newTheme)
}

// 初始化主题
const savedTheme = localStorage.getItem('theme') || 'dark'
document.documentElement.dataset.theme = savedTheme
document.documentElement.classList.toggle('dark', savedTheme === 'dark')

function navigate(path: string) {
  router.push(path)
}
</script>

<style scoped>
.wk-layout {
  display: flex;
  min-height: 100vh;
}

.wk-header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.wk-header-title {
  font-size: 16px;
  font-weight: 600;
}

.wk-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>