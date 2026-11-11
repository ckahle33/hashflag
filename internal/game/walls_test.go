package game

import (
	"reflect"
	"testing"

	"hashflag/internal/config"
)

// Test map: BaseA x0..4 y5..9, flag A home (2,7); BaseB x25..29, flag B (27,7).

func placeDef(t *testing.T, w *World, id string, team Team, pos Pos) *Player {
	t.Helper()
	p := place(t, w, id, team, pos)
	p.Role = Defender
	return p
}

func wallAt(d Dir) Intent { return Intent{Kind: PlaceWall, Dir: d} }

// ringFlagA walls every neighbour of flag A except the listed cells.
func ringFlagA(w *World, except ...Pos) {
	skip := map[Pos]bool{}
	for _, p := range except {
		skip[p] = true
	}
	f := w.Map.FlagA
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			p := Pos{f.X + dx, f.Y + dy}
			if (dx == 0 && dy == 0) || skip[p] {
				continue
			}
			w.Walls[p] = Wall{Owner: "x", Team: TeamA, Hits: 1}
		}
	}
}

func TestPlaceWallInBase(t *testing.T) {
	w := newTestWorld(t, nil)
	d := placeDef(t, w, "d", TeamA, Pos{4, 6})
	ev := w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	wl, ok := w.Walls[Pos{3, 6}]
	if !ok || wl.Owner != "d" || wl.Team != TeamA || wl.Hits != 1 {
		t.Fatalf("wall = %+v ok=%v", wl, ok)
	}
	if d.WallsPlaced != 1 {
		t.Fatalf("WallsPlaced = %d", d.WallsPlaced)
	}
	want := []Event{{Tick: 0, Kind: WallPlaced, Player: "d", Team: TeamA, Pos: Pos{3, 6}}}
	if !reflect.DeepEqual(ev, want) {
		t.Fatalf("events = %+v", ev)
	}
}

func TestAttackerCannotPlace(t *testing.T) {
	w := newTestWorld(t, nil)
	place(t, w, "a", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"a": wallAt(W_)})
	if len(w.Walls) != 0 {
		t.Fatal("attacker placed a wall")
	}
}

func TestOutsideBaseRejected(t *testing.T) {
	w := newTestWorld(t, nil)
	placeDef(t, w, "d", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"d": wallAt(E)}) // (5,6) is outside base A
	if len(w.Walls) != 0 {
		t.Fatal("placed outside own base")
	}
	// Enemy base is also rejected.
	placeDef(t, w, "e", TeamA, Pos{24, 6})
	w.Step(map[PlayerID]Intent{"e": wallAt(E)}) // (25,6) in base B
	if len(w.Walls) != 0 {
		t.Fatal("placed in enemy base")
	}
}

func TestInvalidTargetsRejected(t *testing.T) {
	w := newTestWorld(t, nil)
	w.Map.Obstacle[6][3] = true
	placeDef(t, w, "d", TeamA, Pos{4, 6})
	placeDef(t, w, "e", TeamA, Pos{1, 7})
	placeDef(t, w, "f", TeamA, Pos{0, 5})
	w.Step(map[PlayerID]Intent{
		"d": wallAt(W_), // obstacle
		"e": wallAt(E),  // flag cell (2,7)
		"f": wallAt(N),  // out of bounds
	})
	if len(w.Walls) != 0 {
		t.Fatalf("walls = %v", w.Walls)
	}
	// occupied by a player
	placeDef(t, w, "g", TeamA, Pos{3, 9})
	place(t, w, "h", TeamB, Pos{3, 8})
	w.Step(map[PlayerID]Intent{"g": wallAt(N)})
	if len(w.Walls) != 0 {
		t.Fatal("placed on a player")
	}
}

func TestPerPlayerLimit(t *testing.T) {
	w := newTestWorld(t, nil)
	d := placeDef(t, w, "d", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	w.Step(map[PlayerID]Intent{"d": wallAt(S)})
	if len(w.Walls) != 1 || d.WallsPlaced != 1 {
		t.Fatalf("walls=%d placed=%d", len(w.Walls), d.WallsPlaced)
	}
}

func TestTeamCapAndFreedWhenBroken(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Walls.TeamCap = 1 })
	d1 := placeDef(t, w, "d1", TeamA, Pos{3, 5})
	d2 := placeDef(t, w, "d2", TeamA, Pos{4, 8})
	w.Step(map[PlayerID]Intent{"d1": wallAt(S)}) // wall at (3,6)
	w.Step(map[PlayerID]Intent{"d2": wallAt(W_)})
	if len(w.Walls) != 1 || d2.WallsPlaced != 0 {
		t.Fatal("team cap not enforced")
	}
	// Enemy shoots the wall from the safe side.
	place(t, w, "e", TeamB, Pos{8, 6})
	w.Step(map[PlayerID]Intent{"e": throw(W_)})
	w.Step(nil)
	w.Step(nil)
	if len(w.Walls) != 0 || d1.WallsPlaced != 0 {
		t.Fatalf("wall not broken: %v", w.Walls)
	}
	w.Step(map[PlayerID]Intent{"d2": wallAt(W_)})
	if _, ok := w.Walls[Pos{3, 8}]; !ok || d2.WallsPlaced != 1 {
		t.Fatal("slot not freed")
	}
}

func TestSealingPlacementRejected(t *testing.T) {
	w := newTestWorld(t, nil)
	ringFlagA(w, Pos{3, 7})
	if !w.sealsFlag(TeamA, Pos{3, 7}) {
		t.Fatal("sealsFlag should be true")
	}
	if w.sealsFlag(TeamA, Pos{4, 4}) {
		t.Fatal("sealsFlag should be false for unrelated cell")
	}
	placeDef(t, w, "d", TeamA, Pos{4, 7})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	if _, ok := w.Walls[Pos{3, 7}]; ok {
		t.Fatal("sealing wall placed")
	}
}

func TestMustLeavePathFalseAllowsSeal(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Walls.MustLeavePath = false })
	ringFlagA(w, Pos{3, 7})
	placeDef(t, w, "d", TeamA, Pos{4, 7})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	if _, ok := w.Walls[Pos{3, 7}]; !ok {
		t.Fatal("wall should be allowed")
	}
}

func TestSimultaneousPlacementsCannotTogetherSeal(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Walls.PerPlayer = 2 })
	ringFlagA(w, Pos{3, 6}, Pos{3, 8})
	placeDef(t, w, "d1", TeamA, Pos{4, 6})
	placeDef(t, w, "d2", TeamA, Pos{4, 8})
	w.Step(map[PlayerID]Intent{"d1": wallAt(W_), "d2": wallAt(W_)})
	if _, ok := w.Walls[Pos{3, 6}]; !ok {
		t.Fatal("first placement should succeed")
	}
	if _, ok := w.Walls[Pos{3, 8}]; ok {
		t.Fatal("second placement should be rejected")
	}
}

func TestWallBlocksMoveAndProjectile(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Walls.HitsToBreak = 2 })
	placeDef(t, w, "d", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	d := w.Players["d"]
	w.Step(map[PlayerID]Intent{"d": move(W_)})
	if d.Pos != (Pos{4, 6}) {
		t.Fatalf("moved through wall to %v", d.Pos)
	}
	w.Step(map[PlayerID]Intent{"d": move(N)}) // clear the firing row
	place(t, w, "e", TeamB, Pos{1, 6})
	place(t, w, "a", TeamB, Pos{8, 6})
	w.Step(map[PlayerID]Intent{"a": throw(W_)})
	w.Step(nil)
	w.Step(nil)
	if w.Players["e"].Out() || w.Walls[Pos{3, 6}].Hits != 1 {
		t.Fatalf("projectile passed wall or hits wrong: %+v", w.Walls[Pos{3, 6}])
	}
}

func TestWallHitsFromConfig(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Walls.HitsToBreak = 2 })
	placeDef(t, w, "d", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	if h := w.Walls[Pos{3, 6}].Hits; h != 2 {
		t.Fatalf("Hits = %d", h)
	}
}

func TestPlaceWallDoesNotMove(t *testing.T) {
	w := newTestWorld(t, nil)
	d := placeDef(t, w, "d", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	if d.Pos != (Pos{4, 6}) {
		t.Fatalf("moved to %v", d.Pos)
	}
}

func TestRoleWallOverride(t *testing.T) {
	two := 2
	w := newTestWorld(t, func(c *config.Config) {
		c.Roles.Defender = &config.RoleOverride{Walls: &config.WallsOverride{PerPlayer: &two}}
	})
	d := placeDef(t, w, "d", TeamA, Pos{4, 6})
	w.Step(map[PlayerID]Intent{"d": wallAt(W_)})
	w.Step(map[PlayerID]Intent{"d": wallAt(S)})
	if d.WallsPlaced != 2 {
		t.Fatalf("WallsPlaced = %d", d.WallsPlaced)
	}
}
