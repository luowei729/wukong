// 路由配置
import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'PublicHome',
      component: () => import('@/views/public/PublicHome.vue'),
      meta: { title: '服务器状态', public: true },
    },
    {
      path: '/server/:id',
      name: 'PublicServerDetail',
      component: () => import('@/views/public/PublicServerDetail.vue'),
      meta: { title: '服务器详情', public: true },
    },
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/Login.vue'),
      meta: { title: '登录', public: true },
    },
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: 'dashboard',
          name: 'Dashboard',
          component: () => import('@/views/Dashboard.vue'),
          meta: { title: '总览', icon: 'Odometer', requiresAuth: true },
        },
        {
          path: 'nodes',
          name: 'Nodes',
          component: () => import('@/views/Nodes.vue'),
          meta: { title: '节点列表', icon: 'Monitor', requiresAuth: true },
        },
        {
          path: 'nodes/:id',
          name: 'NodeDetail',
          component: () => import('@/views/NodeDetail.vue'),
          meta: { title: '节点详情', requiresAuth: true },
        },
        {
          path: 'alerts',
          name: 'Alerts',
          component: () => import('@/views/Alerts.vue'),
          meta: { title: '告警中心', icon: 'WarningFilled', requiresAuth: true },
        },
        {
          path: 'settings',
          name: 'Settings',
          component: () => import('@/views/Settings.vue'),
          meta: { title: '系统设置', icon: 'Setting', requiresAuth: true },
        },
      ],
    },
  ],
})

// 路由守卫：公开页面允许未登录访问，只有后台管理页需要 JWT。
router.beforeEach((to) => {
  const token = localStorage.getItem('access_token')
  if (to.meta.requiresAuth && !token) {
    return { name: 'Login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'Login' && token) {
    return { name: 'Dashboard' }
  }
})

// 路由切换后更新浏览器标签页标题；站点标题优先使用后台主题接口写入的 localStorage。
router.afterEach((to) => {
  const siteTitle = localStorage.getItem('site_title') || 'wukong 监控'
  const pageTitle = typeof to.meta.title === 'string' ? to.meta.title : ''
  document.title = pageTitle ? `${pageTitle} - ${siteTitle}` : siteTitle
})

export default router
