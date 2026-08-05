<template>
  <!-- ===== 节点详情页 ===== -->
  <div class="node-detail">
    <!-- 顶部：返回按钮 + 节点标题 + 状态灯 -->
    <div class="detail-header">
      <el-button text class="back-btn" @click="$router.back()">
        <el-icon><ArrowLeft /></el-icon>
        返回
      </el-button>
    </div>

    <!-- 节点标题行：状态灯 + 节点名称 + 修改名称按钮 -->
    <div class="title-row">
      <span :class="['wk-status-dot', node?.online ? 'online' : 'offline']" />
      <h2 class="node-title">{{ nodeName }}</h2>
      <el-button size="small" type="primary" plain @click="openRename">修改名称</el-button>
    </div>

    <!-- ===== 服务器配置区域 ===== -->
    <div class="wk-card-solid config-card">
      <h3>服务器配置</h3>
      <!-- 配置说明提示 -->
      <el-alert
        title="采集频率和 Ping 频率会写入 SQLite 固化；已安装探针重启后生效，后续会接入签名热更新。"
        type="info"
        :closable="false"
        class="config-alert"
      />
      <!-- 配置表单：节点名称、采集频率、Ping 频率、保存按钮 -->
      <el-form :inline="true" class="config-form">
        <el-form-item label="节点名称">
          <el-input v-model="configForm.name" placeholder="自定义节点名称" style="width: 180px;" />
        </el-form-item>
        <el-form-item label="采集频率（秒）">
          <el-input-number v-model="configForm.collect_intv" :min="1" :max="3600" :step="1" />
        </el-form-item>
        <el-form-item label="Ping 频率（秒）">
          <el-input-number v-model="configForm.ping_intv" :min="1" :max="3600" :step="1" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingConfig" @click="saveConfig">保存配置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- ===== 24h Ping K线图区域 ===== -->
    <div class="wk-card-solid chart-card">
      <!-- 图表标题行 -->
      <div class="chart-header">
        <h3>网络延时 - 最近 24 小时</h3>
        <span class="chart-sub">所有启用运营商线路</span>
      </div>
      <!-- 无 ISP 目标时显示空状态提示 -->
      <el-empty v-if="ispTargets.length === 0" description="请先在设置页配置并启用 Ping 运营商目标" />
      <!-- 无 Ping 数据时显示空状态提示 -->
      <el-empty v-else-if="Object.keys(pingSeries).length === 0 && !pingLoading" description="暂无真实 Ping 数据" />
      <!-- ECharts 图表容器 -->
      <div v-loading="pingLoading" ref="chartRef" :style="{ height: ispTargets.length ? '360px' : '0' }"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
// ===== 节点详情页逻辑 =====
import { ref, reactive, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '@/utils/http'
import * as echarts from 'echarts'

const route = useRoute()
// 从路由参数获取节点 ID
const agentId = route.params.id as string
// 节点详情数据
const node = ref<any | null>(null)
// 节点显示名称
const nodeName = ref('节点详情')
// ECharts 图表容器引用
const chartRef = ref<HTMLDivElement>()
// ISP 目标列表（仅启用的）
const ispTargets = ref<any[]>([])
// Ping 聚合数据，按 ISP 名称分组
const pingSeries = ref<Record<string, any[]>>({})
// Ping 数据加载状态
const pingLoading = ref(false)
// 配置保存中状态
const savingConfig = ref(false)
// 配置表单数据
const configForm = reactive({
  name: '',           // 节点名称
  collect_intv: 1,    // 采集频率（秒）
  ping_intv: 1,       // Ping 频率（秒）
})
// ECharts 实例
let chart: echarts.ECharts | null = null
// Ping 数据定时刷新定时器
let pingTimer: number | null = null

// 新主题 ECharts 配色方案：蓝色主色调 + 多彩辅助色
const chartColors = ['#3b82f6', '#34d399', '#fbbf24', '#f87171', '#8b5cf6', '#14b8a6', '#ec4899']

// 获取节点详情：请求 /api/agents/:id
async function fetchNode() {
  try {
    const res = await http.get(`/api/agents/${agentId}?_=${Date.now()}`)
    node.value = res.data
    // 设置节点显示名称：优先自定义名称，其次主机名，最后截断 ID
    nodeName.value = res.data.name || res.data.hostname || `节点 ${agentId.slice(0, 8)}`
    // 同步配置表单数据
    configForm.name = nodeName.value
    configForm.collect_intv = res.data.collect_intv || 1
    configForm.ping_intv = res.data.ping_intv || 1
  } catch (e) {
    console.error('获取节点详情失败', e)
  }
}

// 修改节点名称：弹出输入框，调用 PUT /api/agents/:id 更新
async function openRename() {
  try {
    const { value } = await ElMessageBox.prompt('请输入新的服务器节点名称', '修改节点名称', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: nodeName.value,
      inputPattern: /^.{1,64}$/,
      inputErrorMessage: '节点名称长度必须为 1-64 个字符',
    })
    // 调用后端更新节点名称
    await http.put(`/api/agents/${agentId}`, { name: value })
    ElMessage.success('节点名称已保存')
    // 刷新节点详情显示新名称
    await fetchNode()
  } catch (e: any) {
    // 用户点取消不报错，其他错误显示后端返回的错误信息
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '修改节点名称失败')
    }
  }
}

// 保存服务器配置：更新节点名称、采集频率、Ping 频率
async function saveConfig() {
  // 校验节点名称不能为空
  if (!configForm.name.trim()) {
    ElMessage.warning('节点名称不能为空')
    return
  }
  savingConfig.value = true
  try {
    // 调用后端保存配置
    await http.put(`/api/agents/${agentId}`, {
      name: configForm.name.trim(),
      collect_intv: configForm.collect_intv,
      ping_intv: configForm.ping_intv,
    })
    ElMessage.success('服务器配置已保存')
    // 刷新节点详情显示最新配置
    await fetchNode()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存服务器配置失败')
  } finally {
    savingConfig.value = false
  }
}

// 加载 ISP 目标列表：请求 /api/isp-targets，只保留启用的目标
async function loadISPTargets() {
  try {
    const res = await http.get(`/api/isp-targets?_=${Date.now()}`)
    // 过滤出已启用的 ISP 目标
    ispTargets.value = (res.data || []).filter((item: any) => item.enabled)
  } catch (e) {
    console.error('加载 ISP 目标失败', e)
  }
}

// 加载 Ping 聚合数据：为每个 ISP 目标请求 /api/agents/:id/ping-agg
async function loadPingAgg() {
  // 没有 ISP 目标时清空数据直接返回
  if (ispTargets.value.length === 0) {
    pingSeries.value = {}
    return
  }
  pingLoading.value = true
  try {
    // 并行请求所有 ISP 的 Ping 聚合数据
    const results = await Promise.all(ispTargets.value.map(async (isp: any) => {
      const res = await http.get(`/api/agents/${agentId}/ping-agg`, {
        params: { isp: isp.name, _: Date.now() },
      })
      return [isp.name, res.data || []] as const
    }))
    // 过滤掉没有数据的 ISP，构建 pingSeries 映射
    pingSeries.value = Object.fromEntries(results.filter(([, points]) => points.length > 0))
    // 等待 DOM 更新后渲染图表
    await nextTick()
    renderChart()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '加载 Ping 数据失败')
  } finally {
    pingLoading.value = false
  }
}

// ECharts 渲染延时图：按 ISP 分组展示延时和丢包率
function renderChart() {
  // 图表容器不存在或无数据时不渲染
  if (!chartRef.value || Object.keys(pingSeries.value).length === 0) return
  // 初始化 ECharts 实例（仅首次创建）
  if (!chart) chart = echarts.init(chartRef.value, 'dark')

  // 收集所有时间点（bucket_min），去重排序
  const allBuckets = Array.from(new Set(
    Object.values(pingSeries.value).flatMap(points => points.map(point => point.bucket_min))
  )).sort()
  // 格式化时间轴标签
  const labels = allBuckets.map(point => formatTime(point))

  // 构建每个 ISP 的延时和丢包率时间映射
  // 关键修复：data 必须是数字类型，不能用 .toFixed() 转成字符串，
  // 否则 ECharts trigger:'axis' 的 tooltip 无法正确聚合多个 series。
  // loss_rate 来自 ping -c 3 的单次探测丢包率（0/0.33/0.67/1.0），按秒级展示
  const ispLossByTime = new Map<string, Map<string, number>>()
  const series = Object.entries(pingSeries.value).map(([isp, points], index) => {
    // 延时映射：bucket_min -> avg_lat
    const byTime = new Map<string, number>()
    // 丢包率映射：bucket_min -> loss_rate * 100
    const lossMap = new Map<string, number>()
    for (const point of points) {
      byTime.set(point.bucket_min, Number(point.avg_lat || 0))
      lossMap.set(point.bucket_min, Number(point.loss_rate || 0) * 100)
    }
    ispLossByTime.set(isp, lossMap)
    // 图例名称显示最新丢包率概览
    const lastPoint = points.length > 0 ? points[points.length - 1] : null
    const lossPercent = lastPoint ? (Number(lastPoint.loss_rate || 0) * 100).toFixed(1) : '0.0'
    return {
      name: `${isp} ${lossPercent}%loss`,
      type: 'line',
      // 按时间轴对齐数据，缺失的点用 null 表示
      data: allBuckets.map(bucket => byTime.has(bucket) ? byTime.get(bucket)! : null),
      smooth: false,
      sampling: 'lttb',
      symbol: 'none',
      connectNulls: true,
      // 使用新主题配色
      lineStyle: { color: chartColors[index % chartColors.length], width: 1.8 },
      _ispName: isp,
    }
  })

  // 设置 ECharts 配置项
  chart.setOption({
    animation: false,
    // tooltip：背景色和边框色使用新主题配色
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
          const color = p.color || chartColors[0]
          const ispName = p.series?._ispName || p.seriesName
          // 从 ispLossByTime 中获取该时间点的实际丢包率
          const lossMap = ispLossByTime.get(ispName)
          const bucketKey = allBuckets[p.dataIndex]
          const lossPct = lossMap?.get(bucketKey)?.toFixed(1) ?? '0.0'
          const lat = typeof p.value === 'number' ? `${p.value.toFixed(2)} ms` : `${p.value} ms`
          html += `<div style="display:flex;align-items:center;gap:8px;font-size:12px;line-height:22px">
            <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};flex-shrink:0"></span>
            <span style="color:#f5f5f5;min-width:80px">${ispName}</span>
            <span style="color:#3b82f6;font-weight:600;min-width:70px;text-align:right">${lat}</span>
            <span style="color:#fbbf24;font-size:11px;min-width:55px;text-align:right">${lossPct}% loss</span>
          </div>`
        })
        return html
      },
    },
    // 图例：可滚动
    legend: { type: 'scroll', textStyle: { color: '#a3a3a3' } },
    // 网格布局
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    // X 轴：时间类别轴
    xAxis: {
      type: 'category',
      data: labels,
      axisLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.08)' } },
      axisLabel: { color: '#a3a3a3', fontSize: 11 },
    },
    // Y 轴：延时数值轴
    yAxis: {
      type: 'value',
      name: '延时 (ms)',
      nameTextStyle: { color: '#a3a3a3' },
      splitLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.08)' } },
    },
    // 数据缩放：支持区域缩放和滑块缩放
    dataZoom: [
      { type: 'inside', start: 0, end: 100, throttle: 80 },
      { type: 'slider', start: 0, end: 100, height: 20, bottom: 0 },
    ],
    series,
  }, { notMerge: true, lazyUpdate: true })
}

// 格式化时间：将 ISO 时间字符串转为 HH:MM:SS 格式
function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}:${date.getSeconds().toString().padStart(2, '0')}`
}

// 组件挂载：依次获取节点详情、ISP 目标、Ping 数据，并启动定时刷新
onMounted(async () => {
  await fetchNode()          // 获取节点详情
  await loadISPTargets()     // 加载 ISP 目标列表
  await nextTick()           // 等待 DOM 更新
  await loadPingAgg()        // 加载 Ping 聚合数据并渲染图表
  // 每 60 秒刷新一次 Ping 数据
  pingTimer = window.setInterval(loadPingAgg, 60_000)
})

// 组件卸载：清除定时器并销毁 ECharts 实例，避免内存泄漏
onUnmounted(() => {
  if (pingTimer) window.clearInterval(pingTimer)
  chart?.dispose()
})
</script>

<style scoped>
/* 节点详情页容器 */
.node-detail {
  width: 100%;
}

/* 返回按钮区域 */
.detail-header {
  margin-bottom: 8px;
}

.back-btn {
  color: var(--wk-text-muted);
  font-size: 13px;
  padding: 4px 0;
}

/* 节点标题行：状态灯 + 标题 + 改名按钮 */
.title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 8px 0 20px;
}

/* 节点标题 */
.node-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--wk-text);
}

/* 服务器配置卡片 */
.config-card {
  padding: 20px;
  margin-bottom: 20px;
}

/* 配置说明提示框 */
.config-alert {
  margin-bottom: 16px;
}

/* 配置表单 */
.config-form {
  display: flex;
  flex-wrap: wrap;
  gap: 0;
}

/* Ping 图表卡片 */
.chart-card {
  padding: 20px;
  margin-bottom: 20px;
}

/* 图表标题行：标题 + 副标题 */
.chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.chart-header h3 {
  margin: 0;
}

/* 图表副标题 */
.chart-sub {
  color: var(--wk-text-muted);
  font-size: 12px;
}

/* 响应式：小屏幕下表单换行 */
@media (max-width: 860px) {
  .config-form {
    flex-direction: column;
  }
  .config-form :deep(.el-form-item) {
    margin-bottom: 12px;
  }
}
</style>
