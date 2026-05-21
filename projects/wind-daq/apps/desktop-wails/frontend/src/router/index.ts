import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/:pathMatch(.*)*',
      name: 'dashboard',
      component: () => import('@views/main/MainDashboardView.vue'),
    },
  ],
})

export default router
