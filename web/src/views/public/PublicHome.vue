<template>
  <!-- ============================================================
       公开首页：未登录可访问的服务器状态展示
       结构与后台总览页共用同一套卡片类，保证两个入口视觉一致
       ============================================================ -->
  <div class="wk-public-shell">
    <!-- ---------------- 顶部导航 ---------------- -->
    <header class="wk-public-nav">
      <!-- 顶栏内容必须套上与正文同一个 1240px 居中容器：
           之前 header 直接 space-between 贴视口两侧，而下方内容是居中的，
           导致品牌名与登录按钮跑到屏幕边缘、与正文不对齐（用户反馈布局不合理） -->
      <div class="wk-public-inner wk-public-nav-row">
        <div class="wk-row" style="gap: 10px; min-width: 0">
          <span class="wk-brand-mark" style="width: 32px; height: 32px; font-size: 15px">悟</span>
          <div style="min-width: 0">
            <strong class="wk-public-nav-title">{{ siteTitle }}</strong>
            <div class="wk-sub wk-public-nav-sub">公开服务器状态</div>
          </div>
        </div>
        <el-button type="primary" plain @click="goAdmin">
          {{ hasToken ? '管理后台' : '管理登录' }}
        </el-button>
      </div>
    </header>

    <main class="wk-public-inner wk-public-main">
      <!-- ---------------- Hero：整体可用性 + 汇总指标 ---------------- -->
      <section class="wk-hero">
        <div class="wk-hero-text">
          <div class="wk-eyebrow">
            <span class="wk-status-dot online is-pulse" style="margin-right: 6px" />
            实时监控 · 更新于 {{ updatedText }}
          </div>
          <h1 class="wk-hero-title">服务器运行状态</h1>
          <p class="wk-sub">
            共 {{ summary.total }} 台服务器，秒级上报 CPU / 内存 / 磁盘 / 网络与运营商链路质量
          </p>
        </div>

        <!-- 可用性圆环：纯 CSS conic-gradient，不引图表库就能表达"在线率" -->
        <div class="wk-availability">
          <div class="wk-avail-ring" :style="ringStyle">
            <span class="wk-num wk-avail-value">{{ uptimePercent }}</span>
            <span class="wk-avail-unit">在线率</span>
          </div>
          <div class="wk-avail-meta">
            <span class="wk-badge ok" dot>{{ summary.online }} 在线</span>
            <span v-if="summary.offline > 0" class="wk-badge fail" dot>
              {{ summary.offline }} 离线
            </span>
          </div>
        </div>
      </section>

      <!-- ---------------- 汇总指标（直接使用后端 summary，避免前端重算口径漂移） ---------------- -->
      <section class="wk-grid-6">
        <WkMetric label="服务器" :value="summary.total" unit="台" :loading="loading" hint="已注册探针总数" />
        <WkMetric
          label="在线"
          :value="summary.online"
          unit="台"
          tone="success"
          :loading="loading"
          hint="心跳正常"
        />
        <WkMetric
          label="离线"
          :value="summary.offline"
          unit="台"
          :tone="summary.offline > 0 ? 'danger' : 'default'"
          :loading="loading"
          :hint="summary.offline > 0 ? '需要检查探针' : '全部在线'"
        />
        <WkMetric
          label="平均 CPU"
          :value="formatOneDecimal(summary.avg_cpu)"
          unit="%"
          :tone="toneOf(summary.avg_cpu)"
          :loading="loading"
          hint="所有上报节点均值"
        />
        <WkMetric
          label="平均内存"
          :value="formatOneDecimal(summary.avg_mem)"
          unit="%"
          :tone="toneOf(summary.avg_mem)"
          :loading="loading"
          hint="所有上报节点均值"
        />
        <WkMetric
          label="平均磁盘"
          :value="formatOneDecimal(summary.avg_disk)"
          unit="%"
          :tone="toneOf(summary.avg_disk)"
          :loading="loading"
          hint="所有上报节点均值"
        />
      </section>

      <!-- ---------------- 工具栏：筛选 + 排序 ---------------- -->
      <div class="wk-toolbar" style="margin: var(--wk-space-6) 0 var(--wk-space-4)">
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

        <el-select v-model="sortKey" size="small" style="width: 140px" aria-label="排序方式">
          <el-option label="按名称" value="name" />
          <el-option label="按 CPU" value="cpu" />
          <el-option label="按内存" value="mem" />
          <el-option label="按磁盘" value="disk" />
        </el-select>

        <!-- 操作提示在手机上占用整行，隐藏（卡片可点击是通用交互） -->
        <span class="wk-sub wk-toolbar-hint">点击卡片查看单台服务器详情</span>
      </div>

      <!-- ---------------- 服务器卡片 ---------------- -->
      <div v-if="loading" class="wk-node-grid">
        <div v-for="i in 6" :key="i" class="wk-node-card" style="cursor: default">
          <WkSkeleton width="130px" height="16px" />
          <WkSkeleton height="8px" />
          <WkSkeleton height="8px" />
          <WkSkeleton height="8px" />
          <WkSkeleton width="60%" height="12px" />
        </div>
      </div>

      <WkEmptyState
        v-else-if="servers.length === 0"
        icon="server"
        title="暂无服务器"
        description="请登录管理后台，在「系统设置 → 安装节点」生成安装命令接入探针"
      >
        <template #action>
          <el-button type="primary" @click="goAdmin">前往管理后台</el-button>
        </template>
      </WkEmptyState>

      <WkEmptyState
        v-else-if="filteredServers.length === 0"
        icon="search"
        title="没有匹配的服务器"
        description="切换上方的状态筛选条件试试"
      />

      <section v-else class="wk-node-grid">
        <article
          v-for="server in filteredServers"
          :key="server.id"
          :class="['wk-node-card', { 'is-offline': server.status === 'offline' || server.status === 'unknown' }]"
          @click="router.push(`/server/${server.id}`)"
        >
          <div class="wk-node-card-head">
            <div style="min-width: 0">
              <div class="wk-node-card-name">{{ server.name || '未命名服务器' }}</div>
              <div class="wk-eyebrow" style="margin-top: 3px">{{ serverMeta(server) }}</div>
            </div>
            <WkStatusDot
              :status="dotStatus(server.status)"
              :pulse="server.status === 'online'"
            />
          </div>

          <div class="wk-node-card-meters">
            <WkProgressBar label="CPU" :value="server.cpu" />
            <WkProgressBar label="内存" :value="server.mem" />
            <WkProgressBar label="磁盘" :value="server.disk" />
          </div>

          <div class="wk-node-card-foot">
            <span class="up">↑ {{ formatRateShort(server.net_up) }}</span>
            <span class="down">↓ {{ formatRateShort(server.net_down) }}</span>
            <span class="wk-node-card-time">
              {{ server.uptime_seconds ? formatDuration(server.uptime_seconds) + ' ·' : '' }}
              {{ relativeTime(server.last_seen_at || server.updated_at) }}
            </span>
          </div>
        </article>
      </section>

      <!-- ---------------- 页脚 ---------------- -->
      <footer v-if="siteFooter" class="wk-public-footer">
        {{ siteFooter }}
      </footer>
    </main>
  </div>
</template>

<script setup lang="ts">
// ============ 公开首页逻辑 ============
// 只访问 /api/public/* 脱敏接口，不携带 JWT；数据字段与后端 publicServerSummary 对齐
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import WkEmptyState from '@/components/WkEmptyState.vue'
import WkMetric from '@/components/WkMetric.vue'
import WkProgressBar from '@/components/WkProgressBar.vue'
import WkSkeleton from '@/components/WkSkeleton.vue'
import WkStatusDot from '@/components/WkStatusDot.vue'
import http from '@/utils/http'
import { usePolling } from '@/composables/usePolling'
import { useTheme } from '@/composables/useTheme'
import {
  archText,
  formatClock,
  formatDuration,
  formatRateShort,
  loadLevel,
  relativeTime,
} from '@/utils/format'

// 服务器字段（与后端脱敏接口一致，全部可选，缺字段时前端显示 '-'）
interface PublicServer {
  id: string
  name: string
  online: boolean
  status: 'online' | 'offline' | 'stale' | 'unknown'
  last_seen_at?: string
  updated_at?: string
  os_version?: string
  arch?: string
  platform?: string
  region?: string
  cpu?: number
  mem?: number
  disk?: number
  net_up?: number
  net_down?: number
  uptime_seconds?: number
  cpu_model?: string
}

// 后端 summary 直接给出的聚合值，前端不再重复计算，避免两处口径不一致
interface PublicSummary {
  total: number
  online: number
  offline: number
  avg_cpu: number
  avg_mem: number
  avg_disk: number
}

const router = useRouter()
const { title: siteTitle, footer: siteFooter, load: loadTheme } = useTheme()

const loading = ref(true)
const servers = ref<PublicServer[]>([])
const summary = ref<PublicSummary>({ total: 0, online: 0, offline: 0, avg_cpu: 0, avg_mem: 0, avg_disk: 0 })
const generatedAt = ref<number>(0)

const hasToken = computed(() => Boolean(localStorage.getItem('access_token')))

// 拉取公开服务器列表（每秒静默刷新，保持"实时"承诺）
async function loadData() {
  try {
    const res = await http.get(`/api/public/servers?_=${Date.now()}`)
    servers.value = res.data.servers || []
    if (res.data.summary) summary.value = res.data.summary
    generatedAt.value = res.data.generated_at ? new Date(res.data.generated_at).getTime() : Date.now()
  } catch (error) {
    console.error('获取公开服务器列表失败', error)
  } finally {
    loading.value = false
  }
}

// 每秒一次轮询，页面切到后台自动暂停（见 usePolling）
usePolling(loadData, 1000)

onMounted(() => {
  // 站点标题/页脚由主题单例提供，公开页同样生效
  loadTheme()
})

// ---------------- 展示辅助 ----------------
function formatOneDecimal(value?: number | null): string {
  return typeof value === 'number' ? value.toFixed(1) : '-'
}

function toneOf(value?: number): 'default' | 'warning' | 'danger' {
  const level = loadLevel(value)
  if (level === 'danger') return 'danger'
  if (level === 'warning') return 'warning'
  return 'default'
}

function dotStatus(status: string): 'online' | 'offline' | 'stale' | 'muted' {
  if (status === 'online') return 'online'
  if (status === 'stale') return 'stale'
  if (status === 'offline') return 'offline'
  return 'muted'
}

// 系统 · 区域 · 架构（qio.ng 风格一行交代身份）
function serverMeta(server: PublicServer): string {
  const parts: string[] = []
  if (server.platform) parts.push(server.platform)
  else if (server.os_version) parts.push(server.os_version)
  if (server.region) parts.push(server.region)
  if (server.arch) parts.push(archText(server.arch))
  return parts.length ? parts.join(' · ') : '系统信息待上报'
}

const updatedText = computed(() =>
  generatedAt.value ? formatClock(generatedAt.value) : '连接中'
)

// 在线率：整数百分比，无服务器时显示 0，避免"100%"造成误导
const uptimePercent = computed(() => {
  const { total, online } = summary.value
  if (!total) return '0%'
  return `${Math.round((online / total) * 100)}%`
})

// 可用性圆环：在线比例决定 conic-gradient 角度，颜色跟随状态令牌
const ringStyle = computed(() => {
  const { total, online } = summary.value
  const ratio = total ? online / total : 0
  const color = ratio === 1
    ? 'var(--wk-success)'
    : ratio >= 0.6
      ? 'var(--wk-warning)'
      : 'var(--wk-danger)'
  return {
    background: `conic-gradient(${color} ${ratio * 360}deg, color-mix(in srgb, var(--wk-text) 10%, transparent) 0deg)`,
  }
})

// ---------------- 筛选与排序 ----------------
const statusFilter = ref<'all' | 'online' | 'offline'>('all')
// 默认按名称：与后台节点列表/总览保持一致，也避免每秒按 CPU 重排导致卡片位置跳动
const sortKey = ref<'name' | 'cpu' | 'mem' | 'disk'>('name')

const statusChips = computed(() => [
  { label: '全部', value: 'all' as const, count: servers.value.length },
  {
    label: '在线',
    value: 'online' as const,
    count: servers.value.filter((item) => item.status === 'online').length,
  },
  {
    label: '离线',
    value: 'offline' as const,
    count: servers.value.filter((item) => item.status !== 'online').length,
  },
])

const filteredServers = computed(() => {
  const list = servers.value.filter((item) => {
    if (statusFilter.value === 'online') return item.status === 'online'
    if (statusFilter.value === 'offline') return item.status !== 'online'
    return true
  })
  const sorted = list.slice()
  if (sortKey.value === 'name') {
    sorted.sort((a, b) => (a.name || '').localeCompare(b.name || '', 'zh-Hans-CN'))
  } else {
    const key = sortKey.value
    sorted.sort((a, b) => (Number(b[key]) || -1) - (Number(a[key]) || -1))
  }
  return sorted
})

function goAdmin() {
  router.push(hasToken.value ? '/dashboard' : '/login')
}
</script>

<style scoped>
/* Hero 区：左侧文字 + 右侧可用性圆环 */
.wk-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wk-space-6);
  padding: var(--wk-space-6) 0 var(--wk-space-5);
}

.wk-hero-title {
  font-size: var(--wk-fs-3xl);
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.15;
  margin: var(--wk-space-2) 0;
}

/* 可用性圆环 */
.wk-availability {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wk-space-3);
  flex-shrink: 0;
}

.wk-avail-ring {
  position: relative;
  width: 92px;
  height: 92px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 内圈挖空：用 panel 色覆盖中心，形成圆环而无需 SVG */
.wk-avail-ring::before {
  content: "";
  position: absolute;
  inset: 9px;
  border-radius: 50%;
  background: var(--wk-panel);
  border: 1px solid var(--wk-border);
}

.wk-avail-value {
  position: relative;
  z-index: 1;
  font-size: var(--wk-fs-lg);
  font-weight: 600;
}

.wk-avail-unit {
  position: absolute;
  z-index: 1;
  bottom: 24px;
  font-size: var(--wk-fs-xs);
  color: var(--wk-text-muted);
}

.wk-avail-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: center;
}

/* 六列汇总在小屏降级为三列 */
@media (max-width: 980px) {
  .wk-grid-6 {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  /* 手机上标题与在线率环并排（环缩到 64px 靠右），
     而不是上下堆叠——堆叠会让首屏只看到标题和一块空白 */
  .wk-hero {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    gap: var(--wk-space-4);
    padding: var(--wk-space-4) 0 var(--wk-space-3);
  }

  .wk-hero-title {
    font-size: var(--wk-fs-2xl);
  }

  .wk-avail-ring {
    width: 64px;
    height: 64px;
  }

  .wk-avail-ring::before {
    inset: 7px;
  }

  .wk-avail-value {
    font-size: var(--wk-fs-base);
  }

  .wk-avail-unit {
    bottom: 15px;
    font-size: 9px;
  }

  .wk-grid-6 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .wk-toolbar-hint {
    display: none;
  }
}

/* 工具栏右侧提示：桌面靠右对齐，手机上隐藏 */
.wk-toolbar-hint {
  margin-left: auto;
}

/* 页脚 */
.wk-public-footer {
  margin-top: var(--wk-space-8);
  padding: var(--wk-space-5) 0 var(--wk-space-8);
  border-top: 1px solid var(--wk-border);
  color: var(--wk-text-muted);
  font-size: var(--wk-fs-sm);
  text-align: center;
}
</style>
