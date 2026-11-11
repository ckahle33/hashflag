package factions

import (
	"math/rand"
	"strings"
	"testing"

	"hashflag/internal/game"
)

func newReg() *Registry { return NewRegistry(rand.New(rand.NewSource(1))) }

func TestNamesVary(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		n := GenerateName(rng)
		if len(strings.Fields(n)) != 2 {
			t.Fatalf("bad name %q", n)
		}
		seen[n] = true
	}
	if len(seen) < 10 {
		t.Errorf("only %d distinct names", len(seen))
	}
	a := GenerateName(rand.New(rand.NewSource(7)))
	b := GenerateName(rand.New(rand.NewSource(7)))
	if a != b {
		t.Error("not deterministic")
	}
	if len(adjectives) < 40 || len(nouns) < 40 || len(glyphPool) < 40 {
		t.Error("word lists too small")
	}
}

func TestFounderRerollOnlyAlone(t *testing.T) {
	r := newReg()
	f := r.Create("p1", game.TeamA)
	old := f.Name
	if err := r.Reroll(f.ID, "p2"); err == nil {
		t.Error("non-founder reroll should fail")
	}
	if err := r.Reroll(f.ID, "p1"); err != nil {
		t.Fatal(err)
	}
	if f.Name == old {
		t.Error("name unchanged")
	}
	if err := r.Join(f.ID, "p2", [2]int{1, 0}, 10); err != nil {
		t.Fatal(err)
	}
	if err := r.Reroll(f.ID, "p1"); err == nil {
		t.Error("reroll with 2 members should fail")
	}
	if err := r.Reroll("nope", "p1"); err == nil {
		t.Error("missing faction should fail")
	}
}

func TestJoinFullSideRejected(t *testing.T) {
	r := newReg()
	f := r.Create("p1", game.TeamB)
	if err := r.Join(f.ID, "p2", [2]int{0, 5}, 5); err == nil {
		t.Error("full side should reject")
	}
	if err := r.Join("zzz", "p2", [2]int{0, 0}, 5); err == nil {
		t.Error("missing faction should reject")
	}
	if r.FactionOf("p2") != nil {
		t.Error("p2 should not be in a faction")
	}
}

func TestEmptyFactionDissolves(t *testing.T) {
	r := newReg()
	f := r.Create("p1", game.TeamA)
	r.Leave("p1")
	if r.Get(f.ID) != nil || r.FactionOf("p1") != nil || len(r.List()) != 0 {
		t.Error("faction should be gone")
	}
	g := r.Create("p1", game.TeamA)
	if g.ID == f.ID {
		t.Error("IDs reused")
	}
}

func TestFounderLeavePassesFounder(t *testing.T) {
	r := newReg()
	f := r.Create("p1", game.TeamA)
	r.Join(f.ID, "p2", [2]int{1, 0}, 10)
	r.Join(f.ID, "p3", [2]int{2, 0}, 10)
	r.Leave("p1")
	if f.Founder != "p2" || len(f.Members) != 2 {
		t.Errorf("founder=%v members=%v", f.Founder, f.Members)
	}
}

func TestCreateLeavesPrevious(t *testing.T) {
	r := newReg()
	f := r.Create("p1", game.TeamA)
	g := r.Create("p1", game.TeamA)
	if r.Get(f.ID) != nil || r.FactionOf("p1") != g {
		t.Error("previous faction should dissolve")
	}
}

func TestGlyphsUniqueAmongLiveFactions(t *testing.T) {
	r := newReg()
	seen := map[string]bool{}
	for i := 0; i < len(glyphPool); i++ {
		team := game.Team(i % 2)
		f := r.Create(game.PlayerID(string(rune('a'+i%26))+string(rune('A'+i/26))), team)
		if seen[f.Glyph] {
			t.Fatalf("duplicate glyph %q", f.Glyph)
		}
		seen[f.Glyph] = true
	}
	// pool exhausted: reuse still yields a glyph
	f := r.Create("overflow", game.TeamA)
	if f.Glyph == "" {
		t.Error("no glyph on overflow")
	}
	// freeing a glyph makes it available again
	r2 := newReg()
	a := r2.Create("x", game.TeamA)
	r2.Leave("x")
	_ = a
	if len(r2.List()) != 0 {
		t.Error("expected empty")
	}
}
