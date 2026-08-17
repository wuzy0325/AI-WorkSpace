import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useI18nStore } from '@stores/i18nStore'

function mockLocalStorage() {
  const store: Record<string, string> = {}
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      getItem: (key: string) => store[key] ?? null,
      setItem: (key: string, value: string) => { store[key] = value },
      removeItem: (key: string) => { delete store[key] },
      clear: () => { Object.keys(store).forEach(k => delete store[k]) },
    },
    configurable: true,
    writable: true,
  })
}

describe('i18nStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockLocalStorage()
  })

  it('defaults to zh locale', () => {
    const store = useI18nStore()
    expect(store.locale).toBe('zh')
  })

  it('returns zh translations by default', () => {
    const store = useI18nStore()
    expect(store.t.dashboardHome).toBe('仪表盘')
  })

  it('switches to en locale', () => {
    const store = useI18nStore()
    store.setLocale('en')
    expect(store.locale).toBe('en')
    expect(store.t.dashboardHome).toBe('Dashboard')
    expect(store.t.motionControl).toBe('Motion')
  })

  it('switches back to zh', () => {
    const store = useI18nStore()
    store.setLocale('en')
    store.setLocale('zh')
    expect(store.locale).toBe('zh')
    expect(store.t.dashboardHome).toBe('仪表盘')
  })

  it('persists locale change to localStorage', () => {
    const store = useI18nStore()
    store.setLocale('en')
    expect(localStorage.getItem('WindLabX4.locale')).toBe('en')
  })
})
