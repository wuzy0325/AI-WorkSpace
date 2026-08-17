<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useDialog } from 'naive-ui'
import { Events } from '@wailsio/runtime'
import { requestExit } from '@bridge/deviceBridge'
import { useI18nStore } from '@stores/i18nStore'

const dialog = useDialog()
const i18n = useI18nStore()
let cleanupExitListener: (() => void) | null = null

onMounted(() => {
  cleanupExitListener = Events.On('app:exit-requested', () => {
    dialog.warning({
      title: i18n.t('app.confirmExitTitle'),
      content: i18n.t('app.confirmExitText'),
      positiveText: i18n.t('app.exit'),
      negativeText: i18n.t('common.cancel'),
      showIcon: false,
      onPositiveClick: async () => {
        try {
          await requestExit()
        } catch (err) {
          console.error('[App] requestExit failed:', err)
        }
      },
    })
  })
})

onUnmounted(() => {
  cleanupExitListener?.()
  cleanupExitListener = null
})
</script>

<template></template>
