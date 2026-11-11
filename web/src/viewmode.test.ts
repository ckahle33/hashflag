import { describe, expect, it } from 'vitest'
import { parseView, toggleAscii, toggleGraphical } from './viewmode'

describe('view modes', () => {
  it('defaults to tilted and migrates unknown values', () => {
    expect(parseView(null)).toEqual({ mode: 'tilted', graphical: 'tilted' })
    expect(parseView('flat')).toEqual({ mode: 'flat', graphical: 'flat' })
    expect(parseView('ascii:flat')).toEqual({ mode: 'ascii', graphical: 'flat' })
    expect(parseView('junk')).toEqual({ mode: 'tilted', graphical: 'tilted' })
  })
  it('V toggles tilted and flat, and does nothing in ascii', () => {
    expect(toggleGraphical({ mode: 'tilted', graphical: 'tilted' })).toEqual({ mode: 'flat', graphical: 'flat' })
    expect(toggleGraphical({ mode: 'flat', graphical: 'flat' })).toEqual({ mode: 'tilted', graphical: 'tilted' })
    const a = { mode: 'ascii' as const, graphical: 'flat' as const }
    expect(toggleGraphical(a)).toEqual(a)
  })
  it('T enters ascii and returns to the last graphical mode', () => {
    const a = toggleAscii({ mode: 'flat', graphical: 'flat' })
    expect(a).toEqual({ mode: 'ascii', graphical: 'flat' })
    expect(toggleAscii(a)).toEqual({ mode: 'flat', graphical: 'flat' })
  })
})
