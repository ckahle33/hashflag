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
- `map.obstacles.generate`: `"clusters"` (default) places large irregular blobs covering about `density` (default 0.25) of the open field, with `clusterSize` (default 8) as the mean blob radius, and always puts one across the straight lane to each flag. `"scatter"` sprinkles single cells with probability `density`.

Some schema fields are not implemented yet (for example explicit obstacle rows, the `mazeish` and `chokepoints` generators, and non-default `respawnAt`, `refillWhere` and `placeZone` values). The server rejects a config that sets them, with an error naming the field, rather than silently ignoring them.

## Presets

| Preset | Tick | Notes |
| --- | --- | --- |
| `standard` | 1000 ms | The built-in defaults: 300-tick rounds |
| `tactical` | 300 ms | 1000-tick rounds (5 minutes); out time, safe-line limit and cooldowns scaled to the faster tick |
| `realtime` | 100 ms | Experimental: 3000-tick rounds; timings scaled again. Hard to play by keyboard one action per tick |

Run one with `-config tactical`. `tickMs` accepts 20 to 5000.

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

## Status

The canvas look has not been tuned and game balance is untested. The bot harness is a load tool, not a balance test. Attackers follow a BFS path around obstacles; defenders hover by their flag. In a 120 s run of 40 bots on the `tactical` preset they tagged each other steadily and spent about 1% of their time on the safe line, but completed no captures.
