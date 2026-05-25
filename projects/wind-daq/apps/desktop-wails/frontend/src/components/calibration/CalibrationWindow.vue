<script setup lang="ts">
import { ref, shallowRef, type Component } from 'vue'
import type { CalibrationType } from '@shared/types/calibration'
import CalibrationHome from './CalibrationHome.vue'

const currentView = shallowRef<Component>(CalibrationHome)
const showSettings = ref(false)
const activeCalibrationType = ref<CalibrationType | null>(null)

function handleSelectCalibration(type: CalibrationType) {
  activeCalibrationType.value = type
  switch (type) {
    case 'five-hole':
      import('./five-hole/FiveHoleMain.vue').then((m) => {
        currentView.value = m.default as Component
      })
      break
    case 'three-hole':
      import('./three-hole/ThreeHoleMain.vue').then((m) => {
        currentView.value = m.default as Component
      })
      break
    case 'total-pressure':
      import('./total-pressure/TotalPressureMain.vue').then((m) => {
        currentView.value = m.default as Component
      })
      break
    case 'total-temperature':
      import('./total-temperature/TotalTemperatureMain.vue').then((m) => {
        currentView.value = m.default as Component
      })
      break
  }
}

function handleBack() {
  activeCalibrationType.value = null
  showSettings.value = false
  currentView.value = CalibrationHome
}

function handleOpenSettings() {
  showSettings.value = true
}

function handleCloseSettings() {
  showSettings.value = false
}
</script>

<template>
  <div class="flex-1 min-h-0 w-full">
    <component
      :is="currentView"
      @select-calibration="handleSelectCalibration"
      @back="handleBack"
      @open-settings="handleOpenSettings"
    />

    <!-- 设置弹窗 -->
    <template v-if="showSettings">
      <Suspense v-if="activeCalibrationType === 'five-hole'">
        <template #default>
          <component
            :is="() => import('./five-hole/FiveHoleSettings.vue')"
            @close="handleCloseSettings"
            @saved="handleCloseSettings"
          />
        </template>
        <template #fallback>
          <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
            <div class="animate-spin w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full"></div>
          </div>
        </template>
      </Suspense>
      <Suspense v-else-if="activeCalibrationType === 'three-hole'">
        <template #default>
          <component
            :is="() => import('./three-hole/ThreeHoleSettings.vue')"
            @close="handleCloseSettings"
            @saved="handleCloseSettings"
          />
        </template>
        <template #fallback>
          <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
            <div class="animate-spin w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full"></div>
          </div>
        </template>
      </Suspense>
      <Suspense v-else-if="activeCalibrationType === 'total-pressure'">
        <template #default>
          <component
            :is="() => import('./total-pressure/TotalPressureSettings.vue')"
            @close="handleCloseSettings"
            @saved="handleCloseSettings"
          />
        </template>
        <template #fallback>
          <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
            <div class="animate-spin w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full"></div>
          </div>
        </template>
      </Suspense>
      <Suspense v-else-if="activeCalibrationType === 'total-temperature'">
        <template #default>
          <component
            :is="() => import('./total-temperature/TotalTemperatureSettings.vue')"
            @close="handleCloseSettings"
            @saved="handleCloseSettings"
          />
        </template>
        <template #fallback>
          <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
            <div class="animate-spin w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full"></div>
          </div>
        </template>
      </Suspense>
    </template>
  </div>
</template>
