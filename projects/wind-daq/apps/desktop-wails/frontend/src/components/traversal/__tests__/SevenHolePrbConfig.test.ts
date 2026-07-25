import { createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import SevenHolePrbConfig from '../SevenHolePrbConfig.vue'
import type { SevenHolePrbDraft } from '../../../shared/types/traversal'
import { useTraversalStore } from '../../../stores/traversalStore'

const selectedPaths = vi.hoisted(() => ({ batch: [] as string[] }))

vi.mock('@composables/useFileImport', () => ({
  useFileImport: () => ({
    isImporting: ref(false),
    importFiles: vi.fn(async () => selectedPaths.batch),
    importSingleFile: vi.fn(async () => null)
  })
}))

describe('SevenHolePrbConfig', () => {
  it('切换文件格式时清空上一格式的全部槽位', async () => {
    const draft: SevenHolePrbDraft = {
      source: 'calibration-csv',
      innerFile: { filePath: 'D:/cal/inner.csv', sector: 7 },
      outerFiles: [1, 2, 3, 4, 5, 6].map((sector) => ({
        filePath: `D:/cal/outer-${sector}.csv`,
        sector
      }))
    }
    let updated = draft
    const wrapper = mount(SevenHolePrbConfig, {
      props: {
        modelValue: draft,
        'onUpdate:modelValue': (value: SevenHolePrbDraft) => { updated = value },
        t: new Proxy({}, { get: (_target, key) => String(key) }) as Record<string, string>
      },
      global: { plugins: [createPinia()] }
    })

    const sourceButtons = wrapper.findAll('.mode-row button')
    await sourceButtons[0]!.trigger('click')

    expect(updated).toEqual({
      source: 'prb',
      innerFile: null,
      outerFiles: [null, null, null, null, null, null]
    })
  })

  it('批量选择 PRB 后使用本次文件快照导入，不读取旧 CSV 草稿', async () => {
    const draft: SevenHolePrbDraft = {
      source: 'calibration-csv',
      innerFile: { filePath: 'D:/cal/inner.csv', sector: 7 },
      outerFiles: [1, 2, 3, 4, 5, 6].map((sector) => ({
        filePath: `D:/cal/outer-${sector}.csv`,
        sector
      }))
    }
    selectedPaths.batch = [7, 1, 2, 3, 4, 5, 6].map((n) => `D:/prb/${n}.prb`)
    const pinia = createPinia()
    const store = useTraversalStore(pinia)
    const importPrb = vi.spyOn(store, 'importSevenHolePrbFiles').mockResolvedValue(null)
    const importCsv = vi.spyOn(store, 'importSevenHoleCalibrationCsvFiles').mockResolvedValue(null)
    const wrapper = mount(SevenHolePrbConfig, {
      props: {
        modelValue: draft,
        'onUpdate:modelValue': () => {},
        t: new Proxy({}, { get: (_target, key) => String(key) }) as Record<string, string>
      },
      global: { plugins: [pinia] }
    })

    await wrapper.find('.head-actions button').trigger('click')

    expect(importPrb).toHaveBeenCalledWith('D:/prb/7.prb', [
      'D:/prb/1.prb', 'D:/prb/2.prb', 'D:/prb/3.prb',
      'D:/prb/4.prb', 'D:/prb/5.prb', 'D:/prb/6.prb'
    ])
    expect(importCsv).not.toHaveBeenCalled()
  })

  it('后端导入未完成时禁止切换文件格式', async () => {
    const draft: SevenHolePrbDraft = {
      source: 'calibration-csv',
      innerFile: null,
      outerFiles: [null, null, null, null, null, null]
    }
    selectedPaths.batch = [7, 1, 2, 3, 4, 5, 6].map((n) => `D:/prb/${n}.prb`)
    const pinia = createPinia()
    const store = useTraversalStore(pinia)
    let finishImport!: (value: Awaited<ReturnType<typeof store.importSevenHolePrbFiles>>) => void
    vi.spyOn(store, 'importSevenHolePrbFiles').mockReturnValue(new Promise((resolve) => {
      finishImport = resolve
    }))
    let updated = draft
    const wrapper = mount(SevenHolePrbConfig, {
      props: {
        modelValue: draft,
        'onUpdate:modelValue': (value: SevenHolePrbDraft) => { updated = value },
        t: new Proxy({}, { get: (_target, key) => String(key) }) as Record<string, string>
      },
      global: { plugins: [pinia] }
    })

    const importing = wrapper.find('.head-actions button').trigger('click')
    await vi.waitFor(() => expect(store.importSevenHolePrbFiles).toHaveBeenCalledOnce())

    const sourceButtons = wrapper.findAll('.mode-row button')
    expect(sourceButtons[1]!.attributes('disabled')).toBeDefined()
    await sourceButtons[1]!.trigger('click')
    expect(updated.source).toBe('prb')

    finishImport(null)
    await importing
  })
})
