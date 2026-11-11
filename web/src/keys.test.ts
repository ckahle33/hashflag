import { describe, expect, it } from 'vitest'
import { keyToIntent } from './keys'

describe('keyToIntent', () => {
  it('moves with wasd and diagonals', () => {
    expect(keyToIntent(null, 'w')).toEqual({ prefix: null, intent: { kind: 'move', dir: 0 } })
    expect(keyToIntent(null, 'e').intent).toEqual({ kind: 'move', dir: 1 })
    expect(keyToIntent(null, 'd').intent).toEqual({ kind: 'move', dir: 2 })
    expect(keyToIntent(null, 'c').intent).toEqual({ kind: 'move', dir: 3 })
    expect(keyToIntent(null, 's').intent).toEqual({ kind: 'move', dir: 4 })
    expect(keyToIntent(null, 'z').intent).toEqual({ kind: 'move', dir: 5 })
    expect(keyToIntent(null, 'a').intent).toEqual({ kind: 'move', dir: 6 })
    expect(keyToIntent(null, 'q').intent).toEqual({ kind: 'move', dir: 7 })
  })

  it('f then direction throws', () => {
    const a = keyToIntent(null, 'f')
    expect(a).toEqual({ prefix: 'f', intent: undefined })
    expect(keyToIntent(a.prefix, 'd')).toEqual({ prefix: null, intent: { kind: 'throw', dir: 2 } })
  })

  it('b then direction places a wall', () => {
    const a = keyToIntent(null, 'b')
    expect(keyToIntent(a.prefix, 'q')).toEqual({ prefix: null, intent: { kind: 'wall', dir: 7 } })
  })

  it('escape clears the prefix', () => {
    expect(keyToIntent('f', 'Escape')).toEqual({ prefix: null, intent: undefined })
    expect(keyToIntent(null, 'Escape')).toEqual({ prefix: null, intent: undefined })
  })

  it('space and x stay', () => {
    expect(keyToIntent(null, ' ').intent).toEqual({ kind: 'stay' })
    expect(keyToIntent(null, 'x').intent).toEqual({ kind: 'stay' })
  })

  it('ignores unknown keys, keeping the prefix', () => {
    expect(keyToIntent(null, 'p')).toEqual({ prefix: null, intent: undefined })
    expect(keyToIntent('f', 'p')).toEqual({ prefix: 'f', intent: undefined })
  })

  it('is case insensitive', () => {
    expect(keyToIntent(null, 'W').intent).toEqual({ kind: 'move', dir: 0 })
  })
})

import { shouldIgnoreKeyTarget } from './keys'

describe('shouldIgnoreKeyTarget', () => {
  const el = (tagName: string, extra: Record<string, unknown> = {}) => ({ tagName, ...extra })
  it('ignores form controls, buttons, links and role=button', () => {
    for (const t of ['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON', 'A']) expect(shouldIgnoreKeyTarget(el(t))).toBe(true)
    expect(shouldIgnoreKeyTarget(el('DIV', { isContentEditable: true }))).toBe(true)
    expect(shouldIgnoreKeyTarget(el('DIV', { getAttribute: (n: string) => (n === 'role' ? 'button' : null) }))).toBe(true)
  })
  it('lets game keys through otherwise', () => {
    expect(shouldIgnoreKeyTarget(null)).toBe(false)
    expect(shouldIgnoreKeyTarget(el('BODY'))).toBe(false)
    expect(shouldIgnoreKeyTarget(el('DIV', { getAttribute: () => null }))).toBe(false)
  })
})
