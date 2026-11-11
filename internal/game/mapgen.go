package game

import (
	"fmt"
	"math/rand"

	"hashflag/internal/config"
)

const maxMapAttempts = 20

// GenerateMap builds a map from cfg. If cfg.Obstacles.Seed is non-nil it takes
// precedence over seed. Layout: Team A base on the left edge, Team B base on
// the right edge, both vertically centered; the safe line is a vertical band
// centered in the map. When Width-SafeLineWidth is odd, SafeX0 is rounded
// down ((Width-SafeLineWidth)/2), so the band sits one column left of exact
// center and the column just right of it stays obstacle-free to keep the map
// mirrored. Obstacles are generated for the left half and mirrored.
// If a flag is unreachable from the safe line, it retries with seed+1, up to
// 20 attempts in total, then returns an error.
func GenerateMap(cfg config.MapConfig, seed int64) (*Map, error) {
	o := cfg.Obstacles
	if o.Generate != "scatter" {
		return nil, fmt.Errorf("unsupported obstacle generator %q", o.Generate)
	}
	if len(o.Rows) > 0 {
		return nil, fmt.Errorf("obstacle rows are not supported")
	}
	if cfg.BaseSize < 1 || cfg.SafeLineWidth < 1 {
		return nil, fmt.Errorf("baseSize and safeLineWidth must be positive")
	}
	if cfg.Width < 2*cfg.BaseSize+cfg.SafeLineWidth+4 {
		return nil, fmt.Errorf("map width %d too small (need at least %d)", cfg.Width, 2*cfg.BaseSize+cfg.SafeLineWidth+4)
	}
	if cfg.Height < cfg.BaseSize+2 {
		return nil, fmt.Errorf("map height %d too small (need at least %d)", cfg.Height, cfg.BaseSize+2)
	}
	if o.Seed != nil {
		seed = *o.Seed
	}

	w, h := cfg.Width, cfg.Height
	safeX0 := (w - cfg.SafeLineWidth) / 2
	by0 := (h - cfg.BaseSize) / 2
	m := &Map{
		Width: w, Height: h,
		SafeX0: safeX0, SafeX1: safeX0 + cfg.SafeLineWidth - 1,
		BaseA: Rect{0, by0, cfg.BaseSize - 1, by0 + cfg.BaseSize - 1},
		BaseB: Rect{w - cfg.BaseSize, by0, w - 1, by0 + cfg.BaseSize - 1},
	}
	m.FlagA = Pos{(m.BaseA.X0 + m.BaseA.X1) / 2, (m.BaseA.Y0 + m.BaseA.Y1) / 2}
	m.FlagB = Pos{(m.BaseB.X0 + m.BaseB.X1) / 2, (m.BaseB.Y0 + m.BaseB.Y1) / 2}

	for attempt := 0; attempt < maxMapAttempts; attempt++ {
		m.Obstacle = scatter(m, o.Density, seed+int64(attempt))
		if flagReachable(m, m.FlagA) && flagReachable(m, m.FlagB) {
			return m, nil
		}
	}
	return nil, fmt.Errorf("no map with reachable flags after %d attempts", maxMapAttempts)
}

func scatter(m *Map, density float64, seed int64) [][]bool {
	rng := rand.New(rand.NewSource(seed))
	obs := make([][]bool, m.Height)
	for y := range obs {
		obs[y] = make([]bool, m.Width)
	}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.SafeX0; x++ {
			p := Pos{x, y}
			if m.BaseA.Contains(p) {
				continue
			}
			if rng.Float64() < density {
				obs[y][x] = true
				mx := m.Width - 1 - x
				if !m.BaseB.Contains(Pos{mx, y}) {
					obs[y][mx] = true
				}
			}
		}
	}
	return obs
}

// flagReachable reports whether flag can be reached from any safe-line cell
// by 8-neighbor moves through non-obstacle cells.
func flagReachable(m *Map, flag Pos) bool {
	seen := make([]bool, m.Width*m.Height)
	var q []Pos
	for y := 0; y < m.Height; y++ {
		for x := m.SafeX0; x <= m.SafeX1; x++ {
			seen[y*m.Width+x] = true
			q = append(q, Pos{x, y})
		}
	}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		if p == flag {
			return true
		}
		for d := Dir(0); d < 8; d++ {
			dl := d.Delta()
			n := Pos{p.X + dl.X, p.Y + dl.Y}
			if n.X < 0 || n.Y < 0 || n.X >= m.Width || n.Y >= m.Height {
				continue
			}
			if seen[n.Y*m.Width+n.X] || m.Obstacle[n.Y][n.X] {
				continue
			}
			seen[n.Y*m.Width+n.X] = true
			q = append(q, n)
		}
	}
	return false
}
