package stats

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStatsAppendsValidJSONLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "stats.jsonl")
	l := NewLogger(p)
	for i := 1; i <= 2; i++ {
		err := l.Append(RoundStats{Round: i, Winner: "red", Scores: [2]int{3, 1},
			CapturesByFaction: map[string]int{"A": 1}, TagsByPlayer: map[string]int{"p": 2},
			MeanCarryTicks: 1.5, WallsPlaced: 4, WallsBroken: 2})
		if err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		n++
		if int(m["round"].(float64)) != n || m["wallsPlaced"] == nil || m["meanCarryTicks"] == nil {
			t.Fatalf("line %d: %v", n, m)
		}
	}
	if n != 2 {
		t.Fatalf("lines %d", n)
	}
}
