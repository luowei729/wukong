<template>
  <!-- 节点详情页 -->
  <div class="node-detail">
    <el-button text @click="$router.back()">
      <el-icon><ArrowLeft /></el-icon>
      返回
    </el-button>

    <div style="display: flex; align-items: center; gap: 12px; margin: 16px 0;">
      <span :class="['wk-status-dot', node?.online ? 'online' : 'offline']" />
      <h2 style="font-size: 20px;">{{ nodeName }}</h2>
      <el-button size="small" type="primary" plain @click="openRename">修改名称</el-button>
    </div>

    <!-- 服务器配置 -->
    <div class="wk-card-solid" style="padding: 20px; margin-bottom: 20px;">
      <h3 style="margin-bottom: 16px; font-size: 16px;">服务器配置</h3>
      <el-alert
        title="采集频率和 Ping 频率会写入 SQLite 固化；已安装探针重启后生效，后续会接入签名热更新。"
        type="info"
        :closable="false"
        style="margin-bottom: 16px;"
      />
      <el-form :inline="true">
        <el-form-item label="节点名称">
          <el-input v-model="configForm.name" placeholder="自定义节点名称" style="width: 180px;" />
        </el-form-item>
        <el-form-item label="采集频率（秒）">
          <el-input-number v-model="configForm.collect_intv" :min="1" :max="3600" :step="1" />
        </el-form-item>
        <el-form-item label="Ping 频率（秒）">
          <el-input-number v-model="configForm.ping_intv" :min="5" :max="3600" :step="5" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingConfig" @click="saveConfig">保存配置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- 24h Ping K线图 -->
    <div class="wk-card-solid" style="padding: 20px; margin-bottom: 20px;">
      <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px;">
        <h3 style="font-size: 16px;">网络延时 - 最近 24 小时</h3>
        <el-select v-model="selectedISP" placeholder="选择运营商" style="width: 180px;" @change="loadPingAgg">
          <el-option v-for="isp in ispTargets" :key="isp.name" :label="isp.name" :value="isp.name" />
        </el-select>
      </div>
      <el-empty v-if="!selectedISP" description="请先在设置页配置并启用 Ping 运营商目标" />
      <el-empty v-else-if="pingPoints.length === 0 && !pingLoading" description="暂无真实 Ping 数据" />
      <div v-loading="pingLoading" ref="chartRef" :style="{ height: selectedISP ? '360px' : '0' }"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'
import * as echarts from 'echarts'

const route = useRoute()
const agentId = route.params.id as string
const node = ref<any | null>(null)
const nodeName = ref('节点详情')
const chartRef = ref<HTMLDivElement>()
const ispTargets = ref<any[]>([])
const selectedISP = ref('')
const pingPoints = ref<any[]>([])
const pingLoading = ref(false)
const savingConfig = ref(false)
const configForm = reactive({
  name: '',
  collect_intv: 1,
  ping_intv: 60,
})
let chart: echarts.ECharts | null = null
let pingTimer: number | null = null

function authHeaders() {
  const token = localStorage.getItem('access_token')
  return { Authorization: `Bearer ${token}` }
}

async function fetchNode() {
  try {
    const res = await axios.get(`/api/agents/${agentId}?_=${Date.now()}`, {
      headers: authHeaders(),
    })
    node.value = res.data
    nodeName.value = res.data.name || res.data.hostname || `节点 ${agentId.slice(0, 8)}`
    configForm.name = nodeName.value
    configForm.collect_intv = res.data.collect_intv || 1
    configForm.ping_intv = res.data.ping_intv || 60
  } catch (e) {
    console.error('获取节点详情失败', e)
  }
}

async function openRename() {
  try {
    const { value } = await ElMessageBox.prompt('请输入新的服务器节点名称', '修改节点名称', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: nodeName.value,
      inputPattern: /^.{1,64}$/,
      inputErrorMessage: '节点名称长度必须为 1-64 个字符',
    })
    await axios.put(`/api/agents/${agentId}`, { name: value }, {
      headers: authHeaders(),
    })
    ElMessage.success('节点名称已保存')
    await fetchNode()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '修改节点名称失败')
    }
  }
}

async function saveConfig() {
  if (!configForm.name.trim()) {
    ElMessage.warning('节点名称不能为空')
    return
  }
  savingConfig.value = true
  try {
    await axios.put(`/api/agents/${agentId}`, {
      name: configForm.name.trim(),
      collect_intv: configForm.collect_intv,
      ping_intv: configForm.ping_intv,
    }, { headers: authHeaders() })
    ElMessage.success('服务器配置已保存')
    await fetchNode()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存服务器配置失败')
  } finally {
    savingConfig.value = false
  }
}

async function loadISPTargets() {
  try {
    const res = await axios.get(`/api/isp-targets?_=${Date.now()}`, { headers: authHeaders() })
    ispTargets.value = (res.data || []).filter((item: any) => item.enabled)
    if (!selectedISP.value && ispTargets.value.length > 0) {
      selectedISP.value = ispTargets.value[0].name
    }
  } catch (e) {
    console.error('加载 ISP 目标失败', e)
  }
}

async function loadPingAgg() {
  if (!selectedISP.value) return
  pingLoading.value = true
  try {
    const res = await axios.get(`/api/agents/${agentId}/ping-agg`, {
      params: { isp: selectedISP.value, _: Date.now() },
      headers: authHeaders(),
    })
    pingPoints.value = res.data || []
    await nextTick()
    renderChart()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '加载 Ping 数据失败')
  } finally {
    pingLoading.value = false
  }
}

function renderChart() {
  if (!chartRef.value || !selectedISP.value) return
  if (!chart) chart = echarts.init(chartRef.value, 'dark')
  const labels = pingPoints.value.map(point => formatTime(point.bucket_min))
  const avg = pingPoints.value.map(point => Number(point.avg_lat || 0).toFixed(2))
  const min = pingPoints.value.map(point => Number(point.min_lat || 0).toFixed(2))
  const max = pingPoints.value.map(point => Number(point.max_lat || 0).toFixed(2))
  const loss = pingPoints.value.map(point => Number((point.loss_rate || 0) * 100).toFixed(2))

  chart.setOption({
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(15, 23, 42, 0.9)',
      borderColor: 'rgba(56, 189, 248, 0.3)',
    },
    legend: { data: ['平均延时', '最小延时', '最大延时', '丢包率'], textStyle: { color: 'var(--wk-text-muted)' } },
    grid: { left: '3%', right: '4%', bottom: '8%', containLabel: true },
    xAxis: {
      type: 'category',
      data: labels,
      axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
      axisLabel: { color: 'var(--wk-text-muted)', fontSize: 11 },
    },
    yAxis: [
      {
        type: 'value',
        name: '延时 (ms)',
        nameTextStyle: { color: 'var(--wk-text-muted)' },
        splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
      },
      {
        type: 'value',
        name: '丢包率 (%)',
        min: 0,
        max: 100,
        nameTextStyle: { color: 'var(--wk-text-muted)' },
        splitLine: { show: false },
      },
    ],
    dataZoom: [
      { type: 'inside', start: 0, end: 100 },
      { type: 'slider', start: 0, end: 100, height: 20, bottom: 0 },
    ],
    series: [
      { name: '平均延时', type: 'line', data: avg, smooth: true, symbol: 'none', lineStyle: { color: '#38bdf8', width: 1.8 } },
      { name: '最小延时', type: 'line', data: min, smooth: true, symbol: 'none', lineStyle: { color: '#22c55e', width: 1.2 } },
      { name: '最大延时', type: 'line', data: max, smooth: true, symbol: 'none', lineStyle: { color: '#f97316', width: 1.2 } },
      { name: '丢包率', type: 'line', yAxisIndex: 1, data: loss, smooth: true, symbol: 'none', lineStyle: { color: '#ef4444', width: 1.2 } },
    ],
  })
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`
}

onMounted(async () => {
  await fetchNode()
  await loadISPTargets()
  await nextTick()
  await loadPingAgg()
  pingTimer = window.setInterval(loadPingAgg, 60_000)
})

onUnmounted(() => {
  if (pingTimer) window.clearInterval(pingTimer)
  chart?.dispose()
})
</script>
