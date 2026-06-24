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
            <span>最后活跃</span>
            <strong>{{ formatDateTime(server.updated_at || server.last_seen_at) }}</strong>
          </div>
        </section>

        <section class="spec-grid wk-card-solid">
          <div v-for="item in serverSpecs" :key="item.label" class="spec-item">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
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
          <el-empty v-if="metricPoints.length === 0" description="趋势数据加载中或暂无数据" />
          <div v-else ref="chartRef" class="chart" />
        </section>

        <section class="chart-card wk-card-solid">
          <div class="chart-head">
            <div>
              <h2>网络延迟</h2>
              <p>最近 24 小时所有运营商 Ping 延迟（不同颜色区分线路）</p>
            </div>
          </div>
          <el-empty v-if="pingISPs.length === 0" description="暂无公开 Ping 线路" />
          <el-empty v-else-if="Object.keys(pingSeries).length === 0" description="暂无公开 Ping 数据" />
          <div v-else ref="pingChartRef" class="chart" />
        </section>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import http from '@/utils/http'
import * as echarts from 'echarts'

interface PublicServer {
  id: string
  name: string
  online: boolean
  status: 'online' | 'offline' | 'stale' | 'unknown'
  last_seen_at?: string
  updated_at?: string
  os_version?: string
  arch?: string
  cpu?: number
  mem?: number
  disk?: number
  net_up?: number
  net_down?: number
  uptime_seconds?: number
  boot_time?: number
  mem_total_bytes?: number
  disk_total_bytes?: number
  cpu_model?: string
  cpu_cores?: number
  load1?: number
  load5?: number
  load15?: number
  net_up_total_bytes?: number
  net_down_total_bytes?: number
  region?: string
  platform?: string
}

interface PingISP {
  name: string
}

interface MetricPoint {
  timestamp: string
  cpu: number
  mem: number
  disk: number
  net_up: number
  net_down: number
}

interface PingPoint {
  timestamp: string
  count: number
  avg_lat: number
  min_lat: number
  max_lat: number
  loss_rate: number
}

const route = useRoute()
const router = useRouter()
const server = ref<PublicServer | null>(null)
const metricPoints = ref<MetricPoint[]>([])
const pingISPs = ref<PingISP[]>([])
const pingSeries = ref<Record<string, PingPoint[]>>({})
const loading = ref(false)
const error = ref('')
const chartRef = ref<HTMLDivElement>()
const pingChartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null
let pingChart: echarts.ECharts | null = null
let refreshTimer: ReturnType<typeof setInterval> | null = null
let metricsTimer: ReturnType<typeof setInterval> | null = null
let pingTimer: ReturnType<typeof setInterval> | null = null

const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))
const serverID = computed(() => route.params.id as string)

const serverSpecs = computed(() => [
  { label: 'Status', value: statusText(server.value?.status || 'unknown') },
  { label: 'Uptime', value: formatDuration(server.value?.uptime_seconds) },
  { label: 'Arch', value: archText(server.value?.arch) },
  { label: 'Mem', value: formatBytes(server.value?.mem_total_bytes) },
  { label: 'Disk', value: formatBytes(server.value?.disk_total_bytes) },
  { label: 'Region', value: server.value?.region || '-' },
  { label: 'System', value: systemText(server.value?.platform || server.value?.os_version) },
  { label: 'CPU', value: cpuText(server.value) },
  { label: 'Load', value: loadText(server.value) },
  { label: 'Upload', value: formatBytes(server.value?.net_up_total_bytes) },
  { label: 'Download', value: formatBytes(server.value?.net_down_total_bytes) },
  { label: 'Boot time', value: formatUnixTime(server.value?.boot_time) },
  { label: 'Last active time', value: formatDateTime(server.value?.updated_at || server.value?.last_seen_at) },
])

const currentMetrics = computed(() => [
  { label: 'CPU', value: formatPercent(server.value?.cpu), hint: '当前使用率' },
  { label: '内存', value: formatPercent(server.value?.mem), hint: '当前使用率' },
  { label: '磁盘', value: formatPercent(server.value?.disk), hint: '当前使用率' },
  { label: '上行', value: `${formatBytes(server.value?.net_up)}/s`, hint: '实时速率' },
  { label: '下行', value: `${formatBytes(server.value?.net_down)}/s`, hint: '实时速率' },
])

async function loadTheme() {
  try {
    const res = await http.get(`/api/public/theme?_=${Date.now()}`)
    if (res.data.title) {
      localStorage.setItem('site_title', res.data.title)
      document.title = `服务器详情 - ${res.data.title}`
    }
  } catch {}
}

async function loadServer(showLoading = false) {
  // 详情页使用公开接口，不携带管理 token，防止未登录访问被后台鉴权阻断；定时刷新时静默更新，避免页面闪烁。
  if (showLoading) loading.value = true
  error.value = ''
  try {
    const res = await http.get(`/api/public/servers/${serverID.value}?_=${Date.now()}`)
    server.value = res.data.server
    pingISPs.value = res.data.ping_isps || []
  } catch (e: any) {
    error.value = e.response?.data?.error || '无法加载服务器详情'
  } finally {
    if (showLoading) loading.value = false
  }
}

async function loadMetrics() {
  try {
    const res = await http.get(`/api/public/servers/${serverID.value}/metrics?range=24h&step=60&_=${Date.now()}`)
    metricPoints.value = res.data.points || []
    await nextTick()
    renderChart()
  } catch {
    metricPoints.value = []
  }
}

async function loadPingAgg() {
  if (pingISPs.value.length === 0) {
    pingSeries.value = {}
    return
  }
  try {
    const results = await Promise.all(pingISPs.value.map(async (isp) => {
      const res = await http.get(`/api/public/servers/${serverID.value}/ping-agg`, {
        params: { isp: isp.name, range: '24h', _: Date.now() },
      })
      return [isp.name, res.data.points || []] as const
    }))
    pingSeries.value = Object.fromEntries(results.filter(([, points]) => points.length > 0))
    await nextTick()
    renderPingChart()
  } catch {
    pingSeries.value = {}
  }
}

function renderChart() {
  if (!chartRef.value || metricPoints.value.length === 0) return
  if (!chart) chart = echarts.init(chartRef.value, 'dark')
  const labels = metricPoints.value.map((item) => formatTime(item.timestamp))
  chart.setOption({
    animation: false,
    tooltip: { trigger: 'axis', confine: true, transitionDuration: 0, backgroundColor: 'rgba(15, 23, 42, 0.92)', borderColor: 'rgba(56, 189, 248, 0.3)' },
    legend: { textStyle: { color: 'var(--wk-text-muted)' } },
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    xAxis: { type: 'category', data: labels, axisLabel: { color: 'var(--wk-text-muted)' }, axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } } },
    yAxis: { type: 'value', max: 100, axisLabel: { color: 'var(--wk-text-muted)' }, splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } } },
    dataZoom: [{ type: 'inside', throttle: 80 }, { type: 'slider', height: 18, bottom: 4 }],
    series: [
      lineSeries('CPU', metricPoints.value.map((item) => item.cpu), '#38bdf8'),
      lineSeries('内存', metricPoints.value.map((item) => item.mem), '#22c55e'),
      lineSeries('磁盘', metricPoints.value.map((item) => item.disk), '#f59e0b'),
    ],
  }, { notMerge: true, lazyUpdate: true })
}

function renderPingChart() {
  if (!pingChartRef.value || Object.keys(pingSeries.value).length === 0) return
  if (!pingChart) pingChart = echarts.init(pingChartRef.value, 'dark')
  const colorList = ['#38bdf8', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6', '#14b8a6', '#ec4899']
  const allTimestamps = Array.from(new Set(
    Object.values(pingSeries.value).flatMap(points => points.map(item => item.timestamp))
  )).sort()
  const labels = allTimestamps.map((item) => formatTime(item))

  // 计算每个 ISP 的最新丢包率，用于图例名称显示（如"上海电信 2%loss"）
  const series = Object.entries(pingSeries.value).map(([isp, points], index) => {
    const byTime = new Map(points.map(item => [item.timestamp, item.avg_lat]))
    const lastPoint = points.length > 0 ? points[points.length - 1] : null
    const lossPercent = lastPoint ? (Number(lastPoint.loss_rate || 0) * 100).toFixed(1) : '0.0'
    const displayName = `${isp} ${lossPercent}%loss`
    return lineSeries(displayName, allTimestamps.map(ts => byTime.get(ts) ?? null), colorList[index % colorList.length])
  })

  pingChart.setOption({
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      transitionDuration: 0,
      backgroundColor: 'rgba(15, 23, 42, 0.92)',
      borderColor: 'rgba(56, 189, 248, 0.3)',
      // 自定义 tooltip 显示延时和丢包率
      formatter: (params: any) => {
        if (!Array.isArray(params)) return ''
        let html = `<div style="font-size:12px;color:#94a3b8;margin-bottom:4px">${params[0].axisValue}</div>`
        params.forEach((p: any) => {
          const color = p.color || '#38bdf8'
          const match = p.seriesName.match(/^(.+?)\s+([\d.]+)%loss$/)
          const ispName = match ? match[1] : p.seriesName
          const lossPct = match ? match[2] : '0.0'
          const lat = p.value !== null && p.value !== undefined ? `${p.value.toFixed(2)} ms` : '-'
          html += `<div style="display:flex;align-items:center;gap:6px;font-size:12px">
            <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color}"></span>
            <span style="color:#e2e8f0">${ispName}</span>
            <span style="color:#94a3b8;margin-left:auto">${lat}</span>
            <span style="color:#f59e0b;font-size:11px">${lossPct}%loss</span>
          </div>`
        })
        return html
      },
    },
    legend: { type: 'scroll', textStyle: { color: 'var(--wk-text-muted)' } },
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    xAxis: { type: 'category', data: labels, axisLabel: { color: 'var(--wk-text-muted)' }, axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } } },
    yAxis: { type: 'value', name: 'ms', axisLabel: { color: 'var(--wk-text-muted)' }, splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } } },
    dataZoom: [{ type: 'inside', throttle: 80 }, { type: 'slider', height: 18, bottom: 4 }],
    series,
  }, { notMerge: true, lazyUpdate: true })
}

function lineSeries(name: string, data: Array<number | null>, color: string) {
  return {
    name,
    type: 'line',
    data,
    smooth: false,
    sampling: 'lttb',
    large: true,
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
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  return `${size.toFixed(size >= 10 ? 2 : 2)} ${units[index]}`
}

function formatDuration(seconds?: number) {
  if (!seconds) return '-'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return `${days} Days ${hours} Hours ${minutes} Min`
}

function formatUnixTime(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function cpuText(value?: PublicServer | null) {
  if (!value) return '-'
  const model = value.cpu_model || 'CPU'
  const cores = value.cpu_cores ? `${value.cpu_cores} Virtual Core` : ''
  return `${model}${cores ? ` ${cores}` : ''}`
}

function loadText(value?: PublicServer | null) {
  if (!value) return '-'
  return `1m ${formatLoad(value.load1)} / 5m ${formatLoad(value.load5)} / 15m ${formatLoad(value.load15)}`
}

function formatLoad(value?: number) {
  return typeof value === 'number' ? value.toFixed(2) : '-'
}

function systemText(value?: string) {
  if (!value) return '-'
  if (value.includes(' ')) return value.split(' ')[0]
  return value
}

function archText(value?: string) {
  if (!value) return '-'
  if (value === 'amd64') return 'x86_64'
  if (value === 'arm64') return 'aarch64'
  return value
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
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
  pingChart?.resize()
}

onMounted(() => {
  loadTheme()
  loadServer(true).then(() => loadPingAgg())
  loadMetrics()
  // 详情页当前指标每秒刷新；趋势图使用后端降采样，保留自动加载和低频刷新。
  refreshTimer = setInterval(() => loadServer(false), 1000)
  metricsTimer = setInterval(() => loadMetrics(), 60000)
  pingTimer = setInterval(() => loadPingAgg(), 60000)
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  if (metricsTimer) clearInterval(metricsTimer)
  if (pingTimer) clearInterval(pingTimer)
  chart?.dispose()
  pingChart?.dispose()
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

.metric-grid,
.spec-grid {
  display: grid;
  gap: 16px;
  margin: 18px 0;
}

.metric-grid {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.spec-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  padding: 20px;
}

.metric-card {
  padding: 18px;
}

.spec-item {
  display: grid;
  gap: 8px;
  min-width: 0;
}

.spec-item span {
  color: var(--wk-text-muted);
  font-size: 12px;
}

.spec-item strong {
  font-size: 16px;
  word-break: break-word;
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
  .metric-grid,
  .spec-grid {
    grid-template-columns: 1fr;
  }
}
</style>
