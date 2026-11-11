import type { MapInfo, PlayerState, StateMsg } from './protocol'
import { composeGrid } from './render'
import { BG, OBSTACLE, RING, TEAM_COLOR, TEAM_TINT } from './palette'
import { emojiSize, flagGlyph, playerGlyph } from './draw'
import { BLOCK_H, TILT, cellToScreen, occludes, tiltedSize } from './projection'

const EMOJI_FONT = 'system-ui, "Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", sans-serif'
const WALL_H = 0.6
const FLOOR_TILE = 'rgba(255,255,255,0.045)'
const SAFE_TILE = 'rgba(232,228,90,0.22)'
const SHADOW = 'rgba(0,0,0,0.35)'
const FRONT = 0.7

// shade scales a #rrggbb colour's brightness.
export function shade(hex: string, f: number): string {
  const n = parseInt(hex.slice(1), 16)
  const c = (v: number) => Math.max(0, Math.min(255, Math.round(v * f)))
  return `rgb(${c((n >> 16) & 255)},${c((n >> 8) & 255)},${c(n & 255)})`
}

// Colours computed once at load, so the hot loops only assign constants.
const STONE = { top: OBSTACLE, front: shade(OBSTACLE, FRONT) }
const WALL = {
  A: { top: TEAM_COLOR.A, front: shade(TEAM_COLOR.A, FRONT) },
  B: { top: TEAM_COLOR.B, front: shade(TEAM_COLOR.B, FRONT) },
}

function inRect(r: { x0: number; y0: number; x1: number; y1: number }, x: number, y: number) {
  return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1
}

function ellipse(ctx: CanvasRenderingContext2D, cx: number, cy: number, rx: number, ry: number) {
  ctx.beginPath()
  ctx.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2)
}

function block(
  ctx: CanvasRenderingContext2D, px: number, py: number, cell: number, h: number,
  top: string, front: string, alpha: number,
) {
  const t = cell * TILT
  ctx.globalAlpha = alpha
  ctx.fillStyle = front
  ctx.fillRect(px, py - h + t, cell, h)
  ctx.fillStyle = top
  ctx.fillRect(px, py - h, cell, t)
  ctx.globalAlpha = 1
}

function entity(
  ctx: CanvasRenderingContext2D, p: PlayerState, px: number, py: number, cell: number, isMe: boolean,
) {
  const t = cell * TILT
  const cx = px + cell / 2, cy = py + t / 2
  const color = TEAM_COLOR[p.team]
  ctx.save()
  ctx.globalAlpha = p.out ? 0.35 : 1
  ctx.fillStyle = SHADOW
  ellipse(ctx, cx, cy, cell * 0.4, t * 0.4)
  ctx.fill()
  if (!p.out) {
    ctx.strokeStyle = color
    ctx.lineWidth = Math.max(1, Math.min(2, cell / 6))
    ellipse(ctx, cx, cy, cell * 0.42, t * 0.42)
    ctx.stroke()
  }
  const es = emojiSize(cell)
  ctx.font = `${es}px ${EMOJI_FONT}`
  ctx.fillStyle = '#fff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'bottom'
  ctx.fillText(playerGlyph(p), cx, py + t)
  if (p.hasFlag) {
    const s = cell * 0.7
    flagGlyph(ctx, cx - s * 0.3, py + t - es - s * 0.95, s, color)
  }
  if (isMe) {
    ctx.strokeStyle = RING
    ctx.lineWidth = Math.max(2, Math.min(3, cell / 5))
    ellipse(ctx, cx, cy, cell * 0.5, t * 0.5)
    ctx.stroke()
  }
  ctx.restore()
}

export function drawTilted(
  ctx: CanvasRenderingContext2D, map: MapInfo, state: StateMsg, youId: string, cell: number,
): void {
  const grid = composeGrid(map, state, youId)
  const { w, h } = tiltedSize(map.width, map.height, cell)
  const t = cell * TILT
  const tw = cell - 1, th = Math.max(1, t - 1)
  ctx.lineCap = 'round'
  ctx.fillStyle = BG
  ctx.fillRect(0, 0, w, h)

  // 1. Floor pass.
  for (let y = 0; y < map.height; y++) {
    const py = cell * BLOCK_H + y * t
    for (let x = 0; x < map.width; x++) {
      const px = x * cell
      ctx.fillStyle = grid[y][x].kind === 'safe' ? SAFE_TILE : FLOOR_TILE
      ctx.fillRect(px + 0.5, py + 0.5, tw, th)
      const tint = inRect(map.baseA, x, y) ? TEAM_TINT.A : inRect(map.baseB, x, y) ? TEAM_TINT.B : null
      if (tint) {
        ctx.fillStyle = tint
        ctx.fillRect(px, py, cell, t)
      }
    }
  }

  // Which player is "you", for occlusion.
  let you: { x: number; y: number } | null = null
  for (const p of state.players) if (p.id === youId) you = p

  // 2. Object pass, back to front.
  for (let y = 0; y < map.height; y++) {
    let me: { p: PlayerState; px: number; py: number } | null = null
    for (let x = 0; x < map.width; x++) {
      const c = grid[y][x]
      if (c.kind === 'floor' || c.kind === 'safe') continue
      const { px, py } = cellToScreen(x, y, cell)
      const cx = px + cell / 2, cy = py + t / 2
      switch (c.kind) {
        case 'obstacle':
          block(ctx, px, py, cell, cell * BLOCK_H, STONE.top, STONE.front, you && occludes({ x, y }, you) ? 0.45 : 1)
          break
        case 'wall': {
          const col = WALL[c.team ?? 'A']
          block(ctx, px, py, cell, cell * WALL_H, col.top, col.front, you && occludes({ x, y }, you) ? 0.45 : 1)
          break
        }
        case 'projectile': {
          ctx.fillStyle = SHADOW
          ellipse(ctx, cx, cy, cell * 0.14, t * 0.14)
          ctx.fill()
          ctx.fillStyle = TEAM_COLOR[c.team ?? 'A']
          ctx.beginPath()
          ctx.arc(cx, cy - cell * 0.3, Math.max(1.5, cell * 0.14), 0, Math.PI * 2)
          ctx.fill()
          break
        }
        case 'flag':
          flagGlyph(ctx, px + cell * 0.1, py + t - cell * 0.9, cell * 0.9, TEAM_COLOR[c.team ?? 'A'])
          break
        case 'player':
          entity(ctx, c.player!, px, py, cell, false)
          break
        case 'you':
          me = { p: c.player!, px, py }
          break
      }
    }
    if (me) entity(ctx, me.p, me.px, me.py, cell, true)
  }
}
