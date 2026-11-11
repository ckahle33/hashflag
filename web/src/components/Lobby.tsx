import { useState } from 'react'
import type { ClientMsg, LobbyMsg, RoundEndMsg } from '../protocol'
import { getItem } from '../storage'
import { NAME_KEY } from '../useGame'
import { Leaderboard } from './Leaderboard'

interface Props {
  lobby: LobbyMsg | null
  roundEnd: RoundEndMsg | null
  send: (m: ClientMsg) => void
  rename: (name: string) => void
}

export function Lobby({ lobby, roundEnd, send, rename }: Props) {
  const [name, setName] = useState(getItem(NAME_KEY) ?? '')
  if (!lobby) return <p>Connecting...</p>
  const mine = lobby.factions.find((f) => f.id === lobby.factionId)
  const canReroll = mine !== undefined && mine.members === 1

  return (
    <div className="lobby">
      {roundEnd && (
        <section className="roundend" aria-label="Round result">
          <h2>{roundEnd.winner === 'draw' ? 'Draw' : `Team ${roundEnd.winner} wins`}</h2>
          <p>A {roundEnd.scores[0]} : {roundEnd.scores[1]} B</p>
          <div className="cols">
            <Leaderboard title="Top squads this round" entries={roundEnd.topFactionsThisRound} />
            <Leaderboard title="Top squads all time" entries={roundEnd.topFactionsAllTime} />
          </div>
        </section>
      )}
      {lobby.waiting && <p className="notice">Sides are full, waiting for a slot.</p>}

      <form
        className="row"
        onSubmit={(e) => {
          e.preventDefault()
          if (name !== (getItem(NAME_KEY) ?? '')) rename(name)
        }}
      >
        <label>
          Name <input value={name} maxLength={20} onChange={(e) => setName(e.target.value)} />
        </label>
        <button type="submit">Set name</button>
      </form>
      <p className="muted">Setting a new name starts a fresh identity (you leave your squad).</p>

      <div className="roles">
        <button className="big" onClick={() => send({ t: 'setRole', role: 'attacker' })}>
          Play as attacker
        </button>
        <button className="big" onClick={() => send({ t: 'setRole', role: 'defender' })}>
          Play as defender
        </button>
      </div>
      {lobby.role && <p>Chosen role: {lobby.role}</p>}

      <h3>Squads</h3>
      <div className="row">
        <button onClick={() => send({ t: 'createFaction' })}>Start a squad</button>
        {mine && <button onClick={() => send({ t: 'leaveFaction' })}>Leave squad</button>}
        {canReroll && <button onClick={() => send({ t: 'rerollFaction' })}>Re-roll name</button>}
      </div>
      {lobby.factions.length === 0 ? (
        <p className="muted">No squads yet.</p>
      ) : (
        <ul className="factions">
          {lobby.factions.map((f) => (
            <li key={f.id}>
              <span aria-hidden="true">{f.glyph}</span> <strong>{f.name}</strong>{' '}
              <span className={`side side${f.team}`}>side {f.team}</span>{' '}
              <span className="muted">{f.members} {f.members === 1 ? 'member' : 'members'}</span>{' '}
              {f.id === lobby.factionId ? (
                <em>your squad</em>
              ) : (
                <button onClick={() => send({ t: 'joinFaction', id: f.id })}>Join {f.name}</button>
              )}
            </li>
          ))}
        </ul>
      )}
      <Leaderboard title="All-time leaderboard" entries={lobby.board} />
    </div>
  )
}
