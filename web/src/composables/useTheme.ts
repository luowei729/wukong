// =============================================
// 主题单例 composable
// 原因：主题逻辑此前在 MainLayout / Login / PublicHome / Settings 四处各写一份，
//       且设置页保存的 theme.primary 从未写进 CSS 变量（只写了 data-theme），
//       导致"自定义主色"这个已确认的架构决策（#11）实际不生效。
// 方案：主题状态与 DOM 注入集中到这里，任何页面只调 useTheme()，
//       主色只写 --wk-primary 一个变量，其余派生色由 variables.scss 的 color-mix() 自动联动。
// =============================================
import { computed, onMounted, reactive, ref } from "vue"
import http from "@/utils/http"

export type ThemePreset = "dark" | "light"

// 站点主题（后端 SQLite 固化值）
const state = reactive({
  preset: "dark" as ThemePreset,
  primary: "",
  title: "wukong 监控",
  footer: "",
  loaded: false,
})

// 主题版本号：任何影响图表颜色的变化（preset / primary）都会 +1，
// WkChart 监听它来重建 option，解决"切浅色后图表仍是深色轴"的问题
const themeVersion = ref(0)

const isDark = computed(() => state.preset === "dark")

/** 把 preset 写进 DOM：data-theme 驱动令牌切换，dark class 兼容 Element Plus 深色变量 */
function applyPresetToDOM(preset: ThemePreset) {
  document.documentElement.dataset.theme = preset
  document.documentElement.classList.toggle("dark", preset === "dark")
  // meta theme-color 让移动端浏览器状态栏跟随主题
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute("content", preset === "dark" ? "#0a0a0b" : "#f7f7f8")
}

/**
 * 注入自定义主色
 * 只覆盖 --wk-primary：派生的 hover/soft/glow 与 --el-color-primary 都写成
 * color-mix(in srgb, var(--wk-primary) …)，会在计算阶段自动重算，无需逐个赋值。
 */
function applyPrimaryToDOM(primary: string) {
  const root = document.documentElement
  if (!primary) {
    // 未配置自定义主色时删除内联覆盖，回落到主题令牌默认值
    root.style.removeProperty("--wk-primary")
    return
  }
  root.style.setProperty("--wk-primary", primary)
}

/** 应用当前状态到 DOM 并通知图表刷新 */
function apply() {
  applyPresetToDOM(state.preset)
  applyPrimaryToDOM(state.primary)
  document.title = state.title
  localStorage.setItem("theme", state.preset)
  // 写入站点标题缓存，router.afterEach 依赖它拼页面标题
  localStorage.setItem("site_title", state.title)
  themeVersion.value++
}

/** 切换深浅色（用户本地选择优先于站点默认预设） */
function setPreset(preset: ThemePreset) {
  state.preset = preset
  apply()
}

function toggleTheme() {
  setPreset(state.preset === "dark" ? "light" : "dark")
}

/** 预览主色（设置页选色时即时生效，未保存也能看到效果） */
function setPrimary(primary: string) {
  state.primary = primary
  apply()
}

/**
 * 从后端加载站点主题
 * 有 JWT 时读 /api/theme（字段完整，含 primary），
 * 未登录的公开页只能读 /api/public/theme（后端未返回 primary，属已知限制）
 */
async function load(force = false) {
  if (state.loaded && !force) return
  const token = localStorage.getItem("access_token")
  try {
    const res = await http.get(
      token ? `/api/theme?_=${Date.now()}` : `/api/public/theme?_=${Date.now()}`
    )
    const data = res.data || {}
    state.title = data.title || state.title
    state.footer = data.footer_text || ""
    state.primary = data.primary || ""
    // 首次访问（本地无记录）才采用站点默认预设；已有本地选择则尊重用户，避免被服务端改回
    if (!localStorage.getItem("theme") && data.preset) {
      state.preset = data.preset === "light" ? "light" : "dark"
    }
    state.loaded = true
    apply()
  } catch {
    // 主题接口失败不影响页面可用性，保持本地/默认主题
    state.loaded = true
  }
}

/**
 * 应用启动时调用：先用本地缓存立即上屏（避免首屏闪白），再异步拉取站点主题
 */
function bootstrap() {
  const cached = localStorage.getItem("theme")
  state.preset = cached === "light" ? "light" : "dark"
  apply()
  return load()
}

/**
 * 组件内使用：挂载时保证主题已加载（公开页刷新进来也不会漏掉站点标题）
 * 返回的均为模块级单例状态，多组件共享同一份，不会重复请求
 */
export function useTheme() {
  onMounted(() => {
    if (!state.loaded) load()
  })

  return {
    state,
    isDark,
    themeVersion,
    preset: computed(() => state.preset),
    primary: computed(() => state.primary),
    title: computed(() => state.title),
    footer: computed(() => state.footer),
    bootstrap,
    load,
    apply,
    setPreset,
    setPrimary,
    toggleTheme,
  }
}

/** 应用入口用的非组件版初始化（不依赖生命周期钩子） */
export function initTheme() {
  return bootstrap()
}
