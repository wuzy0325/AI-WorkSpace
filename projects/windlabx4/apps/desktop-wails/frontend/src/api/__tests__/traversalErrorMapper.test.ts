import { describe, expect, it } from 'vitest'

import { traversalSessionWarning } from '@api/traversalErrorMapper'
import { dualTraversalEn, dualTraversalZh } from '@/locales/dual-traversal'
import type { TraversalSessionState } from '@shared/types/traversal'

function failedSession(message: string): TraversalSessionState {
  return {
    config: null,
    status: { status: 'error', lastError: message },
    isStarting: false,
    error: null,
    completeEvent: null,
    realtimePressures: null,
    realtimeResult: null,
    hasLoadedInterpolator: false,
    interpolatorRestoreMessage: null,
  } as unknown as TraversalSessionState
}

describe('traversal error mapper', () => {
  it('formats an acquisition error with the readable device name in Chinese', () => {
    const message = traversalSessionWarning(
      failedSession('device 环境采集仪 is not acquiring; traversal will not move to point 24'),
      dualTraversalZh,
    )

    expect(message).toBe('设备「环境采集仪」未开始采集，请先在设备管理中开始采集')
  })

  it('uses locale punctuation for the English error', () => {
    const message = traversalSessionWarning(
      failedSession('device Environment DAQ is not acquiring; traversal will not move to point 24'),
      dualTraversalEn,
    )

    expect(message).toBe('Device "Environment DAQ" is not acquiring. Start acquisition in Device Manager first.')
    expect(message).not.toContain('：')
  })
})
