import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const doubles = vi.hoisted(() => ({
  traversal: { canStop: false, isStarting: false },
  dual: {
    anyActive: false,
    sessions: {
      probe1: { error: null as string | null },
      probe2: { error: null as string | null },
    },
    close: vi.fn(),
    cleanupLocal: vi.fn(),
    reset: vi.fn(),
  },
  feedback: { pushToast: vi.fn() },
}))

vi.mock('@stores/traversalStore', () => ({ useTraversalStore: () => doubles.traversal }))
vi.mock('@stores/dualTraversalStore', () => ({ useDualTraversalStore: () => doubles.dual }))
vi.mock('@stores/feedbackStore', () => ({ useFeedbackStore: () => doubles.feedback }))
vi.mock('@stores/i18nStore', () => ({
  useI18nStore: () => ({ t: { traversalModeSwitchDisabled: 'busy', dualCloseFailed: 'close failed' } }),
}))

import { useTraversalModeStore } from '@stores/traversalModeStore'

beforeEach(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => 'dual'),
    setItem: vi.fn(),
  })
  setActivePinia(createPinia())
  vi.clearAllMocks()
  doubles.dual.close.mockResolvedValue(true)
  doubles.dual.sessions.probe1.error = null
  doubles.dual.sessions.probe2.error = null
})

describe('traversalModeStore close gate', () => {
  it('close 返回 false 时不 reset、不切换且向用户报告错误', async () => {
    doubles.dual.sessions.probe2.error = 'probe2 close rejected'
    doubles.dual.close.mockImplementation(async (probeId: string) => probeId === 'probe1')
    const store = useTraversalModeStore()

    expect(await store.switchMode('single')).toBe(false)
    expect(store.mode).toBe('dual')
    expect(doubles.dual.reset).not.toHaveBeenCalled()
    expect(doubles.feedback.pushToast).toHaveBeenCalledWith(
      expect.stringContaining('probe2 close rejected'),
      'error',
    )
  })

  it('close failure 后 cleanupOnLeave 只清理本地资源且不重复 server close', async () => {
    doubles.dual.sessions.probe1.error = 'server close failed'
    doubles.dual.close.mockResolvedValueOnce(false).mockResolvedValueOnce(true)
    const store = useTraversalModeStore()

    expect(await store.switchMode('single')).toBe(false)
    expect(doubles.dual.close).toHaveBeenCalledTimes(2)
    store.cleanupOnLeave()

    expect(doubles.dual.close).toHaveBeenCalledTimes(2)
    expect(doubles.dual.cleanupLocal).toHaveBeenCalledWith('probe1')
    expect(doubles.dual.cleanupLocal).toHaveBeenCalledWith('probe2')
    expect(doubles.dual.reset).not.toHaveBeenCalled()
    expect(store.mode).toBe('dual')
    expect(doubles.dual.sessions.probe1.error).toBe('server close failed')
  })
})
