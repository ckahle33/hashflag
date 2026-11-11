import { useEffect, useRef, useState } from 'react'
import { useGame } from './useGame'
import { keyToIntent, shouldIgnoreKeyTarget, type Prefix } from './keys'
import { getItem, setItem } from './storage'
import { Grid } from './components/Grid'
import { Lobby } from './components/Lobby'

const ASCII_KEY = 'hashflag.ascii'

export function App() {
  const game = useGame()
  const [ascii, setAscii] = useState(() => getItem(ASCII_KEY) === '1')
  const [prefix, setPrefix] = useState<Prefix>(null)
  const prefixRef = useRef<Prefix>(null)
  const { send, phase } = game

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (e.repeat || e.ctrlKey || e.metaKey || e.altKey) return
      if (shouldIgnoreKeyTarget(t)) return
      if (phase !== 'playing') return
      if (e.key === 't' || e.key === 'T') {
        setAscii((a) => {
          setItem(ASCII_KEY, a ? '0' : '1')
          return !a
        })
        return
      }
      const r = keyToIntent(prefixRef.current, e.key)
      if (r.intent === undefined && r.prefix === prefixRef.current && e.key !== 'Escape') return
      e.preventDefault()
      prefixRef.current = r.prefix
      setPrefix(r.prefix)
      if (r.intent) send({ t: 'intent', ...r.intent })
    }
    const clear = () => {
      prefixRef.current = null
      setPrefix(null)
    }
    const onVis = () => {
      if (document.visibilityState === 'hidden') clear()
    }
    if (phase !== 'playing') clear()
    window.addEventListener('keydown', onKey)
    window.addEventListener('blur', clear)
    document.addEventListener('visibilitychange', onVis)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('blur', clear)
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [phase, send])

  const { conn, error, dismissError } = game
  const prefixLabel = prefix === 'f' ? 'THROW: press a direction (Esc cancels)' : prefix === 'b' ? 'WALL: press a direction (Esc cancels)' : 'none'

  return (
    <main>
      <header>
        <h1>hashflag</h1>
        {conn !== 'open' && <p role="status" className="banner">Reconnecting...</p>}
      </header>
      {error && (
        <p role="alert" className="notice">
          {error} <button onClick={dismissError}>Dismiss</button>
        </p>
      )}
      {phase === 'playing' && game.map && game.state ? (
        <>
          <section className="hud" aria-label="Status">
            <span>Team A {game.state.scores[0]} vs {game.state.scores[1]} Team B</span>
            <span>Tick {game.state.tick}</span>
            <span>Ammo {game.you?.ammo ?? '-'}</span>
            <span>Cooldown {game.you?.cooldownLeft ?? 0}</span>
            <span>Out {game.you?.outLeft ?? 0}</span>
            <span>Walls {game.you?.wallsPlaced ?? 0}</span>
            <span>{game.you?.hasFlag ? 'CARRYING THE FLAG' : 'No flag'}</span>
            <span>Prefix: {prefixLabel}</span>
            <span>{game.youInfo ? `You: team ${game.youInfo.team}, ${game.youInfo.role}` : ''}</span>
          </section>
          <Grid map={game.map} state={game.state} youId={game.playerId} ascii={ascii} />
          <p className="legend">
            Move WASD, diagonals Q E Z C. F + direction throws. B + direction builds a wall. Space or X stays.
            Esc cancels. T toggles text mode ({ascii ? 'on' : 'off'}).
          </p>
        </>
      ) : (
        <Lobby lobby={game.lobby} roundEnd={game.roundEnd} send={send} rename={game.rename} />
      )}
    </main>
  )
}
