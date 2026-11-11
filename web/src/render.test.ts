import { describe, expect, it } from 'vitest'
import { renderGrid } from './render'
import type { MapInfo, PlayerState, StateMsg } from './protocol'

function mkMap(rows: string[]): MapInfo {
  return {
    width: rows[0].length,
    height: rows.length,
    rows,
    safeX0: 4,
    safeX1: 5,
    baseA: { x0: 0, y0: 0, x1: 1, y1: 1 },
    baseB: { x0: 8, y0: 0, x1: 9, y1: 1 },
    flagA: { x: 0, y: 0 },
    flagB: { x: 9, y: 0 },
  }
}

const rows = ['..........', '..#.......', '..........']
const emptyState = (): StateMsg => ({
  t: 'state', tick: 1, players: [], projectiles: [], walls: [], flags: [], scores: [0, 0], events: [],
})
const player = (id: string, x: number, y: number, extra: Partial<PlayerState> = {}): PlayerState => ({
  id, name: id, x, y, team: 'A', role: 'attacker', glyph: '', out: false, hasFlag: false, ...extra,
})

describe('renderGrid', () => {
  it('renders floor, obstacles and the safe line', () => {
    const g = renderGrid(mkMap(rows), emptyState(), 'me')
    expect(g).toHaveLength(3)
    expect(g[0]).toBe('....::....')
    expect(g[1]).toBe('..#.::....')
  })

  it('renders walls', () => {
    const s = emptyState()
    s.walls = [{ x: 7, y: 2, team: 'B' }]
    expect(renderGrid(mkMap(rows), s, 'me')[2][7]).toBe('|')
  })

  it('own player is @, others are o', () => {
    const s = emptyState()
    s.players = [player('me', 1, 1), player('x', 2, 0)]
    const g = renderGrid(mkMap(rows), s, 'me')
    expect(g[1][1]).toBe('@')
    expect(g[0][2]).toBe('o')
  })

  it('projectile above floor, below player', () => {
    const s = emptyState()
    s.projectiles = [{ x: 3, y: 0, dir: 2, team: 'A' }, { x: 6, y: 0, dir: 2, team: 'A' }]
    s.players = [player('x', 6, 0)]
    const g = renderGrid(mkMap(rows), s, 'me')
    expect(g[0][3]).toBe('*')
    expect(g[0][6]).toBe('o')
  })

  it('layer precedence: safe < obstacle < wall < projectile < flag < player < you', () => {
    const m = mkMap(rows)
    const s = emptyState()
    // on the safe line: wall beats safe, projectile beats wall
    s.walls = [{ x: 4, y: 0, team: 'A' }, { x: 5, y: 0, team: 'A' }]
    s.projectiles = [{ x: 5, y: 0, dir: 0, team: 'A' }]
    let g = renderGrid(m, s, 'me')
    expect(g[0][4]).toBe('|')
    expect(g[0][5]).toBe('*')
    // wall over obstacle
    s.walls = [{ x: 2, y: 1, team: 'A' }]
    s.projectiles = []
    g = renderGrid(m, s, 'me')
    expect(g[1][2]).toBe('|')
    // flag over projectile, player over flag, carrier shows F, you over all
    s.projectiles = [{ x: 9, y: 0, dir: 0, team: 'A' }]
    s.flags = [{ team: 'B', x: 9, y: 0, carrier: '' }]
    g = renderGrid(m, s, 'me')
    expect(g[0][9]).toBe('F')
    s.players = [player('x', 9, 0)]
    expect(renderGrid(m, s, 'me')[0][9]).toBe('o')
    s.players = [player('x', 9, 0, { hasFlag: true })]
    expect(renderGrid(m, s, 'me')[0][9]).toBe('F')
    s.players = [player('x', 9, 0), player('me', 9, 0)]
    expect(renderGrid(m, s, 'me')[0][9]).toBe('@')
  })

  it('draws a dropped flag', () => {
    const s = emptyState()
    s.flags = [{ team: 'A', x: 3, y: 2, carrier: '' }]
    expect(renderGrid(mkMap(rows), s, 'me')[2][3]).toBe('F')
  })

  it('ignores out-of-bounds entities', () => {
    const s = emptyState()
    s.players = [player('x', -1, 0), player('y', 99, 99)]
    s.projectiles = [{ x: 50, y: 0, dir: 0, team: 'A' }]
    s.walls = [{ x: 0, y: -3, team: 'A' }]
    s.flags = [{ team: 'A', x: 100, y: 100, carrier: '' }]
    expect(() => renderGrid(mkMap(rows), s, 'me')).not.toThrow()
    expect(renderGrid(mkMap(rows), s, 'me')[0]).toBe('....::....')
  })

  it('renders 200 players in under 20 ms', () => {
    const big = Array.from({ length: 40 }, () => '.'.repeat(80))
    const s = emptyState()
    s.players = Array.from({ length: 200 }, (_, i) => player(`p${i}`, i % 80, Math.floor(i / 80)))
    const t0 = performance.now()
    renderGrid(mkMap(big), s, 'p0')
    expect(performance.now() - t0).toBeLessThan(20)
  })
})
