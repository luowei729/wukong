<template>
  <!-- ============ 后台管理外壳：顶栏 + 可折叠侧栏 + 内容区 ============ -->
  <!-- 保留原有 IA（顶栏 + 侧栏 + router-view），只重构视觉与数据来源 -->
  <div
    class="wk-app-shell"
    :class="{ 'sidebar-collapsed': sidebarCollapsed, 'sidebar-open': sidebarOpen }"
  >
    <!-- ---------------- 顶栏 ---------------- -->
    <header class="wk-header">
      <div class="wk-header-left">
        <!-- 侧栏折叠：桌面端折叠/展开，移动端开合抽屉 -->
        <button
          type="button"
          class="wk-icon-btn"
          title="折叠/展开侧栏"
          aria-label="切换侧栏"
          @click="toggleSidebar"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
            <rect width="18" height="18" x="3" y="3" rx="2" />
            <path d="M9 3v18" />
          </svg>
        </button>

        <!-- 页面标题 + 副标题：层次由排版建立，不再重复一个 h2 -->
        <div class="wk-header-title">
          <div class="wk-page-title">{{ currentTitle }}</div>
          <div v-if="currentSubtitle" class="wk-page-sub">{{ currentSubtitle }}</div>
        </div>
      </div>

      <div class="wk-header-right">
        <!-- 实时摘要药丸：取代旧版四个重复的顶部指标卡 -->
        <span class="wk-live-pill" :title="`共 ${totalCount} 个节点`">
          <WkStatusDot :status="onlineCount > 0 ? 'online' : 'offline'" :size="6" pulse />
          <span>在线 <b class="wk-num">{{ onlineCount }}</b> / {{ totalCount }}</span>
        </span>

        <!-- 告警入口：有 firing 告警时高亮并显示计数角标 -->
        <el-tooltip :content="firingCount > 0 ? `${firingCount} 条进行中告警` : '暂无进行中告警'" placement="bottom">
          <button
            type="button"
            :class="['wk-icon-btn', firingCount > 0 ? 'is-alert-active' : '']"
            aria-label="告警中心"
            @click="router.push('/alerts')"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
              <path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9" />
              <path d="M10 21a2 2 0 0 0 4 0" />
            </svg>
          </button>
        </el-tooltip>

        <!-- 本地时钟：秒级跳动，运维排障时用来对齐上报时间 -->
        <span class="wk-clock wk-num">{{ clock }}</span>

        <!-- 主题切换 -->
        <el-tooltip :content="isDark ? '切换到浅色主题' : '切换到深色主题'" placement="bottom">
          <button type="button" class="wk-icon-btn" aria-label="切换主题" @click="toggleTheme">
            <svg v-if="isDark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="4" />
              <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
            </svg>
            <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
            </svg>
          </button>
        </el-tooltip>

        <!-- 用户菜单：退出登录（保留原功能，位置改为头像下拉） -->
        <el-dropdown trigger="click" placement="bottom-end" @command="onUserCommand">
          <button type="button" class="wk-user-chip" aria-label="管理员菜单">
            <span class="wk-avatar">A</span>
            <span class="wk-user-name">admin</span>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width: 12px; height: 12px">
              <path d="M6 9l6 6 6-6" />
            </svg>
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="settings">系统设置</el-dropdown-item>
              <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </header>

    <div class="wk-app-body">
      <!-- 移动端抽屉遮罩 -->
      <div class="wk-sidebar-backdrop" @click="sidebarOpen = false"></div>

      <!-- ---------------- 侧栏 ---------------- -->
      <aside class="wk-sidebar">
        <!-- 品牌行移入侧栏：顶栏只负责页面标题，符合 SaaS 控制台惯例 -->
        <div class="wk-side-brand">
          <span class="wk-brand-mark">悟</span>
          <span class="wk-brand-text">{{ siteTitle }}</span>
        </div>

        <nav class="wk-side-nav">
          <div v-for="group in navGroups" :key="group.title" class="wk-nav-group">
            <div class="wk-nav-group-title">{{ group.title }}</div>
            <el-tooltip
              v-for="item in group.items"
              :key="item.path"
              :disabled="!sidebarCollapsed"
              :content="item.label"
              placement="right"
            >
              <a
                :class="['wk-nav-item', { active: currentPath === item.path }]"
                role="link"
                :aria-label="item.label"
                @click="navigate(item.path)"
              >
                <span class="wk-nav-ico" v-html="item.icon"></span>
                <span class="wk-nav-label">{{ item.label }}</span>
                <!-- 告警项挂进行中数量，折叠态隐藏 -->
                <span v-if="item.path === '/alerts' && firingCount > 0" class="wk-nav-count wk-num">
                  {{ firingCount }}
                </span>
              </a>
            </el-tooltip>
          </div>
        </nav>

        <!-- 底部状态行：主控连接是否正常的最小说明 -->
        <div class="wk-side-bottom">
          <div class="wk-status-line">
            <WkStatusDot :status="onlineCount > 0 ? 'online' : 'muted'" :size="6" />
            <span class="wk-side-status-text">
              {{ onlineCount > 0 ? `已接入 ${onlineCount} 个节点` : '等待探针接入' }}
            </span>
          </div>
          <a class="wk-nav-item mini" role="link" aria-label="退出登录" @click="handleLogout">
            <span class="wk-nav-ico">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                <path d="M16 17l5-5-5-5M21 12H9" />
              </svg>
            </span>
            <span class="wk-nav-label">退出登录</span>
          </a>
        </div>
      </aside>

      <!-- ---------------- 内容区 ---------------- -->
      <section class="wk-main-shell">
        <main class="wk-container">
          <router-view />
        </main>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
// ============ 后台外壳逻辑 ============
// 数据来源：useOverview() 共享单例（本组件与 Dashboard/Nodes 共用一个 1s 轮询）
// 主题来源：useTheme() 单例，站点标题与自定义主色都从后端读取
import { computed, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import WkStatusDot from '@/components/WkStatusDot.vue'
import { useOverview } from '@/composables/useOverview'
import { useTheme } from '@/composables/useTheme'

const router = useRouter()
const route = useRoute()

// 共享概览数据：在线数/总数/告警数都不再自己发请求
const { onlineCount, totalCount, firingCount } = useOverview()

// 主题
const { isDark, toggleTheme, title: siteTitle } = useTheme()

// 侧栏折叠状态（桌面端折叠、移动端抽屉），沿用原 localStorage key
const sidebarCollapsed = ref(localStorage.getItem('wk-sidebar-collapsed') === 'true')
const sidebarOpen = ref(false)

// 内联 SVG 图标：不依赖图标库，保证离线部署也能正常显示
const menuItems = [
  {
    path: '/dashboard',
    label: '总览仪表盘',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="9" rx="1.5"/><rect x="14" y="3" width="7" height="5" rx="1.5"/><rect x="14" y="12" width="7" height="9" rx="1.5"/><rect x="3" y="16" width="7" height="5" rx="1.5"/></svg>',
  },
  {
    path: '/nodes',
    label: '节点列表',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="8" rx="2"/><rect x="2" y="13" width="20" height="8" rx="2"/><path d="M6 7h.01M6 17h.01"/></svg>',
  },
  {
    path: '/alerts',
    label: '告警中心',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9"/><path d="M10 21a2 2 0 0 0 4 0"/></svg>',
  },
]

// 分组导航：监控类与系统类分开，避免"设置"和"告警"混在同一层
const navGroups = [
  { title: '监控', items: menuItems },
  {
    title: '系统',
    items: [
      {
        path: '/settings',
        label: '系统设置',
        icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9v.09a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
      },
    ],
  },
]

const currentPath = computed(() => route.path)
const currentTitle = computed(() => (route.meta.title as string) || siteTitle.value)
// 副标题来自路由 meta，未配置时显示站点名，保证顶栏层次稳定
const currentSubtitle = computed(() => (route.meta.subtitle as string) || '')

// 切换侧栏：<=860px 视为移动端，用抽屉而不是折叠成图标栏
function toggleSidebar() {
  if (window.innerWidth <= 860) {
    sidebarOpen.value = !sidebarOpen.value
  } else {
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem('wk-sidebar-collapsed', String(sidebarCollapsed.value))
  }
}

// 导航：移动端点击后自动收起抽屉
function navigate(path: string) {
  router.push(path)
  sidebarOpen.value = false
}

// 用户下拉菜单命令分发
function onUserCommand(command: string) {
  if (command === 'logout') handleLogout()
  if (command === 'settings') navigate('/settings')
}

// 登出：清除双 token 后回登录页（token 由 http.ts 拦截器读写，这里必须同步清理）
function handleLogout() {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
  router.push('/login')
}

// ---- 顶栏时钟：每秒刷新，与其他页面的 1s 轮询节奏一致，便于肉眼对齐 ----
const clock = ref('')

function updateClock() {
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  clock.value = `${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
}

updateClock()
const clockTimer = setInterval(updateClock, 1000)

onUnmounted(() => clearInterval(clockTimer))
</script>

<style scoped>
/* 顶栏标题里的数值用等宽，避免每秒刷新时数字宽度变化引起抖动 */
.wk-live-pill b {
  font-weight: 600;
  color: var(--wk-text);
}

/* 下拉触发器复用 chip 外观，去掉浏览器默认按钮内边距 */
.wk-user-chip {
  padding-right: 8px;
}
</style>
