import { describe, expect, it } from 'vitest'
import { parseServerMsg } from './protocol'

// Samples follow the JSON tags in internal/server/protocol.go.
const samples: Record<string, string> = {
  welcome: '{"t":"welcome","token":"abc","playerId":"p1"}',
  lobby:
    '{"t":"lobby","factions":[{"id":"f1","name":"Damp Pigeons","glyph":"🐦","team":"A","members":2}],' +
    '"board":[{"name":"Damp Pigeons","captures":3}],"waiting":false,"inWorld":false,"role":"","factionId":"f1"}',
  round:
    '{"t":"round","round":1,"map":{"width":2,"height":1,"rows":["#."],"safeX0":0,"safeX1":0,' +
    '"baseA":{"x0":0,"y0":0,"x1":0,"y1":0},"baseB":{"x0":1,"y0":0,"x1":1,"y1":0},' +
    '"flagA":{"x":0,"y":0},"flagB":{"x":1,"y":0}},"you":{"id":"p1","team":"A","role":"attacker"}}',
  state:
    '{"t":"state","tick":4,"players":[{"id":"p1","name":"n","x":1,"y":0,"team":"A","role":"attacker","glyph":"","out":false,"hasFlag":false}],' +
    '"projectiles":[{"x":0,"y":0,"dir":2,"team":"A"}],"walls":[{"x":1,"y":0,"team":"B"}],' +
    '"flags":[{"team":"B","x":1,"y":0,"carrier":""}],"scores":[1,0],"events":[{"kind":"tagged","player":"p2","other":"p1","team":"A","x":0,"y":0}]}',
  you: '{"t":"you","ammo":9,"cooldownLeft":1,"outLeft":0,"hasFlag":false,"wallsPlaced":0}',
  roundEnd:
    '{"t":"roundEnd","winner":"draw","scores":[1,1],"topFactionsThisRound":[{"name":"X","captures":1}],"topFactionsAllTime":[]}',
  error: '{"t":"error","message":"nope"}',
}

describe('parseServerMsg', () => {
  for (const [t, json] of Object.entries(samples)) {
    it(`parses ${t}`, () => {
      const m = parseServerMsg(json)
      expect(m).not.toBeNull()
      expect(m!.t).toBe(t)
    })
  }

  it('exposes typed fields', () => {
    const m = parseServerMsg(samples.state)
    if (!m || m.t !== 'state') throw new Error('expected state')
    expect(m.scores).toEqual([1, 0])
    expect(m.players[0].hasFlag).toBe(false)
    expect(m.events[0].kind).toBe('tagged')
  })

  it('rejects junk and unknown types', () => {
    expect(parseServerMsg('not json')).toBeNull()
    expect(parseServerMsg('{"t":"bogus"}')).toBeNull()
    expect(parseServerMsg('42')).toBeNull()
    expect(parseServerMsg('null')).toBeNull()
  })
})
