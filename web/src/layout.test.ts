import { describe, expect, it } from 'vitest'
import { cellFromPoint, fitCell, MIN_FIT_CELL } from './layout'

describe('fitCell', () => {
  it('picks the limiting axis and floors', () => {
    expect(fitCell(1440, 600, 120, 50)).toBe(12)
    expect(fitCell(1200, 600, 120, 50)).toBe(10) // width-limited (brief's 12 was an arithmetic slip)
    expect(fitCell(1200, 1000, 120, 50)).toBe(10)
    expect(fitCell(1199, 1000, 120, 50)).toBe(9)
  })
  it('never goes below the 3px minimum', () => {
    expect(fitCell(100, 100, 120, 50)).toBe(3)
    expect(MIN_FIT_CELL).toBe(3)
  })
  it('has no upper cap and survives degenerate input', () => {
    expect(fitCell(4000, 4000, 40, 20)).toBe(100)
    expect(fitCell(0, 0, 0, 0)).toBe(3)
  })
})

describe('cellFromPoint', () => {
  it('maps pixels to cells with an offset', () => {
    expect(cellFromPoint(25, 5, 10, 0, 0, 10, 10)).toEqual({ x: 2, y: 0 })
    expect(cellFromPoint(125, 45, 10, 100, 40, 10, 10)).toEqual({ x: 2, y: 0 })
  })
  it('returns null outside the grid', () => {
    expect(cellFromPoint(-1, 5, 10, 0, 0, 10, 10)).toBeNull()
    expect(cellFromPoint(5, -0.5, 10, 0, 0, 10, 10)).toBeNull()
    expect(cellFromPoint(100, 5, 10, 0, 0, 10, 10)).toBeNull()
    expect(cellFromPoint(5, 100, 10, 0, 0, 10, 10)).toBeNull()
    expect(cellFromPoint(99.9, 99.9, 10, 0, 0, 10, 10)).toEqual({ x: 9, y: 9 })
  })
})
