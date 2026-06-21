<template>
  <!-- 告警中心 -->
  <div class="alerts-page">
    <h2 style="margin-bottom: 20px; font-size: 20px;">告警中心</h2>

    <div class="wk-card-solid" style="padding: 20px;">
      <el-table :data="alertList" stripe style="width: 100%">
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 'firing' ? 'danger' : 'success'" size="small">
              {{ row.status === 'firing' ? '进行中' : '已恢复' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="指标" width="100" prop="metric" />
        <el-table-column label="阈值" width="80">
          <template #default="{ row }">{{ row.threshold }}</template>
        </el-table-column>
        <el-table-column label="当前值" width="80">
          <template #default="{ row }">{{ row.value?.toFixed(1) }}</template>
        </el-table-column>
        <el-table-column label="触发时间" width="180" prop="fired_at" />
        <el-table-column label="探针" prop="agent_id" />
      </el-table>
      <div v-if="alertList.length === 0" style="text-align: center; padding: 40px; color: var(--wk-text-muted);">
        暂无告警
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const alertList = ref<any[]>([])

async function fetchAlerts() {
  try {
    const token = localStorage.getItem('access_token')
    const res = await axios.get('/api/alerts/active', {
      headers: { Authorization: `Bearer ${token}` },
    })
    alertList.value = res.data
  } catch (e) {
    console.error('获取告警列表失败', e)
  }
}

onMounted(fetchAlerts)
</script>