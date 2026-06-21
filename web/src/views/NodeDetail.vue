<template>
  <!-- 节点详情页 -->
  <div class="node-detail">
    <el-button text @click="$router.back()">
      <el-icon><ArrowLeft /></el-icon>
      返回
    </el-button>

    <div style="display: flex; align-items: center; gap: 12px; margin: 16px 0;">
      <span :class="['wk-status-dot', 'online']" />
      <h2 style="font-size: 20px;">{{ nodeName }}</h2>
    </div>

    <!-- 24h Ping K线图 -->
    <div class="wk-card-solid" style="padding: 20px; margin-bottom: 20px;">
      <h3 style="margin-bottom: 16px; font-size: 16px;">网络延时 - 最近 24 小时</h3>
      <div ref="chartRef" style="height: 360px;"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import * as echarts from 'echarts'

const route = useRoute()
const agentId = route.params.id as string
const nodeName = ref('节点详情')
const chartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

function initChart() {
  if (!chartRef.value) return

  chart = echarts.init(chartRef.value, 'dark')

  // 生成模拟数据
  const hours = 24
  const data: number[] = []
  const timeLabels: string[] = []
  const now = new Date()

  for (let i = hours * 60 - 1; i >= 0; i--) {
    const t = new Date(now.getTime() - i * 60 * 1000)
    timeLabels.push(`${t.getHours().toString().padStart(2, '0')}:${t.getMinutes().toString().padStart(2, '0')}`)
    // 模拟延时数据：10-200ms 之间，带一些突刺
    data.push(10 + Math.random() * 190 + (Math.random() > 0.95 ? 100 : 0))
  }

  chart.setOption({
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(15, 23, 42, 0.9)',
      borderColor: 'rgba(56, 189, 248, 0.3)',
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      containLabel: true,
    },
    xAxis: {
      type: 'category',
      data: timeLabels.filter((_, i) => i % 10 === 0),
      axisLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
      axisLabel: { color: 'var(--wk-text-muted)', fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      name: '延时 (ms)',
      nameTextStyle: { color: 'var(--wk-text-muted)' },
      axisLine: { show: false },
      splitLine: { lineStyle: { color: 'var(--wk-chart-grid)' } },
    },
    dataZoom: [
      { type: 'inside', start: 0, end: 100 },
      { type: 'slider', start: 0, end: 100, height: 20, bottom: 0 },
    ],
    series: [{
      type: 'line',
      data,
      smooth: true,
      symbol: 'none',
      lineStyle: {
        color: '#38bdf8',
        width: 1.5,
      },
      areaStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: 'rgba(56, 189, 248, 0.3)' },
          { offset: 1, color: 'rgba(56, 189, 248, 0.02)' },
        ]),
      },
    }],
  })
}

onMounted(async () => {
  nodeName.value = `节点 ${agentId.slice(0, 8)}`
  await nextTick()
  initChart()
})

onUnmounted(() => {
  chart?.dispose()
})
</script>