<script setup lang="ts">
import { ref, computed, shallowRef, defineAsyncComponent, type Component } from 'vue'
import type { CalibrationType } from '@shared/types/calibration'
import CalibrationHome from './CalibrationHome.vue'

const settingsComponents: Record<string, Component> = {
  'five-hole': defineAsyncComponent(() => import('./five-hole/FiveHoleSettings.vue')),
  'three-hole': defineAsyncComponent(() => import('./three-hole/ThreeHoleSettings.vue')),
  'total-pressure': defineAsyncComponent(() => import('./total-pressure/TotalPressureSettings.vue')),
  'total-temperature': defineAsyncComponent(() => import('./total-temperature/TotalTemperatureSettings.vue')),
}

const mainComponents: Record<string, Component> = {
  'five-hole': defineAsyncComponent(() => import('./five-hole/FiveHoleMain.vue')),
  'three-hole': defineAsyncComponent(() => import('./three-hole/ThreeHoleMain.vue')),
  'total-pressure': defineAsyncComponent(() => import('./total-pressure/TotalPressureMain.vue')),
  'total-temperature': defineAsyncComponent(() => import('./total-temperature/TotalTemperatureMain.vue')),
}

const currentView = shallowRef<Component>(CalibrationHome)
const showSettings = ref(false)
const activeCalibrationType = ref<CalibrationType | null>(null)

const currentSettings = computed(() =>
  activeCalibrationType.value ? settingsComponents[activeCalibrationType.value] : null,
)

function handleSelectCalibration(type: CalibrationType) {
  activeCalibrationType.value = type
  currentView.value = mainComponents[type]
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
  <div class="flex-1 min-h-0 w-full flex flex-col">
    <component
      :is="currentView"
      @select-calibration="handleSelectCalibration"
      @back="handleBack"
      @open-settings="handleOpenSettings"
    />

    <Suspense v-if="showSettings && currentSettings">
      <component
        :is="currentSettings"
        @close="handleCloseSettings"
        @saved="handleCloseSettings"
      />
      <template #fallback>
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
          <div class="animate-spin w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full"></div>
        </div>
      </template>
    </Suspense>
  </div>
</template>
