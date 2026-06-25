import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useThemeStore } from '@stores/themeStore'

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

describe('themeStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockLocalStorage()
  })

  it('defaults to light theme', () => {
    const store = useThemeStore()
    expect(store.theme).toBe('light')
  })

  it('toggles theme', () => {
    const store = useThemeStore()
    store.toggleTheme()
    expect(store.theme).toBe('dark')
    store.toggleTheme()
    expect(store.theme).toBe('light')
  })

  it('sets theme explicitly', () => {
    const store = useThemeStore()
    store.setTheme('light')
    expect(store.theme).toBe('light')
    expect(store.nextTheme).toBe('dark')
  })

  it('restores theme from localStorage', () => {
    localStorage.setItem('wind-daq.theme', 'light')
    const store = useThemeStore()
    expect(store.theme).toBe('light')
  })

  it('persists theme change to localStorage', () => {
    const store = useThemeStore()
    store.setTheme('light')
    expect(localStorage.getItem('wind-daq.theme')).toBe('light')
  })
})
