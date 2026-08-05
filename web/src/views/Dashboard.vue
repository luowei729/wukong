<template>
  <!-- ============ 总览仪表盘页面 ============ -->
  <div class="dashboard">
    <!-- 页面头部：使用全局 .wk-page-header 样式 -->
    <div class="wk-page-header">
      <h2>总览仪表盘</h2>
      <div class="muted">实时监控所有节点状态与资源使用情况</div>
    </div>

    <!-- ============ 统计卡片行：在线节点 / 平均CPU / 平均内存 / 今日告警 ============ -->
    <!-- 使用 .wk-stat-row > .wk-metric 结构，4 列等宽布局 -->
    <div class="wk-stat-row" v-if="!loading">
      <!-- 在线节点数 -->
      <div class="wk-metric">
        <div class="label">在线节点</div>
        <div class="value">{{ stats.online }}</div>
        <div class="sub">台</div>
      </div>
      <!-- 平均 CPU 使用率 -->
      <div class="wk-metric">
        <div class="label">平均 CPU</div>
        <div class="value">{{ stats.avgCpu }}</div>
        <div class="sub">%</div>
      </div>
      <!-- 平均内存使用率 -->
      <div class="wk-metric">
        <div class="label">平均内存</div>
        <div class="value">{{ stats.avgMem }}</div>
        <div class="sub">%</div>
      </div>
      <!-- 今日告警数 -->
      <div class="wk-metric">
        <div class="label">今日告警</div>
        <div class="value" :class="{ 'text-danger': stats.alerts > 0 }">{{ stats.alerts }}</div>
        <div class="sub">条</div>
      </div>
    </div>

    <!-- 统计卡片骨架屏：数据加载时显示 -->
    <div class="wk-stat-row" v-if="loading">
      <div class="wk-metric" v-for="i in 4" :key="i">
        <div class="wk-skeleton" style="width: 60px; height: 12px;"></div>
        <div class="wk-skeleton" style="width: 80px; height: 28px; margin-top: 8px;"></div>
        <div class="wk-skeleton" style="width: 30px; height: 10px; margin-top: 6px;"></div>
      </div>
    </div>

    <!-- ============ 节点状态列表：用 .wk-card-solid 包裹 ============ -->
    <div class="wk-card-solid node-section">
      <div class="section-header">
        <h3>节点状态</h3>
        <span class="node-count">共 {{ nodeList.length }} 个节点</span>
      </div>

      <!-- 节点卡片骨架屏：加载中显示 -->
      <div class="node-grid" v-if="loading">
        <div class="wk-card node-card" v-for="i in 4" :key="'sk-' + i">
          <div class="node-header">
            <div class="wk-skeleton" style="width: 12px; height: 12px; border-radius: 50%;"></div>
            <div class="wk-skeleton" style="width: 100px; height: 16px;"></div>
          </div>
          <div class="metric-bar" v-for="j in 3" :key="j">
            <div class="wk-skeleton" style="width: 40px; height: 12px;"></div>
            <div class="wk-skeleton" style="flex: 1; height: 12px;"></div>
            <div class="wk-skeleton" style="width: 50px; height: 12px;"></div>
          </div>
        </div>
      </div>

      <!-- 节点卡片网格：数据加载完成后显示 -->
      <div class="node-grid" v-if="!loading">
        <div
          v-for="node in nodeList"
          :key="node.id"
          class="wk-card node-card"
          @click="goToNode(node.id)"
        >
          <!-- 节点头部：状态指示灯 + 节点名称 -->
          <div class="node-header">
            <span :class="['wk-status-dot', node.online ? 'online' : 'offline']" />
            <span class="node-name">{{ node.name }}</span>
            <span class="node-ip" v-if="node.ip_v4">{{ node.ip_v4 }}</span>
          </div>

          <!-- CPU 使用率指标 + 进度条 -->
          <div class="metric-bar">
            <span class="metric-label">CPU</span>
            <div class="progress-wrap">
              <div
                class="progress-bar"
                :class="getProgressClass(node.cpu)"
                :style="{ width: clamp(node.cpu) + '%' }"
              />
            </div>
            <span class="metric-value">{{ formatNum(node.cpu) }}%</span>
          </div>

          <!-- 内存使用率指标 + 进度条 -->
          <div class="metric-bar">
            <span class="metric-label">内存</span>
            <div class="progress-wrap">
              <div
                class="progress-bar"
                :class="getProgressClass(node.mem)"
                :style="{ width: clamp(node.mem) + '%' }"
              />
            </div>
            <span class="metric-value">{{ formatNum(node.mem) }}%</span>
          </div>

          <!-- 磁盘使用率指标 + 进度条 -->
          <div class="metric-bar">
            <span class="metric-label">磁盘</span>
            <div class="progress-wrap">
              <div
                class="progress-bar"
                :class="getProgressClass(node.disk)"
                :style="{ width: clamp(node.disk) + '%' }"
              />
            </div>
            <span class="metric-value">{{ formatNum(node.disk) }}%</span>
          </div>
        </div>

        <!-- 空状态：无节点数据时提示 -->
        <div v-if="nodeList.length === 0" class="empty-state">
          <el-empty description="暂无节点数据，请先安装探针" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// ============ 依赖引入 ============
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import http from '@/utils/http'

// ============ 路由实例，用于跳转节点详情 ============
const router = useRouter()

// ============ 响应式状态 ============
// 节点列表数据
const nodeList = ref<any[]>([])
// 加载状态标志：首次加载时显示骨架屏
const loading = ref(true)
// SSE 事件源引用
let eventSource: EventSource | null = null
// 定时刷新定时器引用
let refreshTimer: ReturnType<typeof setInterval> | null = null

// 统计数据：在线节点数、平均CPU、平均内存、今日告警
const stats = reactive({
  online: 0,       // 在线节点数量
  avgCpu: '-',     // 平均 CPU 使用率（无数据时显示 '-'）
  avgMem: '-',     // 平均内存使用率（无数据时显示 '-'）
  alerts: 0,       // 今日告警条数
})

// ============ 工具函数 ============

/**
 * 格式化数值：保留一位小数，无数据返回 '--'
 * @param val 数值
 */
function formatNum(val?: number): string {
  if (val == null || isNaN(val)) return '--'
  return val.toFixed(1)
}

/**
 * 限制进度条宽度在 0~100 之间
 * @param val 数值
 */
function clamp(val?: number): number {
  if (val == null || isNaN(val)) return 0
  return Math.max(0, Math.min(100, val))
}

/**
 * 根据使用率返回进度条颜色类名
 * - < 60%: 正常（绿色）
 * - 60~85%: 警告（黄色）
 * - > 85%: 危险（红色）
 * @param val 使用率数值
 */
function getProgressClass(val?: number): string {
  if (val == null || isNaN(val)) return ''
  if (val >= 85) return 'danger'
  if (val >= 60) return 'warning'
  return 'success'
}

// ============ 核心功能函数 ============

/**
 * 获取节点列表数据
 * 同时请求 /api/agents（节点基础信息）和 /api/agents/latest（最新指标数据）
 * 将两个接口的数据合并后更新节点列表和统计卡片
 */
async function fetchNodes() {
  try {
    // 并发请求：节点列表 + 最新指标
    const [agentsRes, latestRes] = await Promise.all([
      http.get(`/api/agents?_=${Date.now()}`),
      http.get(`/api/agents/latest?_=${Date.now()}`),
    ])

    // 最新指标数据以节点 ID 为键
    const latest = latestRes.data || {}

    // 合并节点基础信息与最新指标数据
    nodeList.value = (agentsRes.data || []).map((node: any) => ({
      ...node,
      ...(latest[node.id] || {}),
    }))

    // 计算在线节点数
    const onlineNodes = nodeList.value.filter((n: any) => n.online)
    stats.online = onlineNodes.length

    // 计算平均 CPU 和内存使用率（仅统计有指标数据的节点）
    const metricNodes = nodeList.value.filter((n: any) => typeof n.cpu === 'number')
    if (metricNodes.length > 0) {
      const avgCpu = metricNodes.reduce((sum: number, n: any) => sum + n.cpu, 0) / metricNodes.length
      const avgMem = metricNodes.reduce((sum: number, n: any) => sum + n.mem, 0) / metricNodes.length
      stats.avgCpu = avgCpu.toFixed(1)
      stats.avgMem = avgMem.toFixed(1)
    } else {
      stats.avgCpu = '-'
      stats.avgMem = '-'
    }

    // 获取今日告警数（从 /api/alerts 接口获取）
    fetchAlerts()
  } catch (e) {
    console.error('获取节点列表失败', e)
  } finally {
    // 首次加载完成，关闭骨架屏
    loading.value = false
  }
}

/**
 * 获取今日告警数量
 * 调用 /api/alerts 接口，筛选今日的告警记录
 */
async function fetchAlerts() {
  try {
    const res = await http.get(`/api/alerts?_=${Date.now()}`)
    const alerts = Array.isArray(res.data) ? res.data : (res.data?.alerts || [])
    // 获取今天的日期（本地时区）
    const today = new Date()
    const todayStr = today.toDateString()
    // 筛选今天触发的告警
    const todayAlerts = alerts.filter((a: any) => {
      const alertDate = new Date(a.created_at || a.time || a.timestamp)
      return alertDate.toDateString() === todayStr
    })
    stats.alerts = todayAlerts.length
  } catch {
    // 告警接口失败不影响主流程
    stats.alerts = 0
  }
}

/**
 * SSE 实时更新：连接 /api/events 事件流
 * 收到 metrics_update 事件时自动刷新节点数据
 */
function connectSSE() {
  // 从本地存储获取 JWT token
  const token = localStorage.getItem('access_token')
  // 创建 EventSource 连接，通过 URL 传递 token
  eventSource = new EventSource(`/api/events?token=${token}`)

  // 监听消息事件
  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      // 收到指标更新事件时刷新节点列表
      if (data.type === 'metrics_update') {
        fetchNodes()
      }
    } catch {
      // JSON 解析失败时忽略
    }
  }

  // SSE 连接异常时自动重连
  eventSource.onerror = () => {
    eventSource?.close()
    // 5 秒后尝试重连
    setTimeout(() => {
      if (refreshTimer) connectSSE()
    }, 5000)
  }
}

/**
 * 跳转到节点详情页
 * @param id 节点 ID
 */
function goToNode(id: string) {
  router.push(`/nodes/${id}`)
}

// ============ 生命周期 ============

// 组件挂载时：首次获取数据、建立 SSE 连接、启动定时刷新
onMounted(() => {
  // 首次获取节点列表和指标
  fetchNodes()
  // 建立 SSE 实时推送连接
  connectSSE()
  // 后台总览每秒主动刷新一次，避免依赖 SSE 或浏览器缓存导致设备状态不更新
  refreshTimer = setInterval(fetchNodes, 1000)
})

// 组件卸载时：关闭 SSE 连接、清除定时器，避免内存泄漏
onUnmounted(() => {
  eventSource?.close()
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
/* ============ 页面整体布局 ============ */
.dashboard {
  padding: 4px;
}

/* ============ 节点状态区块 ============ */
.node-section {
  padding: 20px;
}

/* 区块头部：标题 + 节点计数 */
.section-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 16px;
}

.section-header h3 {
  margin: 0;
}

.node-count {
  font-size: 12px;
  color: var(--wk-text-muted);
  font-variant-numeric: tabular-nums;
}

/* ============ 节点卡片网格 ============ */
.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}

/* 单个节点卡片 */
.node-card {
  padding: 16px;
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, transform 0.15s ease;
}

/* 节点卡片悬停效果：边框高亮 + 轻微上浮 */
.node-card:hover {
  border-color: var(--wk-primary);
  box-shadow: 0 4px 16px color-mix(in srgb, var(--wk-primary) 15%, transparent);
  transform: translateY(-2px);
}

/* 节点头部：状态灯 + 名称 + IP */
.node-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
}

.node-name {
  font-weight: 650;
  font-size: 14px;
  color: var(--wk-text);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-ip {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 11px;
  color: var(--wk-text-muted);
  font-variant-numeric: tabular-nums;
}

/* ============ 指标进度条 ============ */
.metric-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 0;
  font-size: 12.5px;
}

.metric-label {
  color: var(--wk-text-muted);
  width: 32px;
  flex-shrink: 0;
  font-weight: 600;
}

/* 进度条外层容器 */
.progress-wrap {
  flex: 1;
  height: 6px;
  background: var(--wk-bg-soft);
  border-radius: 3px;
  overflow: hidden;
}

/* 进度条填充部分 */
.progress-bar {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease, background 0.3s ease;
}

/* 进度条颜色：根据使用率分级 */
.progress-bar.success {
  background: var(--wk-success);
}
.progress-bar.warning {
  background: var(--wk-warning);
}
.progress-bar.danger {
  background: var(--wk-danger);
}

/* 指标数值：等宽字体 + tabular-nums 对齐 */
.metric-value {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-weight: 700;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  color: var(--wk-text);
  width: 48px;
  text-align: right;
  flex-shrink: 0;
}

/* ============ 统计卡片数值样式增强 ============ */
.wk-metric .value {
  font-variant-numeric: tabular-nums;
}

.wk-metric .sub {
  color: var(--wk-text-muted);
  font-size: 12px;
  margin-top: 4px;
}

/* 告警数值高亮（红色） */
.text-danger {
  color: var(--wk-danger) !important;
}

/* ============ 空状态 ============ */
.empty-state {
  grid-column: 1 / -1;
  text-align: center;
  padding: 40px 20px;
  color: var(--wk-text-muted);
}
</style>
