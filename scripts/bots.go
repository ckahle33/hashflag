//go:build bots

// Command bots is a load harness: N websocket bots play Hashflag against a
// running server. Run with:
//
//	go run -tags bots ./scripts -n 200 -addr ws://127.0.0.1:8097/ws -duration 90s
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Wire frames (subset of internal/server/protocol.go).
type frame struct {
	T       string `json:"t"`
	Token   string `json:"token"`
	Player  string `json:"playerId"`
	Role    string `json:"role"`
	InWorld bool   `json:"inWorld"`
	Round   int    `json:"round"`
	Map     *struct {
		Width, Height  int
		SafeX0, SafeX1 int
		Rows           []string
		BaseA, BaseB   rect
		FlagA, FlagB   xy
	} `json:"map"`
	You *struct {
		ID, Team, Role string
	} `json:"you"`
	Ammo         int `json:"ammo"`
	CooldownLeft int `json:"cooldownLeft"`
	WallsPlaced  int `json:"wallsPlaced"`
	Players      []struct {
		ID      string
		X, Y    int
		Team    string
		Out     bool
		HasFlag bool
	} `json:"players"`
	Walls []struct{ X, Y int } `json:"walls"`
	Flags []struct {
		Team string
		X, Y int
	} `json:"flags"`
	Events []struct {
		Kind   string
		Player string
		Team   string
	} `json:"events"`
	Tick int `json:"tick"`
}

type rect struct{ X0, Y0, X1, Y1 int }
type xy struct{ X, Y int }

func (r rect) has(p pos) bool { return p.x >= r.X0 && p.x <= r.X1 && p.y >= r.Y0 && p.y <= r.Y1 }

type metrics struct {
	connected, frames, intents atomic.Int64
	mu                         sync.Mutex
	samples, onLine            atomic.Int64 // in-world bot samples, and those on the safe line
	captures, tags             map[string]bool
	gaps                       []float64
}

type bot struct {
	n          int
	rng        *rand.Rand
	m          *metrics
	attacker   bool
	token, id  string
	team       string
	round      int
	gotMap     bool
	rows       []string
	base       rect
	homeFlag   pos
	ammo, cd   int
	walls      int
	rolePicked bool
	lastState  time.Time
	dist       []int // BFS distances to distTo over the round's map
	distTo     pos
	safeX0     int
	safeX1     int
	stuck      stuckState
	lastMove   bool // previous intent was a move ...
	lastDir    int  // ... in this direction
}

func (b *bot) run(ctx context.Context, addr string) {
	for ctx.Err() == nil {
		b.session(ctx, addr)
		select {
		case <-ctx.Done():
		case <-time.After(time.Second + time.Duration(b.rng.Intn(1000))*time.Millisecond):
		}
	}
}

func (b *bot) session(ctx context.Context, addr string) {
	c, _, err := websocket.Dial(ctx, addr, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 22)
	b.m.connected.Add(1)
	defer b.m.connected.Add(-1)
	b.rolePicked, b.lastState = false, time.Time{}
	send := func(v map[string]any) {
		buf, _ := json.Marshal(v)
		c.Write(ctx, websocket.MessageText, buf)
	}
	send(map[string]any{"t": "hello", "token": b.token, "name": fmt.Sprintf("bot-%d", b.n)})
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		b.m.frames.Add(1)
		var f frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		switch f.T {
		case "welcome":
			b.token, b.id = f.Token, f.Player
		case "lobby":
			if f.Role == "" && !b.rolePicked {
				b.rolePicked = true
				role := "defender"
				if b.attacker {
					role = "attacker"
				}
				send(map[string]any{"t": "setRole", "role": role})
			}
		case "round":
			b.round, b.gotMap, b.team = f.Round, false, ""
			if f.You != nil && f.Map != nil {
				b.team, b.gotMap, b.rows = f.You.Team, true, f.Map.Rows
				b.safeX0, b.safeX1 = f.Map.SafeX0, f.Map.SafeX1
				b.stuck, b.lastMove, b.dist = stuckState{}, false, nil
				b.base, b.homeFlag = f.Map.BaseA, pos{f.Map.FlagA.X, f.Map.FlagA.Y}
				if b.team == "B" {
					b.base, b.homeFlag = f.Map.BaseB, pos{f.Map.FlagB.X, f.Map.FlagB.Y}
				}
			}
		case "you":
			b.ammo, b.cd, b.walls = f.Ammo, f.CooldownLeft, f.WallsPlaced
		case "state":
			b.onState(&f, send)
		case "roundEnd":
			b.team = "" // the world is emptied until the next round frame
		case "error":
			// tolerated: errors keep the socket open
		}
	}
}

func (b *bot) onState(f *frame, send func(map[string]any)) {
	now := time.Now()
	if b.n < 10 && !b.lastState.IsZero() {
		b.m.mu.Lock()
		b.m.gaps = append(b.m.gaps, float64(now.Sub(b.lastState).Microseconds())/1000)
		b.m.mu.Unlock()
	}
	b.lastState = now
	for _, e := range f.Events {
		key := fmt.Sprintf("%d:%d:%s", b.round, f.Tick, e.Player)
		b.m.mu.Lock()
		switch e.Kind {
		case "captured":
			b.m.captures[key] = true
		case "tagged":
			b.m.tags[key] = true
		}
		b.m.mu.Unlock()
	}
	if b.team == "" || !b.gotMap {
		return
	}
	kind, dir, ok := b.decide(f)
	b.lastMove, b.lastDir = ok && kind == "move", dir
	if ok {
		send(map[string]any{"t": "intent", "kind": kind, "dir": dir})
		b.m.intents.Add(1)
	}
}

func (m *metrics) gapStats() string {
	m.mu.Lock()
	g := append([]float64(nil), m.gaps...)
	m.mu.Unlock()
	if len(g) == 0 {
		return "gaps n/a"
	}
	sort.Float64s(g)
	return fmt.Sprintf("gaps ms min/med/max %.0f/%.0f/%.0f (n=%d)", g[0], g[len(g)/2], g[len(g)-1], len(g))
}

func main() {
	n := flag.Int("n", 200, "number of bots")
	addr := flag.String("addr", "ws://127.0.0.1:8080/ws", "server websocket url")
	dur := flag.Duration("duration", 90*time.Second, "how long to run")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *dur)
	defer cancel()

	m := &metrics{captures: map[string]bool{}, tags: map[string]bool{}}
	var wg sync.WaitGroup
	for i := 0; i < *n; i++ {
		b := &bot{n: i, rng: rand.New(rand.NewSource(int64(i) + time.Now().UnixNano())), m: m}
		b.attacker = b.rng.Float64() < 0.7
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(i) * 10 * time.Millisecond):
			}
			b.run(ctx, *addr)
		}()
	}

	start := time.Now()
	report := func(label string) {
		m.mu.Lock()
		caps, tags := len(m.captures), len(m.tags)
		m.mu.Unlock()
		el := time.Since(start).Seconds()
		safe := 0.0
		if n := m.samples.Load(); n > 0 {
			safe = 100 * float64(m.onLine.Load()) / float64(n)
		}
		fmt.Printf("[%s %5.0fs] connected=%d frames/s=%.0f intents=%d captures=%d tags=%d onSafeLine=%.0f%% %s\n", label, el,
			m.connected.Load(), float64(m.frames.Load())/el, m.intents.Load(), caps, tags, safe, m.gapStats())
	}
	tk := time.NewTicker(10 * time.Second)
	defer tk.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tk.C:
			report("run")
		}
	}
	wg.Wait()
	report("final")
}
