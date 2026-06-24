<template>
  <!-- 公开状态首页：qio.ng 风格，未登录用户可查看脱敏后的服务器运行状态 -->
  <div class="public-page">
    <div class="glow glow-a" />
    <div class="glow glow-b" />

    <header class="public-nav">
      <div class="brand">
        <span class="brand-mark">悟</span>
        <div>
          <strong>{{ siteTitle }}</strong>
          <small>Public server monitor</small>
        </div>
      </div>
      <div class="nav-actions">
        <el-button type="primary" plain @click="goAdmin">
          {{ hasToken ? '管理后台' : '管理登录' }}
        </el-button>
      </div>
    </header>

    <main class="public-main">
      <!-- 统计摘要卡片 -->
      <section class="summary-grid">
        <div class="summary-card wk-card">
          <span class="summary-label">服务器</span>
          <strong class="summary-value">{{ servers.length }}</strong>
          <small class="summary-unit">台</small>
        </div>
        <div class="summary-card wk-card">
          <span class="summary-label">在线</span>
          <strong class="summary-value summary-online">{{ onlineCount }}</strong>
          <small class="summary-unit">台</small>
        </div>
        <div class="summary-card wk-card">
          <span class="summary-label">离线</span>
          <strong class="summary-value summary-offline">{{ offlineCount }}</strong>
          <small class="summary-unit">台</small>
        </div>
        <div class="summary-card wk-card">
          <span class="summary-label">平均 CPU</span>
          <strong class="summary-value">{{ avgCpu }}</strong>
          <small class="summary-unit">%</small>
        </div>
        <div class="summary-card wk-card">
          <span class="summary-label">平均内存</span>
          <strong class="summary-value">{{ avgMem }}</strong>
          <small class="summary-unit">%</small>
        </div>
        <div class="summary-card wk-card">
          <span class="summary-label">网络流量</span>
          <strong class="summary-value">{{ totalNet }}</strong>
          <small class="summary-unit">/s</small>
        </div>
      </section>

      <!-- 服务器列表 -->
      <section class="section-head">
        <h2>服务器列表</h2>
        <span class="section-hint">点击卡片查看详情</span>
      </section>

      <el-skeleton v-if="loading" :rows="6" animated />
      <el-empty v-else-if="servers.length === 0" description="暂无服务器，请登录管理后台安装探针" />
      <section v-else class="server-grid">
        <article
          v-for="server in servers"
          :key="server.id"
          class="server-card wk-card"
          @click="router.push(`/server/${server.id}`)"
        >
          <div class="server-card-head">
            <div>
              <span class="server-name">{{ server.name || '未命名服务器' }}</span>
              <div class="server-meta">{{ serverMeta(server) }}</div>
            </div>
            <span :class="['status-badge', server.status]">
              <span class="status-dot-inline" :class="server.status" />
              {{ statusText(server.status) }}
            </span>
          </div>
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
              {{ relativeTime(server.updated_at || server.last_seen_at) }}
            </span>
          </div>
        </article>
      </section>

      <!-- 页脚 -->
      <footer v-if="siteFooter" class="public-footer">
        {{ siteFooter }}
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

const router = useRouter()
const loading = ref(false)
const servers = ref<PublicServer[]>([])
const siteTitle = ref('wukong 监控')
const siteFooter = ref('')
const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))
let refreshTimer: ReturnType<typeof setInterval> | null = null

// 加载站点主题（标题和页脚），公开首页也需要显示后台设置的标题
async function loadTheme() {
  try {
    const res = await http.get(`/api/public/theme?_=${Date.now()}`)
    if (res.data.title) siteTitle.value = res.data.title
    if (res.data.footer_text) siteFooter.value = res.data.footer_text
    if (res.data.preset) {
      document.documentElement.dataset.theme = res.data.preset
      document.documentElement.classList.toggle('dark', res.data.preset === 'dark')
    }
  } catch {}
}

// 统计摘要计算
const onlineCount = computed(() => servers.value.filter(s => s.status === 'online').length)
const offlineCount = computed(() => servers.value.filter(s => s.status !== 'online').length)
const avgCpu = computed(() => {
  const list = servers.value.filter(s => typeof s.cpu === 'number')
  return list.length ? (list.reduce((sum, s) => sum + (s.cpu || 0), 0) / list.length).toFixed(1) : '-'
})
const avgMem = computed(() => {
  const list = servers.value.filter(s => typeof s.mem === 'number')
  return list.length ? (list.reduce((sum, s) => sum + (s.mem || 0), 0) / list.length).toFixed(1) : '-'
})
const totalNet = computed(() => {
  const up = servers.value.reduce((sum, s) => sum + (s.net_up || 0), 0)
  const down = servers.value.reduce((sum, s) => sum + (s.net_down || 0), 0)
  return `${formatBytesShort(up)}↑ ${formatBytesShort(down)}↓`
})

async function loadData(showLoading = false) {
  // 公开首页只访问 /api/public/servers，不携带 JWT，确保未登录也可展示
  if (showLoading) loading.value = true
  try {
    const res = await http.get(`/api/public/servers?_=${Date.now()}`)
    servers.value = res.data.servers || []
  } finally {
    if (showLoading) loading.value = false
  }
}

function goAdmin() {
  router.push(hasToken.value ? '/dashboard' : '/login')
}

// 服务器元信息：系统 + 区域，类似 qio.ng 风格
function serverMeta(server: PublicServer) {
  const parts = []
  if (server.platform) parts.push(server.platform)
  else if (server.os_version) parts.push(server.os_version)
  if (server.region) parts.push(server.region)
  if (server.arch) parts.push(server.arch)
  return parts.length ? parts.join(' · ') : '系统信息待上报'
}

function statusText(status: string) {
  return ({ online: '在线', offline: '离线', stale: '数据延迟', unknown: '未知' } as Record<string, string>)[status] || '未知'
}

function metricPercent(value?: number) {
  return Math.max(0, Math.min(100, Math.round(value || 0)))
}

// 进度条颜色：低绿色、中蓝色、高红色，类似 qio.ng
function progressColor(value?: number): string {
  if (typeof value !== 'number') return '#38bdf8'
  if (value < 50) return '#22c55e'
  if (value < 80) return '#38bdf8'
  if (value < 90) return '#f59e0b'
  return '#ef4444'
}

function formatPercent(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)}%` : '-'
}

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

function relativeTime(value?: string) {
  if (!value) return '暂无上报'
  const diff = Date.now() - new Date(value).getTime()
  if (diff < 0) return '刚刚'
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

onMounted(() => {
  loadTheme()
  loadData(true)
  // 首页状态卡片每秒刷新一次，保证公开展示接近实时
  refreshTimer = setInterval(() => loadData(false), 1000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.public-page {
  min-height: 100vh;
  background: radial-gradient(circle at top left, rgba(56, 189, 248, 0.22), transparent 34%), var(--wk-bg);
  color: var(--wk-text);
  position: relative;
  overflow: hidden;
}

.glow {
  position: fixed;
  width: 360px;
  height: 360px;
  border-radius: 999px;
  filter: blur(80px);
  opacity: 0.28;
  pointer-events: none;
}

.glow-a { top: 120px; right: 8%; background: #38bdf8; }
.glow-b { bottom: 10%; left: 4%; background: #8b5cf6; }

.public-nav,
.public-main {
  width: min(1180px, calc(100% - 32px));
  margin: 0 auto;
  position: relative;
  z-index: 1;
}

.public-nav {
  height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.brand-mark {
  width: 42px;
  height: 42px;
  border-radius: 14px;
  display: grid;
  place-items: center;
  color: #00111f;
  background: linear-gradient(135deg, #38bdf8, #22c55e);
  font-weight: 800;
}

.brand strong,
.brand small {
  display: block;
}

.brand small {
  color: var(--wk-text-muted);
}

.nav-actions {
  display: flex;
  gap: 10px;
}

/* 统计摘要区域 */
.summary-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 16px;
  margin-top: 24px;
}

.summary-card {
  padding: 18px;
  text-align: center;
}

.summary-label {
  display: block;
  color: var(--wk-text-muted);
  font-size: 12px;
  margin-bottom: 8px;
}

.summary-value {
  display: block;
  font-size: 28px;
  font-weight: 700;
  color: var(--wk-primary);
  font-family: 'JetBrains Mono', monospace;
}

.summary-online { color: #22c55e; }
.summary-offline { color: #ef4444; }

.summary-unit {
  display: block;
  color: var(--wk-text-muted);
  font-size: 11px;
  margin-top: 4px;
}

/* 服务器列表区域 */
.section-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin: 32px 0 18px;
}

.section-head h2 {
  margin: 0;
  font-size: 20px;
}

.section-hint {
  color: var(--wk-text-muted);
  font-size: 12px;
}

.server-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  padding-bottom: 64px;
}

.server-card {
  padding: 20px;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.server-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
}

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

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  border: 1px solid var(--wk-border);
  background: rgba(15, 23, 42, 0.56);
  white-space: nowrap;
}

.status-dot-inline {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
}

.status-dot-inline.online { background: #22c55e; box-shadow: 0 0 6px #22c55e; }
.status-dot-inline.offline { background: #ef4444; }
.status-dot-inline.stale,
.status-dot-inline.unknown { background: #f59e0b; }

.status-badge.online { color: #22c55e; }
.status-badge.offline { color: #ef4444; }
.status-badge.stale,
.status-badge.unknown { color: #f59e0b; }

/* 指标进度条 */
.metric-bars {
  display: grid;
  gap: 12px;
  margin: 16px 0;
}

.metric-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.metric-label {
  width: 36px;
  font-size: 12px;
  color: var(--wk-text-muted);
}

.metric-row .el-progress {
  flex: 1;
}

.metric-val {
  width: 52px;
  text-align: right;
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  font-weight: 600;
}

/* 卡片底部 */
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

.foot-icon.up { color: #22c55e; }
.foot-icon.down { color: #38bdf8; }

.foot-time {
  margin-left: auto;
}

/* 响应式 */
@media (max-width: 980px) {
  .summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .server-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
/* 页脚 */
.public-footer {
  text-align: center;
  padding: 24px 0 48px;
  color: var(--wk-text-muted);
  font-size: 12px;
  opacity: 0.6;
}
</style>
