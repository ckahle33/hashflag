import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  ClientMsg, LobbyMsg, MapInfo, RoundEndMsg, StateMsg, YouInfo, YouMsg,
} from './protocol'
import { parseServerMsg } from './protocol'
import { getItem, removeItem, setItem } from './storage'

export type Conn = 'connecting' | 'open' | 'reconnecting'
export type Phase = 'lobby' | 'playing'

export interface GameState {
  conn: Conn
  playerId: string
  lobby: LobbyMsg | null
  map: MapInfo | null
  youInfo: YouInfo | null
  state: StateMsg | null
  you: YouMsg | null
  roundEnd: RoundEndMsg | null
  round: number
  error: string | null
}

const initial: GameState = {
  conn: 'connecting', playerId: '', lobby: null, map: null, youInfo: null,
  state: null, you: null, roundEnd: null, round: 0, error: null,
}

export const NAME_KEY = 'hashflag.name'
const TOKEN_KEY = 'hashflag.token'

export function wsUrl(loc: Pick<Location, 'protocol' | 'host'> = window.location): string {
  return `${loc.protocol === 'https:' ? 'wss:' : 'ws:'}//${loc.host}/ws`
}

export function backoffMs(attempt: number): number {
  return Math.min(1000 * 2 ** attempt, 10000)
}

export function useGame() {
  const [g, setG] = useState<GameState>(initial)
  const ws = useRef<WebSocket | null>(null)
  const attempt = useRef(0)
  const timer = useRef<number | undefined>(undefined)
  const closed = useRef(false)

  const send = useCallback((m: ClientMsg) => {
    const s = ws.current
    if (s && s.readyState === WebSocket.OPEN) s.send(JSON.stringify(m))
  }, [])

  const connect = useCallback(() => {
    const s = new WebSocket(wsUrl())
    ws.current = s
    s.onopen = () => {
      attempt.current = 0
      setG((p) => ({ ...p, conn: 'open' }))
      const token = getItem(TOKEN_KEY) ?? undefined
      send({ t: 'hello', token, name: getItem(NAME_KEY) ?? '' })
    }
    s.onmessage = (ev) => {
      const m = typeof ev.data === 'string' ? parseServerMsg(ev.data) : null
      if (!m) return
      setG((p) => {
        switch (m.t) {
          case 'welcome':
            setItem(TOKEN_KEY, m.token)
            return { ...p, playerId: m.playerId }
          case 'lobby':
            return { ...p, lobby: m }
          case 'round':
            return {
              ...p, map: m.map, youInfo: m.you, round: m.round, roundEnd: null,
              state: null, you: null,
            }
          case 'state':
            return { ...p, state: m }
          case 'you':
            return { ...p, you: m }
          case 'roundEnd':
            return { ...p, roundEnd: m }
          case 'error':
            return { ...p, error: m.message }
        }
      })
    }
    s.onclose = () => {
      if (closed.current || ws.current !== s) return
      setG((p) => ({ ...p, conn: 'reconnecting' }))
      timer.current = window.setTimeout(connect, backoffMs(attempt.current++))
    }
    s.onerror = () => s.close()
  }, [send])

  useEffect(() => {
    closed.current = false
    connect()
    return () => {
      closed.current = true
      window.clearTimeout(timer.current)
      ws.current?.close()
    }
  }, [connect])

  // A new name needs a fresh identity: drop the token and reconnect.
  const rename = useCallback((name: string) => {
    setItem(NAME_KEY, name)
    removeItem(TOKEN_KEY)
    setG({ ...initial, conn: 'reconnecting' })
    const old = ws.current
    ws.current = null
    old?.close()
    window.clearTimeout(timer.current)
    connect()
  }, [connect])

  const dismissError = useCallback(() => setG((p) => ({ ...p, error: null })), [])

  const phase: Phase = g.lobby?.inWorld && g.map && g.state ? 'playing' : 'lobby'
  return { ...g, phase, send, rename, dismissError }
}
