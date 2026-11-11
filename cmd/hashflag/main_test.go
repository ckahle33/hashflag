package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hashflag/internal/config"
)

func TestOpenBoardHealthyUsesPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := openBoard(p)
	if err != nil || b == nil {
		t.Fatalf("openBoard = %v, %v", b, err)
	}
	b.AddCapture("x")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("board not saved at the requested path: %v", err)
	}
}

func TestOpenBoardUnreadableFallsBackWithoutTouchingOriginal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "board.json")
	// A directory where the file should be: reading it fails and it is not
	// "missing", so LoadBoard returns an error.
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := openBoard(p)
	if err == nil {
		t.Fatal("want the load error reported")
	}
	if b == nil {
		t.Fatal("want a usable fallback board")
	}
	b.AddCapture("x")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		t.Fatalf("original path was touched: %v %v", st, err)
	}
	if _, err := os.Stat(p + ".recovered"); err != nil {
		t.Fatalf("recovered board not written: %v", err)
	}
}

func TestOpenBoardCorruptFileWhoseBackupFailsIsPreserved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "board.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at board.json.bad makes the backup rename fail.
	if err := os.MkdirAll(filepath.Join(p+".bad", "keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := openBoard(p)
	if err == nil || b == nil {
		t.Fatalf("openBoard = %v, %v; want fallback board and an error", b, err)
	}
	b.AddCapture("x")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "{not json" {
		t.Fatalf("original file changed: %q %v", got, err)
	}
}

func TestCheckRunnableRejectsBadGeometry(t *testing.T) {
	cfg := config.Defaults()
	cfg.Map.Width = 20
	cfg.Map.BaseSize = 9
	if err := cfg.Validate(); err != nil {
		t.Fatalf("setup: Validate should accept this config: %v", err)
	}
	err := checkRunnable(&cfg)
	if err == nil || !strings.Contains(err.Error(), "cannot generate a map") {
		t.Fatalf("checkRunnable = %v, want a map generation error", err)
	}
}

func TestCheckRunnableAcceptsEveryPreset(t *testing.T) {
	files, err := filepath.Glob("../../configs/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no presets found: %v", err)
	}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		cfg, err := config.Resolve("../../configs", id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if err := checkRunnable(cfg); err != nil {
			t.Errorf("preset %s: %v", id, err)
		}
	}
}
