package game

import (
	"reflect"
	"testing"

	"hashflag/internal/config"
)

func testCfg(density float64) config.MapConfig {
	return config.MapConfig{Width: 60, Height: 30, SafeLineWidth: 4, BaseSize: 6,
		Obstacles: config.Obstacles{Generate: "scatter", Density: density}}
}

func TestMapMirrored(t *testing.T) {
	m, err := GenerateMap(testCfg(0.12), 7)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.Obstacle[y][x] != m.Obstacle[y][m.Width-1-x] {
				t.Fatalf("not mirrored at %d,%d", x, y)
			}
		}
	}
}

func TestSafeLineAndBasesClear(t *testing.T) {
	m, err := GenerateMap(testCfg(0.3), 3)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			p := Pos{x, y}
			if (x >= m.SafeX0 && x <= m.SafeX1) || m.BaseA.Contains(p) || m.BaseB.Contains(p) {
				if m.Obstacle[y][x] {
					t.Fatalf("obstacle at protected %v", p)
				}
			}
		}
	}
	if !m.BaseA.Contains(m.FlagA) || !m.BaseB.Contains(m.FlagB) || m.FlagA.X >= m.FlagB.X {
		t.Fatal("bad flags")
	}
}

func TestSameSeedSameMap(t *testing.T) {
	a, _ := GenerateMap(testCfg(0.2), 42)
	b, _ := GenerateMap(testCfg(0.2), 42)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("maps differ")
	}
	cfg := testCfg(0.2)
	s := int64(5)
	cfg.Obstacles.Seed = &s
	c, _ := GenerateMap(cfg, 1)
	d, _ := GenerateMap(cfg, 2)
	if !reflect.DeepEqual(c, d) {
		t.Fatal("config seed should take precedence")
	}
}

func reachable(m *Map, flag Pos) bool {
	seen := map[Pos]bool{}
	var q []Pos
	for y := 0; y < m.Height; y++ {
		for x := m.SafeX0; x <= m.SafeX1; x++ {
			seen[Pos{x, y}] = true
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
			n := Pos{p.X + d.Delta().X, p.Y + d.Delta().Y}
			if n.X < 0 || n.Y < 0 || n.X >= m.Width || n.Y >= m.Height || seen[n] || m.Obstacle[n.Y][n.X] {
				continue
			}
			seen[n] = true
			q = append(q, n)
		}
	}
	return false
}

func TestFlagsReachableFromSafeLine(t *testing.T) {
	for _, dens := range []float64{0.12, 0.4} {
		for seed := int64(0); seed < 50; seed++ {
			m, err := GenerateMap(testCfg(dens), seed)
			if err != nil {
				continue // allowed
			}
			if !reachable(m, m.FlagA) || !reachable(m, m.FlagB) {
				t.Fatalf("unreachable flag density %v seed %d", dens, seed)
			}
		}
	}
}

func TestRejectsTooSmallMap(t *testing.T) {
	cfg := testCfg(0.1)
	cfg.Width = 2*cfg.BaseSize + cfg.SafeLineWidth + 3
	if _, err := GenerateMap(cfg, 1); err == nil {
		t.Fatal("expected width error")
	}
	cfg = testCfg(0.1)
	cfg.Height = cfg.BaseSize + 1
	if _, err := GenerateMap(cfg, 1); err == nil {
		t.Fatal("expected height error")
	}
}

func TestRejectsUnsupportedGenerator(t *testing.T) {
	cfg := testCfg(0.1)
	cfg.Obstacles.Generate = "rows"
	if _, err := GenerateMap(cfg, 1); err == nil {
		t.Fatal("expected error")
	}
	cfg = testCfg(0.1)
	cfg.Obstacles.Rows = []string{"..."}
	if _, err := GenerateMap(cfg, 1); err == nil {
		t.Fatal("expected error for rows")
	}
}
