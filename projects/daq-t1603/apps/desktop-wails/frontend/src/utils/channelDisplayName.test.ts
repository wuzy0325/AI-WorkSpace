import { describe, expect, it } from 'vitest'
import { channelDisplayName } from './channelDisplayName'

const t = (_key: string, params?: Record<string, string | number>) =>
  `Channel ${params?.n}`

describe('channelDisplayName', () => {
  it('preserves a user-defined channel name', () => {
    expect(channelDisplayName(0, 'Thermocouple A', t)).toBe('Thermocouple A')
  })

  it('localizes an empty channel name', () => {
    expect(channelDisplayName(0, '', t)).toBe('Channel 1')
    expect(channelDisplayName(15, '', t)).toBe('Channel 16')
  })
})
