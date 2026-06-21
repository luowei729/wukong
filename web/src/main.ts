// Vue3 应用入口
// 初始化 Element Plus、Pinia、Vue Router，设置暗黑科技风双主题
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './styles/index.scss'

const app = createApp(App)

// 注册 Element Plus 图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus, {
  // 默认暗黑模式（通过 class="dark" 控制）
})

// 设置默认主题为暗黑
document.documentElement.classList.add('dark')
document.documentElement.setAttribute('data-theme', 'dark')

app.mount('#app')