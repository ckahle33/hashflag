package game

import "hashflag/internal/config"

// wallsFor returns the walls config with the player's role overrides applied.
func (w *World) wallsFor(p *Player) config.Walls {
	c := w.Cfg.Walls
	var o *config.RoleOverride
	if p.Role == Defender {
		o = w.Cfg.Roles.Defender
	} else {
		o = w.Cfg.Roles.Attacker
	}
	if o == nil || o.Walls == nil {
		return c
	}
	setBool(&c.Enabled, o.Walls.Enabled)
	setInt(&c.PerPlayer, o.Walls.PerPlayer)
	setInt(&c.TeamCap, o.Walls.TeamCap)
	setInt(&c.HitsToBreak, o.Walls.HitsToBreak)
	setInt(&c.PlaceRangeCells, o.Walls.PlaceRangeCells)
	setBool(&c.MustLeavePath, o.Walls.MustLeavePath)
	return c
}

// resolvePlacements is step 6a. Placements are processed in ascending
// PlayerID order, each validated against walls added earlier this tick.
// Invalid placements are silently ignored.
func (w *World) resolvePlacements(intents map[PlayerID]Intent, events *[]Event) {
	if len(intents) == 0 {
		return
	}
	for _, id := range w.order {
		in, ok := intents[id]
		if !ok || in.Kind != PlaceWall || in.Dir < 0 || in.Dir > 7 {
			continue
		}
		p := w.Players[id]
		if p.Role != Defender || p.Out() {
			continue
		}
		cfg := w.wallsFor(p)
		if !cfg.Enabled || p.WallsPlaced >= cfg.PerPlayer {
			continue
		}
		d := in.Dir.Delta()
		target := Pos{p.Pos.X + d.X, p.Pos.Y + d.Y}
		if !w.base(p.Team).Contains(target) || !w.cellFreeForWall(target) {
			continue
		}
		if w.teamWalls(p.Team) >= cfg.TeamCap {
			continue
		}
		if cfg.MustLeavePath && w.sealsFlag(p.Team, target) {
			continue
		}
		w.Walls[target] = Wall{Owner: id, Team: p.Team, Hits: cfg.HitsToBreak}
		p.WallsPlaced++
		*events = append(*events, Event{Tick: w.Tick, Kind: WallPlaced, Player: id, Team: p.Team, Pos: target})
	}
}

// cellFreeForWall reports whether nothing occupies c: it must be in bounds,
// with no obstacle, wall, flag (home or current position) or in-play player.
func (w *World) cellFreeForWall(c Pos) bool {
	if w.blocked(c) {
		return false
	}
	for i := range w.Flags {
		if w.Flags[i].Home == c || w.Flags[i].Pos == c {
			return false
		}
	}
	for _, id := range w.order {
		if p := w.Players[id]; !p.Out() && p.Pos == c {
			return false
		}
	}
	return true
}

func (w *World) teamWalls(t Team) int {
	n := 0
	for _, wl := range w.Walls {
		if wl.Team == t {
			n++
		}
	}
	return n
}

// sealsFlag reports whether treating cell as blocked leaves no path from the
// safe-line cells to team's flag Home. Obstacles, walls, cell and the edge
// block; players do not. Movement (resolveMoves) only checks the destination
// cell, so a diagonal step between two blocked orthogonal neighbours is
// legal ("corner cutting" allowed); the 8-neighbour BFS matches that.
func (w *World) sealsFlag(team Team, cell Pos) bool {
	goal := w.Flags[team].Home
	if goal == cell {
		return true
	}
	closed := func(p Pos) bool { return p == cell || w.blocked(p) }
	seen := map[Pos]bool{}
	var queue []Pos
	for x := w.Map.SafeX0; x <= w.Map.SafeX1; x++ {
		for y := 0; y < w.Map.Height; y++ {
			p := Pos{x, y}
			if !closed(p) {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == goal {
			return false
		}
		for _, d := range dirDeltas {
			n := Pos{cur.X + d.X, cur.Y + d.Y}
			if !seen[n] && !closed(n) {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return !seen[goal]
}
