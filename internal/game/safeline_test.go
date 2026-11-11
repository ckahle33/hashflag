package game

import (
	"testing"

	"hashflag/internal/config"
)

// safeLineWorld has a thrower "a" at x=13 next to target "b" parked on the
// safe line at x=14 (SafeX 14..15), with no throw cooldown.
func safeLineWorld(t *testing.T, maxTicks int) (*World, *Player) {
	t.Helper()
	w := newTestWorld(t, func(c *config.Config) {
		c.Projectile.CooldownTicks = 0
		c.Tag.SafeLineMaxTicks = maxTicks
	})
	place(t, w, "a", TeamA, Pos{13, 2})
	b := place(t, w, "b", TeamB, Pos{14, 2})
	return w, b
}

func TestSafeLineImmunityExpires(t *testing.T) {
	const max = 3
	w, b := safeLineWorld(t, max)
	for tick := 1; tick <= max; tick++ {
		w.Step(map[PlayerID]Intent{"a": throw(E)})
		if b.Out() {
			t.Fatalf("tagged on safe-line tick %d, want immune for %d ticks", tick, max)
		}
		if b.SafeTicks != tick {
			t.Fatalf("SafeTicks = %d after tick %d", b.SafeTicks, tick)
		}
	}
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if !b.Out() {
		t.Fatalf("not tagged on safe-line tick %d (SafeTicks=%d)", max+1, b.SafeTicks)
	}
}

func TestSafeLineCounterResetsOffLine(t *testing.T) {
	const max = 2
	w, b := safeLineWorld(t, max)
	b.Pos = Pos{15, 2} // far column of the band
	w.Step(nil)
	w.Step(nil)
	if b.SafeTicks != max {
		t.Fatalf("SafeTicks = %d, want %d", b.SafeTicks, max)
	}
	w.Step(map[PlayerID]Intent{"b": move(E)}) // x=16: off the line for one tick
	if b.Pos.X != 16 || b.SafeTicks != 0 {
		t.Fatalf("off the line: pos=%v SafeTicks=%d, want x=16 and 0", b.Pos, b.SafeTicks)
	}
	// Back on the line while a throws through the band: immune again.
	for tick := 1; tick <= max; tick++ {
		in := map[PlayerID]Intent{"a": throw(E)}
		if tick == 1 {
			in["b"] = move(W_)
		}
		w.Step(in)
		if b.Out() || b.SafeTicks != tick {
			t.Fatalf("re-entry tick %d: out=%d SafeTicks=%d, want immune with %d", tick, b.OutLeft, b.SafeTicks, tick)
		}
	}
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if !b.Out() {
		t.Fatalf("not tagged after %d ticks back on the line", max+1)
	}
}

func TestSafeLineMaxZeroUnlimited(t *testing.T) {
	w, b := safeLineWorld(t, 0)
	for tick := 1; tick <= 50; tick++ {
		w.Step(map[PlayerID]Intent{"a": throw(E)})
		if b.Out() {
			t.Fatalf("tagged on tick %d with safeLineMaxTicks=0", tick)
		}
		if a := w.Players["a"]; a.Ammo == 0 {
			a.Ammo = 10
		}
	}
}

func TestRespawnResetsSafeTicks(t *testing.T) {
	w, b := safeLineWorld(t, 3)
	b.SafeTicks = 99
	b.OutLeft = 1
	w.Step(nil)
	if b.Out() || b.SafeTicks != 0 {
		t.Fatalf("after respawn: out=%d SafeTicks=%d, want in with 0", b.OutLeft, b.SafeTicks)
	}
	// AddPlayer never inherits a counter.
	p := w.AddPlayer(Player{ID: "c", Team: TeamA, SafeTicks: 7})
	if p == nil || p.SafeTicks != 0 {
		t.Fatalf("AddPlayer SafeTicks = %v, want 0", p)
	}
}
