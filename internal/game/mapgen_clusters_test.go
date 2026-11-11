package game

import (
	"reflect"
	"sort"
	"testing"

	"hashflag/internal/config"
)

func clustersCfg() config.MapConfig {
	return config.Defaults().Map
}

// clusterMap generates a default clusters map; the seeds these tests use
// (0..19) are expected to generate.
func clusterMap(t *testing.T, seed int64) *Map {
	t.Helper()
	m, err := GenerateMap(clustersCfg(), seed)
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	return m
}

func TestClustersDeterministicAndMirrored(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		a := clusterMap(t, seed)
		b := clusterMap(t, seed)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("seed %d: maps differ", seed)
		}
		for y := 0; y < a.Height; y++ {
			for x := 0; x < a.Width; x++ {
				if a.Obstacle[y][x] != a.Obstacle[y][a.Width-1-x] {
					t.Fatalf("seed %d: not mirrored at %d,%d", seed, x, y)
				}
				p := Pos{x, y}
				if a.Obstacle[y][x] && (a.BaseA.Contains(p) || a.BaseB.Contains(p) || (x >= a.SafeX0-1 && x <= a.SafeX1+1)) {
					t.Fatalf("seed %d: obstacle in protected cell %v", seed, p)
				}
			}
		}
	}
	c, _ := GenerateMap(clustersCfg(), 1)
	d, _ := GenerateMap(clustersCfg(), 2)
	if reflect.DeepEqual(c, d) {
		t.Fatal("different seeds gave the same map")
	}
}

func TestClustersCoverageNearDensity(t *testing.T) {
	cfg := clustersCfg()
	for seed := int64(0); seed < 20; seed++ {
		m := clusterMap(t, seed)
		elig := clusterEligible(m)
		var n, covered int
		for y := 0; y < m.Height; y++ {
			for x := 0; x < m.SafeX0; x++ {
				if elig(Pos{x, y}) {
					n++
					if m.Obstacle[y][x] {
						covered++
					}
				}
			}
		}
		got := float64(covered) / float64(n)
		if got < cfg.Obstacles.Density-0.08 || got > cfg.Obstacles.Density+0.08 {
			t.Errorf("seed %d: coverage %.3f, want %.2f +- 0.08", seed, got, cfg.Obstacles.Density)
		}
	}
}

// obstacleComponents returns the sizes of 4-connected obstacle components.
func obstacleComponents(m *Map) []int {
	seen := make([]bool, m.Width*m.Height)
	var sizes []int
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if !m.Obstacle[y][x] || seen[y*m.Width+x] {
				continue
			}
			n, q := 0, []Pos{{x, y}}
			seen[y*m.Width+x] = true
			for len(q) > 0 {
				p := q[len(q)-1]
				q = q[:len(q)-1]
				n++
				for _, d := range []Pos{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					c := Pos{p.X + d.X, p.Y + d.Y}
					if c.X < 0 || c.Y < 0 || c.X >= m.Width || c.Y >= m.Height || seen[c.Y*m.Width+c.X] || !m.Obstacle[c.Y][c.X] {
						continue
					}
					seen[c.Y*m.Width+c.X] = true
					q = append(q, c)
				}
			}
			sizes = append(sizes, n)
		}
	}
	return sizes
}

func TestClustersAreChunky(t *testing.T) {
	size := clustersCfg().Obstacles.ClusterSize
	for seed := int64(0); seed < 20; seed++ {
		m := clusterMap(t, seed)
		s := obstacleComponents(m)
		if len(s) == 0 {
			t.Fatalf("seed %d: no obstacles", seed)
		}
		sort.Ints(s)
		if med := s[len(s)/2]; med < 3*size {
			t.Errorf("seed %d: median component %d cells, want >= %d (sizes %v)", seed, med, 3*size, s)
		}
		if s[0] < size {
			t.Errorf("seed %d: component of %d cells survived pruning (min %d)", seed, s[0], size)
		}
	}
}

func TestClustersBlockStraightLane(t *testing.T) {
	generated := 0
	defer func() {
		if generated < 48 {
			t.Errorf("only %d of 50 seeds generated", generated)
		}
	}()
	for seed := int64(0); seed < 50; seed++ {
		m, err := GenerateMap(clustersCfg(), seed)
		if err != nil {
			continue // counted by TestClustersFlagsReachable
		}
		generated++
		x0, x1 := m.BaseA.X1+baseClearance+1, m.SafeX0-safeClearance-1
		for y := m.FlagA.Y - 1; y <= m.FlagA.Y+1; y++ {
			blockedA, blockedB := false, false
			for x := x0; x <= x1; x++ {
				blockedA = blockedA || m.Obstacle[y][x]
				blockedB = blockedB || m.Obstacle[y][m.Width-1-x]
			}
			if !blockedA || !blockedB {
				t.Errorf("seed %d: row %d has a clear straight lane (A blocked=%v, B blocked=%v)", seed, y, blockedA, blockedB)
			}
		}
	}
}

// halfReach counts free cells on the flag's own half and how many of them
// an 8-neighbour BFS from the flag reaches.
func halfReach(m *Map, flag Pos) (reached, free int) {
	own := func(x int) bool { return x < m.SafeX0 }
	if flag.X > m.SafeX1 {
		own = func(x int) bool { return x > m.SafeX1 }
	}
	seen := map[Pos]bool{flag: true}
	q := []Pos{flag}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		if own(p.X) {
			reached++
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
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if own(x) && !m.Obstacle[y][x] {
				free++
			}
		}
	}
	return reached, free
}

func TestClustersFlagsReachable(t *testing.T) {
	fails := 0
	for seed := int64(0); seed < 50; seed++ {
		m, err := GenerateMap(clustersCfg(), seed)
		if err != nil {
			fails++
			continue
		}
		if !reachable(m, m.FlagA) || !reachable(m, m.FlagB) {
			t.Fatalf("seed %d: flag unreachable from the safe line", seed)
		}
		for _, f := range []Pos{m.FlagA, m.FlagB} {
			if r, n := halfReach(m, f); r*10 < n*9 {
				t.Fatalf("seed %d: flag %v reaches %d of %d free cells on its half", seed, f, r, n)
			}
		}
	}
	if fails > 2 {
		t.Fatalf("%d of 50 seeds failed to generate, want <= 2", fails)
	}
}

func TestClustersZeroDensityIsEmpty(t *testing.T) {
	cfg := clustersCfg()
	cfg.Obstacles.Density = 0
	m, err := GenerateMap(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	for y := range m.Obstacle {
		for x := range m.Obstacle[y] {
			if m.Obstacle[y][x] {
				t.Fatalf("obstacle at %d,%d with density 0", x, y)
			}
		}
	}
}

func TestScatterStillWorks(t *testing.T) {
	cfg := clustersCfg()
	cfg.Obstacles.Generate = "scatter"
	cfg.Obstacles.Density = 0.12
	m, err := GenerateMap(cfg, 3)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for y := range m.Obstacle {
		for x := range m.Obstacle[y] {
			if m.Obstacle[y][x] {
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("scatter produced no obstacles")
	}
	// Scatter makes many single cells; clusters would not.
	single := 0
	for _, s := range obstacleComponents(m) {
		if s == 1 {
			single++
		}
	}
	if single < 50 {
		t.Fatalf("scatter produced only %d single-cell obstacles", single)
	}
}
