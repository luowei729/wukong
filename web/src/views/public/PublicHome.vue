<template>
  <!-- ============================================================
       公开首页：未登录用户可查看脱敏后的服务器运行状态
       设计参考 QuantKing：蓝色主色调 + 圆角卡片 + 双主题
       ============================================================ -->
  <div class="public-page">
    <!-- 顶部导航栏：品牌标志 + 站点标题 + 管理登录按钮 -->
    <header class="public-nav">
      <div class="nav-brand">
        <span class="wk-brand-mark">悟</span>
        <div class="nav-brand-text">
          <strong>{{ siteTitle }}</strong>
          <small>公开服务器监控</small>
        </div>
      </div>
      <el-button type="primary" plain @click="goAdmin">
        {{ hasToken ? '管理后台' : '管理登录' }}
      </el-button>
    </header>

    <!-- 主内容区域 -->
    <main class="public-main">
      <!-- 统计摘要区域：6 个指标卡，展示服务器数 / 在线 / 离线 / 平均 CPU / 平均内存 / 网络流量 -->
      <section class="wk-metrics summary-grid">
        <div class="wk-metric">
          <span class="label">服务器</span>
          <span class="value">{{ servers.length }}</span>
          <span class="sub">台</span>
        </div>
        <div class="wk-metric">
          <span class="label">在线</span>
          <span class="value green">{{ onlineCount }}</span>
          <span class="sub">台</span>
        </div>
        <div class="wk-metric">
          <span class="label">离线</span>
          <span class="value red">{{ offlineCount }}</span>
          <span class="sub">台</span>
        </div>
        <div class="wk-metric">
          <span class="label">平均 CPU</span>
          <span class="value">{{ avgCpu }}</span>
          <span class="sub">%</span>
        </div>
        <div class="wk-metric">
          <span class="label">平均内存</span>
          <span class="value">{{ avgMem }}</span>
          <span class="sub">%</span>
        </div>
        <div class="wk-metric">
          <span class="label">网络流量</span>
          <span class="value">{{ totalNet }}</span>
          <span class="sub">/s</span>
        </div>
      </section>

      <!-- 服务器列表区域标题 -->
      <div class="wk-page-header">
        <h2>服务器列表</h2>
        <span class="muted">点击卡片查看详情</span>
      </div>

      <!-- 数据加载时显示骨架屏 -->
      <div v-if="loading" class="server-grid">
        <div v-for="i in 6" :key="i" class="server-card wk-card">
          <div class="wk-skeleton skeleton-line" style="width: 60%" />
          <div class="wk-skeleton skeleton-line" style="width: 40%; margin-top: 8px" />
          <div class="wk-skeleton skeleton-bar" />
          <div class="wk-skeleton skeleton-bar" />
          <div class="wk-skeleton skeleton-bar" />
          <div class="wk-skeleton skeleton-line" style="width: 80%; margin-top: 12px" />
        </div>
      </div>

      <!-- 空状态提示 -->
      <div v-else-if="servers.length === 0" class="empty-state">
        <div class="empty-icon">📭</div>
        <p class="empty-title">暂无服务器</p>
        <p class="empty-desc">请登录管理后台安装探针</p>
      </div>

      <!-- 服务器卡片网格 -->
      <section v-else class="server-grid">
        <article
          v-for="server in servers"
          :key="server.id"
          class="server-card wk-card"
          @click="router.push(`/server/${server.id}`)"
        >
          <!-- 卡片头部：名称 + 状态灯 -->
          <div class="server-card-head">
            <div class="server-info">
              <span class="server-name">{{ server.name || '未命名服务器' }}</span>
              <div class="server-meta">{{ serverMeta(server) }}</div>
            </div>
            <span :class="['wk-status-dot', server.status === 'online' ? 'online' : 'offline']" />
          </div>

          <!-- 指标进度条：CPU / 内存 / 磁盘 -->
          <div class="metric-bars">
            <div class="metric-row">
              <span class="metric-label">CPU</span>
              <el-progress
                :percentage="metricPercent(server.cpu)"
                :show-text="false"
                :color="progressColor(server.cpu)"
                :stroke-width="8"
              />
              <strong class="metric-val">{{ formatPercent(server.cpu) }}</strong>
            </div>
            <div class="metric-row">
              <span class="metric-label">内存</span>
              <el-progress
                :percentage="metricPercent(server.mem)"
                :show-text="false"
                :color="progressColor(server.mem)"
                :stroke-width="8"
              />
              <strong class="metric-val">{{ formatPercent(server.mem) }}</strong>
            </div>
            <div class="metric-row">
              <span class="metric-label">磁盘</span>
              <el-progress
                :percentage="metricPercent(server.disk)"
                :show-text="false"
                :color="progressColor(server.disk)"
                :stroke-width="8"
              />
              <strong class="metric-val">{{ formatPercent(server.disk) }}</strong>
            </div>
          </div>

          <!-- 卡片底部：上下行流量 + 最后活跃时间 -->
          <div class="server-foot">
            <span class="foot-item">
              <span class="foot-icon up">↑</span>
              {{ formatBytes(server.net_up) }}/s
            </span>
            <span class="foot-item">
              <span class="foot-icon down">↓</span>
              {{ formatBytes(server.net_down) }}/s
            </span>
            <span class="foot-item foot-time">
              {{ relativeTime(server.last_seen_at || server.updated_at) }}
            </span>
          </div>
        </article>
      </section>

      <!-- 页脚：站点 footer 文本 -->
      <footer v-if="siteFooter" class="public-footer">
        <a href="https://github.com/luowei729/wukong" target="_blank" rel="noopener noreferrer">
          {{ siteFooter }}
        </a>
      </footer>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import http from '@/utils/http'

// 服务器数据接口，与公开 API 返回字段对齐
interface PublicServer {
  id: string
  name: string
  online: boolean
  status: 'online' | 'offline' | 'stale' | 'unknown'
  last_seen_at?: string
  updated_at?: string
  os_version?: string
  arch?: string
  platform?: string
  region?: string
  cpu?: number
  mem?: number
  disk?: number
  net_up?: number
  net_down?: number
  uptime_seconds?: number
}

// 响应式状态
const router = useRouter()
const loading = ref(false)                     // 加载状态标志
const servers = ref<PublicServer[]>([])         // 服务器列表数据
const siteTitle = ref('wukong 监控')            // 站点标题，从主题接口加载
const siteFooter = ref('')                      // 站点页脚，从主题接口加载
const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))  // 是否已登录
let refreshTimer: ReturnType<typeof setInterval> | null = null                  // 定时刷新计时器

// ==================== 主题加载 ====================

// 加载站点主题（标题和页脚），公开首页也需要显示后台设置的标题
async function loadTheme() {
  try {
    const res = await http.get(`/api/public/theme?_=${Date.now()}`)
    // 设置站点标题并写入 localStorage 供其他页面读取
    if (res.data.title) {
      siteTitle.value = res.data.title
      localStorage.setItem('site_title', res.data.title)
      document.title = res.data.title
    }
    // 设置页脚文本
    if (res.data.footer_text) siteFooter.value = res.data.footer_text
    // 设置主题预设（dark / light）
    if (res.data.preset) {
      document.documentElement.dataset.theme = res.data.preset
      document.documentElement.classList.toggle('dark', res.data.preset === 'dark')
    }
  } catch {}
}

// ==================== 统计摘要计算 ====================

// 在线服务器数量
const onlineCount = computed(() => servers.value.filter(s => s.status === 'online').length)
// 离线服务器数量（非 online 均算离线）
const offlineCount = computed(() => servers.value.filter(s => s.status !== 'online').length)
// 平均 CPU 使用率
const avgCpu = computed(() => {
  const list = servers.value.filter(s => typeof s.cpu === 'number')
  return list.length ? (list.reduce((sum, s) => sum + (s.cpu || 0), 0) / list.length).toFixed(1) : '-'
})
// 平均内存使用率
const avgMem = computed(() => {
  const list = servers.value.filter(s => typeof s.mem === 'number')
  return list.length ? (list.reduce((sum, s) => sum + (s.mem || 0), 0) / list.length).toFixed(1) : '-'
})
// 网络流量汇总（上行 + 下行）
const totalNet = computed(() => {
  const up = servers.value.reduce((sum, s) => sum + (s.net_up || 0), 0)
  const down = servers.value.reduce((sum, s) => sum + (s.net_down || 0), 0)
  return `${formatBytesShort(up)}↑ ${formatBytesShort(down)}↓`
})

// ==================== 数据加载 ====================

// 获取公开服务器列表，公开首页只访问 /api/public/servers，不携带 JWT
async function loadData(showLoading = false) {
  if (showLoading) loading.value = true
  try {
    const res = await http.get(`/api/public/servers?_=${Date.now()}`)
    servers.value = res.data.servers || []
  } finally {
    if (showLoading) loading.value = false
  }
}

// 跳转管理后台，已登录去 dashboard，未登录去 login
function goAdmin() {
  router.push(hasToken.value ? '/dashboard' : '/login')
}

// ==================== 工具函数 ====================

// 服务器元信息：系统 + 区域 + 架构，类似 qio.ng 风格
function serverMeta(server: PublicServer) {
  const parts: string[] = []
  if (server.platform) parts.push(server.platform)
  else if (server.os_version) parts.push(server.os_version)
  if (server.region) parts.push(server.region)
  if (server.arch) parts.push(server.arch)
  return parts.length ? parts.join(' · ') : '系统信息待上报'
}

// 状态文本映射
function statusText(status: string) {
  return ({ online: '在线', offline: '离线', stale: '数据延迟', unknown: '未知' } as Record<string, string>)[status] || '未知'
}

// 数值转百分比（0-100），用于进度条
function metricPercent(value?: number) {
  return Math.max(0, Math.min(100, Math.round(value || 0)))
}

// 进度条颜色：低绿色、中蓝色、高红色，参考 QuantKing 配色
function progressColor(value?: number): string {
  if (typeof value !== 'number') return '#3b82f6'
  if (value < 50) return '#34d399'
  if (value < 80) return '#3b82f6'
  if (value < 90) return '#fbbf24'
  return '#f87171'
}

// 格式化百分比显示
function formatPercent(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)}%` : '-'
}

// 格式化字节数（完整单位）
function formatBytes(value?: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  return `${size.toFixed(size >= 10 ? 0 : 1)} ${units[index]}`
}

// 格式化字节数（简短单位，用于摘要区域）
function formatBytesShort(value: number) {
  if (!value) return '0B'
  const units = ['B', 'K', 'M', 'G']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  return `${size.toFixed(size >= 10 ? 0 : 1)}${units[index]}`
}

// 相对时间格式化（如"5分钟前"）
function relativeTime(value?: string) {
  if (!value) return '暂无上报'
  const diff = Date.now() - new Date(value).getTime()
  if (diff < 0) return '0秒前'
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

// ==================== 生命周期 ====================

onMounted(() => {
  // 初始化加载主题和服务器数据
  loadTheme()
  loadData(true)
  // 首页状态卡片每秒刷新一次，保证公开展示接近实时
  refreshTimer = setInterval(() => loadData(false), 1000)
})

onUnmounted(() => {
  // 组件卸载时清除定时器，避免内存泄漏
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
/* 公开首页容器：简洁渐变背景，移除旧的 glow 装饰效果 */
.public-page {
  min-height: 100vh;
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--wk-primary) 8%, transparent) 0%, transparent 280px),
    var(--wk-bg);
  color: var(--wk-text);
}

/* 顶部导航栏和主内容区统一宽度居中 */
.public-nav,
.public-main {
  width: min(1180px, calc(100% - 32px));
  margin: 0 auto;
}

/* 导航栏：左右两端对齐 */
.public-nav {
  height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* 品牌区域：标志 + 标题文字 */
.nav-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 品牌标志放大（覆盖全局 .wk-brand-mark 的 28px） */
.nav-brand .wk-brand-mark {
  width: 40px;
  height: 40px;
  font-size: 18px;
  border-radius: 12px;
}

/* 品牌标题文字 */
.nav-brand-text strong {
  display: block;
  font-size: 16px;
  font-weight: 700;
}

.nav-brand-text small {
  display: block;
  color: var(--wk-text-muted);
  font-size: 12px;
}

/* 统计摘要网格：6 列 */
.summary-grid {
  margin-top: 24px;
}

/* 页面标题区域 */
.public-main .wk-page-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin: 32px 0 18px;
}

.public-main .wk-page-header h2 {
  font-size: 20px;
  font-weight: 700;
  margin: 0;
}

/* 服务器卡片网格：3 列 */
.server-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  padding-bottom: 64px;
}

/* 服务器卡片：可点击，悬停浮起 */
.server-card {
  cursor: pointer;
  margin-bottom: 0;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.server-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--wk-shadow-md);
}

/* 卡片头部：名称 + 状态灯 */
.server-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.server-name {
  font-size: 16px;
  font-weight: 700;
  display: block;
}

.server-meta {
  color: var(--wk-text-muted);
  font-size: 12px;
  margin-top: 4px;
}

/* 指标进度条容器 */
.metric-bars {
  display: grid;
  gap: 12px;
  margin: 16px 0;
}

/* 单行指标：标签 + 进度条 + 数值 */
.metric-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.metric-label {
  width: 36px;
  font-size: 12px;
  color: var(--wk-text-muted);
  flex-shrink: 0;
}

.metric-row .el-progress {
  flex: 1;
}

.metric-val {
  width: 52px;
  text-align: right;
  font-family: ui-monospace, 'JetBrains Mono', monospace;
  font-size: 13px;
  font-weight: 600;
}

/* 卡片底部：流量 + 时间 */
.server-foot {
  display: flex;
  align-items: center;
  gap: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--wk-border);
  font-size: 12px;
  color: var(--wk-text-muted);
}

.foot-item {
  display: flex;
  align-items: center;
  gap: 2px;
}

.foot-icon.up { color: var(--wk-success); }
.foot-icon.down { color: var(--wk-primary); }

.foot-time {
  margin-left: auto;
}

/* 骨架屏样式 */
.skeleton-line {
  height: 16px;
  border-radius: 6px;
}

.skeleton-bar {
  height: 8px;
  border-radius: 4px;
  margin-top: 12px;
}

/* 空状态提示 */
.empty-state {
  text-align: center;
  padding: 80px 20px;
  color: var(--wk-text-muted);
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--wk-text);
  margin-bottom: 8px;
}

.empty-desc {
  font-size: 14px;
}

/* 页脚 */
.public-footer {
  text-align: center;
  padding: 24px 0 48px;
  color: var(--wk-text-muted);
  font-size: 12px;
  opacity: 0.6;
}

.public-footer a {
  color: inherit;
  text-decoration: none;
}

.public-footer a:hover {
  color: var(--wk-primary);
}

/* 响应式：中等屏幕 2 列 */
@media (max-width: 980px) {
  .server-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/* 响应式：小屏幕单列 */
@media (max-width: 640px) {
  .server-grid {
    grid-template-columns: 1fr;
  }
}
</style>
