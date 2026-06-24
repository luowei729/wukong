<template>
  <!-- 公开状态首页：未登录用户也可以查看脱敏后的服务器运行状态 -->
  <div class="public-page">
    <div class="glow glow-a" />
    <div class="glow glow-b" />

    <header class="public-nav">
      <div class="brand">
        <span class="brand-mark">悟</span>
        <div>
          <strong>wukong status</strong>
          <small>Public server monitor</small>
        </div>
      </div>
      <div class="nav-actions">
        <el-button text @click="loadData">刷新状态</el-button>
        <el-button type="primary" plain @click="goAdmin">
          {{ hasToken ? '管理后台' : '管理登录' }}
        </el-button>
      </div>
    </header>

    <main class="public-main">
      <section class="section-head">
        <div>
          <h2>服务器列表</h2>
        </div>
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
              <div class="server-meta">{{ server.os_version || '系统信息待上报' }}</div>
            </div>
            <span :class="['status-badge', server.status]">{{ statusText(server.status) }}</span>
          </div>
          <div class="metric-bars">
            <div class="metric-row">
              <span>CPU</span>
              <el-progress :percentage="metricPercent(server.cpu)" :show-text="false" />
              <strong>{{ formatPercent(server.cpu) }}</strong>
            </div>
            <div class="metric-row">
              <span>内存</span>
              <el-progress :percentage="metricPercent(server.mem)" :show-text="false" />
              <strong>{{ formatPercent(server.mem) }}</strong>
            </div>
            <div class="metric-row">
              <span>磁盘</span>
              <el-progress :percentage="metricPercent(server.disk)" :show-text="false" />
              <strong>{{ formatPercent(server.disk) }}</strong>
            </div>
          </div>
          <div class="server-foot">
            <span>↑ {{ formatBytes(server.net_up) }}/s</span>
            <span>↓ {{ formatBytes(server.net_down) }}/s</span>
            <span>{{ relativeTime(server.updated_at || server.last_seen_at) }}</span>
          </div>
        </article>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import http from '@/utils/http'

interface PublicServer {
  id: string
  name: string
  online: boolean
  status: 'online' | 'offline' | 'stale' | 'unknown'
  last_seen_at?: string
  updated_at?: string
  os_version?: string
  cpu?: number
  mem?: number
  disk?: number
  net_up?: number
  net_down?: number
}

const router = useRouter()
const loading = ref(false)
const servers = ref<PublicServer[]>([])
const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))
let refreshTimer: ReturnType<typeof setInterval> | null = null

async function loadData(showLoading = false) {
  // 公开首页只访问 /api/public/servers，不携带 JWT，确保未登录也可展示；定时刷新时不切 loading，避免每秒闪烁。
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

function statusText(status: string) {
  return ({ online: '在线', offline: '离线', stale: '数据延迟', unknown: '未知' } as Record<string, string>)[status] || '未知'
}

function metricPercent(value?: number) {
  return Math.max(0, Math.min(100, Math.round(value || 0)))
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

function relativeTime(value?: string) {
  if (!value) return '暂无上报'
  const diff = Date.now() - new Date(value).getTime()
  if (diff < 0) return '刚刚'
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds} 秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  return `${Math.floor(hours / 24)} 天前`
}

onMounted(() => {
  loadData(true)
  // 首页状态卡片每秒刷新一次，保证公开展示接近实时。
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

.brand small,
.server-meta,
.server-foot {
  color: var(--wk-text-muted);
}

.nav-actions {
  display: flex;
  gap: 10px;
}

.hero {
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: 24px;
  align-items: stretch;
  padding: 64px 0 32px;
}

.hero h1 {
  margin: 18px 0 14px;
  font-size: clamp(36px, 6vw, 72px);
  letter-spacing: -0.06em;
}

.hero p {
  max-width: 620px;
  line-height: 1.8;
}

.status-pill,
.status-badge {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 7px 12px;
  font-size: 13px;
  border: 1px solid var(--wk-border);
  background: rgba(15, 23, 42, 0.56);
}

.status-pill.online,
.status-badge.online { color: var(--wk-success); }
.status-pill.offline,
.status-badge.offline { color: var(--wk-danger); }
.status-pill.unknown,
.status-badge.unknown,
.status-badge.stale { color: var(--wk-warning); }

.hero-panel {
  padding: 24px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 10px;
}

.hero-panel strong {
  font-size: 28px;
}

.summary-grid,
.server-grid {
  display: grid;
  gap: 16px;
}

.summary-grid {
  grid-template-columns: repeat(6, minmax(0, 1fr));
}

.summary-card,
.server-card {
  padding: 18px;
}

.summary-card span,
.summary-card small,
.summary-card strong {
  display: block;
}

.summary-card strong {
  margin: 10px 0 6px;
}

.section-head {
  display: flex;
  justify-content: space-between;
  margin: 32px 0 18px;
}

.section-head h2 {
  margin: 0 0 8px;
}

.server-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  padding-bottom: 64px;
}

.server-card {
  cursor: pointer;
}

.server-card:hover {
  transform: translateY(-3px);
}

.server-card-head,
.server-foot,
.metric-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.server-name {
  font-size: 18px;
  font-weight: 700;
}

.metric-bars {
  display: grid;
  gap: 14px;
  margin: 24px 0;
}

.metric-row span {
  width: 42px;
  color: var(--wk-text-muted);
}

.metric-row .el-progress {
  flex: 1;
}

.metric-row strong {
  width: 58px;
  text-align: right;
  font-family: 'JetBrains Mono', monospace;
}

.server-foot {
  font-size: 12px;
  flex-wrap: wrap;
}

@media (max-width: 980px) {
  .hero,
  .summary-grid,
  .server-grid {
    grid-template-columns: 1fr;
  }
}
</style>
