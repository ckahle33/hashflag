//go:build bots

package main

import "math/rand"

type pos struct{ x, y int }

// deltas are the 8 directions, clockwise from north (matches the server).
var deltas = [8]pos{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func dirOf(dx, dy int) int {
	for i, d := range deltas {
		if d.x == dx && d.y == dy {
			return i
		}
	}
	return 0
}

// stepToward is the greedy 8-direction step from one cell toward another.
func stepToward(from, to pos) (int, bool) {
	dx, dy := sign(to.x-from.x), sign(to.y-from.y)
	if dx == 0 && dy == 0 {
		return 0, false
	}
	return dirOf(dx, dy), true
}

// aim returns the direction to throw at "to" when it lies on a straight or
// diagonal line from "from" within rng cells.
func aim(from, to pos, rng int) (int, bool) {
	dx, dy := to.x-from.x, to.y-from.y
	dist := max(abs(dx), abs(dy))
	if dist == 0 || dist > rng || (dx != 0 && dy != 0 && abs(dx) != abs(dy)) {
		return 0, false
	}
	return dirOf(sign(dx), sign(dy)), true
}

// pickMove steps toward target; if that cell is blocked it sidesteps to a
// random free neighbouring direction. With probability noise it moves
// randomly instead. ok is false when no free move exists.
func pickMove(rng *rand.Rand, from, target pos, blocked func(pos) bool, noise float64) (int, bool) {
	free := func(d int) bool { return !blocked(pos{from.x + deltas[d].x, from.y + deltas[d].y}) }
	var opts []int
	if rng.Float64() >= noise {
		if d, ok := stepToward(from, target); ok {
			if free(d) {
				return d, true
			}
			// sidestep without turning back: 45 or 90 degrees off the direct line
			for _, off := range []int{1, 7, 2, 6} {
				if nd := (d + off) % 8; free(nd) {
					opts = append(opts, nd)
				}
			}
			if len(opts) > 0 {
				return opts[rng.Intn(len(opts))], true
			}
		}
	}
	for d := 0; d < 8; d++ {
		if free(d) {
			opts = append(opts, d)
		}
	}
	if len(opts) == 0 {
		return 0, false
	}
	return opts[rng.Intn(len(opts))], true
}

// distField returns the 8-neighbour BFS distance from every cell to target
// through non-'#' cells of rows, indexed y*width+x; -1 means unreachable.
func distField(rows []string, target pos) []int {
	h := len(rows)
	if h == 0 {
		return nil
	}
	w := len(rows[0])
	dist := make([]int, w*h)
	for i := range dist {
		dist[i] = -1
	}
	if target.x < 0 || target.y < 0 || target.x >= w || target.y >= h {
		return dist
	}
	dist[target.y*w+target.x] = 0
	q := []pos{target}
	for i := 0; i < len(q); i++ {
		p := q[i]
		for _, d := range deltas {
			n := pos{p.x + d.x, p.y + d.y}
			if n.x < 0 || n.y < 0 || n.x >= w || n.y >= h || len(rows[n.y]) != w || rows[n.y][n.x] == '#' || dist[n.y*w+n.x] >= 0 {
				continue
			}
			dist[n.y*w+n.x] = dist[p.y*w+p.x] + 1
			q = append(q, n)
		}
	}
	return dist
}

// downhill picks a random free neighbour with the lowest distance below the
// current cell's, or failing that one at the same distance (to slide around
// a player or wall in the way). ok is false on the target, on an
// unreachable cell, or when no such neighbour is free.
func downhill(rng *rand.Rand, dist []int, w int, from pos, blocked func(pos) bool) (int, bool) {
	at := func(p pos) int {
		if p.x < 0 || p.y < 0 || p.x >= w || p.y*w+p.x >= len(dist) {
			return -1
		}
		return dist[p.y*w+p.x]
	}
	cur := at(from)
	if cur <= 0 {
		return 0, false
	}
	best, opts := cur+1, []int(nil)
	for d := range deltas {
		n := pos{from.x + deltas[d].x, from.y + deltas[d].y}
		v := at(n)
		if v < 0 || v > cur || blocked(n) {
			continue
		}
		if v < best {
			best, opts = v, opts[:0]
		}
		if v == best {
			opts = append(opts, d)
		}
	}
	if len(opts) == 0 {
		return 0, false
	}
	return opts[rng.Intn(len(opts))], true
}

// stuckState detects a bot whose move intents are having no effect.
type stuckState struct {
	last      pos
	same      int // consecutive frames at last while moving
	dir, left int // active sidestep direction and ticks remaining after this one
}

// step is called once per state frame with the bot's position and whether
// its previous intent was a move in direction lastDir. After 3 consecutive
// frames at an unchanged position while moving, it starts a sidestep
// perpendicular to lastDir (randomly left or right) lasting 2 ticks, and
// reports that direction while the sidestep is active.
func (s *stuckState) step(rng *rand.Rand, me pos, moving bool, lastDir int) (int, bool) {
	defer func() { s.last = me }()
	if s.left > 0 {
		s.left--
		s.same = 0
		return s.dir, true
	}
	if moving && me == s.last {
		s.same++
	} else {
		s.same = 0
	}
	if s.same < 3 {
		return 0, false
	}
	s.same, s.left = 0, 1
	s.dir = (lastDir + 2 + 4*rng.Intn(2)) % 8
	return s.dir, true
}

func (b *bot) onSafeLine(p pos) bool { return p.x >= b.safeX0 && p.x <= b.safeX1 }

func (b *bot) decide(f *frame) (string, int, bool) {
	var me pos
	found, carrying := false, false
	occupied := map[pos]bool{}
	var enemies []pos
	for _, p := range f.Players {
		at := pos{p.X, p.Y}
		if p.ID == b.id {
			if p.Out {
				return "", 0, false
			}
			me, found, carrying = at, true, p.HasFlag
			continue
		}
		if p.Out {
			continue
		}
		occupied[at] = true
		if p.Team != b.team {
			enemies = append(enemies, at)
		}
	}
	if !found {
		return "", 0, false
	}
	b.m.samples.Add(1)
	onLine := b.onSafeLine(me)
	if onLine {
		b.m.onLine.Add(1)
	}
	for _, w := range f.Walls {
		occupied[pos{w.X, w.Y}] = true
	}
	blocked := func(p pos) bool {
		if p.y < 0 || p.y >= len(b.rows) || p.x < 0 || p.x >= len(b.rows[p.y]) {
			return true
		}
		return b.rows[p.y][p.x] == '#' || occupied[p]
	}
	// throws from the safe line are refused, so do not waste the tick on one
	if b.ammo > 0 && b.cd == 0 && !onLine {
		for _, e := range enemies {
			if d, ok := aim(me, e, 14); ok {
				return "throw", d, true
			}
		}
	}
	if d, ok := b.stuck.step(b.rng, me, b.lastMove, b.lastDir); ok {
		return "move", d, true
	}
	if b.attacker {
		target := b.homeFlag
		if !carrying {
			target = me
			for _, fl := range f.Flags {
				if fl.Team != b.team {
					target = pos{fl.X, fl.Y}
				}
			}
		}
		// follow the BFS distance field around obstacles; a little noise
		// and a random fallback keep bots from jamming each other
		if b.dist == nil || b.distTo != target {
			b.dist, b.distTo = distField(b.rows, target), target
		}
		if b.rng.Float64() >= 0.1 {
			if d, ok := downhill(b.rng, b.dist, len(b.rows[0]), me, blocked); ok {
				return "move", d, true
			}
		}
		d, ok := pickMove(b.rng, me, target, blocked, 1)
		return "move", d, ok
	}
	// defender: hover near the home flag (never on it), now and then wall up the base
	inner := blocked
	blocked = func(p pos) bool { return p == b.homeFlag || inner(p) }
	if max(abs(me.x-b.homeFlag.x), abs(me.y-b.homeFlag.y)) > 3 {
		d, ok := pickMove(b.rng, me, b.homeFlag, blocked, 0)
		return "move", d, ok
	}
	if b.rng.Float64() < 0.1 && b.walls < 3 {
		var opts []int
		for d := 0; d < 8; d++ {
			t := pos{me.x + deltas[d].x, me.y + deltas[d].y}
			if b.base.has(t) && t != b.homeFlag && !blocked(t) {
				opts = append(opts, d)
			}
		}
		if len(opts) > 0 {
			return "wall", opts[b.rng.Intn(len(opts))], true
		}
	}
	if b.rng.Float64() < 0.4 {
		d, ok := pickMove(b.rng, me, b.homeFlag, blocked, 1)
		if ok {
			nx, ny := me.x+deltas[d].x, me.y+deltas[d].y
			if max(abs(nx-b.homeFlag.x), abs(ny-b.homeFlag.y)) <= 3 {
				return "move", d, true
			}
		}
	}
	return "", 0, false
}
