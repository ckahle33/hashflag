export type Graphical = 'tilted' | 'flat'
export type ViewMode = Graphical | 'ascii'
export interface View {
  mode: ViewMode
  graphical: Graphical // the mode T returns to when leaving ascii
}

export const VIEW_KEY = 'hashflag.view'
const DEFAULT: View = { mode: 'tilted', graphical: 'tilted' }

// Stored as "tilted", "flat" or "ascii:<graphical>".
export function parseView(s: string | null): View {
  if (s === 'tilted' || s === 'flat') return { mode: s, graphical: s }
  if (s === 'ascii:tilted' || s === 'ascii:flat') return { mode: 'ascii', graphical: s.slice(6) as Graphical }
  return DEFAULT
}

export const serializeView = (v: View): string => (v.mode === 'ascii' ? `ascii:${v.graphical}` : v.mode)

export function toggleGraphical(v: View): View {
  if (v.mode === 'ascii') return v
  const g: Graphical = v.mode === 'tilted' ? 'flat' : 'tilted'
  return { mode: g, graphical: g }
}

export function toggleAscii(v: View): View {
  return v.mode === 'ascii' ? { mode: v.graphical, graphical: v.graphical } : { mode: 'ascii', graphical: v.graphical }
}
