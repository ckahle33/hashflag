//go:build bots

package main

import (
	"math/rand"
	"testing"
)

func TestStepToward(t *testing.T) {
	cases := []struct {
		from, to pos
		dir      int
		ok       bool
	}{
		{pos{5, 5}, pos{5, 2}, 0, true},
		{pos{5, 5}, pos{9, 1}, 1, true},
		{pos{5, 5}, pos{9, 5}, 2, true},
		{pos{5, 5}, pos{7, 9}, 3, true},
		{pos{5, 5}, pos{5, 9}, 4, true},
		{pos{5, 5}, pos{1, 9}, 5, true},
		{pos{5, 5}, pos{0, 5}, 6, true},
		{pos{5, 5}, pos{0, 0}, 7, true},
		{pos{5, 5}, pos{5, 5}, 0, false},
	}
	for _, c := range cases {
		d, ok := stepToward(c.from, c.to)
		if ok != c.ok || (ok && d != c.dir) {
			t.Errorf("stepToward(%v,%v) = %d,%v want %d,%v", c.from, c.to, d, ok, c.dir, c.ok)
		}
	}
}

func TestAim(t *testing.T) {
	from := pos{10, 10}
	cases := []struct {
		to  pos
		dir int
		ok  bool
	}{
		{pos{10, 5}, 0, true},
		{pos{13, 7}, 1, true},
		{pos{24, 10}, 2, true},  // exactly range 14
		{pos{25, 10}, 0, false}, // out of range
		{pos{12, 13}, 0, false}, // not aligned
		{pos{10, 10}, 0, false}, // same cell
		{pos{7, 13}, 5, true},
		{pos{4, 10}, 6, true},
		{pos{8, 8}, 7, true},
	}
	for _, c := range cases {
		d, ok := aim(from, c.to, 14)
		if ok != c.ok || (ok && d != c.dir) {
			t.Errorf("aim(%v) = %d,%v want %d,%v", c.to, d, ok, c.dir, c.ok)
		}
	}
}

func TestPickMoveSidesteps(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	blockedN := func(p pos) bool { return p == (pos{5, 4}) }
	for i := 0; i < 50; i++ {
		d, ok := pickMove(rng, pos{5, 5}, pos{5, 0}, blockedN, 0)
		if !ok || (d != 1 && d != 7 && d != 2 && d != 6) {
			t.Fatalf("expected a forward-ish sidestep, got %d,%v", d, ok)
		}
		if blockedN(pos{5 + deltas[d].x, 5 + deltas[d].y}) {
			t.Fatal("moved into blocked cell")
		}
	}
	all := func(pos) bool { return true }
	if _, ok := pickMove(rng, pos{5, 5}, pos{5, 0}, all, 0); ok {
		t.Fatal("fully enclosed bot should not move")
	}
}

func TestStuckSidestep(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var s stuckState
	at := pos{5, 5}
	// The first frame only records the position.
	if _, ok := s.step(rng, at, true, 2); ok {
		t.Fatal("sidestep on first frame")
	}
	for i := 1; i <= 2; i++ {
		if _, ok := s.step(rng, at, true, 2); ok {
			t.Fatalf("sidestep after %d unchanged frames, want 3", i)
		}
	}
	d, ok := s.step(rng, at, true, 2)
	if !ok || (d != 0 && d != 4) {
		t.Fatalf("after 3 unchanged frames moving E: got %d,%v, want N or S", d, ok)
	}
	d2, ok := s.step(rng, pos{5, 4}, true, d)
	if !ok || d2 != d {
		t.Fatalf("second sidestep tick: got %d,%v, want %d", d2, ok, d)
	}
	if _, ok := s.step(rng, pos{5, 3}, true, d); ok {
		t.Fatal("sidestep lasted more than 2 ticks")
	}

	// Standing still without move intents is not being stuck.
	s = stuckState{}
	for i := 0; i < 10; i++ {
		if _, ok := s.step(rng, at, false, 0); ok {
			t.Fatal("sidestep without move intents")
		}
	}
	// A position change resets the count.
	s = stuckState{}
	s.step(rng, at, true, 0)
	s.step(rng, at, true, 0)
	s.step(rng, at, true, 0)
	if _, ok := s.step(rng, pos{6, 5}, true, 0); ok {
		t.Fatal("sidestep after moving")
	}
	// Diagonal moves get a perpendicular diagonal.
	s = stuckState{}
	for i := 0; i < 3; i++ {
		s.step(rng, at, true, 1)
	}
	if d, ok := s.step(rng, at, true, 1); !ok || (d != 3 && d != 7) {
		t.Fatalf("NE stuck: got %d,%v, want SE or NW", d, ok)
	}
}

func TestOnSafeLine(t *testing.T) {
	b := &bot{safeX0: 59, safeX1: 60}
	for x, want := range map[int]bool{58: false, 59: true, 60: true, 61: false} {
		if got := b.onSafeLine(pos{x, 3}); got != want {
			t.Errorf("x=%d: got %v want %v", x, got, want)
		}
	}
}

func TestDownhillRoutesAroundBlob(t *testing.T) {
	// A U-shaped obstacle opening away from the target: greedy steps from
	// inside it walk into the wall; following the distance field gets out.
	rows := []string{
		"..............",
		"..#######.....",
		"........#.....",
		"........#....T",
		"........#.....",
		"..#######.....",
		"..............",
	}
	target := pos{13, 3}
	dist := distField(rows, target)
	blocked := func(p pos) bool {
		return p.y < 0 || p.y >= len(rows) || p.x < 0 || p.x >= len(rows[0]) || rows[p.y][p.x] == '#'
	}
	rng := rand.New(rand.NewSource(1))
	at := pos{6, 3}
	for steps := 0; at != target; steps++ {
		if steps > 30 {
			t.Fatalf("did not reach target, stuck at %v", at)
		}
		d, ok := downhill(rng, dist, len(rows[0]), at, blocked)
		if !ok {
			t.Fatalf("no downhill move at %v", at)
		}
		at = pos{at.x + deltas[d].x, at.y + deltas[d].y}
		if blocked(at) {
			t.Fatalf("moved into obstacle at %v", at)
		}
	}
	if dist[target.y*len(rows[0])+target.x] != 0 {
		t.Fatal("target distance not 0")
	}
	if _, ok := downhill(rng, dist, len(rows[0]), target, blocked); ok {
		t.Fatal("moved while on target")
	}
}
