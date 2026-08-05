<template>
  <!-- ============================================================
       公开服务器详情页：展示单台服务器脱敏后的实时状态和趋势
       设计参考 QuantKing：蓝色主色调 + 圆角卡片 + 双主题
       ============================================================ -->
  <div class="detail-page">
    <!-- 顶部导航：返回按钮 + 管理后台入口 -->
    <header class="detail-nav">
      <el-button text @click="router.push('/')">← 全部服务器</el-button>
      <el-button type="primary" plain @click="router.push(hasToken ? '/dashboard' : '/login')">
        {{ hasToken ? '管理后台' : '管理登录' }}
      </el-button>
    </header>

    <main class="detail-main">
      <!-- 加载骨架屏 -->
      <el-skeleton v-if="loading" :rows="8" animated />
      <!-- 错误提示 -->
      <el-result v-else-if="error" icon="warning" title="服务器不存在" :sub-title="error">
        <template #extra>
          <el-button type="primary" @click="router.push('/')">返回首页</el-button>
        </template>
      </el-result>

      <template v-else-if="server">
        <!-- 服务器 Hero 区域：状态药丸 + 名称 + 系统信息 + 最后活跃时间 -->
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

        <!-- 服务器规格网格 -->
        <section class="spec-grid wk-card-solid">
          <div v-for="item in serverSpecs" :key="item.label" class="spec-item">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </section>

        <!-- 当前指标卡片网格 -->
        <section class="metric-grid">
          <div v-for="item in currentMetrics" :key="item.label" class="metric-card wk-card">
            <span>{{ item.label }}</span>
            <strong class="wk-metric-value">{{ item.value }}</strong>
            <small>{{ item.hint }}</small>
          </div>
        </section>

        <!-- 资源趋势图：24h CPU / 内存 / 磁盘 -->
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

        <!-- 网络延迟图：24h Ping 延迟 -->
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
// ==================== 依赖导入 ====================
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import http from '@/utils/http'
import * as echarts from 'echarts'

// ==================== 类型定义 ====================
// 服务器公开信息接口（与公开 API 返回字段对齐）
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

// Ping ISP 接口
interface PingISP { name: string }

// 资源指标点接口
interface MetricPoint {
  timestamp: string
  cpu: number
  mem: number
  disk: number
  net_up: number
  net_down: number
}

// Ping 数据点接口
interface PingPoint {
  timestamp: string
  count: number
  avg_lat: number
  min_lat: number
  max_lat: number
  loss_rate: number
}

// ==================== 响应式状态 ====================
const route = useRoute()
const router = useRouter()
const server = ref<PublicServer | null>(null)       // 服务器详情数据
const metricPoints = ref<MetricPoint[]>([])         // 资源趋势数据点
const pingISPs = ref<PingISP[]>([])                 // Ping ISP 列表
const pingSeries = ref<Record<string, PingPoint[]>>({})  // Ping 延迟数据
const loading = ref(false)                          // 加载状态
const error = ref('')                               // 错误信息
const chartRef = ref<HTMLDivElement>()              // 资源趋势图 DOM 引用
const pingChartRef = ref<HTMLDivElement>()          // Ping 延迟图 DOM 引用
let chart: echarts.ECharts | null = null            // ECharts 资源图实例
let pingChart: echarts.ECharts | null = null        // ECharts Ping 图实例
let refreshTimer: ReturnType<typeof setInterval> | null = null
let metricsTimer: ReturnType<typeof setInterval> | null = null
let pingTimer: ReturnType<typeof setInterval> | null = null

// 是否已登录（决定按钮显示"管理后台"还是"管理登录"）
const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))
// 当前服务器 ID（从路由参数获取）
const serverID = computed(() => route.params.id as string)

// ==================== 计算属性 ====================

// 服务器规格列表（qio.ng 风格展示）
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

// 当前实时指标卡片
const currentMetrics = computed(() => [
  { label: 'CPU', value: formatPercent(server.value?.cpu), hint: '当前使用率' },
  { label: '内存', value: formatPercent(server.value?.mem), hint: '当前使用率' },
  { label: '磁盘', value: formatPercent(server.value?.disk), hint: '当前使用率' },
  { label: '上行', value: `${formatBytes(server.value?.net_up)}/s`, hint: '实时速率' },
  { label: '下行', value: `${formatBytes(server.value?.net_down)}/s`, hint: '实时速率' },
])

// ==================== 数据加载 ====================

// 加载站点主题（标题），公开详情页也需要显示站点标题
async function loadTheme() {
  try {
    const res = await http.get(`/api/public/theme?_=${Date.now()}`)
    if (res.data.title) {
      localStorage.setItem('site_title', res.data.title)
      document.title = `服务器详情 - ${res.data.title}`
    }
  } catch {}
}

// 加载服务器详情，使用公开接口不携带 JWT
async function loadServer(showLoading = false) {
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

// 加载 24h 资源趋势数据
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

// 加载 Ping 聚合数据
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

// ==================== ECharts 渲染 ====================

// 渲染资源趋势图（CPU / 内存 / 磁盘折线图）
// 配色使用新主题：#3b82f6(蓝) #34d399(绿) #fbbf24(橙)
function renderChart() {
  if (!chartRef.value || metricPoints.value.length === 0) return
  if (!chart) chart = echarts.init(chartRef.value, 'dark')
  const labels = metricPoints.value.map((item) => formatTime(item.timestamp))
  chart.setOption({
    animation: false,
    tooltip: {
      trigger: 'axis', confine: true, transitionDuration: 0,
      backgroundColor: 'rgba(38, 38, 38, 0.92)',
      borderColor: 'rgba(59, 130, 246, 0.3)',
    },
    legend: { textStyle: { color: 'var(--wk-text-muted)' } },
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    xAxis: {
      type: 'category', data: labels,
      axisLabel: { color: 'var(--wk-text-muted)' },
      axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
    },
    yAxis: {
      type: 'value', max: 100,
      axisLabel: { color: 'var(--wk-text-muted)' },
      splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
    },
    dataZoom: [{ type: 'inside', throttle: 80 }, { type: 'slider', height: 18, bottom: 4 }],
    series: [
      lineSeries('CPU', metricPoints.value.map((item) => item.cpu), '#3b82f6'),
      lineSeries('内存', metricPoints.value.map((item) => item.mem), '#34d399'),
      lineSeries('磁盘', metricPoints.value.map((item) => item.disk), '#fbbf24'),
    ],
  }, { notMerge: true, lazyUpdate: true })
}

// 渲染 Ping 延迟图
// 配色使用新主题：#3b82f6 #34d399 #fbbf24 #f87171 #8b5cf6 #14b8a6 #ec4899
function renderPingChart() {
  if (!pingChartRef.value || Object.keys(pingSeries.value).length === 0) return
  if (!pingChart) pingChart = echarts.init(pingChartRef.value, 'dark')
  const colorList = ['#3b82f6', '#34d399', '#fbbf24', '#f87171', '#8b5cf6', '#14b8a6', '#ec4899']
  const allTimestamps = Array.from(new Set(
    Object.values(pingSeries.value).flatMap(points => points.map(item => item.timestamp))
  )).sort()
  const labels = allTimestamps.map((item) => formatTime(item))

  // 构建每个 ISP 的延时和丢包率时间映射
  const ispLossByTime = new Map<string, Map<string, number>>()
  const series = Object.entries(pingSeries.value).map(([isp, points], index) => {
    const byTime = new Map<string, number>()
    const lossMap = new Map<string, number>()
    for (const item of points) {
      byTime.set(item.timestamp, Number(item.avg_lat || 0))
      lossMap.set(item.timestamp, Number(item.loss_rate || 0) * 100)
    }
    ispLossByTime.set(isp, lossMap)
    // 图例名称显示最新丢包率概览
    const lastPoint = points.length > 0 ? points[points.length - 1] : null
    const lossPercent = lastPoint ? (Number(lastPoint.loss_rate || 0) * 100).toFixed(1) : '0.0'
    const displayName = `${isp} ${lossPercent}%loss`
    return {
      ...lineSeries(displayName, allTimestamps.map(ts => byTime.has(ts) ? byTime.get(ts)! : null), colorList[index % colorList.length]),
      connectNulls: true,
      _ispName: isp,
    }
  })

  pingChart.setOption({
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      transitionDuration: 0,
      backgroundColor: 'rgba(38, 38, 38, 0.92)',
      borderColor: 'rgba(59, 130, 246, 0.3)',
      axisPointer: { type: 'cross', lineStyle: { color: 'rgba(59, 130, 246, 0.3)' } },
      // 自定义 tooltip：显示同一时间点所有运营商的延时和丢包率
      formatter: (params: any) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        let html = `<div style="font-size:12px;color:#a3a3a3;margin-bottom:6px;font-weight:600">${params[0].axisValue}</div>`
        params.forEach((p: any) => {
          if (p.value === null || p.value === undefined) return
          const color = p.color || '#3b82f6'
          const ispName = p.series?._ispName || p.seriesName
          const lossMap = ispLossByTime.get(ispName)
          const tsKey = allTimestamps[p.dataIndex]
          const lossPct = lossMap?.get(tsKey)?.toFixed(1) ?? '0.0'
          const lat = typeof p.value === 'number' ? `${p.value.toFixed(2)} ms` : `${p.value} ms`
          html += `<div style="display:flex;align-items:center;gap:8px;font-size:12px;line-height:22px">
            <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};flex-shrink:0"></span>
            <span style="color:#d4d4d4;min-width:80px">${ispName}</span>
            <span style="color:#3b82f6;font-weight:600;min-width:70px;text-align:right">${lat}</span>
            <span style="color:#fbbf24;font-size:11px;min-width:55px;text-align:right">${lossPct}% loss</span>
          </div>`
        })
        return html
      },
    },
    legend: { type: 'scroll', textStyle: { color: 'var(--wk-text-muted)' } },
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    xAxis: {
      type: 'category', data: labels,
      axisLabel: { color: 'var(--wk-text-muted)' },
      axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
    },
    yAxis: {
      type: 'value', name: 'ms',
      axisLabel: { color: 'var(--wk-text-muted)' },
      splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
    },
    dataZoom: [{ type: 'inside', throttle: 80 }, { type: 'slider', height: 18, bottom: 4 }],
    series,
  }, { notMerge: true, lazyUpdate: true })
}

// 折线图 series 工厂函数
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

// ==================== 工具函数 ====================

// 状态文本映射
function statusText(status: string) {
  return ({ online: '在线', offline: '离线', stale: '数据延迟', unknown: '未知' } as Record<string, string>)[status] || '未知'
}

// 格式化百分比
function formatPercent(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)}%` : '-'
}

// 格式化字节数
function formatBytes(value?: number) {
  if (!value) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index++
  }
  return `${size.toFixed(2)} ${units[index]}`
}

// 格式化运行时长
function formatDuration(seconds?: number) {
  if (!seconds) return '-'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return `${days} Days ${hours} Hours ${minutes} Min`
}

// 格式化 Unix 时间戳
function formatUnixTime(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

// CPU 信息文本
function cpuText(value?: PublicServer | null) {
  if (!value) return '-'
  const model = value.cpu_model || 'CPU'
  const cores = value.cpu_cores ? `${value.cpu_cores} Virtual Core` : ''
  return `${model}${cores ? ` ${cores}` : ''}`
}

// 负载信息文本
function loadText(value?: PublicServer | null) {
  if (!value) return '-'
  return `1m ${formatLoad(value.load1)} / 5m ${formatLoad(value.load5)} / 15m ${formatLoad(value.load15)}`
}

function formatLoad(value?: number) {
  return typeof value === 'number' ? value.toFixed(2) : '-'
}

// 系统信息文本
function systemText(value?: string) {
  if (!value) return '-'
  if (value.includes(' ')) return value.split(' ')[0]
  return value
}

// 架构信息文本
function archText(value?: string) {
  if (!value) return '-'
  if (value === 'amd64') return 'x86_64'
  if (value === 'arm64') return 'aarch64'
  return value
}

// 格式化日期时间
function formatDateTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

// 格式化时间（HH:MM:SS）
function formatTime(value: string) {
  const d = new Date(value)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
}

// 窗口大小变化时重绘图表
function handleResize() {
  chart?.resize()
  pingChart?.resize()
}

// ==================== 生命周期 ====================

onMounted(() => {
  // 初始化加载主题和服务器数据
  loadTheme()
  loadServer(true).then(() => loadPingAgg())
  loadMetrics()
  // 详情页当前指标每秒刷新；趋势图低频刷新
  refreshTimer = setInterval(() => loadServer(false), 1000)
  metricsTimer = setInterval(() => loadMetrics(), 60000)
  pingTimer = setInterval(() => loadPingAgg(), 60000)
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  // 清除定时器和 ECharts 实例，避免内存泄漏
  if (refreshTimer) clearInterval(refreshTimer)
  if (metricsTimer) clearInterval(metricsTimer)
  if (pingTimer) clearInterval(pingTimer)
  chart?.dispose()
  pingChart?.dispose()
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
/* 详情页容器：简洁渐变背景 */
.detail-page {
  min-height: 100vh;
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--wk-primary) 8%, transparent) 0%, transparent 280px),
    var(--wk-bg);
  color: var(--wk-text);
}

/* 导航栏和主内容区统一宽度居中 */
.detail-nav,
.detail-main {
  width: min(1100px, calc(100% - 32px));
  margin: 0 auto;
}

/* 导航栏 */
.detail-nav {
  height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* 服务器 Hero 区域 */
.server-hero {
  padding: 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.server-hero h1 {
  margin: 16px 0 8px;
  font-size: clamp(28px, 4vw, 44px);
  font-weight: 750;
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
  font-size: 20px;
  font-family: ui-monospace, 'JetBrains Mono', monospace;
}

/* 状态药丸 */
.status-pill {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 7px 12px;
  border: 1px solid var(--wk-border);
  background: var(--wk-bg-soft);
  font-size: 13px;
  font-weight: 600;
}

.status-pill.online { color: var(--wk-success); }
.status-pill.offline { color: var(--wk-danger); }
.status-pill.stale,
.status-pill.unknown { color: var(--wk-warning); }

/* 规格网格 */
.spec-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  padding: 20px;
  margin: 18px 0;
}

.spec-item {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.spec-item span {
  color: var(--wk-text-muted);
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: .03em;
}

.spec-item strong {
  font-size: 14px;
  word-break: break-word;
}

/* 当前指标卡片网格 */
.metric-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 16px;
  margin: 18px 0;
}

.metric-card {
  padding: 18px;
  margin-bottom: 0;
}

.metric-card strong {
  display: block;
  margin: 10px 0 6px;
}

/* 图表卡片 */
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
  font-size: 16px;
  font-weight: 650;
}

.chart {
  height: 380px;
}

/* 响应式 */
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
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .spec-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .metric-grid,
  .spec-grid {
    grid-template-columns: 1fr;
  }
}
</style>
