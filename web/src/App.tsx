import { useEffect, useRef, useState } from 'react'
import { useGame } from './useGame'
import { keyToIntent, shouldIgnoreKeyTarget, type Prefix } from './keys'
import { clearHeld, heldIntent, newHeld, pressKey, releaseKey, snapDir } from './input'
import { getItem, setItem } from './storage'
import { Grid, type AimPhase } from './components/Grid'
import { Lobby } from './components/Lobby'
import { VIEW_KEY, parseView, serializeView, toggleAscii, toggleGraphical, type View } from './viewmode'


export function App() {
  const game = useGame()
  const [view, setView] = useState<View>(() => parseView(getItem(VIEW_KEY)))
  const changeView = (f: (v: View) => View) =>
    setView((v) => {
      const n = f(v)
      setItem(VIEW_KEY, serializeView(n))
      return n
    })
  const [prefix, setPrefix] = useState<Prefix>(null)
  const prefixRef = useRef<Prefix>(null)
  const { send, phase } = game
  const held = useRef(newHeld())
  const pointerCell = useRef<{ x: number; y: number } | null>(null)
  const stateRef = useRef(game.state)
  stateRef.current = game.state
  const youIdRef = useRef(game.playerId)
  youIdRef.current = game.playerId

  // Point the held mouse at the latest pointer cell, relative to the local player.
  const aim = () => {
    const me = stateRef.current?.players.find((p) => p.id === youIdRef.current)
    const c = pointerCell.current
    held.current.mouseDir = me && c ? snapDir(c.x - me.x, c.y - me.y) : null
  }

  const onAim = (ph: AimPhase, cell: { x: number; y: number } | null, shift: boolean) => {
    if (ph === 'up') {
      pointerCell.current = null
      held.current.mouseDir = null
      return
    }
    held.current.shift = shift || held.current.shift
    pointerCell.current = cell
    aim()
    if (ph === 'down') {
      const i = heldIntent(held.current)
      if (i) send({ t: 'intent', ...i })
    }
  }

  // Hold to repeat: one intent per state frame while something is held.
  const frame = game.state
  useEffect(() => {
    if (phase !== 'playing' || !frame) return
    aim()
    const i = heldIntent(held.current)
    if (i) send({ t: 'intent', ...i })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [frame])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (e.repeat || e.ctrlKey || e.metaKey || e.altKey) return
      if (shouldIgnoreKeyTarget(t)) return
      if (phase !== 'playing') return
      if (e.key === 't' || e.key === 'T') {
        changeView(toggleAscii)
        return
      }
      if (e.key === 'v' || e.key === 'V') {
        changeView(toggleGraphical)
        return
      }
      if (e.key === 'Shift') {
        pressKey(held.current, e.key, false)
        return
      }
      held.current.shift = e.shiftKey
      if (e.key.startsWith('Arrow')) {
        e.preventDefault()
        pressKey(held.current, e.key, false)
        const i = heldIntent(held.current)
        if (i) send({ t: 'intent', ...i })
        return
      }
      const r = keyToIntent(prefixRef.current, e.key)
      if (r.intent?.kind === 'move') pressKey(held.current, e.key, true)
      if (r.intent === undefined && r.prefix === prefixRef.current && e.key !== 'Escape') return
      e.preventDefault()
      prefixRef.current = r.prefix
      setPrefix(r.prefix)
      if (r.intent) send({ t: 'intent', ...r.intent })
    }
    const onUp = (e: KeyboardEvent) => releaseKey(held.current, e.key)
    const clear = () => {
      clearHeld(held.current)
      pointerCell.current = null
      prefixRef.current = null
      setPrefix(null)
    }
    const onVis = () => {
      if (document.visibilityState === 'hidden') clear()
    }
    if (phase !== 'playing') clear()
    window.addEventListener('keydown', onKey)
    window.addEventListener('keyup', onUp)
    window.addEventListener('blur', clear)
    document.addEventListener('visibilitychange', onVis)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('keyup', onUp)
      window.removeEventListener('blur', clear)
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [phase, send])

  const { conn, error, dismissError, reconnect } = game
  const prefixLabel = prefix === 'f' ? 'THROW: press a direction (Esc cancels)' : 'WALL: press a direction (Esc cancels)'

  const notices = (
    <>
      {conn === 'replaced' && (
        <p role="alert" className="banner">
          This session was opened in another tab. <button onClick={reconnect}>Reconnect</button>
        </p>
      )}
      {conn !== 'open' && conn !== 'replaced' && <p role="status" className="banner">Reconnecting...</p>}
      {error && (
        <p role="alert" className="notice">
          {error} <button onClick={dismissError}>Dismiss</button>
        </p>
      )}
    </>
  )

  if (phase === 'playing' && game.map && game.state) {
    const you = game.you
    return (
      <main className="play">
        <section className="hud" aria-label="Status">
          <strong>hashflag</strong>
          <span>A {game.state.scores[0]} : {game.state.scores[1]} B</span>
          <span>Tick {game.state.tick}</span>
          <span>Ammo {you?.ammo ?? '-'}</span>
          <span>CD {you?.cooldownLeft ?? 0}</span>
          <span>Out {you?.outLeft ?? 0}</span>
          <span>Walls {you?.wallsPlaced ?? 0}</span>
          <span>{you?.hasFlag ? 'CARRYING THE FLAG' : 'No flag'}</span>
          {prefix && <span>{prefixLabel}</span>}
          <span>View: {view.mode} (V tilt/flat, T text)</span>
          <span>{game.youInfo ? `${game.youInfo.team} ${game.youInfo.role}` : ''}</span>
        </section>
        {notices}
        <Grid map={game.map} state={game.state} youId={game.playerId} mode={view.mode} onAim={onAim} />
        <details className="legend">
          <summary>Controls</summary>
          <p>
            Move WASD, diagonals Q E Z C. Arrow keys throw (two arrows for a diagonal); Shift + arrow builds a wall.
            Click the map to throw toward the pointer; Shift + click builds a wall. Hold a key or the button to repeat.
            F + direction throws, B + direction builds a wall. Space or X stays. Esc cancels. V switches tilted and flat view, T toggles text mode ({view.mode === 'ascii' ? 'on' : 'off'}).
          </p>
        </details>
      </main>
    )
  }

  return (
    <main>
      <header>
        <h1>hashflag</h1>
      </header>
      {notices}
      <Lobby lobby={game.lobby} roundEnd={game.roundEnd} send={send} rename={game.rename} />
    </main>
  )
}
