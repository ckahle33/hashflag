import { describe, expect, it } from 'vitest'
import { REPLACED_REASON, shouldReconnect } from './reconnect'

describe('shouldReconnect', () => {
  it('does not reconnect after the session was replaced', () => {
    expect(shouldReconnect(1000, REPLACED_REASON)).toBe(false)
    expect(shouldReconnect(1000, 'replaced by a newer connection')).toBe(false)
  })
  it('reconnects after any other close', () => {
    expect(shouldReconnect(1000, '')).toBe(true)
    expect(shouldReconnect(1001, 'server shutting down')).toBe(true)
    expect(shouldReconnect(1006, '')).toBe(true)
    expect(shouldReconnect(1008, 'rate limit exceeded')).toBe(true)
    expect(shouldReconnect(1013, 'too slow')).toBe(true)
  })
})
