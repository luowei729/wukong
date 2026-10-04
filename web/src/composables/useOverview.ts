// =============================================
// 集群概览数据单例
// 原因：MainLayout 每 5s 拉 3 个接口、Dashboard 每 1s 拉 2 个接口、Nodes 每 1s 拉 2 个接口，
//       同一份"节点 + 最新指标"数据被重复请求，且各页各自算平均，口径容易漂移。
// 方案：模块级单例 + 引用计数，多个组件共享一个 1s 定时器与一份合并后的数据；
//       同时在内存里维护最近若干采样点（滚动历史），供 KPI 卡的 sparkline 与集群趋势图使用，
//       无需新增后端聚合接口（保持本次"纯前端重构"的约束）。
// =============================================
import { computed, onMounted, onUnmounted, ref } from "vue"
import http from "@/utils/http"
import { average, nodeState } from "@/utils/format"

// 滚动历史最大采样点：1s 采集 × 600 = 最近 10 分钟，够画 sparkline 与短周期趋势
const HISTORY_LIMIT = 600

export interface OverviewHistoryPoint {
  /** 采样时刻（毫秒） */
  t: number
  /** 集群平均 CPU */
  cpu: number | null
  /** 集群平均内存 */
  mem: number | null
  /** 在线节点数 */
  online: number
}

// 模块级共享状态：所有组件读到的是同一份数据
const nodes = ref<any[]>([])
const alerts = ref<any[]>([])
const loading = ref(true)
const lastUpdated = ref<number>(0)
const history = ref<OverviewHistoryPoint[]>([])

// 引用计数：决定共享定时器是否需要启动/停止
let refCount = 0
let timer: ReturnType<typeof setInterval> | null = null
let inFlight = false

/** 合并探针元数据与最新指标，得到页面直接可用的节点对象 */
async function fetchOnce() {
  // 上一次请求未回来时跳过本轮，避免慢接口导致请求堆积
  if (inFlight) return
  inFlight = true
  try {
    const stamp = Date.now()
    const [agentsRes, latestRes, alertsRes] = await Promise.all([
      http.get(`/api/agents?_=${stamp}`),
      http.get(`/api/agents/latest?_=${stamp}`),
      // 告警接口失败不应影响节点展示，单独兜底成空数组
      http.get(`/api/alerts?_=${stamp}`).catch(() => ({ data: [] as any[] })),
    ])

    const latest = latestRes.data || {}
    nodes.value = (agentsRes.data || []).map((node: any) => ({
      ...node,
      ...(latest[node.id] || {}),
    }))
    alerts.value = Array.isArray(alertsRes.data) ? alertsRes.data : []

    // 追加滚动历史采样点，超出上限丢弃最旧的
    const metricNodes = nodes.value.filter((n: any) => typeof n.cpu === "number")
    const next = history.value.slice()
    next.push({
      t: Date.now(),
      cpu: average(metricNodes.map((n: any) => n.cpu)),
      mem: average(metricNodes.map((n: any) => n.mem)),
      online: nodes.value.filter((n: any) => nodeState(n) === "online").length,
    })
    if (next.length > HISTORY_LIMIT) next.splice(0, next.length - HISTORY_LIMIT)
    // 整体重新赋值而不是 push：保持数组引用变化，下游 watch(浅比较) 与图表才能感知到新增采样
    history.value = next
    lastUpdated.value = Date.now()
    loading.value = false
  } catch (error) {
    // 保留上一次数据，避免网络抖动导致页面瞬间清空
    console.error("获取集群概览数据失败", error)
    loading.value = false
  } finally {
    inFlight = false
  }
}

function startShared() {
  if (timer) return
  void fetchOnce()
  timer = setInterval(() => void fetchOnce(), 1000)
}

function stopShared() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

// 页面切到后台时暂停共享轮询，回到前台立即补一次
function onVisibilityChange() {
  if (refCount <= 0) return
  if (document.visibilityState === "visible") {
    startShared()
  } else {
    stopShared()
  }
}

/**
 * 在线使用的概览数据。多个组件同时使用时只维持一个 1s 定时器，
 * 全部组件卸载后自动停止，不会残留后台轮询。
 */
export function useOverview() {
  onMounted(() => {
    if (refCount === 0) {
      document.addEventListener("visibilitychange", onVisibilityChange)
      startShared()
    }
    refCount++
  })

  onUnmounted(() => {
    refCount = Math.max(0, refCount - 1)
    if (refCount === 0) {
      document.removeEventListener("visibilitychange", onVisibilityChange)
      stopShared()
    }
  })

  // 在线节点：统一用 nodeState（连接 + 指标新鲜度），与公开页、节点列表状态灯同一口径；
  // 不能只看 agents.online，否则“流在但采集已挂”的节点会被顶栏当成在线
  const onlineNodes = computed(() => nodes.value.filter((n: any) => nodeState(n) === "online"))

  // 有实时指标的在线节点（平均值只统计它们，避免用 0 拉低集群均值）
  const metricNodes = computed(() =>
    nodes.value.filter((n: any) => typeof n.cpu === "number")
  )

  const avgCpu = computed(() => average(metricNodes.value.map((n: any) => n.cpu)))
  const avgMem = computed(() => average(metricNodes.value.map((n: any) => n.mem)))
  const avgDisk = computed(() => average(metricNodes.value.map((n: any) => n.disk)))
  const totalNetUp = computed(() =>
    metricNodes.value.reduce((acc: number, n: any) => acc + (n.net_up || 0), 0)
  )
  const totalNetDown = computed(() =>
    metricNodes.value.reduce((acc: number, n: any) => acc + (n.net_down || 0), 0)
  )

  const firingAlerts = computed(() =>
    alerts.value.filter((a: any) => a.status === "firing")
  )

  // 今日告警：按本地日期比较 fired_at
  const todayAlerts = computed(() => {
    const today = new Date().toDateString()
    return alerts.value.filter((a: any) => {
      const fired = new Date(a.fired_at || a.created_at || a.timestamp)
      return !Number.isNaN(fired.getTime()) && fired.toDateString() === today
    })
  })

  // 告警涉及的去重节点数，用于告警中心概览
  const alertNodeCount = computed(
    () => new Set(firingAlerts.value.map((a: any) => a.agent_id)).size
  )

  const cpuHistory = computed(() =>
    history.value.map((item) => item.cpu).filter((v): v is number => v !== null)
  )
  const memHistory = computed(() =>
    history.value.map((item) => item.mem).filter((v): v is number => v !== null)
  )

  return {
    // 数据
    nodes,
    onlineNodes,
    alerts,
    history,
    loading,
    lastUpdated,
    // 派生指标
    totalCount: computed(() => nodes.value.length),
    onlineCount: computed(() => onlineNodes.value.length),
    offlineCount: computed(() => nodes.value.length - onlineNodes.value.length),
    avgCpu,
    avgMem,
    avgDisk,
    totalNetUp,
    totalNetDown,
    firingAlerts,
    firingCount: computed(() => firingAlerts.value.length),
    todayAlerts,
    alertNodeCount,
    cpuHistory,
    memHistory,
    // 操作
    refresh: fetchOnce,
  }
}

/** 强制刷新共享数据（供手动"刷新"按钮调用） */
export function refreshOverview() {
  return fetchOnce()
}
