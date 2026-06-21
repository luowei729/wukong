<template>
  <!-- 公开服务器详情页：展示单台服务器脱敏后的实时状态和趋势 -->
  <div class="detail-page">
    <header class="detail-nav">
      <el-button text @click="router.push('/')">← 全部服务器</el-button>
      <el-button type="primary" plain @click="router.push(hasToken ? '/dashboard' : '/login')">
        {{ hasToken ? '管理后台' : '管理登录' }}
      </el-button>
    </header>

    <main class="detail-main">
      <el-skeleton v-if="loading" :rows="8" animated />
      <el-result v-else-if="error" icon="warning" title="服务器不存在" :sub-title="error">
        <template #extra>
          <el-button type="primary" @click="router.push('/')">返回首页</el-button>
        </template>
      </el-result>

      <template v-else-if="server">
        <section class="server-hero wk-card">
          <div>
            <div :class="['status-pill', server.status]">
              <span :class="['wk-status-dot', server.status === 'online' ? 'online' : 'offline']" />
              {{ statusText(server.status) }}
            </div>
            <h1>{{ server.name }}</h1>
            <p>{{ server.os_version || '系统信息待上报' }}</p>
          </div>
          <div class="hero-meta">
            <span>最后上报</span>
            <strong>{{ relativeTime(server.updated_at || server.last_seen_at) }}</strong>
          </div>
        </section>

        <section class="metric-grid">
          <div v-for="item in currentMetrics" :key="item.label" class="metric-card wk-card">
            <span>{{ item.label }}</span>
            <strong class="wk-metric-value">{{ item.value }}</strong>
            <small>{{ item.hint }}</small>
          </div>
        </section>

        <section class="chart-card wk-card-solid">
          <div class="chart-head">
            <div>
              <h2>资源趋势</h2>
              <p>最近 24 小时 CPU / 内存 / 磁盘使用率</p>
            </div>
            <el-button text @click="loadMetrics">刷新</el-button>
          </div>
          <el-empty v-if="metricPoints.length === 0" description="暂无趋势数据" />
          <div v-else ref="chartRef" class="chart" />
        </section>

        <section class="chart-card wk-card-solid">
          <div class="chart-head">
            <div>
              <h2>网络延迟</h2>
              <p>公开 Ping 数据需要指定 ISP，当前先展示占位，后续可自动发现线路。</p>
            </div>
          </div>
          <el-empty description="暂无公开 Ping 数据" />
        </section>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import * as echarts from 'echarts'

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

interface MetricPoint {
  timestamp: string
  cpu: number
  mem: number
  disk: number
  net_up: number
  net_down: number
}

const route = useRoute()
const router = useRouter()
const server = ref<PublicServer | null>(null)
const metricPoints = ref<MetricPoint[]>([])
const loading = ref(false)
const error = ref('')
const chartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))
const serverID = computed(() => route.params.id as string)

const currentMetrics = computed(() => [
  { label: 'CPU', value: formatPercent(server.value?.cpu), hint: '当前使用率' },
  { label: '内存', value: formatPercent(server.value?.mem), hint: '当前使用率' },
  { label: '磁盘', value: formatPercent(server.value?.disk), hint: '当前使用率' },
  { label: '上行', value: `${formatBytes(server.value?.net_up)}/s`, hint: '实时速率' },
  { label: '下行', value: `${formatBytes(server.value?.net_down)}/s`, hint: '实时速率' },
])

async function loadServer() {
  // 详情页使用公开接口，不携带管理 token，防止未登录访问被后台鉴权阻断。
  loading.value = true
  error.value = ''
  try {
    const res = await axios.get(`/api/public/servers/${serverID.value}`)
    server.value = res.data.server
    await loadMetrics()
  } catch (e: any) {
    error.value = e.response?.data?.error || '无法加载服务器详情'
  } finally {
    loading.value = false
  }
}

async function loadMetrics() {
  try {
    const res = await axios.get(`/api/public/servers/${serverID.value}/metrics?range=24h`)
    metricPoints.value = res.data.points || []
    await nextTick()
    renderChart()
  } catch {
    metricPoints.value = []
  }
}

function renderChart() {
  if (!chartRef.value || metricPoints.value.length === 0) return
  chart?.dispose()
  chart = echarts.init(chartRef.value, 'dark')
  const labels = metricPoints.value.map((item) => formatTime(item.timestamp))
  chart.setOption({
    tooltip: { trigger: 'axis', backgroundColor: 'rgba(15, 23, 42, 0.92)', borderColor: 'rgba(56, 189, 248, 0.3)' },
    legend: { textStyle: { color: 'var(--wk-text-muted)' } },
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    xAxis: { type: 'category', data: labels, axisLabel: { color: 'var(--wk-text-muted)' }, axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } } },
    yAxis: { type: 'value', max: 100, axisLabel: { color: 'var(--wk-text-muted)' }, splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } } },
    dataZoom: [{ type: 'inside' }, { type: 'slider', height: 18, bottom: 4 }],
    series: [
      lineSeries('CPU', metricPoints.value.map((item) => item.cpu), '#38bdf8'),
      lineSeries('内存', metricPoints.value.map((item) => item.mem), '#22c55e'),
      lineSeries('磁盘', metricPoints.value.map((item) => item.disk), '#f59e0b'),
    ],
  })
}

function lineSeries(name: string, data: number[], color: string) {
  return {
    name,
    type: 'line',
    data,
    smooth: true,
    symbol: 'none',
    lineStyle: { color, width: 1.8 },
    areaStyle: { color: `${color}22` },
  }
}

function statusText(status: string) {
  return ({ online: '在线', offline: '离线', stale: '数据延迟', unknown: '未知' } as Record<string, string>)[status] || '未知'
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

function formatTime(value: string) {
  const d = new Date(value)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function handleResize() {
  chart?.resize()
}

onMounted(() => {
  loadServer()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  chart?.dispose()
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.detail-page {
  min-height: 100vh;
  background: radial-gradient(circle at 80% 10%, rgba(56, 189, 248, 0.2), transparent 30%), var(--wk-bg);
  color: var(--wk-text);
}

.detail-nav,
.detail-main {
  width: min(1100px, calc(100% - 32px));
  margin: 0 auto;
}

.detail-nav {
  height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.server-hero {
  padding: 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.server-hero h1 {
  margin: 16px 0 8px;
  font-size: clamp(34px, 5vw, 56px);
}

.server-hero p,
.hero-meta span,
.metric-card span,
.metric-card small,
.chart-head p {
  color: var(--wk-text-muted);
}

.hero-meta {
  text-align: right;
}

.hero-meta strong {
  display: block;
  margin-top: 8px;
  font-size: 24px;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 7px 12px;
  border: 1px solid var(--wk-border);
  background: rgba(15, 23, 42, 0.62);
}

.status-pill.online { color: var(--wk-success); }
.status-pill.offline { color: var(--wk-danger); }
.status-pill.stale,
.status-pill.unknown { color: var(--wk-warning); }

.metric-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 16px;
  margin: 18px 0;
}

.metric-card {
  padding: 18px;
}

.metric-card strong {
  display: block;
  margin: 10px 0 6px;
}

.chart-card {
  padding: 24px;
  margin-top: 18px;
}

.chart-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.chart-head h2 {
  margin: 0 0 8px;
}

.chart {
  height: 380px;
}

@media (max-width: 860px) {
  .server-hero,
  .chart-head {
    flex-direction: column;
    align-items: flex-start;
  }
  .hero-meta {
    text-align: left;
  }
  .metric-grid {
    grid-template-columns: 1fr;
  }
}
</style>
