import { describe, expect, it } from 'vitest'
import { emojiSize, playerGlyph } from './draw'

describe('playerGlyph', () => {
  it('uses the faction glyph when present', () => {
    expect(playerGlyph({ glyph: '🐦', role: 'attacker' })).toBe('🐦')
  })
  it('falls back to a role default', () => {
    expect(playerGlyph({ glyph: '', role: 'attacker' })).toBe('\u{1F3C3}')
    expect(playerGlyph({ glyph: '', role: 'defender' })).toBe('\u{1F6E1}️')
  })
})

describe('emojiSize', () => {
  it('is about 0.85 of the cell and never exceeds it', () => {
    expect(emojiSize(20)).toBe(17)
    for (const c of [3, 4, 5, 12, 33]) expect(emojiSize(c)).toBeLessThanOrEqual(c)
    expect(emojiSize(3)).toBeGreaterThanOrEqual(1)
  })
})
