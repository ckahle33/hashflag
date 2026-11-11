package game

import (
	"math"
	"math/rand"
)

const (
	// baseClearance is the ring of cells around each base kept free of
	// cluster obstacles.
	baseClearance = 2
	// safeClearance is the number of columns beside the safe band kept free
	// of cluster obstacles.
	safeClearance = 1
	// minHalfReach is the share of a half's free cells that must be reachable
	// from that half's flag; below it the attempt is rejected.
	minHalfReach = 0.9
	// maxBlobs caps blob placement per attempt.
	maxBlobs = 400
	// overshootTol is how far past the target coverage (as a share of the
	// eligible cells) one blob may push it before it is scaled down.
	overshootTol = 0.04
	// maxCoverageOver rejects an attempt whose final coverage, after pocket
	// filling, exceeds density by more than this.
	maxCoverageOver = 0.07
)

// clusterEligible reports whether a left-half cell may hold a cluster
// obstacle: left of the safe band and its clearance, and outside Team A's
// base and its clearance ring. Right-half cells are never eligible; they are
// filled by mirroring.
func clusterEligible(m *Map) func(Pos) bool {
	ring := Rect{m.BaseA.X0 - baseClearance, m.BaseA.Y0 - baseClearance,
		m.BaseA.X1 + baseClearance, m.BaseA.Y1 + baseClearance}
	return func(p Pos) bool {
		return p.X >= 0 && p.Y >= 0 && p.Y < m.Height &&
			p.X < m.SafeX0-safeClearance && !ring.Contains(p)
	}
}

// clusters builds the "clusters" obstacle layout for one attempt: irregular
// blobs on the left half, mirrored to the right. It returns ok=false when the
// attempt must be retried with another seed (straight lane left open, or a
// large walkable pocket sealed off). Density 0 yields an empty map.
//
// Steps: (1) a blob straddling the flag row between the base clearance and
// the safe band, so no straight horizontal run to the flag exists on rows
// flagY-1..flagY+1; (2) more blobs, radius around size*(0.6..1.4), shrunk
// near the end and capped by the eligible area's extent, until about density
// of the eligible cells are covered (a blob that would overshoot by more
// than overshootTol is scaled down, or dropped); (3) prune obstacle
// components smaller than size cells (or than the largest blob the extent
// allows, if smaller); (4) mirror; (5) require each flag to reach at least
// 90% of its half's free cells, and fill the remaining unreachable pockets
// so nobody can spawn in them, rejecting the attempt if that pushes
// coverage past density+maxCoverageOver.
func clusters(m *Map, density float64, size int, seed int64) ([][]bool, bool) {
	obs := newGrid(m.Width, m.Height)
	if density <= 0 {
		return obs, true
	}
	size = max(size, 2)
	rng := rand.New(rand.NewSource(seed))
	elig := clusterEligible(m)

	var cells []Pos
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.SafeX0; x++ {
			if elig(Pos{x, y}) {
				cells = append(cells, Pos{x, y})
			}
		}
	}
	if len(cells) == 0 {
		return obs, true
	}
	target := int(density*float64(len(cells)) + 0.5)
	tol := int(overshootTol * float64(len(cells)))
	covered := 0

	laneX0, laneX1 := m.BaseA.X1+baseClearance+1, m.SafeX0-safeClearance-1
	hasLane := laneX0 <= laneX1
	// Cap blob radii by the eligible extent so one blob cannot fill (and
	// seal) a narrow strip.
	extent := m.SafeX0 - safeClearance
	if hasLane {
		extent = laneX1 - laneX0 + 1
	}
	capR := max(1, min(float64(extent)/2, float64(m.Height)/4))

	// place adds a blob at c, scaling it down while it would overshoot the
	// target by more than tol. The lane blob is always placed, at its
	// smallest scale if need be.
	place := func(c Pos, r float64, lane bool) {
		shape := blobShape(rng, r, lane)
		for scale := 1.0; ; scale *= 0.75 {
			fresh := map[Pos]bool{}
			renderBlob(c, shape, scale, lane, func(p Pos) {
				if elig(p) && !obs[p.Y][p.X] {
					fresh[p] = true
				}
			})
			last := r*scale < 1
			if covered+len(fresh) <= target+tol || (lane && last) {
				for p := range fresh {
					obs[p.Y][p.X] = true
				}
				covered += len(fresh)
				return
			}
			if last {
				return
			}
		}
	}

	if hasLane {
		c := Pos{laneX0 + rng.Intn(laneX1-laneX0+1), m.FlagA.Y}
		place(c, blobRadius(rng, size, target-covered, capR), true)
	}
	for i := 0; covered < target && i < maxBlobs; i++ {
		c := cells[rng.Intn(len(cells))]
		for try := 0; try < 20 && obs[c.Y][c.X]; try++ {
			c = cells[rng.Intn(len(cells))]
		}
		place(c, blobRadius(rng, size, target-covered, capR), false)
	}

	pruneSmall(obs, m.SafeX0, min(size, max(2, int(capR*capR))))
	if hasLane && !laneBlocked(obs, m.FlagA.Y, laneX0, laneX1) {
		return nil, false
	}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.SafeX0; x++ {
			if obs[y][x] {
				obs[y][m.Width-1-x] = true
			}
		}
	}
	if !sealPockets(m, obs) {
		return nil, false
	}
	final := 0
	for _, p := range cells {
		if obs[p.Y][p.X] {
			final++
		}
	}
	return obs, float64(final) <= (density+maxCoverageOver)*float64(len(cells))
}

func newGrid(w, h int) [][]bool {
	g := make([][]bool, h)
	for y := range g {
		g[y] = make([]bool, w)
	}
	return g
}

// blobRadius draws a radius around size*(0.6..1.4), shrunk when the blob
// would overshoot the remaining coverage need by a lot, and never above capR.
func blobRadius(rng *rand.Rand, size, need int, capR float64) float64 {
	r := float64(size) * (0.6 + 0.8*rng.Float64())
	// A blob of 3-5 ellipses with axes 0.5..1 r covers roughly 2.2 r^2 cells.
	if 2.2*r*r > float64(need) {
		r = math.Sqrt(float64(max(need, 0)) / 2.2)
	}
	return min(max(r, 1.5, 0.35*float64(size)), capR)
}

// blobEllipse is one ellipse of a blob, relative to the blob centre.
type blobEllipse struct{ dx, dy, rx, ry, theta float64 }

// blobShape draws the union of 3-5 rotated ellipses of radius about r.
// Every ellipse contains the centre, so the blob is connected.
func blobShape(rng *rand.Rand, r float64, lane bool) []blobEllipse {
	k := 3 + rng.Intn(3)
	shape := make([]blobEllipse, 0, k)
	for i := 0; i < k; i++ {
		e := blobEllipse{rx: r * (0.5 + 0.5*rng.Float64()), ry: r * (0.5 + 0.5*rng.Float64())}
		e.theta = rng.Float64() * math.Pi
		if i == 0 && lane {
			e.theta = 0
		} else if i > 0 {
			a, d := rng.Float64()*2*math.Pi, 0.8*min(e.rx, e.ry)*rng.Float64()
			e.dx, e.dy = d*math.Cos(a), d*math.Sin(a)
		}
		shape = append(shape, e)
	}
	return shape
}

// renderBlob paints shape around c, scaled. With lane set, the first
// ellipse is axis-aligned and at least 1.5 cells tall in each direction, so
// it covers rows c.Y-1..c.Y+1 at column c.X.
func renderBlob(c Pos, shape []blobEllipse, scale float64, lane bool, paint func(Pos)) {
	for i, e := range shape {
		rx, ry := max(e.rx*scale, 0.5), max(e.ry*scale, 0.5)
		if i == 0 && lane {
			ry = max(ry, 1.5)
		}
		ellipse(float64(c.X)+e.dx*scale, float64(c.Y)+e.dy*scale, rx, ry, e.theta, paint)
	}
}

func ellipse(cx, cy, rx, ry, theta float64, paint func(Pos)) {
	cos, sin := math.Cos(theta), math.Sin(theta)
	ext := int(math.Ceil(max(rx, ry)))
	x0, y0 := int(math.Round(cx)), int(math.Round(cy))
	for y := y0 - ext; y <= y0+ext; y++ {
		for x := x0 - ext; x <= x0+ext; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			u, v := dx*cos+dy*sin, -dx*sin+dy*cos
			if (u*u)/(rx*rx)+(v*v)/(ry*ry) <= 1 {
				paint(Pos{x, y})
			}
		}
	}
}

// pruneSmall clears 4-connected obstacle components smaller than minSize
// cells among columns 0..width-1.
func pruneSmall(obs [][]bool, width, minSize int) {
	h := len(obs)
	seen := newGrid(width, h)
	for y := 0; y < h; y++ {
		for x := 0; x < width; x++ {
			if !obs[y][x] || seen[y][x] {
				continue
			}
			comp := []Pos{{x, y}}
			seen[y][x] = true
			for i := 0; i < len(comp); i++ {
				p := comp[i]
				for _, d := range [4]Pos{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					n := Pos{p.X + d.X, p.Y + d.Y}
					if n.X < 0 || n.Y < 0 || n.X >= width || n.Y >= h || seen[n.Y][n.X] || !obs[n.Y][n.X] {
						continue
					}
					seen[n.Y][n.X] = true
					comp = append(comp, n)
				}
			}
			if len(comp) < minSize {
				for _, p := range comp {
					obs[p.Y][p.X] = false
				}
			}
		}
	}
}

// laneBlocked reports whether each of rows y-1..y+1 has an obstacle in
// columns x0..x1.
func laneBlocked(obs [][]bool, y, x0, x1 int) bool {
	for r := y - 1; r <= y+1; r++ {
		if r < 0 || r >= len(obs) {
			continue
		}
		found := false
		for x := x0; x <= x1 && !found; x++ {
			found = obs[r][x]
		}
		if !found {
			return false
		}
	}
	return true
}

// sealPockets checks that each flag reaches at least minHalfReach of the
// free cells on its own half (8-neighbour moves), then turns every free cell
// not reachable from Team A's flag into an obstacle. The map is mirrored and
// the safe band joins the halves, so one flood covers both sides.
func sealPockets(m *Map, obs [][]bool) bool {
	seen := floodFree(m, obs, m.FlagA)
	var reachA, freeA, reachB, freeB int
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if obs[y][x] {
				continue
			}
			r := seen[y*m.Width+x]
			switch {
			case x < m.SafeX0:
				freeA++
				if r {
					reachA++
				}
			case x > m.SafeX1:
				freeB++
				if r {
					reachB++
				}
			}
		}
	}
	if float64(reachA) < minHalfReach*float64(freeA) || float64(reachB) < minHalfReach*float64(freeB) {
		return false
	}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if !obs[y][x] && !seen[y*m.Width+x] {
				obs[y][x] = true
			}
		}
	}
	return true
}

// floodFree returns the cells reachable from start by 8-neighbour moves
// through non-obstacle cells, indexed y*Width+x.
func floodFree(m *Map, obs [][]bool, start Pos) []bool {
	seen := make([]bool, m.Width*m.Height)
	if obs[start.Y][start.X] {
		return seen
	}
	seen[start.Y*m.Width+start.X] = true
	q := []Pos{start}
	for i := 0; i < len(q); i++ {
		p := q[i]
		for d := Dir(0); d < 8; d++ {
			dl := d.Delta()
			n := Pos{p.X + dl.X, p.Y + dl.Y}
			if n.X < 0 || n.Y < 0 || n.X >= m.Width || n.Y >= m.Height {
				continue
			}
			if seen[n.Y*m.Width+n.X] || obs[n.Y][n.X] {
				continue
			}
			seen[n.Y*m.Width+n.X] = true
			q = append(q, n)
		}
	}
	return seen
}
