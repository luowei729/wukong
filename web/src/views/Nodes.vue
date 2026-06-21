<template>
  <!-- 节点列表页 -->
  <div class="nodes-page">
    <h2 style="margin-bottom: 20px; font-size: 20px;">节点列表</h2>

    <el-table :data="nodeList" stripe style="width: 100%">
      <el-table-column label="状态" width="80">
        <template #default="{ row }">
          <span :class="['wk-status-dot', row.online ? 'online' : 'offline']" />
        </template>
      </el-table-column>
      <el-table-column prop="name" label="名称">
        <template #default="{ row }">
          <span>{{ row.name || row.hostname || row.id }}</span>
          <el-button size="small" type="primary" link @click.stop="openRename(row)">改名</el-button>
        </template>
      </el-table-column>
      <el-table-column label="CPU" width="100">
        <template #default="{ row }">{{ formatPercent(row.cpu) }}</template>
      </el-table-column>
      <el-table-column label="内存" width="100">
        <template #default="{ row }">{{ formatPercent(row.mem) }}</template>
      </el-table-column>
      <el-table-column label="磁盘" width="100">
        <template #default="{ row }">{{ formatPercent(row.disk) }}</template>
      </el-table-column>
      <el-table-column label="系统版本" min-width="150">
        <template #default="{ row }">{{ row.os_version || '-' }}</template>
      </el-table-column>
      <el-table-column label="最后上报" width="180">
        <template #default="{ row }">{{ row.last_seen_at || '-' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120">
        <template #default="{ row }">
          <el-button size="small" type="primary" link @click="goToNode(row.id)">
            详情
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const router = useRouter()
const nodeList = ref<any[]>([])
let refreshTimer: ReturnType<typeof setInterval> | null = null

async function fetchNodes() {
  try {
    const token = localStorage.getItem('access_token')
    const headers = { Authorization: `Bearer ${token}` }
    const [agentsRes, latestRes] = await Promise.all([
      axios.get(`/api/agents?_=${Date.now()}`, { headers }),
      axios.get(`/api/agents/latest?_=${Date.now()}`, { headers }),
    ])
    const latest = latestRes.data || {}
    nodeList.value = (agentsRes.data || []).map((node: any) => ({
      ...node,
      ...(latest[node.id] || {}),
    }))
  } catch (e) {
    console.error('获取节点列表失败', e)
  }
}

function formatPercent(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)}%` : '-'
}

async function openRename(row: any) {
  try {
    const { value } = await ElMessageBox.prompt('请输入新的服务器节点名称', '修改节点名称', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: row.name || row.hostname || '',
      inputPattern: /^.{1,64}$/,
      inputErrorMessage: '节点名称长度必须为 1-64 个字符',
    })
    const token = localStorage.getItem('access_token')
    await axios.put(`/api/agents/${row.id}`, { name: value }, {
      headers: { Authorization: `Bearer ${token}` },
    })
    ElMessage.success('节点名称已保存')
    await fetchNodes()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '修改节点名称失败')
    }
  }
}

function goToNode(id: string) {
  router.push(`/nodes/${id}`)
}

onMounted(() => {
  fetchNodes()
  // 后台设备页每秒刷新一次，避免浏览器或反代缓存导致状态不更新。
  refreshTimer = setInterval(fetchNodes, 1000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>