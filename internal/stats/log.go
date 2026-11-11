// Package stats appends per-round statistics as JSON lines.
package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// RoundStats summarizes one finished round.
type RoundStats struct {
	Round             int            `json:"round"`
	Winner            string         `json:"winner"`
	Scores            [2]int         `json:"scores"`
	CapturesByFaction map[string]int `json:"capturesByFaction"`
	TagsByPlayer      map[string]int `json:"tagsByPlayer"`
	MeanCarryTicks    float64        `json:"meanCarryTicks"`
	WallsPlaced       int            `json:"wallsPlaced"`
	WallsBroken       int            `json:"wallsBroken"`
}

// Logger appends RoundStats to a JSON-lines file.
type Logger struct {
	mu   sync.Mutex
	path string
}

func NewLogger(path string) *Logger { return &Logger{path: path} }

// Append writes one compact JSON object plus newline.
func (l *Logger) Append(s RoundStats) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
