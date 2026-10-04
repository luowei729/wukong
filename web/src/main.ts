// Vue3 应用入口
// 初始化 Element Plus、Pinia、Vue Router，并在挂载前完成主题引导（避免首屏闪白）
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import { initTheme } from './composables/useTheme'
import './styles/index.scss'

const app = createApp(App)

// 注册 Element Plus 图标：保持全量注册以兼容旧模板中的直接引用
// 说明：本次重构的图标一律用内联 SVG（不依赖 EP 图标库），保留注册只为向后兼容
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

// 主题引导必须在 mount 之前：先按 localStorage 立刻上屏深浅色，
// 再异步拉取站点主题（标题/主色），杜绝"暗黑站点先闪一下白"的观感问题
initTheme()

app.use(createPinia())
app.use(router)
app.use(ElementPlus)

app.mount('#app')
