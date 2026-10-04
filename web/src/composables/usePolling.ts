// =============================================
// 轮询 composable
// 原因：全站多处 setInterval(fetch, 1000)，切到后台标签页仍在每秒打接口，
//       多个页面叠加会放大 SQLite 写锁竞争（参考 2026-07-23 主控卡死事故）。
// 方案：统一的轮询封装——立即执行一次、组件卸载自动清理、页面不可见时暂停、
//       重新可见时立刻补一次刷新，用户回到标签页看到的就是最新数据。
// =============================================
import { onUnmounted, ref } from "vue"

export interface PollingHandle {
  /** 是否正在运行 */
  running: ReturnType<typeof ref<boolean>>
  start: () => void
  stop: () => void
  /** 手动立即执行一次（例如点击"刷新"按钮） */
  tick: () => void
}

export function usePolling(
  task: () => void | Promise<unknown>,
  intervalMs: number,
  options: { immediate?: boolean; autoStart?: boolean } = {}
): PollingHandle {
  const immediate = options.immediate ?? true
  const running = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null

  const run = () => {
    // 单次执行异常不应打断定时器，错误由调用方自行 console/提示
    try {
      void task()
    } catch (error) {
      console.error("轮询任务执行失败", error)
    }
  }

  const start = () => {
    if (timer) return
    running.value = true
    if (immediate) run()
    timer = setInterval(run, intervalMs)
  }

  const stop = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
    running.value = false
  }

  // 页面不可见时暂停轮询，回到前台立即补一次刷新再恢复定时器
  const onVisibilityChange = () => {
    if (document.visibilityState === "visible") {
      start()
    } else {
      stop()
    }
  }

  if (options.autoStart ?? true) {
    start()
  }
  document.addEventListener("visibilitychange", onVisibilityChange)
  onUnmounted(() => {
    document.removeEventListener("visibilitychange", onVisibilityChange)
    stop()
  })

  return { running, start, stop, tick: run }
}
