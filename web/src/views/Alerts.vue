<template>
  <!-- ============ 告警中心 ============ -->
  <div class="wk-stack">
    <!-- ---------------- 概览计数 ---------------- -->
    <div class="wk-grid-4">
      <WkMetric
        label="进行中告警"
        :value="firingCount"
        unit="条"
        :tone="firingCount > 0 ? 'danger' : 'success'"
        :hint="firingCount > 0 ? '需要关注处理' : '当前没有触发中的告警'"
      />
      <WkMetric label="已恢复告警" :value="resolvedCount" unit="条" hint="历史告警记录" />
      <WkMetric label="今日新增" :value="todayAlerts.length" unit="条" hint="按触发时间统计（本地日期）" />
      <WkMetric
        label="涉及节点"
        :value="alertNodeCount"
        unit="个"
        :tone="alertNodeCount > 0 ? 'warning' : 'default'"
        :hint="`共 ${totalCount} 个节点`"
      />
    </div>

    <!-- ---------------- 筛选工具栏 ---------------- -->
    <div class="wk-toolbar">
      <div class="wk-segment" role="group" aria-label="状态筛选">
        <button
          v-for="option in statusOptions"
          :key="option.value"
          type="button"
          :class="['wk-segment-item', { active: statusFilter === option.value }]"
          @click="statusFilter = option.value"
        >
          {{ option.label }}
          <span class="wk-num">{{ option.count }}</span>
        </button>
      </div>

      <el-select v-model="metricFilter" size="small" style="width: 148px" aria-label="指标筛选">
        <el-option label="全部指标" value="all" />
        <el-option v-for="item in metricOptions" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>

      <div class="wk-search">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round">
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-3.5-3.5" />
        </svg>
        <input v-model="keyword" type="search" placeholder="搜索节点名称" aria-label="搜索节点" />
      </div>

      <span class="wk-sub" style="margin-left: auto">
        显示 {{ filteredAlerts.length }} / {{ alerts.length }} 条
      </span>
    </div>

    <!-- ---------------- 告警列表 ---------------- -->
    <WkCard padding="none">
      <div v-if="loading" style="padding: var(--wk-space-5)">
        <WkSkeleton v-for="i in 5" :key="i" height="18px" style="margin-bottom: 12px" />
      </div>

      <WkEmptyState
        v-else-if="alerts.length === 0"
        icon="shield"
        ok
        title="暂无告警"
        description="所有节点指标都在阈值范围内；阈值与持续时间可在「系统设置 → 告警阈值」调整"
      >
        <template #action>
          <el-button @click="router.push('/settings')">调整告警阈值</el-button>
        </template>
      </WkEmptyState>

      <WkEmptyState
        v-else-if="filteredAlerts.length === 0"
        icon="search"
        title="没有匹配的告警"
        description="调整状态、指标或节点搜索条件试试"
      />

      <el-table v-else :data="filteredAlerts" style="width: 100%" @row-click="goToNode">
        <!-- 状态 + 左侧 severity 色条：一眼区分进行中/已恢复 -->
        <el-table-column label="状态" width="112">
          <template #default="{ row }">
            <span :class="['wk-severity', row.status === 'firing' ? 'is-firing' : 'is-resolved']">
              <WkBadge :tone="row.status === 'firing' ? 'fail' : 'ok'" dot>
                {{ row.status === 'firing' ? '进行中' : '已恢复' }}
              </WkBadge>
            </span>
          </template>
        </el-table-column>

        <el-table-column label="指标" width="104">
          <template #default="{ row }">
            <span class="wk-metric-name">{{ metricText(row.metric) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="节点" min-width="160">
          <template #default="{ row }">
            <div class="wk-node-cell">
              <span class="wk-node-link">{{ row.agent_name || row.agent_id }}</span>
              <!-- 有自定义名称时附带 ID 前缀，便于和探针日志对齐 -->
              <span v-if="row.agent_name && row.agent_name !== row.agent_id" class="wk-num wk-node-id">
                {{ String(row.agent_id).slice(0, 8) }}
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="阈值" width="96" align="right">
          <template #default="{ row }">
            <span class="wk-num wk-sub">{{ formatValue(row.metric, row.threshold) }}</span>
          </template>
        </el-table-column>

        <!-- 当前值 + 超阈程度条：达到阈值为半格，超得越多越满 -->
        <el-table-column label="当前值 / 超阈程度" min-width="180">
          <template #default="{ row }">
            <div class="wk-value-cell">
              <span :class="['wk-num', isOver(row) ? 'red' : '']">
                {{ formatValue(row.metric, row.value) }}
              </span>
              <div class="wk-over-track">
                <div
                  :class="['wk-over-fill', isOver(row) ? 'is-over' : '']"
                  :style="{ width: `${overRatio(row)}%` }"
                />
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="触发时间" width="118">
          <template #default="{ row }">
            <span class="wk-num wk-sub" :title="formatDateTime(row.fired_at)">
              {{ relativeTime(row.fired_at) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="恢复时间" width="132">
          <template #default="{ row }">
            <span v-if="row.resolved_at" class="wk-num wk-sub">
              {{ relativeTime(row.resolved_at) }}
            </span>
            <span v-else class="wk-badge warn">持续 {{ formatDuration(survivalSeconds(row)) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </WkCard>
  </div>
</template>

<script setup lang="ts">
// ============ 告警中心逻辑 ============
// 告警列表来自 useOverview() 共享单例（每秒刷新），本页不再独立轮询 /api/alerts
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import WkBadge from '@/components/WkBadge.vue'
import WkCard from '@/components/WkCard.vue'
import WkEmptyState from '@/components/WkEmptyState.vue'
import WkMetric from '@/components/WkMetric.vue'
import WkSkeleton from '@/components/WkSkeleton.vue'
import { useOverview } from '@/composables/useOverview'
import { formatDateTime, formatDuration, relativeTime } from '@/utils/format'

const router = useRouter()
const { alerts, loading, firingCount, todayAlerts, alertNodeCount, totalCount } = useOverview()

// ---------------- 筛选状态 ----------------
const statusFilter = ref<'all' | 'firing' | 'resolved'>('all')
const metricFilter = ref('all')
const keyword = ref('')

const resolvedCount = computed(
  () => alerts.value.filter((item: any) => item.status === 'resolved').length
)

// 状态切换按钮附带数量，避免用户切完才发现是空的
const statusOptions = computed(() => [
  { label: '全部', value: 'all' as const, count: alerts.value.length },
  { label: '进行中', value: 'firing' as const, count: firingCount.value },
  { label: '已恢复', value: 'resolved' as const, count: resolvedCount.value },
])

// 指标字典：与后端 alert.metric 取值一一对应
const METRIC_TEXT: Record<string, string> = {
  offline: '离线',
  cpu: 'CPU',
  mem: '内存',
  disk: '磁盘',
  ping_latency: 'Ping 延迟',
  ping_loss: 'Ping 丢包',
}

const metricOptions = Object.entries(METRIC_TEXT).map(([value, label]) => ({ value, label }))

function metricText(metric: string): string {
  return METRIC_TEXT[metric] || metric
}

// 数值单位：离线用秒、Ping 延迟用 ms、其余按百分比
function formatValue(metric: string, value?: number): string {
  if (typeof value !== 'number') return '-'
  if (metric === 'offline') return value > 1 ? `${value.toFixed(0)} 秒` : '离线'
  if (metric === 'ping_latency') return `${value.toFixed(1)} ms`
  return `${value.toFixed(1)}%`
}

// 是否真的超阈：只有进行中且数值超过阈值才标红
function isOver(row: any): boolean {
  if (row.status !== 'firing') return false
  return typeof row.value === 'number' && typeof row.threshold === 'number'
    ? row.value > row.threshold
    : true
}

// 超阈程度：达到阈值显示半格（50%），达到两倍阈值填满
function overRatio(row: any): number {
  if (typeof row.value !== 'number' || typeof row.threshold !== 'number' || row.threshold <= 0) {
    return row.status === 'firing' ? 100 : 0
  }
  const ratio = (row.value / row.threshold) * 50
  return Math.max(0, Math.min(100, ratio))
}

// 告警存活时长：进行中用"现在 - 触发时间"，已恢复不显示
function survivalSeconds(row: any): number {
  const fired = new Date(row.fired_at || 0).getTime()
  if (Number.isNaN(fired)) return 0
  return Math.max(0, (Date.now() - fired) / 1000)
}

// 过滤逻辑：状态 → 指标 → 关键字（节点名或 ID）
const filteredAlerts = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  return alerts.value.filter((item: any) => {
    if (statusFilter.value !== 'all' && item.status !== statusFilter.value) return false
    if (metricFilter.value !== 'all' && item.metric !== metricFilter.value) return false
    if (text) {
      const haystack = `${item.agent_name || ''} ${item.agent_id || ''}`.toLowerCase()
      if (!haystack.includes(text)) return false
    }
    return true
  })
})

// 点击行进入节点详情，方便直接排查
function goToNode(row: any) {
  if (row?.agent_id) router.push(`/nodes/${row.agent_id}`)
}
</script>

<style scoped>
/* severity 色条：状态列左侧一条 3px 竖条，扫视时比读文字更快 */
.wk-severity {
  display: inline-flex;
  align-items: center;
  padding-left: var(--wk-space-3);
  border-left: 3px solid transparent;
}

.wk-severity.is-firing {
  border-left-color: var(--wk-danger);
}

.wk-severity.is-resolved {
  border-left-color: var(--wk-success);
}

.wk-metric-name {
  font-weight: 500;
  color: var(--wk-text);
}

.wk-node-cell {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.wk-node-link {
  font-weight: 500;
  color: var(--wk-text);
}

.wk-node-id {
  font-size: var(--wk-fs-xs);
  color: var(--wk-text-muted);
}

/* 当前值 + 超阈程度条 */
.wk-value-cell {
  display: flex;
  align-items: center;
  gap: var(--wk-space-3);
}

.wk-over-track {
  flex: 1;
  min-width: 60px;
  height: 4px;
  border-radius: var(--wk-radius-pill);
  background: color-mix(in srgb, var(--wk-text) 9%, transparent);
  overflow: hidden;
}

.wk-over-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--wk-text-muted);
  transition: width var(--wk-dur-base) var(--wk-ease);
}

.wk-over-fill.is-over {
  background: var(--wk-danger);
}

/* 搜索框样式与节点列表页保持一致 */
.wk-search {
  display: flex;
  align-items: center;
  gap: 8px;
  height: var(--wk-ctl-md);
  padding: 0 10px;
  min-width: 200px;
  background: var(--wk-bg-recess);
  border: 1px solid var(--wk-border);
  border-radius: var(--wk-radius-sm);
}

.wk-search:focus-within {
  border-color: var(--wk-primary);
  box-shadow: var(--wk-ring);
}

.wk-search svg {
  width: 15px;
  height: 15px;
  color: var(--wk-text-muted);
}

.wk-search input {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--wk-text);
  font-size: var(--wk-fs-base);
  font-family: inherit;
  outline: none;
}

@media (max-width: 860px) {
  .wk-search {
    min-width: 140px;
  }
}
</style>
