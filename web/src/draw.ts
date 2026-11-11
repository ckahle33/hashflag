import type { MapInfo, PlayerState, RoleName, StateMsg } from './protocol'
import { composeGrid, cellChar } from './render'
import { BG, FLOOR, OBSTACLE, RING, SAFE, TEAM_COLOR, TEAM_TINT } from './palette'

const EMOJI_FONT = 'system-ui, "Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", sans-serif'

// Role defaults for players whose faction has no glyph.
const ROLE_GLYPH = { attacker: '\u{1F3C3}', defender: '\u{1F6E1}\uFE0F' } as const

export function playerGlyph(p: { glyph: string; role: RoleName }): string {
  return p.glyph || ROLE_GLYPH[p.role]
}

// emojiSize is about 0.85 of the cell, never larger than the cell.
export function emojiSize(cell: number): number {
  return Math.max(1, Math.min(cell, Math.floor(cell * 0.85)))
}

function inRect(r: { x0: number; y0: number; x1: number; y1: number }, x: number, y: number) {
  return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1
}

export function flagGlyph(ctx: CanvasRenderingContext2D, x: number, y: number, s: number, color: string) {
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

function drawPlayer(
  ctx: CanvasRenderingContext2D, p: PlayerState, px: number, py: number, cell: number, isMe: boolean,
): void {
  const cx = px + cell / 2, cy = py + cell / 2
  const color = TEAM_COLOR[p.team]
  ctx.save()
  ctx.globalAlpha = p.out ? 0.35 : 1
  if (!p.out) {
    ctx.strokeStyle = color
    ctx.lineWidth = Math.max(1, Math.min(2, cell / 6))
    ctx.beginPath()
    ctx.arc(cx, cy, cell * 0.44, 0, Math.PI * 2)
    ctx.stroke()
  }
  ctx.font = `${emojiSize(cell)}px ${EMOJI_FONT}`
  ctx.fillStyle = '#fff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(playerGlyph(p), cx, cy + cell * 0.04)
  if (p.hasFlag) flagGlyph(ctx, px + cell * 0.35, py - cell * 0.4, cell * 0.8, color)
  if (isMe) {
    ctx.strokeStyle = RING
    ctx.lineWidth = Math.max(2, Math.min(3, cell / 5))
    ctx.beginPath()
    ctx.arc(cx, cy, cell * 0.5, 0, Math.PI * 2)
    ctx.stroke()
  }
  ctx.restore()
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

  let me: { p: PlayerState; px: number; py: number } | null = null
  for (let y = 0; y < map.height; y++) {
    for (let x = 0; x < map.width; x++) {
      const c = grid[y][x]
      const px = x * cell, py = y * cell
      const cx = px + cell / 2, cy = py + cell / 2
      const color = c.team ? TEAM_COLOR[c.team] : '#fff'
      if (ascii) {
        // Text mode: every cell is one character, nothing else.
        const out = c.player?.out
        ctx.globalAlpha = out ? 0.35 : 1
        ctx.fillStyle = c.kind === 'floor' ? FLOOR : c.kind === 'safe' ? SAFE : c.kind === 'obstacle' ? OBSTACLE : color
        ctx.fillText(cellChar(c), cx, cy)
        ctx.globalAlpha = 1
        continue
      }
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
          drawPlayer(ctx, c.player!, px, py, cell, false)
          break
        case 'you':
          me = { p: c.player!, px, py }
          break
      }
    }
  }
  // The local player is drawn last so it is always on top.
  if (me) drawPlayer(ctx, me.p, me.px, me.py, cell, true)
}
