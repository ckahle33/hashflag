import { describe, expect, it } from 'vitest'
import { arrowsDir, clearHeld, heldIntent, newHeld, pressKey, releaseKey, snapDir } from './input'

describe('snapDir', () => {
  it('maps all eight octants, y grows downward', () => {
    expect(snapDir(0, -5)).toBe(0)
    expect(snapDir(5, -5)).toBe(1)
    expect(snapDir(5, 0)).toBe(2)
    expect(snapDir(5, 5)).toBe(3)
    expect(snapDir(0, 5)).toBe(4)
    expect(snapDir(-5, 5)).toBe(5)
    expect(snapDir(-5, 0)).toBe(6)
    expect(snapDir(-5, -5)).toBe(7)
  })
  it('snaps near-axis points to the axis and returns null at the origin', () => {
    expect(snapDir(1, -10)).toBe(0)
    expect(snapDir(10, -1)).toBe(2)
    expect(snapDir(-1, 10)).toBe(4)
    expect(snapDir(-10, 1)).toBe(6)
    expect(snapDir(3, -7)).toBe(1) // about 23 degrees: past the N/NE boundary
    expect(snapDir(3, -8)).toBe(0) // about 20.6 degrees
    expect(snapDir(0, 0)).toBeNull()
  })
})

describe('arrow mapping', () => {
  it('single arrows', () => {
    expect(arrowsDir(['ArrowUp'])).toBe(0)
    expect(arrowsDir(['ArrowRight'])).toBe(2)
    expect(arrowsDir(['ArrowDown'])).toBe(4)
    expect(arrowsDir(['ArrowLeft'])).toBe(6)
  })
  it('pairs make diagonals', () => {
    expect(arrowsDir(['ArrowUp', 'ArrowRight'])).toBe(1)
    expect(arrowsDir(['ArrowRight', 'ArrowDown'])).toBe(3)
    expect(arrowsDir(['ArrowDown', 'ArrowLeft'])).toBe(5)
    expect(arrowsDir(['ArrowLeft', 'ArrowUp'])).toBe(7)
  })
  it('opposites cancel and nothing is null', () => {
    expect(arrowsDir([])).toBeNull()
    expect(arrowsDir(['ArrowUp', 'ArrowDown'])).toBeNull()
    expect(arrowsDir(['ArrowUp', 'ArrowDown', 'ArrowRight'])).toBe(2)
  })
})

describe('heldIntent', () => {
  it('is null when nothing is held', () => {
    expect(heldIntent(newHeld())).toBeNull()
  })
  it('most recently pressed move key wins', () => {
    const h = newHeld()
    pressKey(h, 'w', true)
    pressKey(h, 'd', true)
    expect(heldIntent(h)).toEqual({ kind: 'move', dir: 2 })
    releaseKey(h, 'd')
    expect(heldIntent(h)).toEqual({ kind: 'move', dir: 0 })
    releaseKey(h, 'w')
    expect(heldIntent(h)).toBeNull()
  })
  it('does not hold a move key when a prefix is pending', () => {
    const h = newHeld()
    pressKey(h, 'w', false)
    expect(heldIntent(h)).toBeNull()
  })
  it('arrows throw, shift makes a wall, fire beats move', () => {
    const h = newHeld()
    pressKey(h, 'w', true)
    pressKey(h, 'ArrowLeft', true)
    expect(heldIntent(h)).toEqual({ kind: 'throw', dir: 6 })
    pressKey(h, 'ArrowUp', true)
    expect(heldIntent(h)).toEqual({ kind: 'throw', dir: 7 })
    pressKey(h, 'Shift', true)
    expect(heldIntent(h)).toEqual({ kind: 'wall', dir: 7 })
    releaseKey(h, 'Shift')
    releaseKey(h, 'ArrowLeft')
    releaseKey(h, 'ArrowUp')
    expect(heldIntent(h)).toEqual({ kind: 'move', dir: 0 })
  })
  it('opposite arrows fall back to move', () => {
    const h = newHeld()
    pressKey(h, 's', true)
    pressKey(h, 'ArrowLeft', true)
    pressKey(h, 'ArrowRight', true)
    expect(heldIntent(h)).toEqual({ kind: 'move', dir: 4 })
  })
  it('mouse fire counts and respects shift', () => {
    const h = newHeld()
    h.mouseDir = 3
    expect(heldIntent(h)).toEqual({ kind: 'throw', dir: 3 })
    h.shift = true
    expect(heldIntent(h)).toEqual({ kind: 'wall', dir: 3 })
    pressKey(newHeld(), 'a', true)
  })
  it('clearHeld resets everything', () => {
    const h = newHeld()
    pressKey(h, 'w', true)
    pressKey(h, 'Shift', true)
    h.mouseDir = 1
    clearHeld(h)
    expect(heldIntent(h)).toBeNull()
    expect(h.shift).toBe(false)
  })
})
