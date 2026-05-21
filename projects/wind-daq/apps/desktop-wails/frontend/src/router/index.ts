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
      name: 'motion',
      component: () => import('@views/MotionView.vue'),
    },
    {
      path: '/calibration',
      name: 'calibration',
      component: () => import('@views/CalibrationView.vue'),
    },
    {
      path: '/traversal',
      name: 'traversal',
      component: () => import('@views/TraversalView.vue'),
    },
    {
      path: '/log',
      name: 'log',
      component: () => import('@views/LogViewer.vue'),
    },
    {
      path: '/storage',
      name: 'storage',
      component: () => import('@views/StorageView.vue'),
    },
  ],
})

export default router
