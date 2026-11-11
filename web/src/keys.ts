import type { IntentKind } from './protocol'

export type Prefix = 'f' | 'b' | null
export interface Intent {
  kind: IntentKind
  dir?: number
}

// Dir values are 0..7 clockwise from north.
const DIRS: Record<string, number> = { w: 0, e: 1, d: 2, c: 3, s: 4, z: 5, a: 6, q: 7 }

// moveDirForKey is the direction for a WASD/QEZC key, or undefined.
export function moveDirForKey(key: string): number | undefined {
  return DIRS[key.toLowerCase()]
}

export const DIR_NAMES = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW']

// keyToIntent maps a key press to the next pending prefix and an optional
// intent to send. Unknown keys change nothing.
export function keyToIntent(prefix: Prefix, key: string): { prefix: Prefix; intent?: Intent } {
  const k = key.length === 1 ? key.toLowerCase() : key
  if (k === 'Escape') return { prefix: null, intent: undefined }
  if (k in DIRS) {
    const dir = DIRS[k]
    if (prefix === 'f') return { prefix: null, intent: { kind: 'throw', dir } }
    if (prefix === 'b') return { prefix: null, intent: { kind: 'wall', dir } }
    return { prefix: null, intent: { kind: 'move', dir } }
  }
  if (k === 'f' || k === 'b') return { prefix: k, intent: undefined }
  if (k === ' ' || k === 'x') return { prefix: null, intent: { kind: 'stay' } }
  return { prefix, intent: undefined }
}

// shouldIgnoreKeyTarget is true when a key event belongs to a form control or
// other focused widget, so game keys must not steal it.
export function shouldIgnoreKeyTarget(el: { tagName?: string; isContentEditable?: boolean; getAttribute?: (n: string) => string | null } | null): boolean {
  if (!el) return false
  const tag = (el.tagName ?? '').toUpperCase()
  if (['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON', 'A'].includes(tag)) return true
  if (el.isContentEditable) return true
  return el.getAttribute?.('role') === 'button'
}
