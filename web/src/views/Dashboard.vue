<template>
  <!-- ============ 总览仪表盘 ============ -->
  <!-- 结构：KPI 行 → 节点状态（卡片/列表双视图）→ 集群趋势 + 最近告警 -->
  <div class="wk-stack">
    <!-- ---------------- KPI 行 ---------------- -->
    <div class="wk-grid-4">
      <!-- 在线节点：全在线为绿色，有离线则降为警告色 -->
      <WkMetric
        label="在线节点"
        :value="onlineCount"
        :unit="`/ ${totalCount}`"
        :loading="loading"
        :tone="offlineCount > 0 ? 'warning' : 'success'"
        :hint="offlineCount > 0 ? `${offlineCount} 个节点离线` : '全部节点在线'"
        :spark="onlineHistory"
        :spark-color="'var(--wk-success)'"
      />
      <!-- 平均 CPU：阈值分级由 WkMetric 的 tone 体现，趋势来自前端滚动采样 -->
      <WkMetric
        label="平均 CPU"
        :value="formatOneDecimal(avgCpu)"
        unit="%"
        :loading="loading"
        :tone="toneOf(avgCpu)"
        :hint="`共 ${metricCount} 个节点上报指标`"
        :spark="cpuHistory"
        :spark-max="100"
        :delta="cpuDelta"
        :delta-tone="deltaTone(cpuDelta)"
      />
      <!-- 平均内存 -->
      <WkMetric
        label="平均内存"
        :value="formatOneDecimal(avgMem)"
        unit="%"
        :loading="loading"
        :tone="toneOf(avgMem)"
        :hint="avgDisk !== null ? `平均磁盘 ${formatOneDecimal(avgDisk)}%` : '暂无磁盘数据'"
        :spark="memHistory"
        :spark-max="100"
        :delta="memDelta"
        :delta-tone="deltaTone(memDelta)"
      />
      <!-- 今日告警：0 时显示"一切正常"的积极态 -->
      <WkMetric
        label="今日告警"
        :value="todayAlerts.length"
        unit="条"
        :loading="loading"
        :tone="firingCount > 0 ? 'danger' : todayAlerts.length > 0 ? 'warning' : 'default'"
        :hint="firingCount > 0 ? `${firingCount} 条进行中` : todayAlerts.length > 0 ? '今日告警均已恢复' : '今日无告警'"
      />
    </div>

    <!-- ---------------- 节点状态 ---------------- -->
    <WkCard
      title="节点状态"
      :subtitle="`共 ${totalCount} 个节点 · ${onlineCount} 个在线，每秒自动刷新`"
      padding="none"
    >
      <template #actions>
        <!-- 视图切换：卡片适合看趋势，列表适合看数值，选择结果本地持久化 -->
        <div class="wk-segment" role="tablist" aria-label="节点视图切换">
          <button
            type="button"
            :class="['wk-segment-item', { active: viewMode === 'card' }]"
            role="tab"
            aria-label="卡片视图"
            @click="setViewMode('card')"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
              <rect x="3" y="3" width="7" height="7" rx="1.5" />
              <rect x="14" y="3" width="7" height="7" rx="1.5" />
              <rect x="3" y="14" width="7" height="7" rx="1.5" />
              <rect x="14" y="14" width="7" height="7" rx="1.5" />
            </svg>
            卡片
          </button>
          <button
            type="button"
            :class="['wk-segment-item', { active: viewMode === 'list' }]"
            role="tab"
            aria-label="列表视图"
            @click="setViewMode('list')"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round">
              <path d="M4 6h16M4 12h16M4 18h16" />
            </svg>
            列表
          </button>
        </div>
      </template>

      <!-- 首次加载骨架：与真实卡片同尺寸，加载完成不跳动 -->
      <div v-if="loading" class="wk-node-grid" style="padding: var(--wk-space-5)">
        <div v-for="i in 4" :key="'sk-' + i" class="wk-node-card" style="cursor: default">
          <WkSkeleton width="120px" height="16px" />
          <WkSkeleton height="8px" />
          <WkSkeleton height="8px" />
          <WkSkeleton height="8px" />
          <WkSkeleton width="60%" height="12px" />
        </div>
      </div>

      <!-- 空状态：还没有探针接入 -->
      <WkEmptyState
        v-else-if="nodes.length === 0"
        icon="server"
        title="还没有节点接入"
        description="到「系统设置 → 安装节点」生成安装命令，在服务器上执行即可自动注册上报"
      >
        <template #action>
          <el-button type="primary" @click="router.push('/settings')">前往安装节点</el-button>
        </template>
      </WkEmptyState>

      <!-- 卡片视图 -->
      <div
        v-else-if="viewMode === 'card'"
        class="wk-node-grid"
        style="padding: 0 var(--wk-space-5) var(--wk-space-5)"
      >
        <article
          v-for="node in nodes"
          :key="node.id"
          :class="['wk-node-card', { 'is-offline': !node.online }]"
          @click="goToNode(node.id)"
        >
          <div class="wk-node-card-head">
            <div style="min-width: 0">
              <div class="wk-node-card-name">{{ displayName(node) }}</div>
              <div class="wk-eyebrow" style="margin-top: 3px">{{ nodeMeta(node) }}</div>
            </div>
            <WkStatusDot :status="node.online ? 'online' : 'offline'" :pulse="node.online" />
          </div>

          <div class="wk-node-card-meters">
            <WkProgressBar label="CPU" :value="node.cpu" />
            <WkProgressBar label="内存" :value="node.mem" />
            <WkProgressBar label="磁盘" :value="node.disk" />
          </div>

          <div class="wk-node-card-foot">
            <span class="up">↑ {{ formatRate(node.net_up) }}</span>
            <span class="down">↓ {{ formatRate(node.net_down) }}</span>
            <span class="wk-node-card-time">
              {{ node.uptime_seconds ? formatDuration(node.uptime_seconds) + '·' : ''
              }}{{ relativeTime(node.last_seen_at || node.updated_at) }}
            </span>
          </div>
        </article>
      </div>

      <!-- 列表视图：与节点列表页同一套数值口径，只是更紧凑 -->
      <el-table v-else :data="nodes" style="width: 100%" :row-class-name="rowClassName">
        <el-table-column label="状态" width="72" align="center">
          <template #default="{ row }">
            <WkStatusDot :status="row.online ? 'online' : 'offline'" :size="6" />
          </template>
        </el-table-column>
        <el-table-column label="名称" min-width="180">
          <template #default="{ row }">
            <span class="wk-node-name" @click="goToNode(row.id)">{{ displayName(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="CPU" min-width="150">
          <template #default="{ row }">
            <WkProgressBar :value="row.cpu" :show-value="true" />
          </template>
        </el-table-column>
        <el-table-column label="内存" min-width="150">
          <template #default="{ row }">
            <WkProgressBar :value="row.mem" />
          </template>
        </el-table-column>
        <el-table-column label="磁盘" min-width="150">
          <template #default="{ row }">
            <WkProgressBar :value="row.disk" />
          </template>
        </el-table-column>
        <el-table-column label="上行" width="110" align="right">
          <template #default="{ row }">
            <span class="wk-num">{{ formatRate(row.net_up) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="下行" width="110" align="right">
          <template #default="{ row }">
            <span class="wk-num">{{ formatRate(row.net_down) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="最近上报" width="120" align="right">
          <template #default="{ row }">
            <span class="wk-sub">{{ relativeTime(row.last_seen_at || row.updated_at) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </WkCard>

    <!-- ---------------- 集群趋势 + 最近告警 ---------------- -->
    <div class="wk-dash-bottom">
      <!-- 趋势图数据源是前端内存滚动采样（最近 10 分钟），不需要额外后端接口 -->
      <WkCard title="集群负载趋势" subtitle="最近 10 分钟平均 CPU / 内存（前端滚动采样）">
        <WkChart
          :builder="buildTrendOption"
          :deps="history"
          height="260px"
          :loading="loading"
        />
        <template #footer>
          <span>采样点 {{ history.length }} 个</span>
          <span style="margin-left: auto">
            更新于 {{ lastUpdated ? formatClock(lastUpdated) : '-' }}
          </span>
        </template>
      </WkCard>

      <!-- 最近告警：只列 5 条，更多进告警中心 -->
      <WkCard title="最近告警" subtitle="按触发时间倒序，最多显示 5 条">
        <template #actions>
          <el-button text size="small" @click="router.push('/alerts')">告警中心</el-button>
        </template>

        <WkEmptyState
          v-if="recentAlerts.length === 0"
          icon="shield"
          ok
          title="暂无告警记录"
          description="所有节点指标都在阈值范围内"
        />

        <ul v-else class="wk-alert-list">
          <li
            v-for="alert in recentAlerts"
            :key="alert.id"
            class="wk-alert-item"
            @click="goToNode(alert.agent_id)"
          >
            <WkBadge :tone="alert.status === 'firing' ? 'fail' : 'ok'" dot>
              {{ alert.status === 'firing' ? '进行中' : '已恢复' }}
            </WkBadge>
            <div class="wk-alert-body">
              <div class="wk-alert-title">
                {{ metricText(alert.metric) }} · {{ alert.agent_name || alert.agent_id }}
              </div>
              <div class="wk-sub">
                当前 <b class="wk-num">{{ formatValue(alert.metric, alert.value) }}</b>
                / 阈值 {{ formatValue(alert.metric, alert.threshold) }}
                · {{ relativeTime(alert.fired_at) }}
              </div>
            </div>
          </li>
        </ul>
      </WkCard>
    </div>
  </div>
</template>

<script setup lang="ts">
// ============ 总览仪表盘逻辑 ============
// 数据全部来自 useOverview() 共享单例：本页面不再自己写 setInterval 轮询，
// 与顶栏、节点列表页共用同一个 1s 定时器，减少重复请求（历史事故：多页重复轮询放大 SQLite 写锁竞争）
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import WkCard from '@/components/WkCard.vue'
import WkMetric from '@/components/WkMetric.vue'
import WkProgressBar from '@/components/WkProgressBar.vue'
import WkStatusDot from '@/components/WkStatusDot.vue'
import WkBadge from '@/components/WkBadge.vue'
import WkEmptyState from '@/components/WkEmptyState.vue'
import WkSkeleton from '@/components/WkSkeleton.vue'
import WkChart from '@/components/WkChart.vue'
import { useOverview } from '@/composables/useOverview'
import {
  baseCategoryAxis,
  baseDataZoom,
  baseGrid,
  baseLegend,
  baseTooltip,
  baseValueAxis,
  buildLineSeries,
  readChartTokens,
} from '@/utils/charts'
import {
  archText,
  formatClock,
  formatDuration,
  formatRate,
  loadLevel,
  relativeTime,
} from '@/utils/format'

const router = useRouter()

const {
  nodes,
  loading,
  history,
  lastUpdated,
  onlineCount,
  totalCount,
  offlineCount,
  avgCpu,
  avgMem,
  avgDisk,
  firingCount,
  todayAlerts,
  cpuHistory,
  memHistory,
  alerts,
} = useOverview()

// ---------------- 视图模式（卡片 / 列表） ----------------
const VIEW_KEY = 'wk-dashboard-view'
const viewMode = ref<'card' | 'list'>(
  localStorage.getItem(VIEW_KEY) === 'list' ? 'list' : 'card'
)

function setViewMode(mode: 'card' | 'list') {
  viewMode.value = mode
  // 记住用户偏好：运维在大屏看卡片、在窄屏看列表，刷新后保持
  localStorage.setItem(VIEW_KEY, mode)
}

// ---------------- 展示辅助 ----------------
// 节点显示名：优先后台自定义名称，其次主机名，最后截断 ID
function displayName(node: any): string {
  return node.name || node.hostname || `节点 ${String(node.id).slice(0, 8)}`
}

// 节点头部元信息：系统 · 区域 · 架构（与公开页同一口径）
function nodeMeta(node: any): string {
  const parts: string[] = []
  const os = node.platform || node.os_version
  if (os) parts.push(os)
  if (node.region) parts.push(node.region)
  if (node.arch) parts.push(archText(node.arch))
  return parts.length ? parts.join(' · ') : '系统信息待上报'
}

// 一位小数格式化：null 统一显示短横线
function formatOneDecimal(value: number | null): string {
  return value === null || value === undefined ? '-' : value.toFixed(1)
}

// KPI 卡语气色跟随统一负载阈值（≥70 警告 / ≥85 危险）
function toneOf(value: number | null): 'default' | 'warning' | 'danger' {
  const level = loadLevel(value ?? undefined)
  if (level === 'danger') return 'danger'
  if (level === 'warning') return 'warning'
  return 'default'
}

// 相对 1 分钟前的变化摘要（采样历史不足时不显示，避免误导）
function deltaText(series: number[]): string {
  if (series.length < 60) return ''
  const current = series[series.length - 1]
  const previous = series[series.length - 60]
  const diff = current - previous
  if (Math.abs(diff) < 0.05) return ''
  return `${diff > 0 ? '+' : ''}${diff.toFixed(1)}%`
}

const cpuDelta = computed(() => deltaText(cpuHistory.value))
const memDelta = computed(() => deltaText(memHistory.value))

function deltaTone(delta: string): 'ok' | 'fail' | 'neutral' {
  if (!delta) return 'neutral'
  // 负载上升视为风险（红），下降视为缓解（绿）
  return delta.startsWith('-') ? 'ok' : 'fail'
}

// 在线数趋势（用于"在线节点"卡的迷你条），无数据时不画
const onlineHistory = computed(() => history.value.map((item) => item.online))

// 有指标的节点数：作为"共 N 个节点上报指标"的说明
const metricCount = computed(
  () => nodes.value.filter((n: any) => typeof n.cpu === 'number').length
)

// 离线行弱化
function rowClassName({ row }: { row: any }) {
  return row.online ? '' : 'row-offline'
}

// ---------------- 最近告警 ----------------
const ALERT_METRIC_TEXT: Record<string, string> = {
  offline: '离线',
  cpu: 'CPU',
  mem: '内存',
  disk: '磁盘',
  ping_latency: 'Ping 延迟',
  ping_loss: 'Ping 丢包',
}

function metricText(metric: string): string {
  return ALERT_METRIC_TEXT[metric] || metric
}

// 告警数值单位：离线用秒、Ping 延迟用 ms、其余按百分比
function formatValue(metric: string, value?: number): string {
  if (typeof value !== 'number') return '-'
  if (metric === 'offline') return value > 1 ? `${value.toFixed(0)} 秒` : '离线'
  if (metric === 'ping_latency') return `${value.toFixed(1)} ms`
  return `${value.toFixed(1)}%`
}

// 按触发时间倒序取前 5 条
const recentAlerts = computed(() => {
  const list = alerts.value.slice()
  list.sort((a: any, b: any) => {
    const ta = new Date(a.fired_at || 0).getTime()
    const tb = new Date(b.fired_at || 0).getTime()
    return tb - ta
  })
  return list.slice(0, 5)
})

// ---------------- 集群趋势图 ----------------
// builder 在渲染时刻读取当前主题令牌，切浅色/改主色后图表颜色自动跟随
function buildTrendOption() {
  const tokens = readChartTokens()
  const points = history.value
  const labels = points.map((item) => formatClock(item.t))
  const cpu = points.map((item) => item.cpu)
  const mem = points.map((item) => item.mem)

  return {
    animation: false,
    // 只启用滚轮缩放，没有底部滑块，因此 grid 下方不需要额外让位
    grid: baseGrid({ bottom: 8 }),
    tooltip: baseTooltip(tokens),
    legend: baseLegend(tokens),
    xAxis: baseCategoryAxis(tokens, labels),
    yAxis: baseValueAxis(tokens, { max: 100, formatter: '{value}%' }),
    // 10 分钟窗口不需要缩放滑块，只保留滚轮缩放
    dataZoom: [baseDataZoom(tokens)[0]],
    series: [
      buildLineSeries('CPU', cpu, tokens.palette[0], { area: true }),
      // 第二色用粉：主色与紫/蓝同色相容易混淆，粉色在深浅两套主题下都能与主色拉开
      buildLineSeries('内存', mem, tokens.palette[6], { area: true }),
    ],
  }
}

// 跳转节点详情
function goToNode(id: string) {
  if (!id) return
  router.push(`/nodes/${id}`)
}
</script>

<style scoped>
/* 底部两栏：趋势图占主，告警列表占辅；窄屏单列 */
.wk-dash-bottom {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
  gap: var(--wk-gap-card);
}

@media (max-width: 1100px) {
  .wk-dash-bottom {
    grid-template-columns: 1fr;
  }
}

/* 表格里的节点名称可点击跳转 */
.wk-node-name {
  cursor: pointer;
  font-weight: 500;
  color: var(--wk-text);
}

.wk-node-name:hover {
  color: var(--wk-primary);
}

/* 最近告警列表 */
.wk-alert-list {
  display: flex;
  flex-direction: column;
  list-style: none;
  margin: calc(var(--wk-space-2) * -1) 0;
}

.wk-alert-item {
  display: flex;
  align-items: flex-start;
  gap: var(--wk-space-3);
  padding: var(--wk-space-3) 0;
  border-bottom: 1px solid var(--wk-border);
  cursor: pointer;
  transition: background var(--wk-dur-fast) var(--wk-ease);
}

.wk-alert-item:last-child {
  border-bottom: none;
}

.wk-alert-item:hover .wk-alert-title {
  color: var(--wk-primary);
}

.wk-alert-body {
  min-width: 0;
}

.wk-alert-title {
  font-size: var(--wk-fs-base);
  font-weight: 500;
  color: var(--wk-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
