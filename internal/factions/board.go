package factions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// Entry is one leaderboard row, keyed by faction name.
type Entry struct {
	Name     string `json:"name"`
	Captures int    `json:"captures"`
}

// Board is the persistent capture leaderboard. It knows nothing about the
// Registry, so entries survive after a faction dissolves. It is not safe for
// concurrent use; the server room serializes access.
type Board struct {
	path   string
	counts map[string]int
}

type boardFile struct {
	Entries []Entry `json:"entries"`
}

// LoadBoard reads the board at path. A missing file yields an empty board; a
// corrupt file is renamed to path+".bad" (replacing any older one) and an
// empty board is returned. Errors are returned only for unusable conditions.
func LoadBoard(path string) (*Board, error) {
	b := &Board{path: path, counts: map[string]int{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return b, nil
		}
		return nil, err
	}
	var f boardFile
	if err := json.Unmarshal(data, &f); err != nil {
		if err := os.Rename(path, path+".bad"); err != nil {
			return nil, err
		}
		return b, nil
	}
	for _, e := range f.Entries {
		b.counts[e.Name] += e.Captures
	}
	return b, nil
}

// AddCapture records one capture for the named faction.
func (b *Board) AddCapture(factionName string) {
	b.counts[factionName]++
}

// Top returns at most n entries sorted by captures descending, then name.
func (b *Board) Top(n int) []Entry {
	all := make([]Entry, 0, len(b.counts))
	for name, c := range b.counts {
		all = append(all, Entry{Name: name, Captures: c})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Captures != all[j].Captures {
			return all[i].Captures > all[j].Captures
		}
		return all[i].Name < all[j].Name
	})
	if n < 0 {
		n = 0
	}
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// Save writes the board atomically (temp file in the same directory, then rename).
func (b *Board) Save() error {
	data, err := json.Marshal(boardFile{Entries: b.Top(len(b.counts))})
	if err != nil {
		return err
	}
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(b.path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, b.path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
