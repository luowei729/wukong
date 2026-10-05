<template>
  <!-- ============================================================
       公开服务器详情页：未登录可见的单机状态与趋势
       与后台详情页共用图表封装与卡片类，区别只在数据源走 /api/public/*
       ============================================================ -->
  <div class="wk-public-shell">
    <!-- ---------------- 顶部导航 ---------------- -->
    <header class="wk-public-nav">
      <!-- 与首页同样：顶栏内容套进 1240px 居中容器，与下方正文左边缘对齐 -->
      <div class="wk-public-inner wk-public-nav-row">
        <button type="button" class="wk-back-btn" aria-label="返回全部服务器" @click="router.push('/')">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
            <path d="M15 18l-6-6 6-6" />
          </svg>
          <span class="wk-back-text">全部服务器</span>
        </button>
        <el-button type="primary" plain @click="router.push(hasToken ? '/dashboard' : '/login')">
          {{ hasToken ? '管理后台' : '管理登录' }}
        </el-button>
      </div>
    </header>

    <main class="wk-public-inner wk-public-main">
      <!-- ---------------- 加载与错误态 ---------------- -->
      <div v-if="loading" class="wk-stack">
        <WkSkeleton height="96px" rounded="var(--wk-radius-lg)" />
        <WkSkeleton height="120px" rounded="var(--wk-radius-lg)" />
        <WkSkeleton height="260px" rounded="var(--wk-radius-lg)" />
      </div>

      <WkEmptyState
        v-else-if="error"
        icon="search"
        :title="error"
        description="该服务器可能已被删除，或探针尚未完成注册"
      >
        <template #action>
          <el-button type="primary" @click="router.push('/')">返回首页</el-button>
        </template>
      </WkEmptyState>

      <template v-else-if="server">
        <!-- ---------------- Hero ---------------- -->
        <section class="wk-detail-hero">
          <div style="min-width: 0">
            <span :class="['wk-status-pill', server.status]">
              <WkStatusDot
                :status="dotStatus(server.status)"
                :size="8"
                :pulse="server.status === 'online'"
              />
              {{ statusText(server.status) }}
            </span>
            <h1 class="wk-hero-name">{{ server.name }}</h1>
            <div class="wk-eyebrow">{{ heroMeta }}</div>
          </div>
          <div class="wk-hero-side">
            <div class="wk-sub">最近活跃</div>
            <div class="wk-num wk-hero-time">
              {{ formatDateTime(server.updated_at || server.last_seen_at) }}
            </div>
          </div>
        </section>

        <!-- ---------------- 实时指标 ---------------- -->
        <section class="wk-grid-6">
          <WkMetric
            label="CPU"
            :value="formatOneDecimal(server.cpu)"
            unit="%"
            :tone="toneOf(server.cpu)"
            :hint="server.cpu_model || '型号待上报'"
          />
          <WkMetric
            label="内存"
            :value="formatOneDecimal(server.mem)"
            unit="%"
            :tone="toneOf(server.mem)"
            :hint="`共 ${formatBytes(server.mem_total_bytes)}`"
          />
          <WkMetric
            label="磁盘"
            :value="formatOneDecimal(server.disk)"
            unit="%"
            :tone="toneOf(server.disk)"
            :hint="`共 ${formatBytes(server.disk_total_bytes)}`"
          />
          <WkMetric label="上行" :value="formatRate(server.net_up)" tone="primary" :hint="`累计 ${formatBytes(server.net_up_total_bytes)}`" />
          <WkMetric label="下行" :value="formatRate(server.net_down)" tone="primary" :hint="`累计 ${formatBytes(server.net_down_total_bytes)}`" />
          <WkMetric
            label="运行时长"
            :value="formatDuration(server.uptime_seconds)"
            :hint="server.boot_time ? `${formatUnixTime(server.boot_time)} 启动` : '启动时间待上报'"
          />
        </section>

        <!-- ---------------- 资源趋势 ---------------- -->
        <WkCard title="资源趋势" :subtitle="`${rangeLabel}内 CPU / 内存 / 磁盘使用率与网络速率`">
          <template #actions>
            <div class="wk-chips">
              <button
                v-for="option in rangeOptions"
                :key="option.value"
                type="button"
                :class="['wk-chip', { active: range === option.value }]"
                @click="setRange(option.value)"
              >
                {{ option.label }}
              </button>
            </div>
          </template>

          <WkEmptyState
            v-if="!metricsLoading && metricPoints.length === 0"
            icon="box"
            title="暂无趋势数据"
            description="该时间窗内没有采集记录，稍后自动刷新"
          />
          <WkChart
            v-else
            :builder="buildResourceOption"
            :deps="metricPoints"
            height="300px"
            :loading="metricsLoading"
          />
        </WkCard>

        <!-- ---------------- 网络质量 ---------------- -->
        <WkCard
          title="网络质量"
          :subtitle="`最近 24 小时运营商线路延时（ms）；上方色条按分钟粒度显示丢包（${stripBucketLabel}）`"
        >
          <WkEmptyState
            v-if="pingISPs.length === 0"
            icon="search"
            title="暂无公开 Ping 线路"
            description="管理端配置并启用运营商 Ping 目标后，这里会显示延时与丢包"
          />
          <WkEmptyState
            v-else-if="Object.keys(pingSeries).length === 0"
            icon="search"
            title="暂无公开 Ping 数据"
            description="线路已配置，但最近 24 小时还没有探测结果"
          />

          <template v-else>
            <!-- 24h 丢包色条：Statuspage 风格，一格一分钟（按桶聚合到 120 格） -->
            <div class="wk-strips">
              <div v-for="row in lossStrips" :key="row.name" class="wk-strip-row">
                <span class="wk-strip-label" :title="row.name">{{ row.name }}</span>
                <div class="wk-strip">
                  <span
                    v-for="(cell, index) in row.cells"
                    :key="index"
                    :class="['wk-strip-cell', `is-${cell}`]"
                    :title="`${row.name} 第 ${index + 1} 段：${cellLabel(cell)}`"
                  />
                </div>
                <!-- 色条只负责“什么时段丢包”；延时统计已在下方分线路图标题里，不重复写 -->
                <span class="wk-strip-value">丢 {{ row.loss.toFixed(1) }}%</span>
              </div>
            </div>

            <!-- 分线路/叠加双模式：避免 5.4ms 与 5.6ms 这类接近的线路在同一纵轴上互相盖住 -->
            <WkPingChart :series="pingSeries" />
          </template>
        </WkCard>

        <!-- ---------------- 服务器规格（定义列表，一屏读完） ---------------- -->
        <WkCard title="服务器规格" subtitle="公开接口仅返回脱敏字段，不含出口 IP 与探针密钥">
          <dl class="wk-kv">
            <div v-for="item in serverSpecs" :key="item.label" class="wk-kv-item">
              <dt>{{ item.label }}</dt>
              <dd :class="item.mono ? 'wk-num' : ''">{{ item.value }}</dd>
            </div>
          </dl>
        </WkCard>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
// ==================== 公开详情页逻辑 ====================
// 数据源：/api/public/servers/{id}（状态与规格）、/metrics（趋势）、/ping-agg（网络质量）
// 全部为脱敏只读接口，不携带 JWT
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import WkCard from '@/components/WkCard.vue'
import WkChart from '@/components/WkChart.vue'
import WkEmptyState from '@/components/WkEmptyState.vue'
import WkMetric from '@/components/WkMetric.vue'
import WkPingChart from '@/components/WkPingChart.vue'
import WkSkeleton from '@/components/WkSkeleton.vue'
import WkStatusDot from '@/components/WkStatusDot.vue'
import http from '@/utils/http'
import { usePolling } from '@/composables/usePolling'
import { useTheme } from '@/composables/useTheme'
import {
  baseCategoryAxis,
  baseDataZoom,
  baseGrid,
  baseLegend,
  baseTooltip,
  baseValueAxis,
  buildLineSeries,
  readChartTokens,
  tooltipRow,
} from '@/utils/charts'
import {
  archText,
  average,
  cpuText,
  formatBytes,
  formatBytesShort,
  formatClock,
  formatDateTime,
  formatDuration,
  formatHourMinute,
  formatRate,
  formatUnixTime,
  loadLevel,
  loadText,
  lossPercent,
  statusText,
} from '@/utils/format'

// ==================== 类型定义 ====================
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

// ==================== 状态 ====================
const route = useRoute()
const router = useRouter()
const { load: loadTheme } = useTheme()

const server = ref<PublicServer | null>(null)
const metricPoints = ref<MetricPoint[]>([])
const pingISPs = ref<Array<{ name: string }>>([])
const pingSeries = ref<Record<string, PingPoint[]>>({})
const loading = ref(true)
const metricsLoading = ref(false)
const error = ref('')

const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))
const serverID = computed(() => route.params.id as string)

// ==================== Hero 与派生展示 ====================
const heroMeta = computed(() => {
  const item = server.value
  if (!item) return ''
  const parts: string[] = []
  const os = item.platform || item.os_version
  if (os) parts.push(os)
  if (item.region) parts.push(item.region)
  if (item.arch) parts.push(archText(item.arch))
  if (item.cpu_cores) parts.push(`${item.cpu_cores} 核`)
  return parts.join(' · ')
})

// 规格定义列表：沿用原 13 项内容，仅改为 label/value 双列排版
const serverSpecs = computed(() => {
  const item = server.value
  return [
    { label: '状态', value: statusText(item?.status || 'unknown'), mono: false },
    { label: '运行时长', value: formatDuration(item?.uptime_seconds), mono: true },
    { label: '架构', value: archText(item?.arch), mono: true },
    { label: '内存总量', value: formatBytes(item?.mem_total_bytes), mono: true },
    { label: '磁盘总量', value: formatBytes(item?.disk_total_bytes), mono: true },
    { label: '区域', value: item?.region || '-', mono: false },
    { label: '系统', value: item?.platform || item?.os_version || '-', mono: false },
    { label: 'CPU', value: cpuText(item), mono: false },
    { label: '负载 1/5/15', value: loadText(item), mono: true },
    { label: '累计上行', value: formatBytes(item?.net_up_total_bytes), mono: true },
    { label: '累计下行', value: formatBytes(item?.net_down_total_bytes), mono: true },
    { label: '启动时间', value: formatUnixTime(item?.boot_time), mono: false },
    { label: '最近活跃', value: formatDateTime(item?.updated_at || item?.last_seen_at), mono: false },
  ]
})

function formatOneDecimal(value?: number | null): string {
  return typeof value === 'number' ? value.toFixed(1) : '-'
}

function toneOf(value?: number): 'default' | 'warning' | 'danger' {
  const level = loadLevel(value)
  if (level === 'danger') return 'danger'
  if (level === 'warning') return 'warning'
  return 'default'
}

function dotStatus(status: string): 'online' | 'offline' | 'stale' | 'muted' {
  if (status === 'online') return 'online'
  if (status === 'stale') return 'stale'
  if (status === 'offline') return 'offline'
  return 'muted'
}

// ==================== 数据加载 ====================
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

// 趋势时间窗：range 直接透传给后端 time.ParseDuration
const rangeOptions = [
  { label: '1 小时', value: '1h' as const },
  { label: '6 小时', value: '6h' as const },
  { label: '24 小时', value: '24h' as const },
]
const range = ref<'1h' | '6h' | '24h'>('24h')
const rangeLabel = computed(
  () => rangeOptions.find((item) => item.value === range.value)?.label || ''
)

function setRange(value: '1h' | '6h' | '24h') {
  range.value = value
  loadMetrics()
}

async function loadMetrics() {
  metricsLoading.value = true
  try {
    const res = await http.get(
      `/api/public/servers/${serverID.value}/metrics?range=${range.value}&step=60&_=${Date.now()}`
    )
    metricPoints.value = res.data.points || []
  } catch {
    metricPoints.value = []
  } finally {
    metricsLoading.value = false
  }
}

async function loadPingAgg() {
  if (pingISPs.value.length === 0) {
    pingSeries.value = {}
    return
  }
  try {
    const results = await Promise.all(
      pingISPs.value.map(async (isp) => {
        const res = await http.get(`/api/public/servers/${serverID.value}/ping-agg`, {
          params: { isp: isp.name, range: '24h', _: Date.now() },
        })
        return [isp.name, (res.data.points || []) as PingPoint[]] as const
      })
    )
    pingSeries.value = Object.fromEntries(results.filter(([, points]) => points.length > 0))
  } catch {
    pingSeries.value = {}
  }
}

// ==================== 资源趋势图 ====================
function buildResourceOption() {
  const tokens = readChartTokens()
  const points = metricPoints.value
  const labeler = range.value === '1h' ? formatClock : formatHourMinute
  const labels = points.map((item) => labeler(item.timestamp))
  const netFormatter = (value: number) => formatBytesShort(value)

  return {
    animation: false,
    grid: baseGrid(),
    tooltip: {
      ...baseTooltip(tokens),
      formatter: (params: any) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        let html = `<div style="font-size:11px;opacity:.7;margin-bottom:6px;font-weight:600">${params[0].axisValue}</div>`
        params.forEach((param: any) => {
          if (param.value === null || param.value === undefined) return
          const isNet = param.seriesName.includes('行')
          const text = isNet
            ? `${netFormatter(param.value)}/s`
            : `${Number(param.value).toFixed(1)}%`
          html += tooltipRow(param.color, param.seriesName, text)
        })
        return html
      },
    },
    legend: baseLegend(tokens),
    xAxis: baseCategoryAxis(tokens, labels),
    yAxis: [
      baseValueAxis(tokens, { max: 100, formatter: '{value}%' }),
      {
        ...baseValueAxis(tokens, { formatter: (value: number) => netFormatter(value) }),
        splitLine: { show: false },
      },
    ],
    dataZoom: baseDataZoom(tokens),
    series: [
      buildLineSeries('CPU', points.map((item) => item.cpu), tokens.palette[0], { area: true }),
      // 内存粉 / 磁盘橙 / 上行青 / 下行紫：与主色拉开色相，主色自定义时也分得清
      buildLineSeries('内存', points.map((item) => item.mem), tokens.palette[6]),
      buildLineSeries('磁盘', points.map((item) => item.disk), tokens.palette[2]),
      buildLineSeries('上行', points.map((item) => item.net_up), tokens.palette[4], {
        width: 1.2,
        yAxisIndex: 1,
      }),
      buildLineSeries('下行', points.map((item) => item.net_down), tokens.palette[5], {
        width: 1.2,
        yAxisIndex: 1,
      }),
    ],
  }
}

// ==================== Ping 延时图 + 丢包色条 ====================
// 色条格数：24h 按分钟最多 1440 个点，聚合到 120 格（每格约 12 分钟）才在窄屏也读得清
const STRIP_CELLS = 120
const stripBucketLabel = computed(() => `每格约 ${Math.round((24 * 60) / STRIP_CELLS)} 分钟`)

interface LossStripRow {
  name: string
  cells: Array<'ok' | 'warn' | 'bad' | 'none'>
  avgLat: number
  loss: number
}

// 一格取桶内最差丢包：有丢包但不足一半算 warn，半数以上算 bad
function buildLossStrip(points: PingPoint[]): Array<'ok' | 'warn' | 'bad' | 'none'> {
  if (points.length === 0) return []
  const sorted = points.slice().sort((a, b) => a.timestamp.localeCompare(b.timestamp))
  const start = new Date(sorted[0].timestamp).getTime()
  const end = new Date(sorted[sorted.length - 1].timestamp).getTime()
  const span = Math.max(1, end - start)
  const bucketMs = span / STRIP_CELLS
  const buckets: number[][] = Array.from({ length: STRIP_CELLS }, () => [] as number[])

  for (const point of sorted) {
    const offset = new Date(point.timestamp).getTime() - start
    const index = Math.min(STRIP_CELLS - 1, Math.floor(offset / bucketMs))
    buckets[index].push(lossPercent(point.loss_rate))
  }

  return buckets.map((bucket) => {
    if (bucket.length === 0) return 'none'
    const worst = Math.max(...bucket)
    if (worst <= 0) return 'ok'
    if (worst < 50) return 'warn'
    return 'bad'
  })
}

const lossStrips = computed<LossStripRow[]>(() => {
  return Object.entries(pingSeries.value).map(([name, points]) => ({
    name,
    cells: buildLossStrip(points),
    avgLat: average(points.map((item) => Number(item.avg_lat || 0))) ?? 0,
    loss: average(points.map((item) => lossPercent(Number(item.loss_rate || 0)))) ?? 0,
  }))
})

function cellLabel(cell: string): string {
  if (cell === 'ok') return '无丢包'
  if (cell === 'warn') return '部分丢包'
  if (cell === 'bad') return '严重丢包'
  return '无数据'
}

// ==================== 生命周期 ====================
// 实时指标每秒刷新；趋势与 Ping 每分钟刷新（历史曲线不需要秒级粒度）
onMounted(async () => {
  loadTheme()
  await loadServer(true)
  await Promise.all([loadMetrics(), loadPingAgg()])
})

usePolling(() => loadServer(false), 1000)
usePolling(loadMetrics, 60_000, { immediate: false })
usePolling(loadPingAgg, 60_000, { immediate: false })
</script>

<style scoped>
.wk-detail-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wk-space-5);
  padding: var(--wk-space-6) 0 var(--wk-space-4);
}

.wk-back-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: var(--wk-ctl-md);
  padding: 0 10px 0 4px;
  border: none;
  border-radius: var(--wk-radius-sm);
  background: transparent;
  color: var(--wk-text-muted);
  font-size: var(--wk-fs-base);
}

.wk-back-btn:hover {
  color: var(--wk-text);
  background: var(--wk-bg-soft);
}

.wk-back-btn svg {
  width: 16px;
  height: 16px;
}

.wk-hero-name {
  font-size: var(--wk-fs-3xl);
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.15;
  margin: var(--wk-space-3) 0 var(--wk-space-2);
}

.wk-hero-side {
  text-align: right;
  flex-shrink: 0;
}

.wk-hero-time {
  font-size: var(--wk-fs-base);
  color: var(--wk-text-secondary);
  margin-top: 2px;
}

/* 丢包色条区与图表之间留一档间距，避免视觉上粘连 */
.wk-strips {
  margin-bottom: var(--wk-space-4);
}

/* 统计文本强制单行，窄屏下也不会出现行尾多一个分隔符的挂列 */
.wk-strip-value {
  width: 168px;
  white-space: nowrap;
}

@media (max-width: 640px) {
  /* 详情页首屏：手机上收紧留白与字号，让实时指标卡能进入首屏，
     否则只能看到节点名与时间，要滚一屏才能看到数据 */
  .wk-detail-hero {
    padding: var(--wk-space-4) 0 var(--wk-space-3);
    gap: var(--wk-space-3);
  }

  .wk-hero-name {
    font-size: var(--wk-fs-xl);
  }

  .wk-hero-time {
    font-size: var(--wk-fs-base);
  }
}

@media (max-width: 860px) {
  .wk-detail-hero {
    flex-direction: column;
  }

  .wk-hero-side {
    text-align: left;
  }
}
</style>
