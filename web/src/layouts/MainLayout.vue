<template>
  <!-- 主布局：顶栏 + 可折叠侧栏 + 主内容区（参考 QuantKing 布局） -->
  <div class="wk-app-shell" :class="{ 'sidebar-collapsed': sidebarCollapsed, 'sidebar-open': sidebarOpen }">
    <!-- 顶栏 -->
    <header class="wk-header">
      <div class="wk-header-left">
        <!-- 侧栏折叠按钮 -->
        <button type="button" class="wk-icon-btn" @click="toggleSidebar" title="折叠/展开侧栏" aria-label="切换侧栏">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>
          </svg>
        </button>
        <!-- 品牌 Logo -->
        <a class="wk-header-brand" @click="router.push('/dashboard')">
          <span class="wk-brand-mark">悟</span>
          <span class="wk-brand-text-side">wukong</span>
        </a>
        <span class="wk-header-divider"></span>
        <div class="wk-page-title">{{ currentTitle }}</div>
      </div>
      <!-- 顶部指标条 -->
      <div class="wk-header-metrics">
        <div class="wk-top-chip"><span>在线</span><b>{{ onlineCount }}</b></div>
        <div class="wk-top-chip"><span>平均 CPU</span><b>{{ avgCpu }}</b></div>
        <div class="wk-top-chip"><span>平均内存</span><b>{{ avgMem }}</b></div>
        <div class="wk-top-chip"><span>告警</span><b>{{ alertCount }}</b></div>
      </div>
      <div class="wk-header-right">
        <span class="wk-clock">{{ clock }}</span>
        <!-- 主题切换按钮 -->
        <button type="button" class="wk-icon-btn" @click="toggleTheme" title="切换浅色/深色" aria-label="切换主题">
          <svg v-if="isDark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="5"/><path d="M12 1v3M12 20v3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M1 12h3M20 12h3M4.22 19.78l2.12-2.12M17.66 6.34l2.12-2.12"/>
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
        </button>
        <!-- 退出按钮 -->
        <button type="button" class="wk-icon-btn" @click="handleLogout" title="退出登录" aria-label="退出">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5M21 12H9"/>
          </svg>
        </button>
      </div>
    </header>

    <div class="wk-app-body">
      <!-- 移动端遮罩 -->
      <div class="wk-sidebar-backdrop" @click="sidebarOpen = false"></div>

      <!-- 侧栏 -->
      <aside class="wk-sidebar">
        <nav class="wk-side-nav">
          <div class="wk-nav-group">
            <div class="wk-nav-group-title">监控</div>
            <a v-for="item in menuItems" :key="item.path"
               :class="['wk-nav-item', { active: currentPath === item.path }]"
               @click="navigate(item.path)">
              <span class="wk-nav-ico" v-html="item.icon"></span>
              <span class="wk-nav-label">{{ item.label }}</span>
            </a>
          </div>
        </nav>
        <div class="wk-side-bottom">
          <div class="wk-status-line">
            <span :class="['wk-status-dot', onlineCount > 0 ? 'online' : 'offline']"></span>
            <span class="wk-side-status-text">{{ onlineCount > 0 ? '系统正常' : '等待探针' }}</span>
          </div>
          <a class="wk-nav-item mini" @click="handleLogout">
            <span class="wk-nav-ico">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5M21 12H9"/></svg>
            </span>
            <span class="wk-nav-label">退出登录</span>
          </a>
        </div>
      </aside>

      <!-- 主内容区 -->
      <section class="wk-main-shell">
        <main class="wk-container">
          <router-view />
        </main>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import http from '@/utils/http'

const router = useRouter()
const route = useRoute()

// 侧栏折叠状态（桌面端折叠/展开，移动端开关）
const sidebarCollapsed = ref(localStorage.getItem('wk-sidebar-collapsed') === 'true')
const sidebarOpen = ref(false)

// 主题状态
const isDark = ref(localStorage.getItem('theme') !== 'light')
const clock = ref('')
const onlineCount = ref(0)
const avgCpu = ref('-')
const avgMem = ref('-')
const alertCount = ref(0)

// 菜单项 - 内联 SVG 图标（参考 QuantKing 风格）
const menuItems = [
  { path: '/dashboard', label: '总览仪表盘', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/></svg>' },
  { path: '/nodes', label: '节点列表', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>' },
  { path: '/alerts', label: '告警中心', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9"/><path d="M10 21a2 2 0 0 0 4 0"/></svg>' },
  { path: '/settings', label: '系统设置', icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><circle cx="12" cy="12" r="3"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>' },
]

const currentPath = computed(() => route.path)
const currentTitle = computed(() => (route.meta.title as string) || 'wukong')

// 切换侧栏折叠
function toggleSidebar() {
  // 移动端：开关侧栏；桌面端：折叠/展开
  if (window.innerWidth <= 860) {
    sidebarOpen.value = !sidebarOpen.value
  } else {
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem('wk-sidebar-collapsed', String(sidebarCollapsed.value))
  }
}

// 切换主题
function toggleTheme() {
  isDark.value = !isDark.value
  const theme = isDark.value ? 'dark' : 'light'
  document.documentElement.dataset.theme = theme
  document.documentElement.classList.toggle('dark', theme === 'dark')
  localStorage.setItem('theme', theme)
}

// 导航
function navigate(path: string) {
  router.push(path)
  sidebarOpen.value = false
}

// 登出：清除 JWT 并跳转登录页
function handleLogout() {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
  router.push('/login')
}

// 获取顶部指标数据
async function fetchTopMetrics() {
  try {
    const [agentsRes, latestRes, alertsRes] = await Promise.all([
      http.get(`/api/agents?_=${Date.now()}`),
      http.get(`/api/agents/latest?_=${Date.now()}`),
      http.get(`/api/alerts?_=${Date.now()}`).catch(() => ({ data: [] })),
    ])
    const latest = latestRes.data || {}
    const nodes = (agentsRes.data || []).map((n: any) => ({ ...n, ...(latest[n.id] || {}) }))
    const online = nodes.filter((n: any) => n.online)
    onlineCount.value = online.length
    const metricNodes = online.filter((n: any) => typeof n.cpu === 'number')
    avgCpu.value = metricNodes.length ? (metricNodes.reduce((s: number, n: any) => s + n.cpu, 0) / metricNodes.length).toFixed(1) + '%' : '-'
    avgMem.value = metricNodes.length ? (metricNodes.reduce((s: number, n: any) => s + n.mem, 0) / metricNodes.length).toFixed(1) + '%' : '-'
    alertCount.value = Array.isArray(alertsRes.data) ? alertsRes.data.filter((a: any) => a.status === 'firing').length : 0
  } catch {}
}

// 时钟
function updateClock() {
  const now = new Date()
  clock.value = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`
}

let metricsTimer: ReturnType<typeof setInterval> | null = null
let clockTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  // 初始化主题
  const savedTheme = localStorage.getItem('theme') || 'dark'
  document.documentElement.dataset.theme = savedTheme
  document.documentElement.classList.toggle('dark', savedTheme === 'dark')

  fetchTopMetrics()
  updateClock()
  metricsTimer = setInterval(fetchTopMetrics, 5000)
  clockTimer = setInterval(updateClock, 1000)
})

onUnmounted(() => {
  if (metricsTimer) clearInterval(metricsTimer)
  if (clockTimer) clearInterval(clockTimer)
})
</script>

<style scoped>
/* 所有样式已在全局 index.scss 中定义 */
</style>
