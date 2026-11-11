package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, dir, id, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsMatchSpec(t *testing.T) {
	c := Defaults()
	if c.TickMs != 1000 || c.RoundTicks != 300 || c.MaxPerTeam != 100 {
		t.Errorf("top-level: %+v", c)
	}
	if c.Ammo.Max != 10 || c.Ammo.RefillPerTick != 1 || c.Ammo.RefillWhere != "ownBase" {
		t.Errorf("ammo: %+v", c.Ammo)
	}
	p := c.Projectile
	if p.CellsPerTick != 2 || p.RangeCells != 14 || p.CooldownTicks != 2 || p.Pierce || p.FriendlyFire {
		t.Errorf("projectile: %+v", p)
	}
	if c.Tag.OutTicks != 10 || c.Tag.RespawnAt != "ownBase" || !c.Tag.SafeOnSafeLine {
		t.Errorf("tag: %+v", c.Tag)
	}
	m := c.Map
	if m.SafeLineWidth != 2 || m.Width != 120 || m.Height != 50 || m.BaseSize != 9 {
		t.Errorf("map: %+v", m)
	}
	if m.Obstacles.Generate != "scatter" || m.Obstacles.Density != 0.12 {
		t.Errorf("obstacles: %+v", m.Obstacles)
	}
	f := c.Flag
	if f.AutoReturnTicks == nil || *f.AutoReturnTicks != 30 || f.CarrierCanThrow || f.CarrierCellsPerTick != 1 || f.CaptureNeedsOwnFlagHome {
		t.Errorf("flag: %+v", f)
	}
	w := c.Walls
	if !w.Enabled || w.PerPlayer != 1 || w.TeamCap != 20 || w.HitsToBreak != 1 || w.DecayTicks != nil ||
		w.PlaceRangeCells != 1 || w.PlaceZone != "ownBase" || !w.MustLeavePath {
		t.Errorf("walls: %+v", w)
	}
	if c.TeamBalance.MaxRoleSkewPct != 60 || !c.TeamBalance.AutoBalanceTeams {
		t.Errorf("teamBalance: %+v", c.TeamBalance)
	}
	if c.Win.Mode != "mostCapturesAtTime" || c.Win.Captures != 3 {
		t.Errorf("win: %+v", c.Win)
	}
	mv := c.Movement
	if mv.CellsPerTick != 1 || !mv.Diagonals || mv.Stackable {
		t.Errorf("movement: %+v", mv)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("defaults invalid: %v", err)
	}
}

func TestResolveExtendsOverrides(t *testing.T) {
	dir := t.TempDir()
	writeCfg(t, dir, "standard", `{"id":"standard","name":"Standard"}`)
	writeCfg(t, dir, "big", `{"id":"big","name":"Big","extends":"standard","walls":{"teamCap":40}}`)
	c, err := Resolve(dir, "big")
	if err != nil {
		t.Fatal(err)
	}
	if c.Walls.TeamCap != 40 || c.Walls.PerPlayer != 1 || c.Walls.HitsToBreak != 1 || !c.Walls.Enabled {
		t.Errorf("walls: %+v", c.Walls)
	}
}

func TestResolveNullAutoReturn(t *testing.T) {
	dir := t.TempDir()
	writeCfg(t, dir, "a", `{"id":"a","name":"A","flag":{"autoReturnTicks":null}}`)
	c, err := Resolve(dir, "a")
	if err != nil {
		t.Fatal(err)
	}
	if c.Flag.AutoReturnTicks != nil {
		t.Errorf("want nil, got %d", *c.Flag.AutoReturnTicks)
	}
}

func TestResolveCycleDetected(t *testing.T) {
	dir := t.TempDir()
	writeCfg(t, dir, "a", `{"id":"a","name":"A","extends":"b"}`)
	writeCfg(t, dir, "b", `{"id":"b","name":"B","extends":"a"}`)
	_, err := Resolve(dir, "a")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestValidateBounds(t *testing.T) {
	c := Defaults()
	c.TickMs = 100
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "tickMs") {
		t.Errorf("tickMs: %v", err)
	}
	c = Defaults()
	c.MaxPerTeam = 101
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "maxPerTeam") {
		t.Errorf("maxPerTeam: %v", err)
	}
}

func TestUnimplementedFieldRejected(t *testing.T) {
	cases := map[string]string{
		"map.obstacles.generate":       `{"map":{"obstacles":{"generate":"mazeish"}}}`,
		"map.obstacles.rows":           `{"map":{"obstacles":{"rows":["#"]}}}`,
		"roles.attacker.spawnZone":     `{"roles":{"attacker":{"spawnZone":"midfield"}}}`,
		"flag.captureNeedsOwnFlagHome": `{"flag":{"captureNeedsOwnFlagHome":true}}`,
		"walls.decayTicks":             `{"walls":{"decayTicks":5}}`,
		"walls.placeZone":              `{"walls":{"placeZone":"ownSide"}}`,
		"ammo.refillWhere":             `{"ammo":{"refillWhere":"everywhere"}}`,
		"tag.respawnAt":                `{"tag":{"respawnAt":"ownFlag"}}`,
	}
	for field, body := range cases {
		dir := t.TempDir()
		full := `{"id":"x","name":"X",` + body[1:]
		writeCfg(t, dir, "x", full)
		_, err := Resolve(dir, "x")
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("%s: got %v", field, err)
		}
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	dir := t.TempDir()
	writeCfg(t, dir, "x", `{"id":"x","name":"X","bogus":1}`)
	if _, err := Resolve(dir, "x"); err == nil {
		t.Fatal("want error")
	}
}

func TestRoleOverrideApplied(t *testing.T) {
	dir := t.TempDir()
	writeCfg(t, dir, "x", `{"id":"x","name":"X","roles":{"defender":{"ammo":{"max":20}}}}`)
	c, err := Resolve(dir, "x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Roles.Defender == nil || c.Roles.Defender.Ammo == nil || *c.Roles.Defender.Ammo.Max != 20 {
		t.Errorf("roles: %+v", c.Roles)
	}
	writeCfg(t, dir, "y", `{"id":"y","name":"Y","roles":{"defender":{"ammo":{"max":500}}}}`)
	if _, err := Resolve(dir, "y"); err == nil {
		t.Error("want bounds error for role ammo.max")
	}
}

func TestLoadStandardPreset(t *testing.T) {
	c, err := Resolve("../../configs", "standard")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "standard" || c.Name != "Standard" || c.TickMs != 1000 {
		t.Errorf("%+v", c)
	}
}
