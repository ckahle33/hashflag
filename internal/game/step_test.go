package game

import (
	"reflect"
	"testing"
	"time"

	"hashflag/internal/config"
)

// Test map (30x15, base 5, safe line 2, no obstacles):
// SafeX 14..15, BaseA x0..4 y5..9, BaseB x25..29 y5..9.
func newTestWorld(t *testing.T, mod func(*config.Config)) *World {
	t.Helper()
	cfg := config.Defaults()
	cfg.Map = config.MapConfig{Width: 30, Height: 15, SafeLineWidth: 2, BaseSize: 5,
		Obstacles: config.Obstacles{Generate: "scatter", Density: 0}}
	if mod != nil {
		mod(&cfg)
	}
	w, err := NewWorld(&cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// place adds a player and moves it to pos.
func place(t *testing.T, w *World, id string, team Team, pos Pos) *Player {
	t.Helper()
	p := w.AddPlayer(Player{ID: PlayerID(id), Name: id, Team: team, Role: Attacker})
	if p == nil {
		t.Fatalf("AddPlayer(%s) returned nil", id)
	}
	p.Pos = pos
	return p
}

func move(d Dir) Intent  { return Intent{Kind: Move, Dir: d} }
func throw(d Dir) Intent { return Intent{Kind: Throw, Dir: d} }

const (
	N  Dir = 0
	E  Dir = 2
	S  Dir = 4
	W_ Dir = 6
)

func TestMoveOneCell(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{7, 2})
	w.Step(map[PlayerID]Intent{"a": move(E)})
	if a.Pos != (Pos{8, 2}) {
		t.Fatalf("pos = %v, want {8 2}", a.Pos)
	}
	if w.Tick != 1 {
		t.Fatalf("tick = %d, want 1", w.Tick)
	}
}

func TestFollowLeaderMoves(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{6, 2})
	b := place(t, w, "b", TeamA, Pos{7, 2})
	w.Step(map[PlayerID]Intent{"a": move(E), "b": move(E)})
	if a.Pos != (Pos{7, 2}) || b.Pos != (Pos{8, 2}) {
		t.Fatalf("a=%v b=%v, want {7 2} {8 2}", a.Pos, b.Pos)
	}
}

func TestBlockedByStandingPlayer(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{6, 2})
	place(t, w, "b", TeamA, Pos{7, 2})
	w.Step(map[PlayerID]Intent{"a": move(E)})
	if a.Pos != (Pos{6, 2}) {
		t.Fatalf("a=%v, want {6 2}", a.Pos)
	}
}

func TestTwoPlayersSameTargetNeitherMoves(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{7, 2})
	b := place(t, w, "b", TeamB, Pos{9, 2})
	w.Step(map[PlayerID]Intent{"a": move(E), "b": move(W_)})
	if a.Pos != (Pos{7, 2}) || b.Pos != (Pos{9, 2}) {
		t.Fatalf("a=%v b=%v, want unchanged", a.Pos, b.Pos)
	}
}

func TestSwapBlocked(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{7, 2})
	b := place(t, w, "b", TeamB, Pos{8, 2})
	w.Step(map[PlayerID]Intent{"a": move(E), "b": move(W_)})
	if a.Pos != (Pos{7, 2}) || b.Pos != (Pos{8, 2}) {
		t.Fatalf("a=%v b=%v, want unchanged", a.Pos, b.Pos)
	}
}

func TestStackableAllowsOverlap(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Movement.Stackable = true })
	a := place(t, w, "a", TeamA, Pos{7, 2})
	b := place(t, w, "b", TeamA, Pos{9, 2})
	w.Step(map[PlayerID]Intent{"a": move(E), "b": move(W_)})
	if a.Pos != (Pos{8, 2}) || b.Pos != (Pos{8, 2}) {
		t.Fatalf("a=%v b=%v, want both {8 2}", a.Pos, b.Pos)
	}
}

func TestMoveStopsAtObstacle(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Movement.CellsPerTick = 3 })
	a := place(t, w, "a", TeamA, Pos{7, 2})
	w.Map.Obstacle[2][9] = true
	w.Step(map[PlayerID]Intent{"a": move(E)})
	if a.Pos != (Pos{8, 2}) {
		t.Fatalf("pos = %v, want {8 2}", a.Pos)
	}
	// Edge stops movement too.
	e := place(t, w, "e", TeamA, Pos{7, 1})
	w.Step(map[PlayerID]Intent{"e": move(N)})
	if e.Pos != (Pos{7, 0}) {
		t.Fatalf("edge pos = %v, want {7 0}", e.Pos)
	}
	// Walls block movement.
	w.Walls[Pos{6, 4}] = Wall{Owner: "x", Team: TeamB, Hits: 1}
	f := place(t, w, "f", TeamA, Pos{6, 6})
	w.Step(map[PlayerID]Intent{"f": move(N)})
	if f.Pos != (Pos{6, 5}) {
		t.Fatalf("wall pos = %v, want {6 5}", f.Pos)
	}
}

func TestThrowHitsFirstEnemy(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{7, 2})
	mate := place(t, w, "mate", TeamA, Pos{8, 2})
	b1 := place(t, w, "b1", TeamB, Pos{9, 2})
	b2 := place(t, w, "b2", TeamB, Pos{10, 2})
	ev := w.Step(map[PlayerID]Intent{"a": throw(E)})
	if mate.OutLeft != 0 {
		t.Fatal("teammate tagged without friendly fire")
	}
	if b1.OutLeft != 10 {
		t.Fatalf("b1.OutLeft = %d, want 10", b1.OutLeft)
	}
	if b2.OutLeft != 0 {
		t.Fatal("b2 tagged; projectile should stop at first enemy")
	}
	if a.Ammo != 9 || a.CooldownLeft != 2 {
		t.Fatalf("ammo=%d cooldown=%d, want 9 2", a.Ammo, a.CooldownLeft)
	}
	if len(w.Projectiles) != 0 {
		t.Fatalf("projectile should vanish on hit, have %d", len(w.Projectiles))
	}
	want := []Event{{Tick: 0, Kind: Tagged, Player: "b1", Other: "a", Team: TeamB, Pos: Pos{9, 2}}}
	if !reflect.DeepEqual(ev, want) {
		t.Fatalf("events = %+v, want %+v", ev, want)
	}
}

func TestThrowDodgedByMovingOutOfPath(t *testing.T) {
	w := newTestWorld(t, nil)
	place(t, w, "a", TeamA, Pos{7, 2})
	b := place(t, w, "b", TeamB, Pos{9, 2})
	ev := w.Step(map[PlayerID]Intent{"a": throw(E), "b": move(S)})
	if b.OutLeft != 0 || len(ev) != 0 {
		t.Fatalf("b tagged despite dodging: out=%d events=%v", b.OutLeft, ev)
	}
	if b.Pos != (Pos{9, 3}) {
		t.Fatalf("b.Pos = %v", b.Pos)
	}
	if len(w.Projectiles) != 1 || w.Projectiles[0].Pos != (Pos{9, 2}) || w.Projectiles[0].Traveled != 2 {
		t.Fatalf("projectiles = %+v, want one at {9 2} traveled 2", w.Projectiles)
	}
}

func TestProjectileIgnoresSafeLine(t *testing.T) {
	w := newTestWorld(t, nil)
	place(t, w, "a", TeamA, Pos{12, 2})
	onLine := place(t, w, "b1", TeamB, Pos{14, 2})
	behind := place(t, w, "b2", TeamB, Pos{16, 2})
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if onLine.OutLeft != 0 {
		t.Fatal("player on safe line was tagged")
	}
	w.Step(nil)
	if onLine.OutLeft != 0 {
		t.Fatal("player on safe line was tagged")
	}
	if behind.OutLeft != 10 {
		t.Fatalf("player behind safe line not tagged: out=%d", behind.OutLeft)
	}
}

func TestCannotThrowFromSafeLine(t *testing.T) {
	w := newTestWorld(t, nil)
	a := place(t, w, "a", TeamA, Pos{14, 2})
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if len(w.Projectiles) != 0 || a.Ammo != 10 || a.CooldownLeft != 0 {
		t.Fatalf("throw from safe line allowed: projectiles=%d ammo=%d cd=%d", len(w.Projectiles), a.Ammo, a.CooldownLeft)
	}
}

func TestCooldownAndAmmo(t *testing.T) {
	t.Run("ammo", func(t *testing.T) {
		w := newTestWorld(t, func(c *config.Config) { c.Projectile.CooldownTicks = 0 })
		a := place(t, w, "a", TeamA, Pos{7, 12})
		for i := 0; i < 10; i++ {
			w.Step(map[PlayerID]Intent{"a": throw(N)})
			if a.Ammo != 9-i {
				t.Fatalf("throw %d: ammo = %d, want %d", i+1, a.Ammo, 9-i)
			}
		}
		w.Step(map[PlayerID]Intent{"a": throw(N)})
		for _, p := range w.Projectiles {
			if p.Traveled <= 2 {
				t.Fatalf("11th throw launched a projectile: %+v", p)
			}
		}
		if a.Ammo != 0 {
			t.Fatalf("ammo = %d, want 0", a.Ammo)
		}
	})
	t.Run("cooldown", func(t *testing.T) {
		w := newTestWorld(t, nil)
		a := place(t, w, "a", TeamA, Pos{7, 12})
		w.Step(map[PlayerID]Intent{"a": throw(N)})
		w.Step(map[PlayerID]Intent{"a": throw(N)})
		if a.Ammo != 9 {
			t.Fatalf("cooldown did not block: ammo = %d, want 9", a.Ammo)
		}
		w.Step(map[PlayerID]Intent{"a": throw(N)})
		if a.Ammo != 8 {
			t.Fatalf("throw after cooldown refused: ammo = %d, want 8", a.Ammo)
		}
	})
	t.Run("carrierCannotThrow", func(t *testing.T) {
		w := newTestWorld(t, nil)
		a := place(t, w, "a", TeamA, Pos{7, 12})
		a.HasFlag = true
		w.Step(map[PlayerID]Intent{"a": throw(N)})
		if a.Ammo != 10 || len(w.Projectiles) != 0 {
			t.Fatal("flag carrier was allowed to throw")
		}
	})
}

func TestTagSendsOutAndRespawns(t *testing.T) {
	w := newTestWorld(t, nil)
	place(t, w, "a", TeamA, Pos{7, 2})
	b := place(t, w, "b", TeamB, Pos{9, 2})
	b.Ammo = 3
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if b.OutLeft != 10 {
		t.Fatalf("OutLeft = %d, want 10", b.OutLeft)
	}
	for i := 0; i < 9; i++ {
		w.Step(nil)
	}
	if b.OutLeft != 1 {
		t.Fatalf("after 9 ticks OutLeft = %d, want 1", b.OutLeft)
	}
	w.Step(nil)
	if b.OutLeft != 0 {
		t.Fatalf("after 10 ticks OutLeft = %d, want 0", b.OutLeft)
	}
	if !w.Map.BaseB.Contains(b.Pos) {
		t.Fatalf("respawned at %v, not in own base %+v", b.Pos, w.Map.BaseB)
	}
	if b.Ammo != 10 {
		t.Fatalf("respawn ammo = %d, want 10", b.Ammo)
	}
}

func TestOutPlayerIntentsIgnored(t *testing.T) {
	w := newTestWorld(t, nil)
	b := place(t, w, "b", TeamB, Pos{10, 2})
	b.OutLeft = 5
	a := place(t, w, "a", TeamA, Pos{9, 2})
	w.Step(map[PlayerID]Intent{"b": throw(W_)})
	if len(w.Projectiles) != 0 || b.Ammo != 10 {
		t.Fatal("out player threw")
	}
	w.Step(map[PlayerID]Intent{"b": move(N)})
	if b.Pos != (Pos{10, 2}) {
		t.Fatalf("out player moved to %v", b.Pos)
	}
	// Out players do not block movement or projectiles.
	w.Step(map[PlayerID]Intent{"a": move(E)})
	if a.Pos != (Pos{10, 2}) {
		t.Fatalf("out player blocked movement: a=%v", a.Pos)
	}
}

func TestAmmoRefillsOnlyInOwnBase(t *testing.T) {
	w := newTestWorld(t, nil)
	home := place(t, w, "home", TeamA, Pos{1, 6})
	enemyBase := place(t, w, "enemy", TeamA, Pos{27, 6})
	field := place(t, w, "field", TeamA, Pos{7, 6})
	full := place(t, w, "full", TeamA, Pos{2, 7})
	home.Ammo, enemyBase.Ammo, field.Ammo = 5, 5, 5
	w.Step(nil)
	if home.Ammo != 6 {
		t.Fatalf("home ammo = %d, want 6", home.Ammo)
	}
	if enemyBase.Ammo != 5 || field.Ammo != 5 {
		t.Fatalf("refilled outside own base: enemy=%d field=%d", enemyBase.Ammo, field.Ammo)
	}
	if full.Ammo != 10 {
		t.Fatalf("ammo exceeded max: %d", full.Ammo)
	}
}

func TestProjectileBreaksWall(t *testing.T) {
	w := newTestWorld(t, nil)
	place(t, w, "a", TeamA, Pos{7, 2})
	def := place(t, w, "d", TeamB, Pos{12, 12})
	def.WallsPlaced = 1
	w.Walls[Pos{9, 2}] = Wall{Owner: "d", Team: TeamB, Hits: 1}
	b := place(t, w, "b", TeamB, Pos{10, 2})
	ev := w.Step(map[PlayerID]Intent{"a": throw(E)})
	if _, ok := w.Walls[Pos{9, 2}]; ok {
		t.Fatal("wall not broken")
	}
	if def.WallsPlaced != 0 {
		t.Fatalf("WallsPlaced = %d, want 0", def.WallsPlaced)
	}
	if b.OutLeft != 0 || len(w.Projectiles) != 0 {
		t.Fatal("projectile should vanish on breaking a wall")
	}
	want := []Event{{Tick: 0, Kind: WallBroken, Player: "d", Other: "a", Team: TeamB, Pos: Pos{9, 2}}}
	if !reflect.DeepEqual(ev, want) {
		t.Fatalf("events = %+v, want %+v", ev, want)
	}
}

func TestProjectileRangeAndObstacle(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Projectile.RangeCells = 3 })
	place(t, w, "a", TeamA, Pos{7, 2})
	far := place(t, w, "far", TeamB, Pos{11, 2})
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	w.Step(nil)
	if far.OutLeft != 0 || len(w.Projectiles) != 0 {
		t.Fatalf("projectile exceeded range: out=%d proj=%+v", far.OutLeft, w.Projectiles)
	}
	place(t, w, "c", TeamA, Pos{7, 4})
	behind := place(t, w, "behind", TeamB, Pos{9, 4})
	w.Map.Obstacle[4][8] = true
	w.Step(map[PlayerID]Intent{"c": throw(E)})
	if behind.OutLeft != 0 || len(w.Projectiles) != 0 {
		t.Fatal("projectile passed through obstacle")
	}
}

func TestAddPlayerSpawn(t *testing.T) {
	w := newTestWorld(t, nil)
	d := w.AddPlayer(Player{ID: "d", Team: TeamA, Role: Defender})
	if d == nil || !w.Map.BaseA.Contains(d.Pos) || d.Pos == w.Map.FlagA || cheb(d.Pos, w.Map.FlagA) > 1 {
		t.Fatalf("defender spawn %+v, flag %v", d, w.Map.FlagA)
	}
	if d.Ammo != 10 {
		t.Fatalf("ammo = %d, want 10", d.Ammo)
	}
	at := w.AddPlayer(Player{ID: "at", Team: TeamB, Role: Attacker})
	if at == nil || at.Pos.X != w.Map.BaseB.X0 || !w.Map.BaseB.Contains(at.Pos) {
		t.Fatalf("attacker spawn %+v, base %+v", at, w.Map.BaseB)
	}
	if w.AddPlayer(Player{ID: "d", Team: TeamA}) != nil {
		t.Fatal("duplicate ID accepted")
	}
	// Fill Team A's base (25 cells minus the flag cell = 24, d included).
	for i := 0; i < 23; i++ {
		p := w.AddPlayer(Player{ID: PlayerID("f" + string(rune('a'+i))), Team: TeamA})
		if p == nil || !w.Map.BaseA.Contains(p.Pos) {
			t.Fatalf("player %d not spawned in base: %+v", i, p)
		}
	}
	// The next one spills to the nearest cell outside the base.
	sp := w.AddPlayer(Player{ID: "spill", Team: TeamA})
	if sp == nil || w.Map.BaseA.Contains(sp.Pos) || sp.Pos != (Pos{0, 4}) {
		t.Fatalf("spill spawn = %+v, want {0 4} (distance 1, lowest y then x)", sp)
	}
	w.RemovePlayer("d")
	d2 := w.AddPlayer(Player{ID: "d2", Team: TeamA, Role: Defender})
	if w.Players["d"] != nil || d2 == nil || !w.Map.BaseA.Contains(d2.Pos) {
		t.Fatal("RemovePlayer did not free the base cell")
	}
}

func TestStepDeterministic(t *testing.T) {
	run := func() *World {
		w := newTestWorld(t, func(c *config.Config) { c.Map.Obstacles.Density = 0.1 })
		for i := 0; i < 20; i++ {
			team := Team(i % 2)
			w.AddPlayer(Player{ID: PlayerID(string(rune('a' + i))), Team: team, Role: Role(i / 2 % 2)})
		}
		for tick := 0; tick < 40; tick++ {
			in := map[PlayerID]Intent{}
			for i := 0; i < 20; i++ {
				k := IntentKind((i + tick) % 3)
				in[PlayerID(string(rune('a'+i)))] = Intent{Kind: k, Dir: Dir((i*3 + tick) % 8)}
			}
			w.Step(in)
		}
		return w
	}
	a, b := run(), run()
	for id, p := range a.Players {
		if !reflect.DeepEqual(*p, *b.Players[id]) {
			t.Fatalf("player %s differs: %+v vs %+v", id, *p, *b.Players[id])
		}
	}
	if !reflect.DeepEqual(a.Projectiles, b.Projectiles) {
		t.Fatal("projectiles differ")
	}
}

func TestIdlePlayersCost(t *testing.T) {
	cfg := config.Defaults()
	cfg.Map.BaseSize = 15
	w, err := NewWorld(&cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		id := PlayerID([]byte{byte('a' + i/26), byte('a' + i%26), byte('0' + i%10)})
		if w.AddPlayer(Player{ID: id, Team: Team(i % 2), Role: Role(i / 2 % 2)}) == nil {
			t.Fatalf("could not add player %d", i)
		}
	}
	const n = 50
	start := time.Now()
	for i := 0; i < n; i++ {
		w.Step(nil)
	}
	per := time.Since(start) / n
	if per > 5*time.Millisecond {
		t.Fatalf("Step with 200 idle players took %v, want < 5ms", per)
	}
}

func cheb(a, b Pos) int {
	dx, dy := a.X-b.X, a.Y-b.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return max(dx, dy)
}

func TestWallWithTwoHitsSurvivesFirst(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Projectile.CooldownTicks = 0 })
	place(t, w, "a", TeamA, Pos{7, 2})
	def := place(t, w, "d", TeamB, Pos{12, 12})
	def.WallsPlaced = 1
	w.Walls[Pos{9, 2}] = Wall{Owner: "d", Team: TeamB, Hits: 2}
	ev := w.Step(map[PlayerID]Intent{"a": throw(E)})
	wall, ok := w.Walls[Pos{9, 2}]
	if !ok || wall.Hits != 1 {
		t.Fatalf("after first hit: wall=%+v ok=%v, want Hits 1", wall, ok)
	}
	if len(ev) != 0 || len(w.Projectiles) != 0 || def.WallsPlaced != 1 {
		t.Fatalf("first hit: events=%v projectiles=%d walls=%d", ev, len(w.Projectiles), def.WallsPlaced)
	}
	ev = w.Step(map[PlayerID]Intent{"a": throw(E)})
	if _, ok := w.Walls[Pos{9, 2}]; ok {
		t.Fatal("wall survived second hit")
	}
	if def.WallsPlaced != 0 || len(ev) != 1 || ev[0].Kind != WallBroken {
		t.Fatalf("second hit: walls=%d events=%v", def.WallsPlaced, ev)
	}
}

func TestSpawnSpillsOutsideBaseWhenFull(t *testing.T) {
	cfg := config.Defaults()
	w, err := NewWorld(&cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[Pos]bool{}
	for i := 0; i < 100; i++ {
		id := PlayerID([]byte{byte('a' + i/26), byte('a' + i%26)})
		p := w.AddPlayer(Player{ID: id, Team: TeamA, Role: Attacker})
		if p == nil {
			t.Fatalf("player %d: AddPlayer returned nil", i)
		}
		if p.Pos.X >= w.Map.SafeX0 {
			t.Fatalf("player %d spawned at %v, on or past safe line", i, p.Pos)
		}
		if seen[p.Pos] || w.Map.Obstacle[p.Pos.Y][p.Pos.X] || p.Pos == w.Map.FlagA {
			t.Fatalf("player %d spawned at bad cell %v", i, p.Pos)
		}
		seen[p.Pos] = true
	}
	// Respawn uses the same rule: an out player comes back on its own side.
	p := w.Players["aa"]
	p.OutLeft = 1
	w.Step(nil)
	if p.Out() || p.Pos.X >= w.Map.SafeX0 {
		t.Fatalf("respawn failed: out=%d pos=%v", p.OutLeft, p.Pos)
	}
}

func TestAddPlayerNilWhenSideFull(t *testing.T) {
	cfg := config.Defaults()
	// Smallest valid map: width 11, height 5, base 3, safe line x=5.
	// Own side is x 0..4: 25 cells minus the flag = 24 spawn cells.
	cfg.Map = config.MapConfig{Width: 11, Height: 5, SafeLineWidth: 1, BaseSize: 3,
		Obstacles: config.Obstacles{Generate: "scatter", Density: 0}}
	w, err := NewWorld(&cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		if w.AddPlayer(Player{ID: PlayerID(string(rune('a' + i))), Team: TeamA}) == nil {
			t.Fatalf("player %d: nil before side full", i)
		}
	}
	if w.AddPlayer(Player{ID: "full", Team: TeamA}) != nil {
		t.Fatal("AddPlayer succeeded with own side full")
	}
	if w.AddPlayer(Player{ID: "other", Team: TeamB}) == nil {
		t.Fatal("team B blocked by team A being full")
	}
}

func TestEnemyStepsOntoRestingProjectile(t *testing.T) {
	w := newTestWorld(t, nil)
	place(t, w, "a", TeamA, Pos{7, 2})
	b := place(t, w, "b", TeamB, Pos{10, 2})
	w.Step(map[PlayerID]Intent{"a": throw(E)})
	if len(w.Projectiles) != 1 || w.Projectiles[0].Pos != (Pos{9, 2}) || b.Out() {
		t.Fatalf("setup: projectiles=%+v b.out=%d", w.Projectiles, b.OutLeft)
	}
	ev := w.Step(map[PlayerID]Intent{"b": move(W_)})
	want := []Event{{Tick: 1, Kind: Tagged, Player: "b", Other: "a", Team: TeamB, Pos: Pos{9, 2}}}
	if !reflect.DeepEqual(ev, want) {
		t.Fatalf("events = %+v, want %+v", ev, want)
	}
	if len(w.Projectiles) != 0 {
		t.Fatal("projectile should vanish on hit")
	}
}

func TestNonPiercingHitsOnlyOneStackedEnemy(t *testing.T) {
	w := newTestWorld(t, func(c *config.Config) { c.Movement.Stackable = true })
	place(t, w, "a", TeamA, Pos{7, 2})
	b2 := place(t, w, "b2", TeamB, Pos{9, 2})
	b1 := place(t, w, "b1", TeamB, Pos{9, 2})
	ev := w.Step(map[PlayerID]Intent{"a": throw(E)})
	if len(ev) != 1 || ev[0].Kind != Tagged || ev[0].Player != "b1" {
		t.Fatalf("events = %+v, want exactly one Tagged for b1", ev)
	}
	if !b1.Out() || b2.Out() {
		t.Fatalf("b1.out=%d b2.out=%d, want only b1 out", b1.OutLeft, b2.OutLeft)
	}
}
