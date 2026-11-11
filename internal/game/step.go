package game

import (
	"fmt"
	"sort"

	"hashflag/internal/config"
)

// World is the full mutable game state for one round.
type World struct {
	Cfg         *config.Config
	Map         *Map
	Players     map[PlayerID]*Player
	Projectiles []Projectile
	Walls       map[Pos]Wall
	Flags       [2]Flag // indexed by the team that owns the flag
	Tick        int
	scores      [2]int // captures per team; read via Scores()

	order     []PlayerID   // sorted player IDs; all ordered iteration uses this
	stats     [2]roleStats // effective per-role stats, indexed by Role
	stackable bool         // overlapping allowed (both roles must allow it)
}

// roleStats are the global config sections with a role's overrides applied.
type roleStats struct {
	move config.Movement
	proj config.Projectile
	ammo config.Ammo
}

// NewWorld generates the map and returns an empty world.
func NewWorld(cfg *config.Config, seed int64) (*World, error) {
	if cfg == nil {
		return nil, fmt.Errorf("game: nil config")
	}
	m, err := GenerateMap(cfg.Map, seed)
	if err != nil {
		return nil, err
	}
	w := &World{
		Cfg:     cfg,
		Map:     m,
		Players: map[PlayerID]*Player{},
		Walls:   map[Pos]Wall{},
	}
	w.Flags[TeamA] = Flag{Team: TeamA, Home: m.FlagA, Pos: m.FlagA}
	w.Flags[TeamB] = Flag{Team: TeamB, Home: m.FlagB, Pos: m.FlagB}
	w.stats[Attacker] = resolveRole(cfg, cfg.Roles.Attacker)
	w.stats[Defender] = resolveRole(cfg, cfg.Roles.Defender)
	w.stackable = w.stats[Attacker].move.Stackable && w.stats[Defender].move.Stackable
	return w, nil
}

func resolveRole(cfg *config.Config, o *config.RoleOverride) roleStats {
	s := roleStats{move: cfg.Movement, proj: cfg.Projectile, ammo: cfg.Ammo}
	if o == nil {
		return s
	}
	if m := o.Movement; m != nil {
		setInt(&s.move.CellsPerTick, m.CellsPerTick)
		setBool(&s.move.Diagonals, m.Diagonals)
		setBool(&s.move.Stackable, m.Stackable)
	}
	if p := o.Projectile; p != nil {
		setInt(&s.proj.CellsPerTick, p.CellsPerTick)
		setInt(&s.proj.RangeCells, p.RangeCells)
		setInt(&s.proj.CooldownTicks, p.CooldownTicks)
		setBool(&s.proj.Pierce, p.Pierce)
		setBool(&s.proj.FriendlyFire, p.FriendlyFire)
	}
	if a := o.Ammo; a != nil {
		setInt(&s.ammo.Max, a.Max)
		setInt(&s.ammo.RefillPerTick, a.RefillPerTick)
		if a.RefillWhere != nil {
			s.ammo.RefillWhere = *a.RefillWhere
		}
	}
	return s
}

func setInt(dst, src *int) {
	if src != nil {
		*dst = *src
	}
}

func setBool(dst, src *bool) {
	if src != nil {
		*dst = *src
	}
}

func (w *World) statsFor(p *Player) *roleStats {
	if p.Role == Defender {
		return &w.stats[Defender]
	}
	return &w.stats[Attacker]
}

// AddPlayer places a copy of p at its spawn (defenders near their own flag,
// attackers at the base edge nearest the safe line) with full ammo. It
// returns nil if the ID is taken or the team's base has no free cell.
func (w *World) AddPlayer(p Player) *Player {
	if _, ok := w.Players[p.ID]; ok {
		return nil
	}
	pos, ok := w.spawnPos(p.Team, p.Role)
	if !ok {
		return nil
	}
	np := p
	np.Pos = pos
	np.Ammo = w.statsFor(&np).ammo.Max
	w.Players[np.ID] = &np
	i := sort.Search(len(w.order), func(i int) bool { return w.order[i] >= np.ID })
	w.order = append(w.order, "")
	copy(w.order[i+1:], w.order[i:])
	w.order[i] = np.ID
	return &np
}

// RemovePlayer deletes a player. Its projectiles stay in flight and its
// walls stay standing.
func (w *World) RemovePlayer(id PlayerID) {
	if _, ok := w.Players[id]; !ok {
		return
	}
	w.dropCarried(w.Players[id])
	delete(w.Players, id)
	i := sort.Search(len(w.order), func(i int) bool { return w.order[i] >= id })
	w.order = append(w.order[:i], w.order[i+1:]...)
}

func (w *World) base(t Team) Rect {
	if t == TeamB {
		return w.Map.BaseB
	}
	return w.Map.BaseA
}

// spawnPos finds a spawn cell for a player of team t and role r.
//
// First it picks the free cell of t's base nearest the role's anchor (own
// flag for defenders, the base edge facing the safe line for attackers) by
// Chebyshev distance. Scanning the whole base this way covers both "near
// the anchor" and the fallback "nearest free cell in the base".
//
// If the base is full it spills to the free cell on t's own side of the map
// (never on or past the safe line) nearest the base rectangle by Chebyshev
// distance. It returns false only when the whole own side is full.
//
// Free means: in bounds, no obstacle, no wall, no flag (home or current
// position of either flag), no player who is in. Ties are broken by y then
// x (scan order with a strict < comparison).
func (w *World) spawnPos(t Team, r Role) (Pos, bool) {
	b := w.base(t)
	anchor := w.Flags[t].Home
	if r != Defender {
		if t == TeamA {
			anchor.X = b.X1
		} else {
			anchor.X = b.X0
		}
	}
	taken := make(map[Pos]bool, len(w.Players)+4)
	for _, p := range w.Players {
		if !p.Out() {
			taken[p.Pos] = true
		}
	}
	for _, f := range w.Flags {
		taken[f.Home] = true
		taken[f.Pos] = true
	}
	free := func(c Pos) bool { return !taken[c] && !w.blocked(c) }

	if c, ok := nearestFree(b, free, func(c Pos) int { return chebyshev(c, anchor) }); ok {
		return c, true
	}
	side := Rect{0, 0, w.Map.SafeX0 - 1, w.Map.Height - 1}
	if t == TeamB {
		side = Rect{w.Map.SafeX1 + 1, 0, w.Map.Width - 1, w.Map.Height - 1}
	}
	return nearestFree(side, free, func(c Pos) int { return rectDist(c, b) })
}

// nearestFree returns the free cell in r with the smallest dist, ties
// broken by y then x.
func nearestFree(r Rect, free func(Pos) bool, dist func(Pos) int) (Pos, bool) {
	best, bestD, found := Pos{}, 0, false
	for y := r.Y0; y <= r.Y1; y++ {
		for x := r.X0; x <= r.X1; x++ {
			c := Pos{x, y}
			if !free(c) {
				continue
			}
			if d := dist(c); !found || d < bestD {
				best, bestD, found = c, d, true
			}
		}
	}
	return best, found
}

func chebyshev(a, b Pos) int {
	return max(abs(a.X-b.X), abs(a.Y-b.Y))
}

// rectDist is the Chebyshev distance from p to the nearest cell of r.
func rectDist(p Pos, r Rect) int {
	return max(r.X0-p.X, p.X-r.X1, r.Y0-p.Y, p.Y-r.Y1, 0)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (w *World) inBounds(p Pos) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < w.Map.Width && p.Y < w.Map.Height
}

// blocked reports a static movement blocker: edge, obstacle, or wall.
func (w *World) blocked(p Pos) bool {
	if !w.inBounds(p) || w.Map.Obstacle[p.Y][p.X] {
		return true
	}
	_, wall := w.Walls[p]
	return wall
}

func (w *World) onSafeLine(p Pos) bool {
	return p.X >= w.Map.SafeX0 && p.X <= w.Map.SafeX1
}

// Step resolves one tick and returns the events it produced. Order is fixed:
//
//  1. Decrement timers (cooldowns, out-time); out players whose time reaches
//     0 respawn at their own base with full ammo.
//  2. Resolve moves, cell by cell, all movers in lock-step.
//  3. Launch throws; a projectile spawns at the thrower's cell.
//  4. Advance every projectile cell by cell, including ones thrown this
//     tick, breaking walls and recording hits.
//  5. Apply tags.
//  6. Wall placements (Task 5), then flag logic (Task 4).
//  7. Ammo refill for players in their own base.
//  8. Tick++.
//
// Out players' intents are ignored. Results never depend on map iteration
// order: players are processed in sorted ID order and projectiles in slice
// (launch) order.
func (w *World) Step(intents map[PlayerID]Intent) []Event {
	var events []Event
	w.decrementTimers()
	w.resolveMoves(intents)
	w.launchThrows(intents)
	hits := w.advanceProjectiles(&events)
	w.applyTags(hits, &events)
	w.resolvePlacements(intents, &events)
	w.resolveFlags(&events)
	w.refillAmmo()
	w.Tick++
	return events
}

// decrementTimers is step 1.
func (w *World) decrementTimers() {
	for _, id := range w.order {
		p := w.Players[id]
		if p.CooldownLeft > 0 {
			p.CooldownLeft--
		}
		if p.OutLeft > 0 {
			p.OutLeft--
			if p.OutLeft == 0 {
				w.respawn(p)
			}
		}
	}
}

func (w *World) respawn(p *Player) {
	pos, ok := w.spawnPos(p.Team, p.Role)
	if !ok {
		p.OutLeft = 1 // base full; retry next tick
		return
	}
	p.Pos = pos
	p.Ammo = w.statsFor(p).ammo.Max
	p.CooldownLeft = 0
}

// mover is a player taking part in move resolution this tick.
type mover struct {
	p      *Player
	delta  Pos
	speed  int
	active bool
	target Pos
	ok     bool
}

// resolveMoves is step 2. Every mover advances one cell per sub-step, up to
// its speed. In each sub-step a proposal is rejected if its target is an
// edge, obstacle or wall; is targeted by another mover; would swap with
// another mover; or is occupied by a player who is not successfully leaving
// it. A rejected mover stops for the rest of the tick. When stacking is
// allowed only the static blockers apply.
func (w *World) resolveMoves(intents map[PlayerID]Intent) {
	if len(intents) == 0 {
		return
	}
	var ms []*mover
	maxSpeed := 0
	for _, id := range w.order {
		in, ok := intents[id]
		if !ok || in.Kind != Move || in.Dir < 0 || in.Dir > 7 {
			continue
		}
		p := w.Players[id]
		if p.Out() {
			continue
		}
		st := w.statsFor(p)
		if in.Dir%2 == 1 && !st.move.Diagonals {
			continue
		}
		speed := st.move.CellsPerTick
		if p.HasFlag {
			speed = w.Cfg.Flag.CarrierCellsPerTick
		}
		ms = append(ms, &mover{p: p, delta: in.Dir.Delta(), speed: speed, active: true})
		maxSpeed = max(maxSpeed, speed)
	}
	for step := 1; step <= maxSpeed; step++ {
		var cur []*mover
		for _, m := range ms {
			if !m.active {
				continue
			}
			m.target = Pos{m.p.Pos.X + m.delta.X, m.p.Pos.Y + m.delta.Y}
			if m.speed < step || w.blocked(m.target) {
				m.active = false
				continue
			}
			m.ok = true
			cur = append(cur, m)
		}
		if len(cur) == 0 {
			return
		}
		if !w.stackable {
			w.rejectConflicts(cur)
		}
		for _, m := range cur {
			if m.ok {
				m.p.Pos = m.target
			} else {
				m.active = false
			}
		}
	}
}

// rejectConflicts clears ok on proposals that are contested, swaps, or
// enter a cell whose occupant stays.
func (w *World) rejectConflicts(cur []*mover) {
	targets := make(map[Pos]int, len(cur))
	from := make(map[Pos]*mover, len(cur))
	for _, m := range cur {
		targets[m.target]++
		from[m.p.Pos] = m
	}
	for _, m := range cur {
		if targets[m.target] > 1 {
			m.ok = false
		} else if o := from[m.target]; o != nil && o.target == m.p.Pos {
			m.ok = false
		}
	}
	occ := make(map[Pos]int, len(w.order))
	for _, id := range w.order {
		if p := w.Players[id]; !p.Out() {
			occ[p.Pos]++
		}
	}
	// A target is enterable only if every occupant is a mover whose own
	// move succeeds. Rejections cascade along chains; iterate to a fixed
	// point.
	for changed := true; changed; {
		changed = false
		leaving := make(map[Pos]int, len(cur))
		for _, m := range cur {
			if m.ok {
				leaving[m.p.Pos]++
			}
		}
		for _, m := range cur {
			if m.ok && occ[m.target] > leaving[m.target] {
				m.ok = false
				changed = true
			}
		}
	}
}

// launchThrows is step 3. A throw needs ammo, no cooldown, the thrower off
// the safe line, and (by default) not carrying a flag.
func (w *World) launchThrows(intents map[PlayerID]Intent) {
	if len(intents) == 0 {
		return
	}
	for _, id := range w.order {
		in, ok := intents[id]
		if !ok || in.Kind != Throw || in.Dir < 0 || in.Dir > 7 {
			continue
		}
		p := w.Players[id]
		if p.Out() || p.Ammo <= 0 || p.CooldownLeft > 0 || w.onSafeLine(p.Pos) {
			continue
		}
		if p.HasFlag && !w.Cfg.Flag.CarrierCanThrow {
			continue
		}
		st := w.statsFor(p).proj
		p.Ammo--
		p.CooldownLeft = st.CooldownTicks
		w.Projectiles = append(w.Projectiles, Projectile{
			Pos: p.Pos, Dir: in.Dir, Owner: p.ID, Team: p.Team,
			speed: st.CellsPerTick, rangeCells: st.RangeCells,
			pierce: st.Pierce, friendlyFire: st.FriendlyFire,
		})
	}
}

// hit records that a projectile from owner struck victim.
type hit struct{ victim, owner PlayerID }

// advanceProjectiles is step 4. Projectiles advance in slice order. Each
// checks every cell it enters: it vanishes at an edge or obstacle; deals one
// hit to a wall and vanishes; hits eligible players in the cell (enemies, or
// anyone but the thrower with friendly fire; players on the safe line are
// passed over) and vanishes unless piercing; and vanishes on reaching its
// range. A non-piercing projectile hits only the first eligible player in a
// cell (lowest PlayerID). An in-flight projectile (Traveled > 0) first
// checks its starting cell, since players moved earlier this tick may have
// stepped onto it.
func (w *World) advanceProjectiles(events *[]Event) []hit {
	if len(w.Projectiles) == 0 {
		return nil
	}
	at := make(map[Pos][]*Player, len(w.order))
	for _, id := range w.order {
		if p := w.Players[id]; !p.Out() {
			at[p.Pos] = append(at[p.Pos], p)
		}
	}
	safe := w.Cfg.Tag.SafeOnSafeLine
	var hits []hit
	kept := w.Projectiles[:0]
	for _, pr := range w.Projectiles {
		speed, rng, pierce, ff := pr.speed, pr.rangeCells, pr.pierce, pr.friendlyFire
		if speed == 0 {
			g := w.Cfg.Projectile
			speed, rng, pierce, ff = g.CellsPerTick, g.RangeCells, g.Pierce, g.FriendlyFire
		}
		// hitAt records hits on eligible players at c (at[c] is in sorted ID
		// order) and reports whether the projectile survives.
		hitAt := func(c Pos) bool {
			if safe && w.onSafeLine(c) {
				return true
			}
			for _, v := range at[c] {
				if v.ID == pr.Owner || (v.Team == pr.Team && !ff) {
					continue
				}
				hits = append(hits, hit{victim: v.ID, owner: pr.Owner})
				if !pierce {
					return false
				}
			}
			return true
		}
		d := pr.Dir.Delta()
		alive := pr.Traveled == 0 || hitAt(pr.Pos)
		for i := 0; i < speed && alive; i++ {
			next := Pos{pr.Pos.X + d.X, pr.Pos.Y + d.Y}
			if !w.inBounds(next) || w.Map.Obstacle[next.Y][next.X] {
				alive = false
				break
			}
			pr.Pos = next
			pr.Traveled++
			if wall, ok := w.Walls[next]; ok {
				w.hitWall(next, wall, pr.Owner, events)
				alive = false
				break
			}
			alive = hitAt(next)
			if pr.Traveled >= rng {
				alive = false
			}
		}
		if alive {
			kept = append(kept, pr)
		}
	}
	clear(w.Projectiles[len(kept):])
	w.Projectiles = kept
	return hits
}

// hitWall takes one hit off the wall at pos. The wall is removed (freeing
// its owner's slot and emitting WallBroken) only when no hits remain.
func (w *World) hitWall(pos Pos, wall Wall, by PlayerID, events *[]Event) {
	wall.Hits--
	if wall.Hits > 0 {
		w.Walls[pos] = wall
		return
	}
	delete(w.Walls, pos)
	if o := w.Players[wall.Owner]; o != nil && o.WallsPlaced > 0 {
		o.WallsPlaced--
	}
	*events = append(*events, Event{Tick: w.Tick, Kind: WallBroken, Player: wall.Owner, Other: by, Team: wall.Team, Pos: pos})
}

// applyTags is step 5. Each victim is tagged once; the first hit in
// projectile order gets the credit. Flag drops for tagged carriers are
// Task 4's job (resolveFlags sees Out() && HasFlag).
func (w *World) applyTags(hits []hit, events *[]Event) {
	for _, h := range hits {
		v := w.Players[h.victim]
		if v == nil || v.Out() {
			continue
		}
		v.OutLeft = w.Cfg.Tag.OutTicks
		*events = append(*events, Event{Tick: w.Tick, Kind: Tagged, Player: v.ID, Other: h.owner, Team: v.Team, Pos: v.Pos})
	}
}

// resolvePlacements is step 6a: wall placement (Task 5).
func (w *World) resolvePlacements(intents map[PlayerID]Intent, events *[]Event) {}

// refillAmmo is step 7.
func (w *World) refillAmmo() {
	for _, id := range w.order {
		p := w.Players[id]
		if p.Out() || !w.base(p.Team).Contains(p.Pos) {
			continue
		}
		a := w.statsFor(p).ammo
		p.Ammo = min(a.Max, p.Ammo+a.RefillPerTick)
	}
}
