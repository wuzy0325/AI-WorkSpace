import { describe, expect, it } from 'vitest'
import { channelDisplayName } from './channelDisplayName'

const labels: Record<string, string> = {
  'config.defaultChannelName': 'Channel {n}',
  'config.atmosphericPressure': 'Atmospheric Pressure',
  'config.atmosphericTemperature': 'Atmospheric Temperature',
}

const t = (key: string, params?: Record<string, string | number>) =>
  (labels[key] ?? key).replace('{n}', String(params?.n ?? ''))

describe('channelDisplayName', () => {
  it('preserves a user-defined channel name', () => {
    expect(channelDisplayName(0, 'Port A', t)).toBe('Port A')
  })

  it('localizes empty pressure and atmospheric channel names', () => {
    expect(channelDisplayName(0, '', t)).toBe('Channel 1')
    expect(channelDisplayName(16, '', t)).toBe('Atmospheric Pressure')
    expect(channelDisplayName(17, '', t)).toBe('Atmospheric Temperature')
  })
})
