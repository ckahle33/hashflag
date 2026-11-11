package factions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoardMissingFileIsEmpty(t *testing.T) {
	b, err := LoadBoard(filepath.Join(t.TempDir(), "board.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Top(10)) != 0 {
		t.Fatal("expected empty board")
	}
}

func TestBoardCorruptFileBackedUp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	os.WriteFile(p+".bad", []byte("older"), 0o644)
	os.WriteFile(p, []byte("{not json"), 0o644)
	b, err := LoadBoard(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Top(10)) != 0 {
		t.Fatal("expected empty board")
	}
	got, err := os.ReadFile(p + ".bad")
	if err != nil || string(got) != "{not json" {
		t.Fatalf("bad file: %q %v", got, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("corrupt file should have been renamed")
	}
}

func TestBoardRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, _ := LoadBoard(p)
	b.AddCapture("Red")
	b.AddCapture("Red")
	b.AddCapture("Blue")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	b2, err := LoadBoard(p)
	if err != nil {
		t.Fatal(err)
	}
	top := b2.Top(10)
	if len(top) != 2 || top[0] != (Entry{"Red", 2}) || top[1] != (Entry{"Blue", 1}) {
		t.Fatalf("got %+v", top)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func TestDissolvedFactionStaysOnBoard(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, _ := LoadBoard(p)
	b.AddCapture("Ghosts")
	b.Save()
	b2, _ := LoadBoard(p)
	top := b2.Top(5)
	if len(top) != 1 || top[0].Name != "Ghosts" {
		t.Fatalf("got %+v", top)
	}
}

func TestBoardSaveIsAtomic(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "board.json")
	b, _ := LoadBoard(p)
	b.AddCapture("A")
	for i := 0; i < 3; i++ {
		if err := b.Save(); err != nil {
			t.Fatal(err)
		}
	}
	ents, _ := os.ReadDir(d)
	if len(ents) != 1 || ents[0].Name() != "board.json" {
		t.Fatalf("leftover files: %v", ents)
	}
}

func TestTopOrderingAndLimit(t *testing.T) {
	b, _ := LoadBoard(filepath.Join(t.TempDir(), "b.json"))
	for _, n := range []string{"Zed", "Amy", "Zed", "Bob", "Bob", "Cat"} {
		b.AddCapture(n)
	}
	top := b.Top(3)
	want := []Entry{{"Bob", 2}, {"Zed", 2}, {"Amy", 1}}
	if len(top) != 3 {
		t.Fatalf("got %+v", top)
	}
	for i := range want {
		if top[i] != want[i] {
			t.Fatalf("got %+v want %+v", top, want)
		}
	}
	if len(b.Top(0)) != 0 {
		t.Fatal("Top(0)")
	}
}
