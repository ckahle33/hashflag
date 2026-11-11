# Hashflag

Hashflag is a browser capture-the-flag game for hundreds of players in one shared, global room. Players pick an attacker or defender role and move on a grid, throwing projectiles to tag opponents, building walls to guard a base, and racing to carry the other team's flag home. Rounds are simulated by a Go server on a fixed tick and drawn by a React canvas client.

## Requirements

- Go 1.27
- Node (with npm) to build the web client

## Build and run

```
cd web && npm install && npm run build && cd ..
go build ./cmd/hashflag && ./hashflag -web-dir web/dist
```

Then open http://localhost:8080.

## Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config-dir` | `./configs` | Directory of game config presets (one `<id>.json` per preset) |
| `-config` | `standard` | Preset id to run |
| `-addr` | `:8080` | Listen address |
| `-data-dir` | `./data` | Where `board.json` (all-time faction leaderboard) and `stats.jsonl` (one line per round) are written |
| `-web-dir` | `./web/dist` | Built web client to serve at `/` |
| `-max-conns` | 0 | Maximum open WebSockets; 0 means 4 x `maxPerTeam`, at least 50 |
| `-max-conns-per-ip` | 0 | Maximum open WebSockets per client IP; 0 means 20 |
| `-origin` | none | Extra allowed WebSocket Origin pattern, repeatable. `localhost:*` and `127.0.0.1:*` are always allowed |

## Custom game configs

A preset is a JSON file in the config directory; select it with `-config <id>`. It needs an `id` (matching the file name) and a `name`. Use `extends` to start from another preset and override only what you change; objects are deep-merged over the parent, and omitting `extends` starts from the built-in defaults.

```json
{
  "id": "quick",
  "name": "Quick",
  "extends": "standard",
  "roundTicks": 120,
  "map": { "width": 80, "height": 36 },
  "projectile": { "rangeCells": 8 }
}
```

The full list of fields, types and bounds is in [docs/superpowers/specs/hashflag-config.schema.json](docs/superpowers/specs/hashflag-config.schema.json). Two fields worth knowing:

- `tag.safeLineMaxTicks` (default 3): the safe line protects a player only for this many consecutive ticks; after that they can be tagged while standing on it. Stepping off resets the count. `0` keeps the old unlimited immunity.
- `map.obstacles.generate`: `"clusters"` (default) places large irregular blobs covering about `density` (default 0.25) of the open field, with `clusterSize` (default 8) as the mean blob radius, and puts one across the straight lane to each flag whenever the map is large enough to have a lane between the bases and the safe band. `"scatter"` sprinkles single cells with probability `density`.

Some schema fields are not implemented yet (for example explicit obstacle rows, the `mazeish` and `chokepoints` generators, and non-default `respawnAt`, `refillWhere` and `placeZone` values). The server rejects a config that sets them, with an error naming the field, rather than silently ignoring them.

## Presets

| Preset | Tick | Notes |
| --- | --- | --- |
| `standard` | 1000 ms | The built-in defaults: 300-tick rounds |
| `tactical` | 300 ms | 1000-tick rounds (5 minutes); out time, safe-line limit and cooldowns scaled to the faster tick |
| `realtime` | 100 ms | Experimental: 3000-tick rounds; timings scaled again. Hard to play by keyboard one action per tick |

Run one with `-config tactical`. `tickMs` accepts 20 to 5000. The 20 ms floor is a hard limit for the server, not a recommendation: below 50 ms the per-connection message limit scales with the tick (see Operations) and the game is not intended for play there.

## Controls

| Key | Action |
| --- | --- |
| W, E, D, C, S, Z, A, Q | Move N, NE, E, SE, S, SW, W, NW |
| Arrow keys | Throw N, E, S, W; two arrows together throw diagonally |
| Shift + arrow keys | Place a wall in that direction |
| Click on the map | Throw toward the pointer (octant from your cell); Shift + click places a wall |
| F then a direction key | Throw in that direction |
| B then a direction key | Place a wall in that direction (defenders, inside their own base) |
| Space or X | Stay |
| Escape | Cancel a pending F or B |
| V | Switch between the tilted (default) and flat graphical views |
| T | Toggle the ASCII view (returns to the last graphical view) |

Holding a key or the mouse button repeats the action every tick (a throw beats a move). One action counts per tick; the last one sent before the tick wins.

## Tests

```
go test ./...
cd web && npm test
```

## Bot load harness

`scripts/bots.go` opens N simulated players against a running server. Each bot picks a role (about 70% attackers), plays one action per tick, reconnects with its token if dropped, and prints a summary every 10 seconds and at the end: bots connected, frames per second, intents sent, captures and tags seen, the share of bot samples standing on the safe line (`onSafeLine`), and the gaps between state frames as seen by a sample of 10 bots.

```
go run -tags bots ./scripts -n 200 -addr ws://127.0.0.1:8080/ws -duration 90s
go test -tags bots ./scripts
```

The server logs a line when a tick takes over 100 ms (at most one per second) and a timing summary (ticks, mean, p95, max) at the end of every round and on shutdown.

## Operations

- Connection caps: at most `-max-conns` open WebSockets (default 4 x `maxPerTeam`, at least 50) and `-max-conns-per-ip` per remote IP (default 20, taken from the socket address; `X-Forwarded-For` is not trusted, so behind a reverse proxy every client counts as the proxy's IP and you should raise it). Over the limit the upgrade is refused with HTTP 503 (full) or 429 (per IP). The bot harness runs all its clients from one IP, so start the server with a higher `-max-conns-per-ip` for it.
- A socket that has not sent a valid `hello` within 10 s is closed. The server pings every 30 s and closes a connection that does not answer within 10 s.
- Message limit: each connection may send `max(20, ceil(3000 / tickMs))` messages per second (burst the same); exceeding it closes the connection.
- Factions: one new faction per player per 5 s, and at most 500 live factions.
- A flag carrier who disconnects drops the flag where they stood immediately; their slot is kept for the 30 s reconnect grace but the flag is not given back.
- The browser keeps the session token per tab (sessionStorage); the player name is shared across tabs. A second connection with the same token replaces the first, and the replaced tab shows a Reconnect button instead of reconnecting by itself.
- If `board.json` cannot be read at startup the server logs the error and runs with an empty leaderboard stored at `board.json.recovered`; the original file is never overwritten. A corrupt file that can be moved aside is renamed to `board.json.bad` as before.
- At startup the server generates a trial map and exits with an error if the chosen config cannot produce one (for example a map too narrow for its bases).

## Status

The emoji, tilted and flat canvas rendering has not been visually reviewed by the author or the build agents; it is only unit-tested. Game balance is untested. The bot harness is a load tool, not a balance test. Attackers follow a BFS path around obstacles; defenders hover by their flag. In a 120 s run of 40 bots on the `tactical` preset they tagged each other steadily and spent about 1% of their time on the safe line, but completed no captures.
