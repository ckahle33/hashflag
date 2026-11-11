import { describe, expect, it } from 'vitest'
import { BLOCK_H, TILT, cellFromPointTilted, cellToScreen, fitCellTilted, occludes, tiltedSize } from './projection'

describe('tiltedSize', () => {
  it('is cols*cell wide and rows*cell*TILT plus block headroom tall', () => {
    expect(TILT).toBe(0.6)
    expect(BLOCK_H).toBe(0.8)
    expect(tiltedSize(120, 50, 10)).toEqual({ w: 1200, h: Math.ceil(50 * 10 * 0.6 + 10 * 0.8) })
    expect(tiltedSize(10, 10, 20)).toEqual({ w: 200, h: 136 })
  })
})

describe('cellToScreen', () => {
  it('puts the floor tile below the block headroom', () => {
    expect(cellToScreen(0, 0, 10)).toEqual({ px: 0, py: 8 })
    expect(cellToScreen(3, 5, 10).px).toBe(30)
    expect(cellToScreen(3, 5, 10).py).toBeCloseTo(8 + 30)
  })
})

describe('fitCellTilted', () => {
  it('returns the largest cell that fits both axes', () => {
    const c = fitCellTilted(1200, 600, 120, 50)
    expect(c).toBe(10)
    const s = tiltedSize(120, 50, c + 1)
    expect(s.w > 1200 || s.h > 600).toBe(true)
  })
  it('is height limited when the viewport is short', () => {
    const c = fitCellTilted(3000, 300, 120, 50)
    expect(tiltedSize(120, 50, c).h).toBeLessThanOrEqual(300)
    expect(tiltedSize(120, 50, c + 1).h).toBeGreaterThan(300)
  })
  it('clamps tiny viewports to 3 and survives degenerate input', () => {
    expect(fitCellTilted(100, 100, 120, 50)).toBe(3)
    expect(fitCellTilted(0, 0, 0, 0)).toBe(3)
  })
})

describe('cellFromPointTilted', () => {
  it('round trips every cell centre', () => {
    for (const cell of [8, 12, 20]) {
      for (let y = 0; y < 50; y++) {
        for (let x = 0; x < 120; x++) {
          const { px, py } = cellToScreen(x, y, cell)
          expect(cellFromPointTilted(px + cell / 2, py + (cell * TILT) / 2, cell, 120, 50)).toEqual({ x, y })
        }
      }
    }
  })
  it('returns null outside the grid, including the headroom above row 0', () => {
    expect(cellFromPointTilted(-1, 20, 10, 10, 10)).toBeNull()
    expect(cellFromPointTilted(5, 7, 10, 10, 10)).toBeNull()
    expect(cellFromPointTilted(100, 20, 10, 10, 10)).toBeNull()
    expect(cellFromPointTilted(5, 8 + 60, 10, 10, 10)).toBeNull()
    expect(cellFromPointTilted(5, 8, 10, 10, 10)).toEqual({ x: 0, y: 0 })
  })
})

describe('occludes', () => {
  const you = { x: 10, y: 10 }
  it('is true for blocks in the two nearer rows within one column', () => {
    for (const dy of [1, 2]) for (const dx of [-1, 0, 1]) expect(occludes({ x: 10 + dx, y: 10 + dy }, you)).toBe(true)
  })
  it('is false otherwise', () => {
    expect(occludes({ x: 10, y: 10 }, you)).toBe(false)
    expect(occludes({ x: 10, y: 9 }, you)).toBe(false)
    expect(occludes({ x: 10, y: 13 }, you)).toBe(false)
    expect(occludes({ x: 12, y: 11 }, you)).toBe(false)
    expect(occludes({ x: 8, y: 12 }, you)).toBe(false)
  })
})
