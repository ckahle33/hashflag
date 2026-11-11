// Package config loads, merges and validates hashflag game configs.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Config struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Extends     string `json:"extends"`

	TickMs     int `json:"tickMs"`
	RoundTicks int `json:"roundTicks"`
	MaxPerTeam int `json:"maxPerTeam"`

	Win         Win         `json:"win"`
	Map         MapConfig   `json:"map"`
	Movement    Movement    `json:"movement"`
	Projectile  Projectile  `json:"projectile"`
	Ammo        Ammo        `json:"ammo"`
	Tag         Tag         `json:"tag"`
	Flag        Flag        `json:"flag"`
	Walls       Walls       `json:"walls"`
	Roles       Roles       `json:"roles"`
	TeamBalance TeamBalance `json:"teamBalance"`
}

type Win struct {
	Mode     string `json:"mode"`
	Captures int    `json:"captures"`
}

// Map is the name used by the brief; MapConfig is the type of the map section.
type MapConfig struct {
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	SafeLineWidth int       `json:"safeLineWidth"`
	BaseSize      int       `json:"baseSize"`
	Obstacles     Obstacles `json:"obstacles"`
}

type Obstacles struct {
	Generate string   `json:"generate"`
	Density  float64  `json:"density"`
	Seed     *int64   `json:"seed"`
	Rows     []string `json:"rows"`
}

type Movement struct {
	CellsPerTick int  `json:"cellsPerTick"`
	Diagonals    bool `json:"diagonals"`
	Stackable    bool `json:"stackable"`
}

type Projectile struct {
	CellsPerTick  int  `json:"cellsPerTick"`
	RangeCells    int  `json:"rangeCells"`
	CooldownTicks int  `json:"cooldownTicks"`
	Pierce        bool `json:"pierce"`
	FriendlyFire  bool `json:"friendlyFire"`
}

type Ammo struct {
	Max           int    `json:"max"`
	RefillPerTick int    `json:"refillPerTick"`
	RefillWhere   string `json:"refillWhere"`
}

type Tag struct {
	OutTicks       int    `json:"outTicks"`
	RespawnAt      string `json:"respawnAt"`
	SafeOnSafeLine bool   `json:"safeOnSafeLine"`
}

type Flag struct {
	CarrierCellsPerTick     int  `json:"carrierCellsPerTick"`
	CarrierCanThrow         bool `json:"carrierCanThrow"`
	AutoReturnTicks         *int `json:"autoReturnTicks"` // nil = dropped flag stays put
	CaptureNeedsOwnFlagHome bool `json:"captureNeedsOwnFlagHome"`
}

type Walls struct {
	Enabled         bool   `json:"enabled"`
	PerPlayer       int    `json:"perPlayer"`
	TeamCap         int    `json:"teamCap"`
	HitsToBreak     int    `json:"hitsToBreak"`
	DecayTicks      *int   `json:"decayTicks"` // nil = never decays
	PlaceRangeCells int    `json:"placeRangeCells"`
	PlaceZone       string `json:"placeZone"`
	MustLeavePath   bool   `json:"mustLeavePath"`
}

type TeamBalance struct {
	MaxRoleSkewPct   int  `json:"maxRoleSkewPct"`
	AutoBalanceTeams bool `json:"autoBalanceTeams"`
}

// Roles holds per-role overrides; nil means no override.
type Roles struct {
	Attacker *RoleOverride `json:"attacker"`
	Defender *RoleOverride `json:"defender"`
}

// RoleOverride fields are pointers so unset values fall back to the global config.
type RoleOverride struct {
	Movement   *MovementOverride   `json:"movement"`
	Projectile *ProjectileOverride `json:"projectile"`
	Ammo       *AmmoOverride       `json:"ammo"`
	Walls      *WallsOverride      `json:"walls"`
	SpawnZone  *string             `json:"spawnZone"` // not implemented in v1
}

type MovementOverride struct {
	CellsPerTick *int  `json:"cellsPerTick"`
	Diagonals    *bool `json:"diagonals"`
	Stackable    *bool `json:"stackable"`
}

type ProjectileOverride struct {
	CellsPerTick  *int  `json:"cellsPerTick"`
	RangeCells    *int  `json:"rangeCells"`
	CooldownTicks *int  `json:"cooldownTicks"`
	Pierce        *bool `json:"pierce"`
	FriendlyFire  *bool `json:"friendlyFire"`
}

type AmmoOverride struct {
	Max           *int    `json:"max"`
	RefillPerTick *int    `json:"refillPerTick"`
	RefillWhere   *string `json:"refillWhere"`
}

type WallsOverride struct {
	Enabled         *bool   `json:"enabled"`
	PerPlayer       *int    `json:"perPlayer"`
	TeamCap         *int    `json:"teamCap"`
	HitsToBreak     *int    `json:"hitsToBreak"`
	DecayTicks      *int    `json:"decayTicks"`
	PlaceRangeCells *int    `json:"placeRangeCells"`
	PlaceZone       *string `json:"placeZone"`
	MustLeavePath   *bool   `json:"mustLeavePath"`
}

func intp(v int) *int { return &v }

// Defaults returns a fresh Config with all schema defaults.
func Defaults() Config {
	return Config{
		TickMs: 1000, RoundTicks: 300, MaxPerTeam: 100,
		Win: Win{Mode: "mostCapturesAtTime", Captures: 3},
		Map: MapConfig{Width: 120, Height: 50, SafeLineWidth: 2, BaseSize: 9,
			Obstacles: Obstacles{Generate: "scatter", Density: 0.12}},
		Movement:    Movement{CellsPerTick: 1, Diagonals: true},
		Projectile:  Projectile{CellsPerTick: 2, RangeCells: 14, CooldownTicks: 2},
		Ammo:        Ammo{Max: 10, RefillPerTick: 1, RefillWhere: "ownBase"},
		Tag:         Tag{OutTicks: 10, RespawnAt: "ownBase", SafeOnSafeLine: true},
		Flag:        Flag{CarrierCellsPerTick: 1, AutoReturnTicks: intp(30)},
		Walls:       Walls{Enabled: true, PerPlayer: 1, TeamCap: 20, HitsToBreak: 1, PlaceRangeCells: 1, PlaceZone: "ownBase", MustLeavePath: true},
		TeamBalance: TeamBalance{MaxRoleSkewPct: 60, AutoBalanceTeams: true},
	}
}

var idRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// Resolve loads <dir>/<id>.json, follows extends chains (deep-merging child
// over parent, root over Defaults), and validates the result.
func Resolve(dir, id string) (*Config, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("config: invalid id %q", id)
	}
	merged, err := loadChain(dir, id, map[string]bool{})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	c := Defaults()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("config %q: %w", id, err)
	}
	if c.ID == "" || c.Name == "" {
		return nil, fmt.Errorf("config %q: id and name are required", id)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("config %q: %w", id, err)
	}
	return &c, nil
}

func loadChain(dir, id string, seen map[string]bool) (map[string]any, error) {
	if seen[id] {
		return nil, fmt.Errorf("config: extends cycle detected at %q", id)
	}
	seen[id] = true
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("config: invalid id %q", id)
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep integers exact (e.g. 64-bit seeds)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("config %q: %w", id, err)
	}
	parent, _ := m["extends"].(string)
	if parent == "" {
		return m, nil
	}
	base, err := loadChain(dir, parent, seen)
	if err != nil {
		return nil, err
	}
	return deepMerge(base, m), nil
}

// deepMerge returns child merged over parent; a null in child overrides.
func deepMerge(parent, child map[string]any) map[string]any {
	out := make(map[string]any, len(parent))
	for k, v := range parent {
		out[k] = v
	}
	for k, cv := range child {
		cm, cok := cv.(map[string]any)
		pm, pok := out[k].(map[string]any)
		if cok && pok {
			out[k] = deepMerge(pm, cm)
		} else {
			out[k] = cv
		}
	}
	return out
}

type validator struct{ errs []error }

func (v *validator) rangeInt(field string, val, lo, hi int) {
	if val < lo || val > hi {
		v.errs = append(v.errs, fmt.Errorf("%s: %d out of range %d..%d", field, val, lo, hi))
	}
}

func (v *validator) enum(field, val string, allowed ...string) {
	for _, a := range allowed {
		if val == a {
			return
		}
	}
	v.errs = append(v.errs, fmt.Errorf("%s: %q not one of %v", field, val, allowed))
}

func (v *validator) unimplemented(field string) {
	v.errs = append(v.errs, fmt.Errorf("%s: not implemented in v1", field))
}

// Validate checks bounds and rejects schema fields not implemented in v1.
func (c *Config) Validate() error {
	v := &validator{}
	if c.ID != "" && !idRe.MatchString(c.ID) {
		v.errs = append(v.errs, fmt.Errorf("id: %q must match %s", c.ID, idRe))
	}
	if len(c.Name) > 40 {
		v.errs = append(v.errs, errors.New("name: longer than 40 characters"))
	}
	if len(c.Description) > 300 {
		v.errs = append(v.errs, errors.New("description: longer than 300 characters"))
	}
	v.rangeInt("tickMs", c.TickMs, 250, 5000)
	v.rangeInt("roundTicks", c.RoundTicks, 30, 3600)
	v.rangeInt("maxPerTeam", c.MaxPerTeam, 1, 100)

	v.enum("win.mode", c.Win.Mode, "firstToCaptures", "mostCapturesAtTime")
	v.rangeInt("win.captures", c.Win.Captures, 1, 20)

	m := c.Map
	v.rangeInt("map.width", m.Width, 20, 300)
	v.rangeInt("map.height", m.Height, 10, 150)
	v.rangeInt("map.safeLineWidth", m.SafeLineWidth, 1, 10)
	v.rangeInt("map.baseSize", m.BaseSize, 3, 30)
	if m.Obstacles.Rows != nil {
		v.unimplemented("map.obstacles.rows")
	}
	if m.Obstacles.Generate != "scatter" {
		v.unimplemented("map.obstacles.generate (only \"scatter\" is supported)")
	}
	if m.Obstacles.Density < 0 || m.Obstacles.Density > 0.4 {
		v.errs = append(v.errs, fmt.Errorf("map.obstacles.density: %v out of range 0..0.4", m.Obstacles.Density))
	}

	validateMovement(v, "movement", c.Movement)
	validateProjectile(v, "projectile", c.Projectile)
	validateAmmo(v, "ammo", c.Ammo)
	validateWalls(v, "walls", c.Walls)

	v.rangeInt("tag.outTicks", c.Tag.OutTicks, 1, 120)
	v.enum("tag.respawnAt", c.Tag.RespawnAt, "ownBase", "ownFlag")
	if c.Tag.RespawnAt != "ownBase" {
		v.unimplemented("tag.respawnAt (only \"ownBase\" is supported)")
	}

	v.rangeInt("flag.carrierCellsPerTick", c.Flag.CarrierCellsPerTick, 1, 3)
	if c.Flag.AutoReturnTicks != nil {
		v.rangeInt("flag.autoReturnTicks", *c.Flag.AutoReturnTicks, 5, 300)
	}
	if c.Flag.CaptureNeedsOwnFlagHome {
		v.unimplemented("flag.captureNeedsOwnFlagHome")
	}

	v.rangeInt("teamBalance.maxRoleSkewPct", c.TeamBalance.MaxRoleSkewPct, 0, 100)

	for name, r := range map[string]*RoleOverride{"attacker": c.Roles.Attacker, "defender": c.Roles.Defender} {
		if r != nil {
			validateRole(v, "roles."+name, c, r)
		}
	}
	return errors.Join(v.errs...)
}

func validateMovement(v *validator, p string, m Movement) {
	v.rangeInt(p+".cellsPerTick", m.CellsPerTick, 1, 3)
}

func validateProjectile(v *validator, p string, x Projectile) {
	v.rangeInt(p+".cellsPerTick", x.CellsPerTick, 1, 6)
	v.rangeInt(p+".rangeCells", x.RangeCells, 2, 60)
	v.rangeInt(p+".cooldownTicks", x.CooldownTicks, 0, 20)
}

func validateAmmo(v *validator, p string, a Ammo) {
	v.rangeInt(p+".max", a.Max, 1, 100)
	v.rangeInt(p+".refillPerTick", a.RefillPerTick, 0, 10)
	v.enum(p+".refillWhere", a.RefillWhere, "ownBase", "anywhereOwnSide", "everywhere")
	if a.RefillWhere != "ownBase" {
		v.unimplemented(p + ".refillWhere (only \"ownBase\" is supported)")
	}
}

func validateWalls(v *validator, p string, w Walls) {
	v.rangeInt(p+".perPlayer", w.PerPlayer, 0, 5)
	v.rangeInt(p+".teamCap", w.TeamCap, 0, 200)
	v.rangeInt(p+".hitsToBreak", w.HitsToBreak, 1, 10)
	v.rangeInt(p+".placeRangeCells", w.PlaceRangeCells, 1, 5)
	v.enum(p+".placeZone", w.PlaceZone, "ownBase", "ownSide", "anywhere")
	if w.PlaceZone != "ownBase" {
		v.unimplemented(p + ".placeZone (only \"ownBase\" is supported)")
	}
	if w.DecayTicks != nil {
		v.unimplemented(p + ".decayTicks")
	}
}

// validateRole applies the override to a copy of the global sections and
// validates the effective values.
func validateRole(v *validator, p string, c *Config, r *RoleOverride) {
	if r.SpawnZone != nil {
		v.unimplemented(p + ".spawnZone")
	}
	if o := r.Movement; o != nil {
		m := c.Movement
		setInt(&m.CellsPerTick, o.CellsPerTick)
		validateMovement(v, p+".movement", m)
	}
	if o := r.Projectile; o != nil {
		x := c.Projectile
		setInt(&x.CellsPerTick, o.CellsPerTick)
		setInt(&x.RangeCells, o.RangeCells)
		setInt(&x.CooldownTicks, o.CooldownTicks)
		validateProjectile(v, p+".projectile", x)
	}
	if o := r.Ammo; o != nil {
		a := c.Ammo
		setInt(&a.Max, o.Max)
		setInt(&a.RefillPerTick, o.RefillPerTick)
		if o.RefillWhere != nil {
			a.RefillWhere = *o.RefillWhere
		}
		validateAmmo(v, p+".ammo", a)
	}
	if o := r.Walls; o != nil {
		w := c.Walls
		setInt(&w.PerPlayer, o.PerPlayer)
		setInt(&w.TeamCap, o.TeamCap)
		setInt(&w.HitsToBreak, o.HitsToBreak)
		setInt(&w.PlaceRangeCells, o.PlaceRangeCells)
		if o.PlaceZone != nil {
			w.PlaceZone = *o.PlaceZone
		}
		w.DecayTicks = o.DecayTicks
		validateWalls(v, p+".walls", w)
	}
}

func setInt(dst *int, src *int) {
	if src != nil {
		*dst = *src
	}
}
