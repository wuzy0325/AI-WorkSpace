import { defineStore } from 'pinia'
import { fetchLastDevices } from '@/api/config'

export type ModuleKey = 'measurement' | 'calibration'

export interface ModuleDeviceSelection {
  /** 单设备兼容字段：始终等于 measureDeviceIds[0] */
  measureDeviceId: string
  /** 多设备勾选列表（保持用户勾选顺序，后端绑定顺序与此一致） */
  measureDeviceIds: string[]
  pressureDeviceId: string
}

export interface DeviceState {
  connectedCount: number
  selections: Record<ModuleKey, ModuleDeviceSelection>
}

function emptySelection(): ModuleDeviceSelection {
  return { measureDeviceId: '', measureDeviceIds: [], pressureDeviceId: '' }
}

export const useDeviceStore = defineStore('device', {
  state: (): DeviceState => ({
    connectedCount: 0,
    selections: {
      measurement: emptySelection(),
      calibration: emptySelection()
    }
  }),

  getters: {
    selectionByModule: (state) => {
      return (module: ModuleKey): ModuleDeviceSelection => state.selections[module]
    }
  },

  actions: {
    setConnectedCount(count: number) {
      this.connectedCount = count
    },

    // 拉取上次成功绑定的设备集合并恢复两个模块的勾选。
    // 设备选择页与工作台侧栏页面加载时调用，恢复用户上次勾选（多设备按勾选顺序）。
    // 后端只在绑定成功后落盘，失败不覆盖上次记录，因此此处无需额外校验。
    async restoreLastDevices() {
      try {
        const last = await fetchLastDevices()
        for (const module of ['measurement', 'calibration'] as ModuleKey[]) {
          this.setModuleSelection(module, {
            measureDeviceIds: last.measureDeviceIds ?? [],
            pressureDeviceId: last.pressureDeviceId ?? ''
          })
        }
      } catch {
        // 拉取失败（如后端未实现）时静默，保留默认勾选。
      }
    },

    // measureDeviceIds 与 measureDeviceId 双向同步：
    // 传数组时单设备字段取首元素，传单设备字段时包装为数组，保证两个视图读取一致。
    setModuleSelection(module: ModuleKey, selection: Partial<ModuleDeviceSelection>) {
      const merged: ModuleDeviceSelection = {
        ...this.selections[module],
        ...selection
      }
      if (selection.measureDeviceIds !== undefined) {
        merged.measureDeviceId = selection.measureDeviceIds[0] ?? ''
      } else if (selection.measureDeviceId !== undefined) {
        merged.measureDeviceIds = selection.measureDeviceId ? [selection.measureDeviceId] : []
      }
      this.selections[module] = merged
    }
  }
})
