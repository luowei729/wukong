<template>
  <!-- 主布局：顶部导航 + 全屏内容区 -->
  <div class="wk-layout">
    <!-- 顶部导航栏 -->
    <header class="wk-topbar">
      <div class="wk-topbar-left">
        <span class="logo-text">🐒 wukong</span>
        <nav class="wk-nav">
          <div
            v-for="item in menuItems"
            :key="item.path"
            :class="['wk-nav-item', { active: currentPath === item.path }]"
            @click="navigate(item.path)"
          >
            <el-icon>
              <component :is="item.icon" />
            </el-icon>
            <span>{{ item.label }}</span>
          </div>
        </nav>
      </div>
      <div class="wk-topbar-right">
        <el-button
          :icon="themeIcon"
          circle
          size="small"
          @click="toggleTheme"
        />
        <el-button text @click="handleLogout" title="退出登录">
          <el-icon><SwitchButton /></el-icon>
        </el-button>
      </div>
    </header>

    <!-- 全屏内容区 -->
    <main class="wk-content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  Odometer, Monitor, WarningFilled, Setting, Moon, Sunny, SwitchButton,
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

// 登出功能：清除本地 JWT 并跳转登录页。
// 原因：旧代码用户图标按钮无登出功能，用户无法主动退出登录。
function handleLogout() {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
  router.push('/login')
}
</script>

<style scoped>
.wk-layout {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.wk-topbar {
  height: 56px;
  border-bottom: 1px solid var(--wk-border);
  background: var(--wk-bg-soft);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 100;
}

.wk-topbar-left {
  display: flex;
  align-items: center;
  gap: 32px;
}

.wk-topbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.wk-nav {
  display: flex;
  align-items: center;
  gap: 4px;
}

.wk-nav-item {
  padding: 8px 16px;
  cursor: pointer;
  color: var(--wk-text-muted);
  transition: all 0.2s;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  border-radius: 6px;
}

.wk-nav-item:hover,
.wk-nav-item.active {
  color: var(--wk-primary);
  background: rgba(56, 189, 248, 0.08);
}
</style>