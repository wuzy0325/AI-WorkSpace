import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'module-hub',
      component: () => import('../views/ModuleHubView.vue')
    },
    {
      path: '/device-management',
      name: 'module-device-management',
      component: () => import('../views/DeviceManagementView.vue')
    },
    {
      path: '/measurement',
      name: 'module-measurement',
      component: () => import('../views/MeasurementView.vue')
    },
    {
      path: '/calibration',
      name: 'module-calibration',
      component: () => import('../views/CalibrationView.vue')
    },
    {
      path: '/multi-pressure',
      name: 'module-multi-pressure',
      component: () => import('../views/multipress/MultiPressView.vue')
    },
    {
      path: '/comm-log',
      name: 'module-comm-log',
      component: () => import('../views/CommLogView.vue')
    }
  ]
})

export default router
