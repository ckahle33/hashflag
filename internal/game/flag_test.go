package game

import (
	"testing"

	"hashflag/internal/config"
)

func hasEvent(evs []Event, k EventKind) bool {
	for _, e := range evs {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func TestPickupAndCapture(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, w.Map.FlagB)
	evs := w.Step(nil)
	if w.Flags[TeamB].Carrier != "a" || !a.HasFlag || !hasEvent(evs, FlagPickedUp) {
		t.Fatalf("pickup failed: %+v hasFlag=%v evs=%v", w.Flags[TeamB], a.HasFlag, evs)
	}
	a.Pos = Pos{w.Map.BaseA.X1 + 1, w.Map.BaseA.Y0 + 1}
	w.Step(nil)
	if w.Flags[TeamB].Pos != a.Pos || !a.HasFlag {
		t.Fatalf("flag should follow carrier: %+v", w.Flags[TeamB])
	}
	a.Pos = Pos{w.Map.BaseA.X1, w.Map.BaseA.Y0 + 1}
	evs = w.Step(nil)
	if !hasEvent(evs, Captured) || w.Scores()[TeamA] != 1 || a.HasFlag ||
		w.Flags[TeamB].Carrier != "" || w.Flags[TeamB].Pos != w.Flags[TeamB].Home {
		t.Fatalf("capture failed: scores=%v flag=%+v evs=%v", w.Scores(), w.Flags[TeamB], evs)
	}
}

func TestPickupLowestIDWinsAndDefenderCannot(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Movement.Stackable = true })
	d := w.AddPlayer(Player{ID: "0d", Team: TeamA, Role: Defender})
	d.Pos = w.Map.FlagB
	b := place(t, w, "b", TeamA, w.Map.FlagB)
	c := place(t, w, "c", TeamA, w.Map.FlagB)
	w.Step(nil)
	if w.Flags[TeamB].Carrier != "b" || !b.HasFlag || c.HasFlag || d.HasFlag {
		t.Fatalf("carrier = %q", w.Flags[TeamB].Carrier)
	}
}

func TestCarrierTaggedDropsFlagInPlace(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, w.Map.FlagB)
	w.Step(nil)
	a.Pos = Pos{20, 3}
	place(t, w, "b", TeamB, Pos{24, 3})
	// b throws west; the projectile reaches a within a few ticks.
	var evs []Event
	evs = append(evs, w.Step(map[PlayerID]Intent{"b": throw(W_)})...)
	for i := 0; i < 6 && !a.Out(); i++ {
		evs = append(evs, w.Step(nil)...)
	}
	if !a.Out() {
		t.Fatal("a was not tagged")
	}
	f := w.Flags[TeamB]
	if a.HasFlag || f.Carrier != "" || f.Pos != a.Pos || f.Pos == f.Home || !hasEvent(evs, FlagDropped) {
		t.Fatalf("flag not dropped in place: %+v carrier pos %v", f, a.Pos)
	}
}

func TestTaggedCarrierDoesNotRespawnHoldingFlag(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Tag.OutTicks = 2 })
	a := place(t, w, "a", TeamA, w.Map.FlagB)
	w.Step(nil)
	a.Pos = Pos{20, 3}
	w.applyTags([]hit{{victim: "a", owner: "x"}}, new([]Event))
	evs := w.Step(nil)
	if !hasEvent(evs, FlagDropped) || a.HasFlag {
		t.Fatalf("no drop on tag tick: %v", evs)
	}
	for i := 0; i < 4; i++ {
		w.Step(nil)
	}
	if a.Out() || a.HasFlag || w.Flags[TeamB].Carrier != "" {
		t.Fatalf("respawned carrying flag: %+v %+v", *a, w.Flags[TeamB])
	}
}

func TestDroppedFlagAutoReturnsAt30(t *testing.T) {
	w := newTestWorld(t, nil)
	w.Flags[TeamB].Pos = Pos{20, 3}
	for i := 1; i < 30; i++ {
		w.Step(nil)
		if w.Flags[TeamB].Pos == w.Flags[TeamB].Home {
			t.Fatalf("returned early at %d", i)
		}
	}
	evs := w.Step(nil)
	if w.Flags[TeamB].Pos != w.Flags[TeamB].Home || w.Flags[TeamB].DroppedFor != 0 || !hasEvent(evs, FlagReturned) {
		t.Fatalf("not returned: %+v", w.Flags[TeamB])
	}
}

func TestDroppedFlagNeverReturnsWhenNil(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Flag.AutoReturnTicks = nil })
	w.Flags[TeamB].Pos = Pos{20, 3}
	for i := 0; i < 100; i++ {
		w.Step(nil)
	}
	if w.Flags[TeamB].Pos != (Pos{20, 3}) {
		t.Fatal("flag returned")
	}
}

func TestTeammateReturnsDroppedFlag(t *testing.T) {
	w := newTestWorld(t, nil)
	w.Flags[TeamB].Pos = Pos{20, 3}
	w.Flags[TeamB].DroppedFor = 4
	d := w.AddPlayer(Player{ID: "d", Team: TeamB, Role: Defender})
	d.Pos = Pos{20, 3}
	evs := w.Step(nil)
	f := w.Flags[TeamB]
	if f.Pos != f.Home || f.DroppedFor != 0 || !hasEvent(evs, FlagReturned) {
		t.Fatalf("not returned: %+v", f)
	}
}

func TestCarrierRemovedDropsFlag(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, w.Map.FlagB)
	w.Step(nil)
	a.Pos = Pos{20, 3}
	w.Step(nil)
	w.RemovePlayer("a")
	f := w.Flags[TeamB]
	if f.Carrier != "" || f.Pos != (Pos{20, 3}) || f.DroppedFor != 0 {
		t.Fatalf("flag = %+v", f)
	}
	w.Step(nil)
	if w.Flags[TeamB].DroppedFor != 1 {
		t.Fatalf("DroppedFor = %d", w.Flags[TeamB].DroppedFor)
	}
}

func TestCarrierCannotThrow(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, w.Map.FlagB)
	w.Step(nil)
	a.Pos = Pos{20, 3}
	ammo := a.Ammo
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if a.Ammo != ammo || len(w.Projectiles) != 0 {
		t.Fatalf("carrier threw: ammo %d projectiles %d", a.Ammo, len(w.Projectiles))
	}
}

func TestWinnerMostCapturesAtTime(t *testing.T) {
	w := newTestWorld(t, nil)
	w.scores = [2]int{2, 1}
	if _, done := w.Winner(); done {
		t.Fatal("done early")
	}
	w.Tick = w.Cfg.RoundTicks
	if tm, done := w.Winner(); !done || tm != TeamA {
		t.Fatalf("got %v %v", tm, done)
	}
	w.scores = [2]int{1, 1}
	if tm, done := w.Winner(); !done || tm != NoTeam {
		t.Fatalf("tie got %v %v", tm, done)
	}
}

func TestWinnerFirstToCaptures(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Win = config.Win{Mode: "firstToCaptures", Captures: 3} })
	w.scores = [2]int{2, 3}
	if tm, done := w.Winner(); !done || tm != TeamB {
		t.Fatalf("got %v %v", tm, done)
	}
	w.scores = [2]int{2, 1}
	if _, done := w.Winner(); done {
		t.Fatal("done early")
	}
	w.Tick = w.Cfg.RoundTicks
	if tm, done := w.Winner(); !done || tm != TeamA {
		t.Fatalf("limit got %v %v", tm, done)
	}
	w.scores = [2]int{2, 2}
	if tm, done := w.Winner(); !done || tm != NoTeam {
		t.Fatalf("tie got %v %v", tm, done)
	}
}
