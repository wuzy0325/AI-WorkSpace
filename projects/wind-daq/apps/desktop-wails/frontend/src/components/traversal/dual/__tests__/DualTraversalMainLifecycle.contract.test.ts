import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DualTraversalMain from '../DualTraversalMain.vue'

const dualStore = vi.hoisted(() => ({
  sessions: {
    probe1: { config: null },
    probe2: { config: null },
  },
  loadConfig: vi.fn(),
  recoverRuntime: vi.fn(),
  loadCheckpoint: vi.fn(),
  cleanupLocal: vi.fn(),
}))

const feedbackStore = vi.hoisted(() => ({ pushToast: vi.fn() }))

vi.mock('pinia', async (importOriginal) => ({
  ...await importOriginal<typeof import('pinia')>(),
  storeToRefs: () => ({ sessions: ref(dualStore.sessions) }),
}))
vi.mock('@stores/dualTraversalStore', () => ({ useDualTraversalStore: () => dualStore }))
vi.mock('@stores/i18nStore', () => ({
  useI18nStore: () => ({
    t: { probe1Label: 'Probe 1', probe2Label: 'Probe 2', dualStartFailed: 'failed' },
  }),
}))
vi.mock('@stores/feedbackStore', () => ({ useFeedbackStore: () => feedbackStore }))

const source = readFileSync(resolve(__dirname, '../DualTraversalMain.vue'), 'utf8')

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((done) => { resolve = done })
  return { promise, resolve }
}

function mountComponent() {
  return mount(DualTraversalMain, {
    global: {
      stubs: {
        DualStatusBar: true,
        DualProbeRow: true,
      },
    },
  })
}

describe('DualTraversalMain runtime lifecycle contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    dualStore.loadConfig.mockResolvedValue(undefined)
    dualStore.recoverRuntime.mockResolvedValue(undefined)
    dualStore.loadCheckpoint.mockResolvedValue(undefined)
  })

  it('loads each config before recovering its keyed runtime', () => {
    expect(source).toMatch(/await dualStore\.loadConfig\(probeId\)[\s\S]*await dualStore\.recoverRuntime\(probeId\)/)
    expect(source).toContain("const probeIds: ProbeId[] = ['probe1', 'probe2']")
  })

  it('releases both keyed runtimes on unmount so re-entry can restore cleanly', () => {
    expect(source).toContain('onUnmounted')
    expect(source).toContain("dualStore.cleanupLocal('probe1')")
    expect(source).toContain("dualStore.cleanupLocal('probe2')")
  })

  it('does not recover runtimes after unmount while config loading is pending', async () => {
    const pendingConfig = deferred()
    dualStore.loadConfig.mockReturnValue(pendingConfig.promise)
    const wrapper = mountComponent()

    wrapper.unmount()
    pendingConfig.resolve()
    await flushPromises()

    expect(dualStore.recoverRuntime).not.toHaveBeenCalled()
    expect(dualStore.loadCheckpoint).not.toHaveBeenCalled()
  })

  it('recovers runtimes when intentionally mounted again', async () => {
    const wrapper = mountComponent()
    await flushPromises()

    expect(dualStore.recoverRuntime).toHaveBeenCalledTimes(2)
    expect(dualStore.recoverRuntime).toHaveBeenCalledWith('probe1')
    expect(dualStore.recoverRuntime).toHaveBeenCalledWith('probe2')

    wrapper.unmount()
  })
})
