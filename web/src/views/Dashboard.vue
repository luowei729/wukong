<template>
  <!-- 总览仪表盘 -->
  <div class="dashboard">
    <h2 style="margin-bottom: 20px; font-size: 20px;">总览仪表盘</h2>

    <!-- 统计卡片 -->
    <el-row :gutter="16" style="margin-bottom: 24px;">
      <el-col :span="6" v-for="stat in stats" :key="stat.label">
        <div class="wk-card dashboard-stat">
          <div class="stat-label">{{ stat.label }}</div>
          <div class="stat-value">{{ stat.value }}</div>
          <div class="stat-unit">{{ stat.unit }}</div>
        </div>
      </el-col>
    </el-row>

    <!-- 在线节点列表 -->
    <div class="wk-card-solid" style="padding: 20px;">
      <h3 style="margin-bottom: 16px; font-size: 16px;">节点状态</h3>
      <div class="node-grid">
        <div
          v-for="node in nodeList"
          :key="node.id"
          class="wk-card node-card"
          @click="goToNode(node.id)"
        >
          <div class="node-header">
            <span :class="['wk-status-dot', node.online ? 'online' : 'offline']" />
            <span class="node-name">{{ node.name }}</span>
          </div>
          <div class="node-metrics">
            <div class="metric">
              <span class="metric-label">CPU</span>
              <span class="metric-value">{{ node.cpu?.toFixed(1) }}%</span>
            </div>
            <div class="metric">
              <span class="metric-label">内存</span>
              <span class="metric-value">{{ node.mem?.toFixed(1) }}%</span>
            </div>
            <div class="metric">
              <span class="metric-label">磁盘</span>
              <span class="metric-value">{{ node.disk?.toFixed(1) }}%</span>
            </div>
          </div>
        </div>
        <div v-if="nodeList.length === 0" class="empty-state">
          暂无节点数据，请先安装探针
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'

const router = useRouter()
const nodeList = ref<any[]>([])
let eventSource: EventSource | null = null

const stats = ref([
  { label: '在线节点', value: '0', unit: '台' },
  { label: '平均 CPU', value: '-', unit: '%' },
  { label: '平均内存', value: '-', unit: '%' },
  { label: '今日告警', value: '0', unit: '条' },
])

async function fetchNodes() {
  try {
    const token = localStorage.getItem('access_token')
    const res = await axios.get('/api/agents', {
      headers: { Authorization: `Bearer ${token}` },
    })
    nodeList.value = res.data
    stats.value[0].value = String(res.data.filter((n: any) => n.online).length)
  } catch (e) {
    console.error('获取节点列表失败', e)
  }
}

// SSE 实时更新
function connectSSE() {
  const token = localStorage.getItem('access_token')
  eventSource = new EventSource(`/api/events?token=${token}`)
  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data.type === 'metrics_update') {
        fetchNodes()
      }
    } catch {}
  }
}

function goToNode(id: string) {
  router.push(`/nodes/${id}`)
}

onMounted(() => {
  fetchNodes()
  connectSSE()
})

onUnmounted(() => {
  eventSource?.close()
})
</script>

<style scoped>
.dashboard-stat {
  padding: 20px;
  text-align: center;
}

.stat-label {
  color: var(--wk-text-muted);
  font-size: 13px;
  margin-bottom: 8px;
}

.stat-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 32px;
  font-weight: 700;
  color: var(--wk-primary);
}

.stat-unit {
  color: var(--wk-text-muted);
  font-size: 12px;
  margin-top: 4px;
}

.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
}

.node-card {
  padding: 16px;
  cursor: pointer;
}

.node-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.node-name {
  font-weight: 600;
  font-size: 14px;
}

.node-metrics .metric {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  font-size: 13px;
}

.metric-label {
  color: var(--wk-text-muted);
}

.metric-value {
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
}

.empty-state {
  grid-column: 1 / -1;
  text-align: center;
  padding: 40px;
  color: var(--wk-text-muted);
}
</style>