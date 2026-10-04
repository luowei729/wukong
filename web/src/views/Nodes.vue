<template>
  <!-- ============ 节点列表页（table-first） ============ -->
  <div class="wk-stack">
    <!-- 工具栏：搜索 + 状态筛选 + 排序，全部为纯前端过滤（节点量级小，无需后端分页） -->
    <div class="wk-toolbar">
      <div class="wk-search">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round">
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-3.5-3.5" />
        </svg>
        <input
          v-model="keyword"
          type="search"
          placeholder="搜索名称 / 主机名 / 出口 IP"
          aria-label="搜索节点"
        />
      </div>

      <div class="wk-chips" role="group" aria-label="状态筛选">
        <button
          v-for="chip in statusChips"
          :key="chip.value"
          type="button"
          :class="['wk-chip', { active: statusFilter === chip.value }]"
          @click="statusFilter = chip.value"
        >
          {{ chip.label }}
          <span class="wk-chip-count">{{ chip.count }}</span>
        </button>
      </div>

      <el-select v-model="sortKey" size="small" style="width: 148px" aria-label="排序方式">
        <el-option label="按名称" value="name" />
        <el-option label="按 CPU 降序" value="cpu" />
        <el-option label="按内存降序" value="mem" />
        <el-option label="按磁盘降序" value="disk" />
        <el-option label="按最近上报" value="recent" />
      </el-select>

      <span class="wk-sub" style="margin-left: auto">
        显示 {{ filteredNodes.length }} / {{ nodes.length }} 个节点
      </span>
    </div>

    <!-- 节点表格 -->
    <WkCard padding="none">
      <!-- 首次加载骨架 -->
      <div v-if="loading" style="padding: var(--wk-space-5)">
        <WkSkeleton v-for="i in 6" :key="i" height="18px" style="margin-bottom: 12px" />
      </div>

      <WkEmptyState
        v-else-if="nodes.length === 0"
        icon="server"
        title="还没有节点接入"
        description="到「系统设置 → 安装节点」生成安装命令，在服务器上执行即可自动注册上报"
      >
        <template #action>
          <el-button type="primary" @click="router.push('/settings')">前往安装节点</el-button>
        </template>
      </WkEmptyState>

      <WkEmptyState
        v-else-if="filteredNodes.length === 0"
        icon="search"
        title="没有匹配的节点"
        description="调整搜索关键字或状态筛选条件试试"
      />

      <el-table
        v-else
        :data="filteredNodes"
        style="width: 100%"
        :row-class-name="rowClassName"
      >
        <!-- 状态：四态统一判定（在线/数据延迟/离线/待上报），与公开页同一口径 -->
        <el-table-column label="状态" width="48" align="center">
          <template #default="{ row }">
            <WkStatusDot
              :status="dotStatus(row)"
              :size="8"
              :pulse="nodeState(row) === 'online'"
              :title="nodeStateText(nodeState(row))"
            />
          </template>
        </el-table-column>

        <!-- 名称：点击进入详情，改名入口悬浮出现，避免行内常驻按钮干扰阅读 -->
        <el-table-column label="名称" min-width="140">
          <template #default="{ row }">
            <div class="wk-name-cell">
              <span class="wk-node-name" @click="goToNode(row.id)">{{ displayName(row) }}</span>
              <button
                type="button"
                class="wk-row-action"
                title="修改名称"
                aria-label="修改名称"
                @click="openRename(row)"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 20h9" />
                  <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z" />
                </svg>
              </button>
            </div>
          </template>
        </el-table-column>

        <!-- CPU / 内存 / 磁盘：数值在上 + 微型的整宽条在下，窄列里也能看清 -->
        <el-table-column label="CPU" min-width="88">
          <template #default="{ row }">
            <WkProgressBar class="wk-cell-meter" :value="row.cpu" />
          </template>
        </el-table-column>
        <el-table-column label="内存" min-width="88">
          <template #default="{ row }">
            <WkProgressBar class="wk-cell-meter" :value="row.mem" />
          </template>
        </el-table-column>
        <el-table-column label="磁盘" min-width="88">
          <template #default="{ row }">
            <WkProgressBar class="wk-cell-meter" :value="row.disk" />
          </template>
        </el-table-column>

        <!-- 上下行速率：单行紧凑格式，列宽可控不被省略号截断 -->
        <el-table-column label="网络 ↑/↓ (B/s)" min-width="150">
          <template #default="{ row }">
            <div class="wk-net-cell">
              <span class="up">↑ {{ netShort(row.net_up) }}</span>
              <span class="down">↓ {{ netShort(row.net_down) }}</span>
            </div>
          </template>
        </el-table-column>

        <!-- 出口 IP：主显示 IPv4，IPv6 放 title，降低列宽占用（仅后台可见） -->
        <el-table-column label="出口 IP" min-width="124">
          <template #default="{ row }">
            <span v-if="row.ip_v4 || row.ip_v6" class="wk-num wk-ip" :title="ipTitle(row)">
              {{ row.ip_v4 || row.ip_v6 }}
            </span>
            <span v-else class="wk-sub">-</span>
          </template>
        </el-table-column>

        <!-- 系统 · 架构：单行 -->
        <el-table-column label="系统" min-width="110">
          <template #default="{ row }">
            <span class="wk-sub">{{ osText(row) }}{{ row.arch ? ` · ${archText(row.arch)}` : '' }}</span>
          </template>
        </el-table-column>

        <!-- 探针版本 -->
        <el-table-column label="探针" width="96">
          <template #default="{ row }">
            <span class="wk-tag">{{ row.agent_ver || '-' }}</span>
          </template>
        </el-table-column>

        <!-- 运行时长 -->
        <el-table-column label="运行" width="78" align="right">
          <template #default="{ row }">
            <span class="wk-num">{{ formatDuration(row.uptime_seconds) }}</span>
          </template>
        </el-table-column>

        <!-- 最近上报：相对时间为主，绝对时间放 title 便于精确排查 -->
        <el-table-column label="最近上报" width="84" align="right">
          <template #default="{ row }">
            <span class="wk-sub" :title="formatDateTime(row.last_seen_at || row.updated_at)">
              {{ relativeTime(row.last_seen_at || row.updated_at) }}
            </span>
          </template>
        </el-table-column>

        <!-- 操作：图标按钮，宽度固定不挤压数据列 -->
        <el-table-column label="操作" width="78" fixed="right" align="right">
          <template #default="{ row }">
            <div class="wk-row-actions">
              <button
                type="button"
                class="wk-row-action"
                title="查看详情"
                aria-label="查看详情"
                @click="goToNode(row.id)"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z" />
                  <circle cx="12" cy="12" r="3" />
                </svg>
              </button>
              <button
                type="button"
                class="wk-row-action is-danger"
                title="删除节点"
                aria-label="删除节点"
                @click="deleteNode(row)"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" />
                </svg>
              </button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </WkCard>
  </div>
</template>

<script setup lang="ts">
// ============ 节点列表页逻辑 ============
// 数据来自 useOverview() 共享单例：与顶栏、总览页共用同一个每秒轮询
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import WkCard from '@/components/WkCard.vue'
import WkEmptyState from '@/components/WkEmptyState.vue'
import WkProgressBar from '@/components/WkProgressBar.vue'
import WkSkeleton from '@/components/WkSkeleton.vue'
import WkStatusDot from '@/components/WkStatusDot.vue'
import http from '@/utils/http'
import { refreshOverview, useOverview } from '@/composables/useOverview'
import {
  archText,
  formatBytesShort,
  formatDateTime,
  formatDuration,
  nodeState,
  nodeStateText,
  relativeTime,
} from '@/utils/format'
import type { NodeState } from '@/utils/format'

const router = useRouter()
const { nodes, loading } = useOverview()

// ---------------- 筛选与排序状态 ----------------
const keyword = ref('')
const statusFilter = ref<'all' | 'online' | 'offline'>('all')
// 默认按名称：运维找机器靠名字，而不是靠“谁 CPU 高”这种每秒变动的顺序
const sortKey = ref<'name' | 'cpu' | 'mem' | 'disk' | 'recent'>('name')

// 状态灯映射：unknown（在线但从未上报）用中性灰，不能假装成离线
function dotStatus(row: any) {
  const state: NodeState = nodeState(row)
  if (state === 'online') return 'online'
  if (state === 'stale') return 'stale'
  if (state === 'offline') return 'offline'
  return 'muted'
}

// chips 带数量，一眼看出各状态规模（数量来自当前节点集合，不受关键字影响）
// 统计口径用 nodeState，与状态灯一致：“在线但数据延迟”的不算在线
const statusChips = computed(() => {
  const online = nodes.value.filter((node: any) => nodeState(node) === 'online').length
  return [
    { label: '全部', value: 'all' as const, count: nodes.value.length },
    { label: '在线', value: 'online' as const, count: online },
    {
      label: '离线',
      value: 'offline' as const,
      count: nodes.value.length - online,
    },
  ]
})

// 节点显示名：后台自定义名称 > 主机名 > ID 前缀
function displayName(node: any): string {
  return node.name || node.hostname || `节点 ${String(node.id).slice(0, 8)}`
}

// 系统展示：platform 更全（含版本号），回退到 os_version
function osText(node: any): string {
  return node.platform || node.os_version || '系统信息待上报'
}

// 速率紧凑写法：3.8M / 12.4G（表头已标明单位是 B/s，单元格不再重复写 /s）
function netShort(value?: number): string {
  return formatBytesShort(value)
}

// 出口 IP 悬浮标题：单元格只放 IPv4 以免拉宽表格，双栈地址在 title 里完整展示
function ipTitle(node: any): string {
  const parts: string[] = []
  if (node.ip_v4) parts.push(`IPv4 ${node.ip_v4}`)
  if (node.ip_v6) parts.push(`IPv6 ${node.ip_v6}`)
  return parts.join(' · ')
}

// 关键字匹配范围：名称、主机名、ID、出口 IP
function matchKeyword(node: any, text: string): boolean {
  if (!text) return true
  const haystack = [
    node.name,
    node.hostname,
    node.id,
    node.ip_v4,
    node.ip_v6,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  return haystack.includes(text.toLowerCase())
}

// 过滤 + 排序后的列表：数值列缺失时按 -1 参与排序，保证离线节点沉底
const filteredNodes = computed(() => {
  const list = nodes.value.filter((node: any) => {
    if (statusFilter.value !== 'all') {
      // “在线”只含数据新鲜的；“离线”包含数据延迟与待上报，与状态灯语义一致
      const onlineLike = nodeState(node) === 'online'
      if (statusFilter.value === 'online' && !onlineLike) return false
      if (statusFilter.value === 'offline' && onlineLike) return false
    }
    return matchKeyword(node, keyword.value.trim())
  })

  const sorted = list.slice()
  if (sortKey.value === 'name') {
    // 中文节点名用本地排序，避免“上海/东京/首尔”按码位排列
    sorted.sort((a: any, b: any) => displayName(a).localeCompare(displayName(b), 'zh-Hans-CN'))
  } else if (sortKey.value === 'recent') {
    sorted.sort(
      (a: any, b: any) =>
        new Date(b.last_seen_at || b.updated_at || 0).getTime() -
        new Date(a.last_seen_at || a.updated_at || 0).getTime()
    )
  } else {
    const key = sortKey.value
    sorted.sort((a: any, b: any) => (Number(b[key]) || -1) - (Number(a[key]) || -1))
  }
  return sorted
})

// 离线行弱化
function rowClassName({ row }: { row: any }) {
  return row.online ? '' : 'row-offline'
}

// ---------------- 节点操作（接口与原来保持一致） ----------------
// 改名：弹出输入框，PUT /api/agents/{id}
async function openRename(row: any) {
  try {
    const { value } = await ElMessageBox.prompt('请输入新的服务器节点名称', '修改节点名称', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: row.name || row.hostname || '',
      inputPattern: /^.{1,64}$/,
      inputErrorMessage: '节点名称长度必须为 1-64 个字符',
    })
    await http.put(`/api/agents/${row.id}`, { name: value })
    ElMessage.success('节点名称已保存')
    // 立即刷新共享数据，顶栏与总览页同步显示新名称
    await refreshOverview()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '修改节点名称失败')
    }
  }
}

// 删除：二次确认后 DELETE /api/agents/{id}
async function deleteNode(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认删除节点“${displayName(row)}”？删除后该节点的探针需要重新注册才能恢复。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
    await http.delete(`/api/agents/${row.id}`)
    ElMessage.success('节点已删除')
    await refreshOverview()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '删除节点失败')
    }
  }
}

function goToNode(id: string) {
  router.push(`/nodes/${id}`)
}
</script>

<style scoped>
/* 表格内的指标条：数值在上 / 短条在下。
   DOM 顺序是 track 在前、value 在后，所以用 order 把数值提到第一行；
   track 必须 flex:none，否则在纵向 flex 里会被压缩到 0 高度而看不见 */
.wk-cell-meter {
  flex-direction: column;
  align-items: stretch;
  gap: 3px;
}

.wk-cell-meter :deep(.wk-meter-value) {
  order: -1;
  width: auto;
  text-align: left;
}

.wk-cell-meter :deep(.wk-meter-track) {
  flex: none;
  height: 4px;
}

/* 搜索框：原生 input + 令牌样式，避免 EP 输入框在工具栏里偏高 */
.wk-search {
  display: flex;
  align-items: center;
  gap: 8px;
  height: var(--wk-ctl-md);
  padding: 0 10px;
  min-width: 260px;
  background: var(--wk-bg-recess);
  border: 1px solid var(--wk-border);
  border-radius: var(--wk-radius-sm);
  transition: border-color var(--wk-dur-fast) var(--wk-ease),
    box-shadow var(--wk-dur-fast) var(--wk-ease);
}

.wk-search:focus-within {
  border-color: var(--wk-primary);
  box-shadow: var(--wk-ring);
}

.wk-search svg {
  width: 15px;
  height: 15px;
  color: var(--wk-text-muted);
  flex-shrink: 0;
}

.wk-search input {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--wk-text);
  font-size: var(--wk-fs-base);
  font-family: inherit;
  outline: none;
  padding: 0;
}

/* 名称单元格：文本占满，改名按钮悬浮时才出现 */
.wk-name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.wk-node-name {
  cursor: pointer;
  font-weight: 500;
  color: var(--wk-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wk-node-name:hover {
  color: var(--wk-primary);
}

/* 行内图标按钮：16px 图标，平时低不透明度弱化，自身 hover 时强化 */
.wk-row-action {
  width: 26px;
  height: 26px;
  border: none;
  border-radius: var(--wk-radius-xs);
  background: transparent;
  color: var(--wk-text-muted);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0.7;
  transition: opacity var(--wk-dur-fast) var(--wk-ease),
    background var(--wk-dur-fast) var(--wk-ease), color var(--wk-dur-fast) var(--wk-ease);
}

.wk-row-action svg {
  width: 15px;
  height: 15px;
}

.wk-row-actions {
  display: inline-flex;
  gap: 2px;
}

.wk-row-action:hover,
.wk-row-action:focus-visible {
  background: var(--wk-bg-soft);
  color: var(--wk-text);
  opacity: 1;
}

.wk-row-action.is-danger:hover {
  color: var(--wk-danger-text);
  background: var(--wk-danger-soft);
}

/* 上下行：单行横向排列，颜色区分方向，列宽更省 */
.wk-net-cell {
  display: flex;
  align-items: center;
  gap: var(--wk-space-3);
  font-family: var(--wk-mono-family);
  font-variant-numeric: tabular-nums;
  font-size: var(--wk-fs-sm);
  white-space: nowrap;
}

.wk-net-cell .up {
  color: var(--wk-success-text);
}

.wk-net-cell .down {
  color: var(--wk-primary);
}

/* IP 单元格：只显示一个地址，完整（含 IPv6）放 hover 标题 */
.wk-ip {
  font-size: var(--wk-fs-sm);
  color: var(--wk-text-secondary);
  white-space: nowrap;
}
</style>
