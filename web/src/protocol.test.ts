import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
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


// Go struct json tags are the source of truth: parse protocol.go and require
// each sample's keys to match the struct exactly (omitempty keys optional).
const goSrc = readFileSync(resolve(process.cwd(), '../internal/server/protocol.go'), 'utf8')

function tags(struct: string): { req: string[]; all: string[] } {
  const m = goSrc.match(new RegExp(`type ${struct} struct \\{([\\s\\S]*?)\\n\\}`))
  if (!m) throw new Error(`no struct ${struct}`)
  const req: string[] = []
  const all: string[] = []
  for (const line of m[1].split('\n')) {
    const t = line.match(/json:"([^",]+)(,omitempty)?"/)
    if (!t) continue
    all.push(t[1])
    if (!t[2]) req.push(t[1])
  }
  return { req, all }
}

function checkKeys(v: Record<string, unknown>, struct: string) {
  const { req, all } = tags(struct)
  const keys = Object.keys(v)
  for (const k of req) expect(keys, `${struct} missing ${k}`).toContain(k)
  for (const k of keys) expect(all, `${struct} unexpected ${k}`).toContain(k)
}

const first = (v: unknown) => (v as Record<string, unknown>[])[0]

describe('samples match protocol.go tags', () => {
  const p = (t: string) => JSON.parse(samples[t]) as Record<string, any>
  it('welcome', () => {
    checkKeys(p('welcome'), 'WelcomeMsg')
    expect(typeof p('welcome').playerId).toBe('string')
    expect(typeof p('welcome').token).toBe('string')
  })
  it('lobby', () => {
    const l = p('lobby')
    checkKeys(l, 'LobbyMsg')
    checkKeys(first(l.factions), 'FactionInfo')
    expect(Object.keys(first(l.board)).sort()).toEqual(['captures', 'name'])
    expect(typeof l.factionId).toBe('string')
    expect(typeof l.inWorld).toBe('boolean')
    expect(typeof l.waiting).toBe('boolean')
    expect(typeof l.role).toBe('string')
  })
  it('round', () => {
    const r = p('round')
    checkKeys(r, 'RoundMsg')
    checkKeys(r.map, 'MapMsg')
    checkKeys(r.map.baseA, 'RectMsg')
    checkKeys(r.map.baseB, 'RectMsg')
    checkKeys(r.map.flagA, 'PosMsg')
    checkKeys(r.you, 'YouInfo')
    expect(typeof r.map.safeX0).toBe('number')
    expect(typeof r.map.safeX1).toBe('number')
  })
  it('state', () => {
    const s = p('state')
    checkKeys(s, 'StateMsg')
    checkKeys(first(s.players), 'PlayerState')
    checkKeys(first(s.projectiles), 'ProjectileState')
    checkKeys(first(s.walls), 'WallState')
    checkKeys(first(s.flags), 'FlagState')
    checkKeys(first(s.events), 'EventMsg')
    expect(typeof first(s.flags).carrier).toBe('string')
    expect(s.scores).toHaveLength(2)
  })
  it('you', () => {
    const y = p('you')
    checkKeys(y, 'YouMsg')
    for (const k of ['ammo', 'cooldownLeft', 'outLeft', 'wallsPlaced']) expect(typeof y[k]).toBe('number')
    expect(typeof y.hasFlag).toBe('boolean')
  })
  it('roundEnd', () => {
    const e = p('roundEnd')
    checkKeys(e, 'RoundEndMsg')
    expect(Array.isArray(e.topFactionsThisRound)).toBe(true)
    expect(Array.isArray(e.topFactionsAllTime)).toBe(true)
  })
  it('error', () => checkKeys(p('error'), 'ErrorMsg'))
  it('detects a renamed field', () => {
    expect(() => checkKeys({ ...p('you'), cooldown: 1 }, 'YouMsg')).toThrow()
  })
})
