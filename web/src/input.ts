import type { Intent } from './keys'
import { moveDirForKey } from './keys'

// Held-input state. moves is in press order (last = most recent).
export interface HeldState {
  moves: number[]
  arrows: string[]
  shift: boolean
  mouseDir: number | null
}

export const newHeld = (): HeldState => ({ moves: [], arrows: [], shift: false, mouseDir: null })

export function clearHeld(h: HeldState): void {
  h.moves = []
  h.arrows = []
  h.shift = false
  h.mouseDir = null
}

const ARROWS = ['ArrowUp', 'ArrowRight', 'ArrowDown', 'ArrowLeft']

// snapDir snaps a vector to one of 8 directions, 0 = north, clockwise.
// y grows downward. Returns null for the zero vector.
export function snapDir(dx: number, dy: number): number | null {
  if (dx === 0 && dy === 0) return null
  const angle = Math.atan2(dx, -dy) // clockwise from north
  return ((Math.round(angle / (Math.PI / 4)) % 8) + 8) % 8
}

// arrowsDir maps the held arrow keys to a throw direction; opposites cancel.
export function arrowsDir(arrows: string[]): number | null {
  const has = (k: string) => arrows.includes(k)
  const dx = (has('ArrowRight') ? 1 : 0) - (has('ArrowLeft') ? 1 : 0)
  const dy = (has('ArrowDown') ? 1 : 0) - (has('ArrowUp') ? 1 : 0)
  return snapDir(dx, dy)
}

// heldIntent is the one intent to send for a tick: fire beats move.
export function heldIntent(h: HeldState): Intent | null {
  const fire = arrowsDir(h.arrows) ?? h.mouseDir
  if (fire !== null) return { kind: h.shift ? 'wall' : 'throw', dir: fire }
  if (h.moves.length > 0) return { kind: 'move', dir: h.moves[h.moves.length - 1] }
  return null
}

// pressKey records a key going down. Move keys are held only when allowMove
// (no F/B prefix is pending).
export function pressKey(h: HeldState, key: string, allowMove: boolean): void {
  if (key === 'Shift') h.shift = true
  else if (ARROWS.includes(key)) {
    if (!h.arrows.includes(key)) h.arrows.push(key)
  } else if (allowMove) {
    const d = moveDirForKey(key)
    if (d !== undefined) h.moves = [...h.moves.filter((m) => m !== d), d]
  }
}

export function releaseKey(h: HeldState, key: string): void {
  if (key === 'Shift') h.shift = false
  else if (ARROWS.includes(key)) h.arrows = h.arrows.filter((k) => k !== key)
  else {
    const d = moveDirForKey(key)
    if (d !== undefined) h.moves = h.moves.filter((m) => m !== d)
  }
}
