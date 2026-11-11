import type { MapInfo, PlayerState, StateMsg, Team } from './protocol'

export type CellKind = 'floor' | 'safe' | 'obstacle' | 'wall' | 'projectile' | 'flag' | 'player' | 'you'

export interface Cell {
  kind: CellKind
  team?: Team
  player?: PlayerState
}

const inBounds = (m: MapInfo, x: number, y: number) =>
  Number.isInteger(x) && Number.isInteger(y) && x >= 0 && y >= 0 && x < m.width && y < m.height

// composeGrid is the single place that decides which layer wins in a cell.
// Lowest to highest: floor, safe line, obstacle, wall, projectile, flag,
// player, yourself. Entities outside the map are ignored.
export function composeGrid(map: MapInfo, state: StateMsg, youId: string): Cell[][] {
  const grid: Cell[][] = []
  for (let y = 0; y < map.height; y++) {
    const row: Cell[] = []
    const src = map.rows[y] ?? ''
    for (let x = 0; x < map.width; x++) {
      if (src[x] === '#') row.push({ kind: 'obstacle' })
      else if (x >= map.safeX0 && x <= map.safeX1) row.push({ kind: 'safe' })
      else row.push({ kind: 'floor' })
    }
    grid.push(row)
  }
  for (const w of state.walls) {
    if (inBounds(map, w.x, w.y)) grid[w.y][w.x] = { kind: 'wall', team: w.team }
  }
  for (const p of state.projectiles) {
    if (inBounds(map, p.x, p.y)) grid[p.y][p.x] = { kind: 'projectile', team: p.team }
  }
  for (const f of state.flags) {
    if (inBounds(map, f.x, f.y)) grid[f.y][f.x] = { kind: 'flag', team: f.team }
  }
  let me: PlayerState | undefined
  for (const p of state.players) {
    if (!inBounds(map, p.x, p.y)) continue
    if (p.id === youId) {
      me = p
      continue
    }
    grid[p.y][p.x] = { kind: 'player', team: p.team, player: p }
  }
  if (me) grid[me.y][me.x] = { kind: 'you', team: me.team, player: me }
  return grid
}

export function cellChar(c: Cell): string {
  switch (c.kind) {
    case 'floor': return '.'
    case 'safe': return ':'
    case 'obstacle': return '#'
    case 'wall': return '|'
    case 'projectile': return '*'
    case 'flag': return 'F'
    case 'you': return '@'
    case 'player': return c.player?.hasFlag ? 'F' : 'o'
  }
}

export function renderGrid(map: MapInfo, state: StateMsg, youId: string): string[] {
  return composeGrid(map, state, youId).map((row) => row.map(cellChar).join(''))
}
