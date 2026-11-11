package factions

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"hashflag/internal/game"
)

// Faction is a named group of players on one team.
type Faction struct {
	ID      string
	Name    string
	Team    game.Team
	Glyph   string
	Founder game.PlayerID
	Members []game.PlayerID
}

// Registry tracks live factions. Not safe for concurrent use; the server room
// serializes access.
type Registry struct {
	rng      *rand.Rand
	next     int
	factions map[string]*Faction
	of       map[game.PlayerID]string
}

// NewRegistry returns an empty registry drawing randomness from rng.
func NewRegistry(rng *rand.Rand) *Registry {
	return &Registry{rng: rng, factions: map[string]*Faction{}, of: map[game.PlayerID]string{}}
}

// Create makes a new faction with founder as its first member. A founder
// already in a faction leaves it first.
func (r *Registry) Create(founder game.PlayerID, team game.Team) *Faction {
	r.Leave(founder)
	r.next++
	f := &Faction{
		ID:      fmt.Sprintf("f%d", r.next),
		Name:    GenerateName(r.rng),
		Team:    team,
		Glyph:   r.pickGlyph(),
		Founder: founder,
		Members: []game.PlayerID{founder},
	}
	r.factions[f.ID] = f
	r.of[founder] = f.ID
	return f
}

// pickGlyph returns a random unused glyph, or the least-used one if all are taken.
func (r *Registry) pickGlyph() string {
	use := map[string]int{}
	for _, f := range r.factions {
		use[f.Glyph]++
	}
	var free []string
	for _, g := range glyphPool {
		if use[g] == 0 {
			free = append(free, g)
		}
	}
	if len(free) > 0 {
		return free[r.rng.Intn(len(free))]
	}
	best, bestN := glyphPool[0], use[glyphPool[0]]
	for _, g := range glyphPool {
		if use[g] < bestN {
			best, bestN = g, use[g]
		}
	}
	return best
}

// Join adds p to a faction, leaving any previous one. It fails if the faction
// does not exist or its side is full.
func (r *Registry) Join(factionID string, p game.PlayerID, teamCounts [2]int, max int) error {
	f := r.factions[factionID]
	if f == nil {
		return errors.New("no such faction")
	}
	if teamCounts[f.Team] >= max {
		return errors.New("team is full")
	}
	if r.of[p] == factionID {
		return nil
	}
	r.Leave(p)
	f.Members = append(f.Members, p)
	r.of[p] = f.ID
	return nil
}

// Leave removes p from their faction, dissolving it if empty and passing the
// founder role to the earliest remaining member.
func (r *Registry) Leave(p game.PlayerID) {
	id, ok := r.of[p]
	if !ok {
		return
	}
	delete(r.of, p)
	f := r.factions[id]
	for i, m := range f.Members {
		if m == p {
			f.Members = append(f.Members[:i:i], f.Members[i+1:]...)
			break
		}
	}
	if len(f.Members) == 0 {
		delete(r.factions, id)
		return
	}
	if f.Founder == p {
		f.Founder = f.Members[0]
	}
}

// Reroll gives the faction a new name. Only the founder may, and only while
// alone in the faction.
func (r *Registry) Reroll(factionID string, by game.PlayerID) error {
	f := r.factions[factionID]
	if f == nil {
		return errors.New("no such faction")
	}
	if f.Founder != by {
		return errors.New("only the founder can reroll")
	}
	if len(f.Members) != 1 {
		return errors.New("can only reroll while alone")
	}
	for {
		if n := GenerateName(r.rng); n != f.Name {
			f.Name = n
			return nil
		}
	}
}

// Get returns the faction with id, or nil.
func (r *Registry) Get(id string) *Faction { return r.factions[id] }

// FactionOf returns p's faction, or nil.
func (r *Registry) FactionOf(p game.PlayerID) *Faction { return r.factions[r.of[p]] }

// List returns all live factions sorted by ID numerically (creation order).
func (r *Registry) List() []*Faction {
	out := make([]*Faction, 0, len(r.factions))
	for _, f := range r.factions {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].ID) != len(out[j].ID) {
			return len(out[i].ID) < len(out[j].ID)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
