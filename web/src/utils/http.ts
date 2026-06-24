// 全局 axios 配置 - 自动处理 Token 和错误
import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

// 创建 axios 实例
const http = axios.create({
  baseURL: '/',
  timeout: 30000,
})

// 请求拦截器 - 自动添加 Token
http.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('access_token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器 - 统一处理错误
http.interceptors.response.use(
  (response) => {
    return response
  },
  (error) => {
    // 处理 401 未授权错误
    if (error.response?.status === 401) {
      // 清除 token 并跳转到登录页
      localStorage.removeItem('access_token')
      localStorage.removeItem('refresh_token')

      // 只有在非登录页的情况下才跳转
      if (router.currentRoute.value.name !== 'Login') {
        ElMessage.error('登录已过期，请重新登录')
        router.push({ name: 'Login', query: { redirect: router.currentRoute.value.fullPath } })
      }
      return Promise.reject(error)
    }

    // 处理其他错误
    const msg = error.response?.data?.error || error.message || '请求失败'
    ElMessage.error(msg)
    return Promise.reject(error)
  }
)

export default http