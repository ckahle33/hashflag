# Hashflag: design

## Intent

Hashflag is a browser-based capture the flag game drawn entirely in ASCII characters, built for very large teams (up to 100 per side, 200 total). The experience to aim for is waves of 20 to 40 players rushing a flag together, defenders improvising walls and holding ground, and slow thrown projectiles that are easy to dodge one-on-one and hard to dodge in a crowd.

It is a strategy game, not a reflex game. Play is turn-based on a fixed one-second tick, which also keeps the netcode small enough for one server to host 200 players.

The whole thing is deliberately simple: one global game room and no accounts. Players choose attacker or defender and organize themselves into lightweight factions: squads with goofy auto-generated names that give the crowd identity and a leaderboard to climb. Short rounds (about five minutes) make "one more round" easy. The pull back is belonging to a squad and watching it climb.

## Goals

- Playable in a browser from one link: pick a name, pick a role, play. No accounts.
- One global room. 100 players per team on one server, one-second ticks, with comfortable headroom.
- Obstacles on the map so the flag cannot be reached in a straight rush.
- Balance is the key to replayability, so every gameplay number is a tweakable parameter, and custom game types are supported (see Config).
- The server records per-round stats (captures, tags per player, average flag carry time, wall counts) so balance can be tuned from data.

## Non-goals

- Faction chat, ranks, player-chosen faction names, or faction-vs-faction matchmaking.
- Per-player lifetime stats or progression.
- Multiple rooms, private matches, or room links.
- Real-time or twitch gameplay. No client prediction, no interpolation.
- Accounts, passwords, or email. Identity is a name stored in the browser.
- Graphics beyond monospace text.
- Mobile-optimized controls (works if it works; not a goal for v1).
- Anti-cheat beyond server authority (the client only sends intents).
- A stats viewing UI.

## Anti-goals

Failure even if technically working: a stalemate where defenders wall off the flag and nobody can win; a game where one role is obviously dominant; a round where most of the 200 players have nothing to do; a game that feels laggy because it waits on the slowest client.

## Game rules (decided unless marked as a guess)

**Tick.** Fixed 1000 ms. Each tick the server takes the latest intent each player queued (move, throw, place wall, or stay), resolves all of them together, and broadcasts the new state. A player who misses a tick simply stays put.

**Map.** A character grid split by a center safe line. Each side has a base and a flag. Obstacles block movement and projectiles. Maps are either generated or authored as ASCII rows (see Config).

**Safe line.** Players standing on it cannot be tagged. Whether they may throw from it is configurable; default: no. (Guess.)

**Roles.** Each player picks attacker or defender. Defenders start near their own flag and are intended to stay close. Attackers start further back and push in waves. Roles can differ in stats via config. A team may not exceed a configurable share on one role (default 60%; my call).

**Teams.** Players without a faction are assigned a side automatically to keep sides balanced (my call). Players in a faction are on that faction's side (see Factions). The balance cap still applies, so a full side blocks joining.

**Throwing.** A projectile travels in a straight line, 2 cells per tick by default, and tags the first player in its path. It checks every cell it passes through, so fast projectiles cannot skip over players or walls (my call). Short cooldown (default 2 ticks).

**Ammo.** Each player carries 10 (decided). Ammo refills by a fixed amount per tick while standing in the player's own base (decided).

**Tagging.** A tagged player is out for a few ticks (default 10) and respawns at their own base. A flag carrier who is tagged drops the flag where they stood.

**Flag.** An attacker who reaches the enemy flag picks it up and must carry it home to capture. A dropped flag returns home after a timeout (default 30 ticks; guess). The carrier cannot throw by default (guess).

**Walls (base defense).** Defenders place wall cells, shown as `|` or similar, in their own base. Decided: walls are permanent (no decay), break in one hit, each defender may have one placed at a time, and the team has a cap of 20 active walls; breaking a wall frees a slot. The server rejects any placement that would completely seal off the flag, so an attack path always exists from the safe line to the flag, counting existing walls as blockers (my call; strongly recommended).

**Movement.** Multi-cell movement cannot pass through obstacles, walls, or other players (my call).

**Round and win.** About 300 ticks. Winner is the team with the most captures at time, or first to N captures, per config (guess at default). Rounds run back to back on a timer. Players can join mid-round; if both sides are full, they wait in a lobby view (my call).

## Factions

A faction is a squad on one side. Any player can start one with a single action. The server generates its name (a goofy adjective-noun style, e.g. "Damp Pigeons"); there is no free-text naming, so no moderation is needed. The founder may reroll the name while the faction has only one member (my call). Other players join from a list in the lobby view and may leave at any time. A faction ceases to exist when its last member leaves (my call). Joining a faction puts the player on its side, subject to the balance cap.

Each capture credits the capturer's faction at that moment. A leaderboard shows top factions for the current round and all-time. The all-time leaderboard persists across server restarts in a small JSON file on disk (my call; no database in v1). A player who switches factions leaves old captures with the old faction (my call). Because factions vanish when empty, the all-time board should keep entries for dissolved factions so history is not lost (my call).

Display: each player is drawn as the first letter of their faction's name, with a distinct color per faction when possible, so squads are visible in a crowd (guess; builder may adjust, e.g. if letters collide).

Factions have no chat, ranks, or matchmaking. Out of scope for v1.

## Identity

A player enters a name, stored in browser local storage. Names are not unique (my call). Each browser tab gets a session token, so a reconnecting player returns to their slot mid-round (my call). Names should be length-limited and lightly filtered.

## Config

All gameplay numbers live in a JSON game config with hard minimum and maximum bounds so a bad config cannot crash or hang the server. A custom game type is a config file with `extends` naming another preset plus the fields it overrides (deep merge, chaining allowed with cycle detection). Roles may override movement, projectile, ammo, and wall settings.

Custom configs are files on the server chosen by the operator; players cannot upload them (my call).

The draft JSON Schema is in `docs/superpowers/specs/hashflag-config.schema.json`. Parameters covered: tick length, round length, team size, win mode, map size and obstacle generation, safe line width, base size, movement speed and diagonals, projectile speed, range, cooldown and pierce, ammo max and refill, tag-out duration, flag carrier rules and auto-return, wall rules, role overrides, and team balance.

For v1, the full schema is validated, but only the core fields must be implemented; unimplemented fields are rejected with a clear error until they exist (my call). Core: tick, round length, team size, win mode, map size and scatter obstacles, safe line, base size, movement, projectile, ammo, tag, flag, walls, team balance. Ship at least a `standard` preset.

## Stats

The server appends per-round stats (captures, tags per player, flag carry times, wall counts, captures per faction) as JSON lines to a file for later analysis (my call). No database in v1, and no stats viewing UI beyond the in-game faction leaderboard.

## Technology (my calls, not agreed except Go and React)

- Server: Go. Decided by the user.
- Client: React (decided: React or similar); my call: Vite and TypeScript, grid rendered as monospace text.
- Transport: WebSockets. My call.
- Repo location: `~/src/hashflag`.

## Left to the builder

Project layout, server architecture and concurrency model, message formats, how conflicts resolve on a tick (two players into one cell, throw versus dodge), the obstacle generators, UI layout and key bindings, the lobby view, and how the first presets are balanced. Conflict rules should be simple, fixed, and documented.
