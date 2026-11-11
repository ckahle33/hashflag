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
