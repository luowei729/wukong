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
      <el-table-column prop="name" label="名称" />
      <el-table-column label="CPU" width="100">
        <template #default="{ row }">-</template>
      </el-table-column>
      <el-table-column label="内存" width="100">
        <template #default="{ row }">-</template>
      </el-table-column>
      <el-table-column label="磁盘" width="100">
        <template #default="{ row }">-</template>
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
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'

const router = useRouter()
const nodeList = ref<any[]>([])

async function fetchNodes() {
  try {
    const token = localStorage.getItem('access_token')
    const res = await axios.get('/api/agents', {
      headers: { Authorization: `Bearer ${token}` },
    })
    nodeList.value = res.data
  } catch (e) {
    console.error('获取节点列表失败', e)
  }
}

function goToNode(id: string) {
  router.push(`/nodes/${id}`)
}

onMounted(fetchNodes)
</script>