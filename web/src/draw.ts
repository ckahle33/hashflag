import type { MapInfo, StateMsg } from './protocol'
import { composeGrid, cellChar } from './render'
import { BG, FLOOR, OBSTACLE, RING, SAFE, TEAM_COLOR, TEAM_TINT } from './palette'

export const MIN_CELL = 8
export const MAX_CELL = 20

export function cellSizeFor(width: number, cols: number): number {
  return Math.max(MIN_CELL, Math.min(MAX_CELL, Math.floor(width / Math.max(1, cols))))
}

function inRect(r: { x0: number; y0: number; x1: number; y1: number }, x: number, y: number) {
  return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1
}

function stick(ctx: CanvasRenderingContext2D, cx: number, cy: number, s: number) {
  const u = s / 10
  ctx.beginPath()
  ctx.arc(cx, cy - 3 * u, 1.6 * u, 0, Math.PI * 2)
  ctx.moveTo(cx, cy - 1.4 * u); ctx.lineTo(cx, cy + 1.8 * u) // body
  ctx.moveTo(cx - 2.2 * u, cy); ctx.lineTo(cx + 2.2 * u, cy) // arms
  ctx.moveTo(cx, cy + 1.8 * u); ctx.lineTo(cx - 1.8 * u, cy + 4 * u) // legs
  ctx.moveTo(cx, cy + 1.8 * u); ctx.lineTo(cx + 1.8 * u, cy + 4 * u)
  ctx.stroke()
}

function flagGlyph(ctx: CanvasRenderingContext2D, x: number, y: number, s: number, color: string) {
  ctx.strokeStyle = color
  ctx.fillStyle = color
  ctx.lineWidth = Math.max(1, s / 10)
  ctx.beginPath()
  ctx.moveTo(x + s * 0.3, y + s * 0.88); ctx.lineTo(x + s * 0.3, y + s * 0.12)
  ctx.stroke()
  ctx.beginPath()
  ctx.moveTo(x + s * 0.3, y + s * 0.12); ctx.lineTo(x + s * 0.85, y + s * 0.33); ctx.lineTo(x + s * 0.3, y + s * 0.54)
  ctx.closePath()
  ctx.fill()
}

export function drawCanvas(
  ctx: CanvasRenderingContext2D, map: MapInfo, state: StateMsg, youId: string,
  cell: number, ascii: boolean,
): void {
  const grid = composeGrid(map, state, youId)
  ctx.fillStyle = BG
  ctx.fillRect(0, 0, map.width * cell, map.height * cell)
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.font = `${Math.floor(cell * 0.8)}px ui-monospace, Menlo, Consolas, monospace`
  ctx.lineCap = 'round'

  // Tints for bases first, so all layers sit above them.
  for (let y = 0; y < map.height; y++) {
    for (let x = 0; x < map.width; x++) {
      const px = x * cell, py = y * cell
      if (inRect(map.baseA, x, y)) { ctx.fillStyle = TEAM_TINT.A; ctx.fillRect(px, py, cell, cell) }
      else if (inRect(map.baseB, x, y)) { ctx.fillStyle = TEAM_TINT.B; ctx.fillRect(px, py, cell, cell) }
    }
  }

  let me: { px: number; py: number } | null = null
  for (let y = 0; y < map.height; y++) {
    for (let x = 0; x < map.width; x++) {
      const c = grid[y][x]
      const px = x * cell, py = y * cell
      const cx = px + cell / 2, cy = py + cell / 2
      const color = c.team ? TEAM_COLOR[c.team] : '#fff'
      switch (c.kind) {
        case 'floor':
          ctx.fillStyle = FLOOR
          ctx.fillText('.', cx, cy)
          break
        case 'safe':
          ctx.fillStyle = SAFE
          ctx.fillText(':', cx, cy)
          break
        case 'obstacle':
          ctx.fillStyle = OBSTACLE
          ctx.fillRect(px + 0.5, py + 0.5, cell - 1, cell - 1)
          break
        case 'wall':
          ctx.fillStyle = color
          ctx.font = `bold ${Math.floor(cell * 0.95)}px ui-monospace, Menlo, Consolas, monospace`
          ctx.fillText('|', cx, cy)
          ctx.font = `${Math.floor(cell * 0.8)}px ui-monospace, Menlo, Consolas, monospace`
          break
        case 'projectile':
          ctx.fillStyle = color
          ctx.fillText('*', cx, cy + cell * 0.1)
          break
        case 'flag':
          flagGlyph(ctx, px, py, cell, color)
          break
        case 'player':
        case 'you': {
          const p = c.player!
          ctx.globalAlpha = p.out ? 0.35 : 1
          ctx.fillStyle = color
          ctx.strokeStyle = color
          ctx.lineWidth = Math.max(1, cell / 10)
          if (ascii) {
            ctx.fillText(cellChar(c), cx, cy)
          } else {
            if (p.glyph) {
              ctx.fillStyle = '#fff'
              ctx.fillText(p.glyph, cx, cy)
              ctx.fillStyle = color
            } else {
              stick(ctx, cx, cy, cell)
            }
            if (p.hasFlag) flagGlyph(ctx, px + cell * 0.35, py - cell * 0.35, cell * 0.8, color)
          }
          ctx.globalAlpha = 1
          if (c.kind === 'you') me = { px, py }
          break
        }
      }
    }
  }
  // Local player ring is drawn last so it is always on top.
  if (me && !ascii) {
    ctx.strokeStyle = RING
    ctx.lineWidth = 2
    ctx.strokeRect(me.px + 1, me.py + 1, cell - 2, cell - 2)
  }
}
