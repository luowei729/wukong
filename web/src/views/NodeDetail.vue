<template>
  <!-- ============ 节点详情页 ============ -->
  <!-- 与公开详情页同源同口径，但这里额外提供采集配置与出口 IP 等后台专属信息 -->
  <div class="wk-stack">
    <!-- ---------------- Hero 行 ---------------- -->
    <div class="wk-detail-hero">
      <button type="button" class="wk-back-btn" aria-label="返回节点列表" @click="router.push('/nodes')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
          <path d="M15 18l-6-6 6-6" />
        </svg>
        节点列表
      </button>

      <div class="wk-hero-main">
        <div class="wk-hero-title">
          <span :class="['wk-status-pill', node?.online ? 'online' : 'offline']">
            <WkStatusDot :status="node?.online ? 'online' : 'offline'" :size="8" :pulse="node?.online" />
            {{ node?.online ? '在线' : '离线' }}
          </span>
          <h1 class="wk-hero-name">{{ nodeName }}</h1>
          <button type="button" class="wk-row-action" title="修改名称" aria-label="修改名称" @click="openRename">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 20h9" />
              <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z" />
            </svg>
          </button>
        </div>
        <!-- eyebrow 一行交代来源与身份：系统 · 区域 · 架构 · 探针版本 -->
        <div class="wk-eyebrow">{{ heroMeta }}</div>
      </div>

      <div class="wk-hero-side">
        <div class="wk-sub">最近上报</div>
        <div class="wk-num wk-hero-time">{{ relativeTime(node?.last_seen_at || node?.updated_at) }}</div>
      </div>
    </div>

    <!-- ---------------- 实时指标 KPI ---------------- -->
    <div class="wk-grid-4">
      <WkMetric
        label="CPU 使用率"
        :value="formatOneDecimal(node?.cpu)"
        unit="%"
        :tone="toneOf(node?.cpu)"
        :hint="node?.cpu_cores ? `${node.cpu_cores} 核 · ${formatLoad(node?.load1)} 负载(1m)` : '核心数待上报'"
      />
      <WkMetric
        label="内存使用率"
        :value="formatOneDecimal(node?.mem)"
        unit="%"
        :tone="toneOf(node?.mem)"
        :hint="memTotalText"
      />
      <WkMetric
        label="磁盘使用率"
        :value="formatOneDecimal(node?.disk)"
        unit="%"
        :tone="toneOf(node?.disk)"
        :hint="diskTotalText"
      />
      <WkMetric
        label="运行时长"
        :value="formatDuration(node?.uptime_seconds)"
        :hint="node?.boot_time ? `${formatDateTime(node.boot_time * 1000)} 启动` : '启动时间待上报'"
      />
      <WkMetric
        label="上行速率"
        :value="formatRate(node?.net_up)"
        tone="primary"
        :hint="`累计 ${formatBytes(node?.net_up_total_bytes)}`"
      />
      <WkMetric
        label="下行速率"
        :value="formatRate(node?.net_down)"
        tone="primary"
        :hint="`累计 ${formatBytes(node?.net_down_total_bytes)}`"
      />
      <WkMetric label="负载 1/5/15 分钟" :value="loadText(node)" hint="1 分钟负载接近核心数即视为满载" />
      <WkMetric
        label="累计流量"
        :value="formatBytes(totalTraffic)"
        hint="上行之和 + 下行之和（探针启动以来）"
      />
    </div>

    <!-- ---------------- 资源趋势 ---------------- -->
    <WkCard title="资源趋势" :subtitle="`${rangeLabel}内的 CPU / 内存 / 磁盘与网络速率`">
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

      <WkChart
        :builder="buildResourceOption"
        :deps="metricPoints"
        height="300px"
        :loading="metricsLoading"
      />
      <WkEmptyState
        v-if="!metricsLoading && metricPoints.length === 0"
        icon="box"
        title="暂无历史指标"
        description="该时间窗内没有查询到采集数据，可能是探针刚接入或采集频率较低"
      />
    </WkCard>

    <!-- ---------------- 网络质量（Ping K 线） ---------------- -->
    <WkCard title="网络质量" subtitle="最近 24 小时运营商线路延时（ms）与丢包，秒级原始数据聚合">
      <template #actions>
        <el-button text size="small" @click="router.push('/settings')">配置运营商</el-button>
      </template>

      <WkEmptyState
        v-if="ispTargets.length === 0"
        icon="search"
        title="尚未配置运营商 Ping 目标"
        description="到「系统设置 → Ping 运营商」新增并启用目标，探针会自动开始探测"
      >
        <template #action>
          <el-button type="primary" @click="router.push('/settings')">去配置</el-button>
        </template>
      </WkEmptyState>

      <template v-else>
        <WkEmptyState
          v-if="Object.keys(pingSeries).length === 0 && !pingLoading"
          icon="search"
          title="暂无 Ping 数据"
          description="已配置运营商目标，但最近 24 小时没有收到探测结果"
        />
        <!-- 叠加对比图 + 每线路统计摘要（均/最低/最高/丢包） -->
        <WkPingChart v-else :series="pingSeries" height="340px" :loading="pingLoading" />
      </template>
    </WkCard>

    <!-- ---------------- 服务器配置 + 系统信息 ---------------- -->
    <div class="wk-detail-cols">
      <WkCard title="采集配置" subtitle="写入 SQLite 固化，已安装探针重启后生效">
        <el-form label-position="top" class="wk-config-form" @submit.prevent="saveConfig">
          <el-form-item label="节点名称">
            <el-input v-model="configForm.name" placeholder="自定义节点名称" />
          </el-form-item>
          <el-form-item label="采集频率（秒）">
            <el-input-number v-model="configForm.collect_intv" :min="1" :max="3600" :step="1" />
          </el-form-item>
          <el-form-item label="Ping 频率（秒）">
            <el-input-number v-model="configForm.ping_intv" :min="1" :max="3600" :step="1" />
          </el-form-item>
          <el-form-item class="wk-config-actions">
            <el-button type="primary" :loading="savingConfig" @click="saveConfig">保存配置</el-button>
          </el-form-item>
        </el-form>
      </WkCard>

      <WkCard title="系统信息" subtitle="探针注册与上报的原始字段（仅后台可见）">
        <dl class="wk-kv">
          <div v-for="item in systemInfo" :key="item.label" class="wk-kv-item">
            <dt>{{ item.label }}</dt>
            <dd :class="item.mono ? 'wk-num' : ''">{{ item.value }}</dd>
          </div>
        </dl>
      </WkCard>
    </div>
  </div>
</template>

<script setup lang="ts">
// ============ 节点详情页逻辑 ============
// 数据来源：
//   实时指标   → useOverview() 共享单例（每秒刷新，无需本页单独轮询）
//   历史趋势   → GET /api/agents/{id}/metrics?since=&until=（60s 刷新）
//   网络质量   → GET /api/agents/{id}/ping-agg?isp=（60s 刷新）
//   配置保存   → PUT /api/agents/{id}（沿用原接口与字段）
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import WkCard from '@/components/WkCard.vue'
import WkChart from '@/components/WkChart.vue'
import WkEmptyState from '@/components/WkEmptyState.vue'
import WkMetric from '@/components/WkMetric.vue'
import WkPingChart from '@/components/WkPingChart.vue'
import WkStatusDot from '@/components/WkStatusDot.vue'
import http from '@/utils/http'
import { refreshOverview, useOverview } from '@/composables/useOverview'
import { usePolling } from '@/composables/usePolling'
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
  formatBytes,
  formatBytesShort,
  formatClock,
  formatDateTime,
  formatDuration,
  formatHourMinute,
  formatLoad,
  formatRate,
  loadLevel,
  loadText,
  relativeTime,
} from '@/utils/format'

const route = useRoute()
const router = useRouter()
const agentId = route.params.id as string

// 复用共享概览：详情页的实时卡片区不再单独发请求
const { nodes } = useOverview()

// 当前节点 = 共享数据里 id 匹配的合并对象（Agent 元数据 + LatestMetric 实时值）
const node = computed<any | null>(
  () => nodes.value.find((item: any) => item.id === agentId) || null
)

const nodeName = computed(() => {
  const current = node.value
  if (current) return current.name || current.hostname || `节点 ${agentId.slice(0, 8)}`
  return '节点详情'
})

// Hero  eyebrow：系统 · 区域 · 架构 · 探针版本
const heroMeta = computed(() => {
  const current = node.value
  if (!current) return '等待节点数据'
  const parts: string[] = []
  const os = current.platform || current.os_version
  if (os) parts.push(os)
  if (current.region) parts.push(current.region)
  if (current.arch) parts.push(archText(current.arch))
  if (current.agent_ver) parts.push(`探针 ${current.agent_ver}`)
  return parts.length ? parts.join(' · ') : '系统信息待上报'
})

// ---------------- 展示辅助 ----------------
function formatOneDecimal(value?: number | null): string {
  return typeof value === 'number' ? value.toFixed(1) : '-'
}

function toneOf(value?: number | null): 'default' | 'warning' | 'danger' {
  const level = loadLevel(value ?? undefined)
  if (level === 'danger') return 'danger'
  if (level === 'warning') return 'warning'
  return 'default'
}

// 内存/磁盘总量提示：使用率旁边给出绝对量，运维才能判断"80% 是 8G 还是 512G"
const memTotalText = computed(() => {
  const total = node.value?.mem_total_bytes
  const used = typeof node.value?.mem === 'number' && total ? (node.value.mem / 100) * total : null
  return used ? `已用 ${formatBytes(used)} / 共 ${formatBytes(total)}` : '总内存待上报'
})

const diskTotalText = computed(() => {
  const total = node.value?.disk_total_bytes
  const used = typeof node.value?.disk === 'number' && total ? (node.value.disk / 100) * total : null
  return used ? `已用 ${formatBytes(used)} / 共 ${formatBytes(total)}` : '总磁盘待上报'
})

// 累计上下行总量
const totalTraffic = computed(() => {
  const up = node.value?.net_up_total_bytes || 0
  const down = node.value?.net_down_total_bytes || 0
  return up + down
})

// 系统信息定义列表：把后端已有但过去未展示的字段全部摊开
const systemInfo = computed(() => {
  const current = node.value || {}
  return [
    { label: '主机名', value: current.hostname || '-', mono: false },
    { label: '节点 ID', value: agentId, mono: true },
    { label: '操作系统', value: current.platform || current.os_version || '-', mono: false },
    { label: '架构', value: current.arch ? archText(current.arch) : '-', mono: true },
    { label: 'CPU 型号', value: current.cpu_model || '-', mono: false },
    { label: 'CPU 核数', value: current.cpu_cores ? String(current.cpu_cores) : '-', mono: true },
    { label: '总内存', value: formatBytes(current.mem_total_bytes), mono: true },
    { label: '总磁盘', value: formatBytes(current.disk_total_bytes), mono: true },
    { label: '区域', value: current.region || '-', mono: false },
    { label: '探针版本', value: current.agent_ver || '-', mono: true },
    { label: '出口 IPv4', value: current.ip_v4 || '-', mono: true },
    { label: '出口 IPv6', value: current.ip_v6 || '-', mono: true },
    { label: '启动时间', value: current.boot_time ? formatDateTime(current.boot_time * 1000) : '-', mono: false },
    { label: '注册时间', value: current.created_at ? formatDateTime(current.created_at) : '-', mono: false },
  ]
})

// ---------------- 历史趋势 ----------------
// 时间窗用现有接口的 since/until 参数实现，不新增后端能力
const rangeOptions = [
  { label: '1 小时', value: '1h' as const, hours: 1 },
  { label: '6 小时', value: '6h' as const, hours: 6 },
  { label: '24 小时', value: '24h' as const, hours: 24 },
]

const range = ref<'1h' | '6h' | '24h'>('1h')
const metricPoints = ref<any[]>([])
const metricsLoading = ref(false)

const rangeLabel = computed(
  () => rangeOptions.find((item) => item.value === range.value)?.label || ''
)

function setRange(value: '1h' | '6h' | '24h') {
  range.value = value
  loadMetrics()
}

// 拉取历史指标：since/until 为 RFC3339，与后端 time.Parse(time.RFC3339) 对齐
async function loadMetrics() {
  metricsLoading.value = true
  try {
    const hours = rangeOptions.find((item) => item.value === range.value)?.hours || 1
    const until = new Date()
    const since = new Date(until.getTime() - hours * 3600 * 1000)
    const res = await http.get(`/api/agents/${agentId}/metrics`, {
      params: { since: since.toISOString(), until: until.toISOString(), _: Date.now() },
    })
    metricPoints.value = Array.isArray(res.data) ? res.data : []
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '加载历史指标失败')
    metricPoints.value = []
  } finally {
    metricsLoading.value = false
  }
}

// 资源趋势图：三条使用率线（左轴 %）+ 上下行速率（右轴 字节/秒）
function buildResourceOption() {
  const tokens = readChartTokens()
  const points = metricPoints.value
  // 短窗口用秒级标签，长窗口用分钟标签，避免轴标签重叠
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
          // 速率系列带单位，使用率系列带百分号
          const isNet = param.seriesName.includes('行')
          const text = isNet ? `${netFormatter(param.value)}/s` : `${Number(param.value).toFixed(1)}%`
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
        ...baseValueAxis(tokens, { name: '速率', formatter: (value: number) => netFormatter(value) }),
        splitLine: { show: false },
      },
    ],
    dataZoom: baseDataZoom(tokens),
    series: [
      buildLineSeries('CPU', points.map((item) => item.cpu), tokens.palette[0], { area: true }),
      // 内存用粉、下行用紫：两者与主色（蓝紫系）都能拉开色相
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

// ---------------- 网络质量 ----------------
const ispTargets = ref<any[]>([])
const pingSeries = ref<Record<string, any[]>>({})
const pingLoading = ref(false)

// 拉取启用中的运营商目标
async function loadISPTargets() {
  try {
    const res = await http.get(`/api/isp-targets?_=${Date.now()}`)
    ispTargets.value = (res.data || []).filter((item: any) => item.enabled)
  } catch (error) {
    console.error('加载 ISP 目标失败', error)
    ispTargets.value = []
  }
}

// 并行拉取每条线路的聚合数据，空线路不参与绘图，避免图例出现无意义项
async function loadPingAgg() {
  // 按节点作用域过滤：/api/isp-targets 已返回 scope / agent_ids，前端自己判断即可。
  // 不给被排除的线路（如不支持 IPv6 的节点上的 IPv6 线路）白跑一次 ping-agg 查询。
  const targets = ispTargets.value.filter((isp: any) => {
    if (isp.scope === 'include') return (isp.agent_ids || []).includes(agentId)
    if (isp.scope === 'exclude') return !(isp.agent_ids || []).includes(agentId)
    return true
  })
  if (targets.length === 0) {
    pingSeries.value = {}
    return
  }
  pingLoading.value = true
  try {
    const results = await Promise.all(
      targets.map(async (isp: any) => {
        const res = await http.get(`/api/agents/${agentId}/ping-agg`, {
          params: { isp: isp.name, _: Date.now() },
        })
        return [isp.name, Array.isArray(res.data) ? res.data : []] as const
      })
    )
    pingSeries.value = Object.fromEntries(results.filter(([, points]) => points.length > 0))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '加载 Ping 数据失败')
  } finally {
    pingLoading.value = false
  }
}

// ---------------- 采集配置 ----------------
const configForm = reactive({ name: '', collect_intv: 1, ping_intv: 1 })
const savingConfig = ref(false)
const configSynced = ref(false)

// 共享数据首次到位后，把当前节点配置回填到表单（只做一次，避免每秒覆盖用户输入）
function syncConfigForm() {
  if (configSynced.value || !node.value) return
  configForm.name = nodeName.value
  configForm.collect_intv = node.value.collect_intv || 1
  configForm.ping_intv = node.value.ping_intv || 1
  configSynced.value = true
}

// 保存：接口与字段保持不变（name / collect_intv / ping_intv）
async function saveConfig() {
  if (!configForm.name.trim()) {
    ElMessage.warning('节点名称不能为空')
    return
  }
  savingConfig.value = true
  try {
    await http.put(`/api/agents/${agentId}`, {
      name: configForm.name.trim(),
      collect_intv: configForm.collect_intv,
      ping_intv: configForm.ping_intv,
    })
    ElMessage.success('服务器配置已保存')
    // 立即刷新共享数据，顶栏与本页名称/频率同步最新值
    await refreshOverview()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存服务器配置失败')
  } finally {
    savingConfig.value = false
  }
}

// 改名：沿用原交互，保存后同步表单
async function openRename() {
  try {
    const { value } = await ElMessageBox.prompt('请输入新的服务器节点名称', '修改节点名称', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: nodeName.value,
      inputPattern: /^.{1,64}$/,
      inputErrorMessage: '节点名称长度必须为 1-64 个字符',
    })
    await http.put(`/api/agents/${agentId}`, { name: value })
    ElMessage.success('节点名称已保存')
    configForm.name = value
    await refreshOverview()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(error?.response?.data?.error || '修改节点名称失败')
    }
  }
}

// ---------------- 生命周期 ----------------
onMounted(async () => {
  syncConfigForm()
  await Promise.all([loadMetrics(), loadISPTargets()])
  await loadPingAgg()
})

// 历史趋势与 Ping 每分钟刷新一次即可，实时区由共享单例每秒更新
usePolling(loadMetrics, 60_000, { immediate: false })
usePolling(loadPingAgg, 60_000, { immediate: false })

// 共享数据到位后回填一次配置表单（节点对象首次出现时触发）
watch(
  () => node.value,
  () => syncConfigForm(),
  { immediate: true }
)
</script>

<style scoped>
/* Hero 区 */
.wk-detail-hero {
  display: flex;
  align-items: flex-start;
  gap: var(--wk-space-4);
  padding: var(--wk-space-4) 0 var(--wk-space-2);
}

.wk-back-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: var(--wk-ctl-md);
  padding: 0 10px 0 6px;
  margin-left: -6px;
  border: none;
  border-radius: var(--wk-radius-sm);
  background: transparent;
  color: var(--wk-text-muted);
  font-size: var(--wk-fs-base);
  transition: color var(--wk-dur-fast) var(--wk-ease),
    background var(--wk-dur-fast) var(--wk-ease);
}

.wk-back-btn:hover {
  color: var(--wk-text);
  background: var(--wk-bg-soft);
}

.wk-back-btn svg {
  width: 16px;
  height: 16px;
}

.wk-hero-main {
  flex: 1;
  min-width: 0;
}

.wk-hero-title {
  display: flex;
  align-items: center;
  gap: var(--wk-space-3);
  flex-wrap: wrap;
}

.wk-hero-name {
  font-size: var(--wk-fs-2xl);
  font-weight: 600;
  letter-spacing: -0.025em;
  line-height: 1.2;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 60vw;
}

.wk-hero-side {
  text-align: right;
  flex-shrink: 0;
}

.wk-hero-time {
  font-size: var(--wk-fs-md);
  color: var(--wk-text-secondary);
  margin-top: 2px;
}

/* 配置表单：三列栅格 + 保存按钮同行，比 inline form 更整齐 */
.wk-config-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 var(--wk-space-4);
}

.wk-config-form :deep(.el-form-item) {
  margin-bottom: var(--wk-space-4);
}

.wk-config-actions {
  grid-column: 1 / -1;
}

/* 双列区域：配置与系统信息并排，窄屏单列 */
.wk-detail-cols {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
  gap: var(--wk-gap-card);
  align-items: start;
}

@media (max-width: 1100px) {
  .wk-detail-cols {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 860px) {
  .wk-detail-hero {
    flex-wrap: wrap;
  }

  .wk-hero-side {
    text-align: left;
  }
}

/* 行内图标按钮（改名） */
.wk-row-action {
  width: 26px;
  height: 26px;
  border: none;
  border-radius: var(--wk-radius-xs);
  background: transparent;
  color: var(--wk-text-muted);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0.7;
  transition: all var(--wk-dur-fast) var(--wk-ease);
}

.wk-row-action svg {
  width: 15px;
  height: 15px;
}

.wk-row-action:hover {
  opacity: 1;
  background: var(--wk-bg-soft);
  color: var(--wk-text);
}
</style>
