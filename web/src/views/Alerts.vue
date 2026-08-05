<template>
  <!-- ===== 告警中心页面 ===== -->
  <div class="alerts-page">
    <!-- 页面头部：标题 + 进行中/已恢复告警统计 -->
    <header class="wk-page-header">
      <h2>告警中心</h2>
      <div class="wk-stat-row" style="grid-template-columns: repeat(2, minmax(0, 1fr));">
        <!-- 进行中告警数（firing 状态） -->
        <div class="wk-metric">
          <div class="label">进行中告警</div>
          <div class="value red">{{ firingCount }}</div>
          <div class="sub">需要关注处理</div>
        </div>
        <!-- 已恢复告警数（resolved 状态） -->
        <div class="wk-metric">
          <div class="label">已恢复告警</div>
          <div class="value green">{{ resolvedCount }}</div>
          <div class="sub">历史告警记录</div>
        </div>
      </div>
    </header>

    <!-- 告警列表卡片：包裹 el-table -->
    <div class="wk-card-solid alert-table-card">
      <!-- 使用 Element Plus 表格展示告警列表 -->
      <el-table
        :data="alertList"
        v-loading="loading"
        style="width: 100%"
        :empty-text="'暂无告警记录'"
        row-key="id"
      >
        <!-- 状态列：用 wk-badge 显示 firing/resolved -->
        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <!-- firing=红色徽章，resolved=绿色徽章 -->
            <span
              :class="['wk-badge', row.status === 'firing' ? 'fail' : 'ok']"
            >
              <span
                class="wk-status-dot"
                :class="row.status === 'firing' ? 'alert' : 'online'"
                style="margin-right: 5px;"
              />
              {{ row.status === 'firing' ? '进行中' : '已恢复' }}
            </span>
          </template>
        </el-table-column>

        <!-- 指标列：metricText 中文映射 -->
        <el-table-column label="指标" width="130">
          <template #default="{ row }">
            <span class="metric-cell">{{ metricText(row.metric) }}</span>
          </template>
        </el-table-column>

        <!-- 阈值列：formatValue 格式化 -->
        <el-table-column label="阈值" width="110" align="right">
          <template #default="{ row }">
            <span class="threshold-cell">{{ formatValue(row.metric, row.threshold) }}</span>
          </template>
        </el-table-column>

        <!-- 当前值列：超过阈值时高亮红色 -->
        <el-table-column label="当前值" width="110" align="right">
          <template #default="{ row }">
            <span
              :class="['value-cell', isOverThreshold(row) ? 'red' : '']"
            >
              {{ formatValue(row.metric, row.value) }}
            </span>
          </template>
        </el-table-column>

        <!-- 触发时间列 -->
        <el-table-column label="触发时间" min-width="180">
          <template #default="{ row }">
            <span class="time-cell">{{ formatTime(row.fired_at) }}</span>
          </template>
        </el-table-column>

        <!-- 恢复时间列 -->
        <el-table-column label="恢复时间" min-width="180">
          <template #default="{ row }">
            <span class="time-cell">{{ formatTime(row.resolved_at) }}</span>
          </template>
        </el-table-column>

        <!-- 节点列：显示节点名称 + ID 缩写 -->
        <el-table-column label="节点" min-width="220">
          <template #default="{ row }">
            <div class="node-cell">
              <span class="node-name">{{ row.agent_name || row.agent_id }}</span>
              <!-- 有名称且与 ID 不同时，附加显示 ID 前 8 位 -->
              <span
                v-if="row.agent_name && row.agent_name !== row.agent_id"
                class="node-id"
              >
                {{ String(row.agent_id).slice(0, 8) }}
              </span>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 空状态：美观的居中提示（非加载中且无数据时显示） -->
      <div v-if="!loading && alertList.length === 0" class="alert-empty">
        <div class="empty-icon">
          <!-- 盾牌图标，表示无告警的安全状态 -->
          <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M12 2L4 6v6c0 5 3.5 9.5 8 10 4.5-.5 8-5 8-10V6l-8-4z" />
            <path d="M9 12l2 2 4-4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </div>
        <div class="empty-title">暂无告警</div>
        <div class="empty-sub">所有节点运行正常，无触发告警</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// ===== 告警中心逻辑 =====
// 功能：拉取 /api/alerts 告警列表，每秒自动刷新，展示状态/指标/阈值/当前值/时间/节点
import { ref, computed, onMounted, onUnmounted } from 'vue'
import http from '@/utils/http'

// 告警列表数据（后端可能返回 null，前端兜底成空数组）
const alertList = ref<any[]>([])
// 加载状态（仅首次加载显示 loading 遮罩，后续静默刷新不显示）
const loading = ref(false)
// 定时刷新定时器引用
let refreshTimer: ReturnType<typeof setInterval> | null = null

// 计算属性：进行中告警数（status === 'firing'）
const firingCount = computed(() =>
  alertList.value.filter((a) => a.status === 'firing').length
)

// 计算属性：已恢复告警数（status === 'resolved'）
const resolvedCount = computed(() =>
  alertList.value.filter((a) => a.status === 'resolved').length
)

// 拉取告警列表
// showLoading=true 时显示加载遮罩（首次加载），后续每秒刷新时静默
async function fetchAlerts(showLoading = false) {
  if (showLoading) loading.value = true
  try {
    // 加时间戳避免浏览器/中间层缓存，确保拿到最新告警状态
    const res = await http.get(`/api/alerts?_=${Date.now()}`)
    // 后端无告警时可能返回 null；前端必须兜底成数组，避免表格闪现后因 .length 报错消失
    alertList.value = Array.isArray(res.data) ? res.data : []
  } catch (e) {
    // 请求失败时清空列表，避免显示过期数据
    console.error('获取告警列表失败', e)
    alertList.value = []
  } finally {
    if (showLoading) loading.value = false
  }
}

// 指标中文名称映射
// 将后端 metric 字段（offline/cpu/mem 等）转为中文展示
function metricText(metric: string) {
  const map: Record<string, string> = {
    offline: '离线',
    cpu: 'CPU',
    mem: '内存',
    disk: '磁盘',
    ping_latency: 'Ping 延迟',
    ping_loss: 'Ping 丢包',
  }
  return map[metric] || metric
}

// 格式化指标数值
// 不同指标使用不同单位：离线=秒、Ping 延迟=ms、其余=百分比
function formatValue(metric: string, value?: number) {
  if (typeof value !== 'number') return '-'
  if (metric === 'offline') return value > 1 ? `${value.toFixed(0)} 秒` : '离线'
  if (metric === 'ping_latency') return `${value.toFixed(1)} ms`
  return `${value.toFixed(1)}%`
}

// 格式化时间戳为本地时间字符串
function formatTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

// 判断当前值是否超过阈值（用于高亮显示）
// 离线指标特殊处理：value>0 即视为超阈值
function isOverThreshold(row: any) {
  if (row.status !== 'firing') return false
  if (typeof row.value !== 'number' || typeof row.threshold !== 'number') return false
  return row.value > row.threshold
}

// 组件挂载：首次加载 + 每秒定时刷新
onMounted(() => {
  // 首次加载显示 loading
  fetchAlerts(true)
  // 告警中心按秒刷新，配合时间戳参数避免缓存，确保告警状态实时更新
  refreshTimer = setInterval(() => fetchAlerts(false), 1000)
})

// 组件卸载：清除定时器，避免内存泄漏
onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
/* ===== 告警中心局部样式 ===== */

/* 表格卡片容器：去掉默认底部间距，表格撑满 */
.alert-table-card {
  margin-bottom: 0;
  padding: 4px 8px 8px;
}

/* 指标单元格：加粗显示 */
.metric-cell {
  font-weight: 600;
  color: var(--wk-text);
}

/* 阈值单元格：次要色 */
.threshold-cell {
  color: var(--wk-text-muted);
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 13px;
}

/* 当前值单元格：等宽字体 */
.value-cell {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 13px;
  font-weight: 600;
}

/* 时间单元格：等宽字体，次要色 */
.time-cell {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 12.5px;
  color: var(--wk-text-muted);
}

/* 节点单元格：名称 + ID 上下排列 */
.node-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;

  .node-name {
    font-weight: 600;
    color: var(--wk-text);
  }

  .node-id {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--wk-text-muted);
  }
}

/* 空状态：居中美化提示 */
.alert-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 56px 20px;
  text-align: center;

  .empty-icon {
    color: var(--wk-success);
    opacity: 0.7;
    margin-bottom: 14px;
  }

  .empty-title {
    font-size: 16px;
    font-weight: 650;
    color: var(--wk-text);
    margin-bottom: 6px;
  }

  .empty-sub {
    font-size: 13px;
    color: var(--wk-text-muted);
  }
}

/* 响应式：小屏下统计卡片单列 */
@media (max-width: 640px) {
  .wk-stat-row {
    grid-template-columns: 1fr !important;
  }
}
</style>
