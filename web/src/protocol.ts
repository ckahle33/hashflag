// Mirrors internal/server/protocol.go. The Go server is the source of truth.

export type Team = 'A' | 'B'
export type RoleName = 'attacker' | 'defender'
export type IntentKind = 'move' | 'throw' | 'wall' | 'stay'

// Client to server.
export type ClientMsg =
  | { t: 'hello'; token?: string; name: string }
  | { t: 'setRole'; role: RoleName }
  | { t: 'createFaction' }
  | { t: 'joinFaction'; id: string }
  | { t: 'leaveFaction' }
  | { t: 'rerollFaction' }
  | { t: 'intent'; kind: IntentKind; dir?: number }

export interface Entry {
  name: string
  captures: number
}

export interface WelcomeMsg {
  t: 'welcome'
  token: string
  playerId: string
}

export interface FactionInfo {
  id: string
  name: string
  glyph: string
  team: Team
  members: number
}

export interface LobbyMsg {
  t: 'lobby'
  factions: FactionInfo[]
  board: Entry[]
  waiting: boolean
  inWorld: boolean
  role: RoleName | ''
  factionId: string
}

export interface Rect {
  x0: number
  y0: number
  x1: number
  y1: number
}

export interface Pos {
  x: number
  y: number
}

export interface MapInfo {
  width: number
  height: number
  rows: string[]
  safeX0: number
  safeX1: number
  baseA: Rect
  baseB: Rect
  flagA: Pos
  flagB: Pos
}

export interface YouInfo {
  id: string
  team: Team
  role: RoleName
}

export interface RoundMsg {
  t: 'round'
  round: number
  map: MapInfo
  you: YouInfo | null
}

export interface PlayerState {
  id: string
  name: string
  x: number
  y: number
  team: Team
  role: RoleName
  glyph: string
  out: boolean
  hasFlag: boolean
}

export interface ProjectileState {
  x: number
  y: number
  dir: number
  team: Team
}

export interface WallState {
  x: number
  y: number
  team: Team
}

export interface FlagState {
  team: Team
  x: number
  y: number
  carrier: string
}

export interface EventMsg {
  kind: string
  player?: string
  other?: string
  team: Team
  x: number
  y: number
}

export interface StateMsg {
  t: 'state'
  tick: number
  players: PlayerState[]
  projectiles: ProjectileState[]
  walls: WallState[]
  flags: FlagState[]
  scores: [number, number]
  events: EventMsg[]
}

export interface YouMsg {
  t: 'you'
  ammo: number
  cooldownLeft: number
  outLeft: number
  hasFlag: boolean
  wallsPlaced: number
}

export interface RoundEndMsg {
  t: 'roundEnd'
  winner: Team | 'draw'
  scores: [number, number]
  topFactionsThisRound: Entry[]
  topFactionsAllTime: Entry[]
}

export interface ErrorMsg {
  t: 'error'
  message: string
}

export type ServerMsg =
  | WelcomeMsg
  | LobbyMsg
  | RoundMsg
  | StateMsg
  | YouMsg
  | RoundEndMsg
  | ErrorMsg

const SERVER_TYPES = ['welcome', 'lobby', 'round', 'state', 'you', 'roundEnd', 'error']

// parseServerMsg returns null for anything that is not a known server frame.
export function parseServerMsg(data: string): ServerMsg | null {
  let v: unknown
  try {
    v = JSON.parse(data)
  } catch {
    return null
  }
  if (typeof v !== 'object' || v === null) return null
  const t = (v as { t?: unknown }).t
  if (typeof t !== 'string' || !SERVER_TYPES.includes(t)) return null
  return v as ServerMsg
}
