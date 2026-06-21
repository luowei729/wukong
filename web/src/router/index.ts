// 路由配置
import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/Login.vue'),
      meta: { title: '登录' },
    },
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'Dashboard',
          component: () => import('@/views/Dashboard.vue'),
          meta: { title: '总览', icon: 'Odometer' },
        },
        {
          path: 'nodes',
          name: 'Nodes',
          component: () => import('@/views/Nodes.vue'),
          meta: { title: '节点列表', icon: 'Monitor' },
        },
        {
          path: 'nodes/:id',
          name: 'NodeDetail',
          component: () => import('@/views/NodeDetail.vue'),
          meta: { title: '节点详情' },
        },
        {
          path: 'alerts',
          name: 'Alerts',
          component: () => import('@/views/Alerts.vue'),
          meta: { title: '告警中心', icon: 'WarningFilled' },
        },
        {
          path: 'settings',
          name: 'Settings',
          component: () => import('@/views/Settings.vue'),
          meta: { title: '系统设置', icon: 'Setting' },
        },
      ],
    },
  ],
})

// 路由守卫（后续添加登录检查）
router.beforeEach((to, _from) => {
  const token = localStorage.getItem('access_token')
  if (to.name !== 'Login' && !token) {
    return { name: 'Login' }
  }
})

export default router