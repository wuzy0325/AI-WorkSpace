import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@views/main/MainDashboardView.vue'),
    },
    {
      path: '/motion',
      name: 'motion-standalone',
      component: () => import('@views/MotionView.vue'),
    },
    {
      // 未匹配路由兜底到主界面
      path: '/:pathMatch(.*)*',
      redirect: '/',
    },
  ],
})

export default router
