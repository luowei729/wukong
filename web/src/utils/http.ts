// 全局 axios 配置 - 自动处理 Token 和错误
// 支持 access token 过期后自动用 refresh token 续期，用户无感知
import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

// 创建 axios 实例
const http = axios.create({
  baseURL: '/',
  timeout: 30000,
})

// 是否正在刷新 token 的标志，避免并发刷新
let isRefreshing = false
// 等待 token 刷新的请求队列
let pendingRequests: Array<(token: string) => void> = []

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

// 响应拦截器 - 统一处理错误，401 时自动刷新 token
http.interceptors.response.use(
  (response) => {
    return response
  },
  (error) => {
    const originalRequest = error.config

    // 处理 401 未授权错误
    if (error.response?.status === 401 && !originalRequest._retry) {
      const refreshToken = localStorage.getItem('refresh_token')

      // 如果有 refresh token，尝试自动续期
      if (refreshToken) {
        // 如果已经在刷新中，将请求加入等待队列
        if (isRefreshing) {
          return new Promise((resolve) => {
            pendingRequests.push((token: string) => {
              originalRequest.headers.Authorization = `Bearer ${token}`
              resolve(http(originalRequest))
            })
          })
        }

        originalRequest._retry = true
        isRefreshing = true

        // 用 refresh token 调用刷新接口
        return axios.post('/api/auth/refresh', { refresh_token: refreshToken })
          .then((res) => {
            const { access_token, refresh_token } = res.data
            // 保存新 token
            localStorage.setItem('access_token', access_token)
            if (refresh_token) {
              localStorage.setItem('refresh_token', refresh_token)
            }
            // 重试所有等待的请求
            pendingRequests.forEach((cb) => cb(access_token))
            pendingRequests = []
            // 重试原始请求
            originalRequest.headers.Authorization = `Bearer ${access_token}`
            return http(originalRequest)
          })
          .catch(() => {
            // 刷新失败，清除 token 并跳转登录页
            pendingRequests = []
            localStorage.removeItem('access_token')
            localStorage.removeItem('refresh_token')
            if (router.currentRoute.value.name !== 'Login') {
              ElMessage.error('登录已过期，请重新登录')
              router.push({ name: 'Login', query: { redirect: router.currentRoute.value.fullPath } })
            }
            return Promise.reject(error)
          })
          .finally(() => {
            isRefreshing = false
          })
      }

      // 没有 refresh token，直接跳转登录
      localStorage.removeItem('access_token')
      localStorage.removeItem('refresh_token')
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
