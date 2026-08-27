// 启动门禁开关 store：与后端 /api/v1/config/gates 同源，
// 在应用 bootstrap 阶段拉取一次；标定/计量 store 从这里读取，
// 避免在两个 store 中硬编码同一份开关。

import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getGatesConfig } from '@/api/config'

export const useGatesStore = defineStore('app/gates', () => {
  // 默认开启：阀门=校准模式是启动必要条件。
  // 后端 GET /config/gates 拉取成功后会覆盖这个值；
  // 拉取失败时保留默认 true，仍按"严格门禁"运行（安全侧默认）。
  const enforceValveCalibrationGate = ref(true)
  const loaded = ref(false)

  async function refresh(): Promise<void> {
    try {
      const config = await getGatesConfig()
      enforceValveCalibrationGate.value = !!config.enforceValveCalibrationGate
      loaded.value = true
    } catch (error) {
      console.warn('[gates] 拉取门禁配置失败，使用默认值（严格门禁）:', error)
      loaded.value = true
    }
  }

  return {
    enforceValveCalibrationGate,
    loaded,
    refresh,
  }
})
