import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useFeedbackStore } from '@stores/feedbackStore'

describe('feedbackStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('pushes a toast with default level', () => {
    const store = useFeedbackStore()
    const id = store.pushToast('test message')
    expect(id).toBe(1)
    expect(store.toasts).toHaveLength(1)
    expect(store.toasts[0].message).toBe('test message')
    expect(store.toasts[0].level).toBe('info')
  })

  it('pushes toast with specified level', () => {
    const store = useFeedbackStore()
    store.pushToast('error occurred', 'error')
    expect(store.toasts[0].level).toBe('error')
  })

  it('removes a toast by id', () => {
    const store = useFeedbackStore()
    const id = store.pushToast('test')
    expect(store.toasts).toHaveLength(1)
    store.removeToast(id)
    expect(store.toasts).toHaveLength(0)
  })

  it('increments toast ids', () => {
    const store = useFeedbackStore()
    store.pushToast('first')
    store.pushToast('second')
    expect(store.toasts[0].id).toBe(1)
    expect(store.toasts[1].id).toBe(2)
  })

  it('shows confirm dialog and resolves true', async () => {
    const store = useFeedbackStore()
    const promise = store.confirm('Are you sure?')
    expect(store.confirmState.open).toBe(true)
    expect(store.confirmState.message).toBe('Are you sure?')
    store.resolveConfirm(true)
    const result = await promise
    expect(result).toBe(true)
    expect(store.confirmState.open).toBe(false)
  })

  it('shows confirm dialog and resolves false', async () => {
    const store = useFeedbackStore()
    const promise = store.confirm('Delete?', { title: 'Delete', confirmText: 'Yes', cancelText: 'No' })
    expect(store.confirmState.title).toBe('Delete')
    expect(store.confirmState.confirmText).toBe('Yes')
    store.resolveConfirm(false)
    const result = await promise
    expect(result).toBe(false)
  })
})
