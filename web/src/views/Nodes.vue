<template>
  <!-- ===== 节点列表页 ===== -->
  <div class="nodes-page">
    <!-- 页面头部：使用全局 .wk-page-header 样式类 -->
    <div class="wk-page-header">
      <h2>节点列表</h2>
      <div class="muted">所有已注册的服务器探针节点，每秒自动刷新</div>
    </div>

    <!-- 节点表格：使用 Element Plus el-table，外层包一层卡片容器适配新主题 -->
    <div class="wk-card-solid nodes-table-wrap">
      <el-table
        :data="nodeList"
        style="width: 100%"
        v-loading="loading"
        :header-cell-style="{ background: 'var(--wk-bg-soft)', color: 'var(--wk-text-muted)' }"
        :row-class-name="rowClassName"
      >
        <!-- 状态列：在线/离线状态灯 -->
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <span :class="['wk-status-dot', row.online ? 'online' : 'offline']" />
          </template>
        </el-table-column>

        <!-- 名称列：显示节点名称 + 改名按钮 -->
        <el-table-column prop="name" label="名称" min-width="180">
          <template #default="{ row }">
            <div class="name-cell">
              <span class="name-text" @click="goToNode(row.id)">{{ row.name || row.hostname || row.id }}</span>
              <el-button size="small" type="primary" link @click.stop="openRename(row)">改名</el-button>
            </div>
          </template>
        </el-table-column>

        <!-- CPU 使用率列 -->
        <el-table-column label="CPU" width="100" align="right">
          <template #default="{ row }">
            <span :class="['metric-val', loadLevel(row.cpu)]">{{ formatPercent(row.cpu) }}</span>
          </template>
        </el-table-column>

        <!-- 内存使用率列 -->
        <el-table-column label="内存" width="100" align="right">
          <template #default="{ row }">
            <span :class="['metric-val', loadLevel(row.mem)]">{{ formatPercent(row.mem) }}</span>
          </template>
        </el-table-column>

        <!-- 磁盘使用率列 -->
        <el-table-column label="磁盘" width="100" align="right">
          <template #default="{ row }">
            <span :class="['metric-val', loadLevel(row.disk)]">{{ formatPercent(row.disk) }}</span>
          </template>
        </el-table-column>

        <!-- 出口 IP 列：同时显示 IPv4 和 IPv6 -->
        <el-table-column label="出口 IP" min-width="220">
          <template #default="{ row }">
            <div class="ip-cell">
              <div v-if="row.ip_v4">IPv4: {{ row.ip_v4 }}</div>
              <div v-if="row.ip_v6">IPv6: {{ row.ip_v6 }}</div>
              <span v-if="!row.ip_v4 && !row.ip_v6">-</span>
            </div>
          </template>
        </el-table-column>

        <!-- 系统版本列 -->
        <el-table-column label="系统版本" min-width="150">
          <template #default="{ row }">{{ row.os_version || '-' }}</template>
        </el-table-column>

        <!-- 探针版本列 -->
        <el-table-column label="探针版本" width="120">
          <template #default="{ row }">
            <span class="ver-tag">{{ row.agent_ver || '-' }}</span>
          </template>
        </el-table-column>

        <!-- 最后上报时间列 -->
        <el-table-column label="最后上报" width="180">
          <template #default="{ row }">{{ row.last_seen_at || '-' }}</template>
        </el-table-column>

        <!-- 操作列：详情跳转 + 删除节点 -->
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="goToNode(row.id)">详情</el-button>
            <el-button size="small" type="danger" link @click="deleteNode(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
// ===== 节点列表页逻辑 =====
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '@/utils/http'

const router = useRouter()
// 节点列表数据（合并 agents 基础信息和 latest 实时指标）
const nodeList = ref<any[]>([])
// 加载状态标志
const loading = ref(false)
// 定时刷新定时器引用
let refreshTimer: ReturnType<typeof setInterval> | null = null

// 获取节点列表：并行请求 /api/agents 和 /api/agents/latest，合并数据
async function fetchNodes() {
  loading.value = true
  try {
    // 并行请求节点基础信息和最新实时指标，加时间戳避免缓存
    const [agentsRes, latestRes] = await Promise.all([
      http.get(`/api/agents?_=${Date.now()}`),
      http.get(`/api/agents/latest?_=${Date.now()}`),
    ])
    // latest 是以节点 id 为 key 的对象，合并到每个节点上
    const latest = latestRes.data || {}
    nodeList.value = (agentsRes.data || []).map((node: any) => ({
      ...node,
      ...(latest[node.id] || {}),
    }))
  } catch (e) {
    console.error('获取节点列表失败', e)
  } finally {
    loading.value = false
  }
}

// 格式化百分比：非数字返回 '-'，否则保留一位小数
function formatPercent(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)}%` : '-'
}

// 根据负载百分比返回颜色级别类名，用于高亮显示
function loadLevel(value?: number) {
  if (typeof value !== 'number') return ''
  if (value >= 90) return 'red'      // 90% 以上红色告警
  if (value >= 70) return 'yellow'   // 70%-90% 黄色警告
  return ''                          // 正常无特殊颜色
}

// 修改节点名称：弹出输入框，调用 PUT /api/agents/:id 更新
async function openRename(row: any) {
  try {
    const { value } = await ElMessageBox.prompt('请输入新的服务器节点名称', '修改节点名称', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: row.name || row.hostname || '',
      inputPattern: /^.{1,64}$/,
      inputErrorMessage: '节点名称长度必须为 1-64 个字符',
    })
    // 调用后端更新节点名称
    await http.put(`/api/agents/${row.id}`, { name: value })
    ElMessage.success('节点名称已保存')
    // 刷新列表显示新名称
    await fetchNodes()
  } catch (e: any) {
    // 用户点取消不报错，其他错误显示后端返回的错误信息
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '修改节点名称失败')
    }
  }
}

// 删除节点：弹出确认框，确认后调用 DELETE /api/agents/:id
async function deleteNode(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认删除节点"${row.name || row.hostname || row.id}"？删除后该节点的探针需要重新注册才能恢复。`,
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
    // 调用后端删除节点
    await http.delete(`/api/agents/${row.id}`)
    ElMessage.success('节点已删除')
    // 刷新列表移除已删除节点
    await fetchNodes()
  } catch (e: any) {
    // 用户点取消不报错，其他错误显示后端返回的错误信息
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '删除节点失败')
    }
  }
}

// 跳转到节点详情页
function goToNode(id: string) {
  router.push(`/nodes/${id}`)
}

// 行样式：离线节点添加灰显样式
function rowClassName({ row }: { row: any }) {
  return row.online ? '' : 'row-offline'
}

// 组件挂载：首次获取数据 + 每秒定时刷新
onMounted(() => {
  fetchNodes()
  // 每秒刷新一次，保证实时状态更新
  refreshTimer = setInterval(fetchNodes, 1000)
})

// 组件卸载：清除定时器避免内存泄漏
onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
/* 节点列表页容器 */
.nodes-page {
  width: 100%;
}

/* 表格容器：使用 wk-card-solid 提供卡片背景和边框 */
.nodes-table-wrap {
  padding: 4px;
  overflow: hidden;
}

/* 表格名称单元格：名称 + 改名按钮水平排列 */
.name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 节点名称文字：可点击跳转详情 */
.name-text {
  cursor: pointer;
  font-weight: 550;
  color: var(--wk-text);
  transition: color .12s;
}
.name-text:hover {
  color: var(--wk-primary);
}

/* 指标数值：等宽字体对齐 */
.metric-val {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

/* IP 单元格：等宽字体换行显示 */
.ip-cell {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  line-height: 1.5;
  color: var(--wk-text-muted);
  word-break: break-all;
}

/* 探针版本标签样式 */
.ver-tag {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 11.5px;
  color: var(--wk-text-muted);
  background: var(--wk-bg-soft);
  padding: 2px 8px;
  border-radius: 6px;
}

/* 离线节点行灰显 */
:deep(.row-offline) {
  opacity: 0.55;
}

/* Element Plus 表格暗黑主题覆盖：适配新设计令牌 */
:deep(.el-table) {
  background: transparent !important;
  --el-table-bg-color: transparent;
  --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: var(--wk-bg-soft);
  --el-table-border-color: var(--wk-border);
  --el-table-text-color: var(--wk-text);
  --el-table-header-text-color: var(--wk-text-muted);
  --el-table-row-hover-bg-color: var(--wk-bg-soft);
}

/* 表格内边距收紧 */
:deep(.el-table .cell) {
  padding: 0 10px;
}

/* 表格行 hover 效果 */
:deep(.el-table tbody tr:hover > td) {
  background-color: var(--wk-primary-soft) !important;
}
</style>
