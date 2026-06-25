<template>
  <!-- 告警中心 -->
  <div class="alerts-page">
    <h2 style="margin-bottom: 20px; font-size: 20px;">告警中心</h2>

    <div class="wk-card-solid" style="padding: 20px;">
      <el-table :data="alertList" stripe style="width: 100%" v-loading="loading">
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'firing' ? 'danger' : 'success'" size="small">
              {{ row.status === 'firing' ? '进行中' : '已恢复' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="指标" width="120">
          <template #default="{ row }">{{ metricText(row.metric) }}</template>
        </el-table-column>
        <el-table-column label="阈值" width="100">
          <template #default="{ row }">{{ formatValue(row.metric, row.threshold) }}</template>
        </el-table-column>
        <el-table-column label="当前值" width="100">
          <template #default="{ row }">{{ formatValue(row.metric, row.value) }}</template>
        </el-table-column>
        <el-table-column label="触发时间" width="190">
          <template #default="{ row }">{{ formatTime(row.fired_at) }}</template>
        </el-table-column>
        <el-table-column label="恢复时间" width="190">
          <template #default="{ row }">{{ formatTime(row.resolved_at) }}</template>
        </el-table-column>
        <el-table-column label="探针" prop="agent_id" min-width="220" />
      </el-table>
      <div v-if="!loading && alertList.length === 0" style="text-align: center; padding: 40px; color: var(--wk-text-muted);">
        暂无告警
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import http from '@/utils/http'

const alertList = ref<any[]>([])
const loading = ref(false)
let refreshTimer: ReturnType<typeof setInterval> | null = null

async function fetchAlerts(showLoading = false) {
  if (showLoading) loading.value = true
  try {
    const res = await http.get(`/api/alerts?_=${Date.now()}`)
    // 后端无告警时可能返回 null；前端必须兜底成数组，避免表格闪现后因 .length 报错消失。
    alertList.value = Array.isArray(res.data) ? res.data : []
  } catch (e) {
    console.error('获取告警列表失败', e)
    alertList.value = []
  } finally {
    if (showLoading) loading.value = false
  }
}

function metricText(metric: string) {
  return ({ offline: '离线', cpu: 'CPU', mem: '内存', disk: '磁盘', ping_latency: 'Ping 延迟', ping_loss: 'Ping 丢包' } as Record<string, string>)[metric] || metric
}

function formatValue(metric: string, value?: number) {
  if (typeof value !== 'number') return '-'
  if (metric === 'offline') return value > 1 ? `${value.toFixed(0)} 秒` : '离线'
  if (metric === 'ping_latency') return `${value.toFixed(1)} ms`
  return `${value.toFixed(1)}%`
}

function formatTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(() => {
  fetchAlerts(true)
  // 告警中心也按秒刷新，配合 no-store 避免页面闪现旧状态后消失。
  refreshTimer = setInterval(() => fetchAlerts(false), 1000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>
