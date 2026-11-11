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

type pos struct{ x, y int }

// deltas are the 8 directions, clockwise from north (matches the server).
var deltas = [8]pos{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func dirOf(dx, dy int) int {
	for i, d := range deltas {
		if d.x == dx && d.y == dy {
			return i
		}
	}
	return 0
}

// stepToward is the greedy 8-direction step from one cell toward another.
func stepToward(from, to pos) (int, bool) {
	dx, dy := sign(to.x-from.x), sign(to.y-from.y)
	if dx == 0 && dy == 0 {
		return 0, false
	}
	return dirOf(dx, dy), true
}

// aim returns the direction to throw at "to" when it lies on a straight or
// diagonal line from "from" within rng cells.
func aim(from, to pos, rng int) (int, bool) {
	dx, dy := to.x-from.x, to.y-from.y
	dist := max(abs(dx), abs(dy))
	if dist == 0 || dist > rng || (dx != 0 && dy != 0 && abs(dx) != abs(dy)) {
		return 0, false
	}
	return dirOf(sign(dx), sign(dy)), true
}

// pickMove steps toward target; if that cell is blocked it sidesteps to a
// random free neighbouring direction. With probability noise it moves
// randomly instead. ok is false when no free move exists.
func pickMove(rng *rand.Rand, from, target pos, blocked func(pos) bool, noise float64) (int, bool) {
	free := func(d int) bool { return !blocked(pos{from.x + deltas[d].x, from.y + deltas[d].y}) }
	var opts []int
	if rng.Float64() >= noise {
		if d, ok := stepToward(from, target); ok {
			if free(d) {
				return d, true
			}
			// sidestep without turning back: 45 or 90 degrees off the direct line
			for _, off := range []int{1, 7, 2, 6} {
				if nd := (d + off) % 8; free(nd) {
					opts = append(opts, nd)
				}
			}
			if len(opts) > 0 {
				return opts[rng.Intn(len(opts))], true
			}
		}
	}
	for d := 0; d < 8; d++ {
		if free(d) {
			opts = append(opts, d)
		}
	}
	if len(opts) == 0 {
		return 0, false
	}
	return opts[rng.Intn(len(opts))], true
}

// Wire frames (subset of internal/server/protocol.go).
type frame struct {
	T       string `json:"t"`
	Token   string `json:"token"`
	Player  string `json:"playerId"`
	Role    string `json:"role"`
	InWorld bool   `json:"inWorld"`
	Round   int    `json:"round"`
	Map     *struct {
		Width, Height int
		Rows          []string
		BaseA, BaseB  rect
		FlagA, FlagB  xy
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
	captures                   map[string]bool
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
	detour     int
	detourLeft int
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
		if e.Kind == "captured" {
			b.m.mu.Lock()
			b.m.captures[fmt.Sprintf("%d:%d:%s", b.round, f.Tick, e.Player)] = true
			b.m.mu.Unlock()
		}
	}
	if b.team == "" || !b.gotMap {
		return
	}
	if kind, dir, ok := b.decide(f); ok {
		d := dir
		send(map[string]any{"t": "intent", "kind": kind, "dir": d})
		b.m.intents.Add(1)
	}
}

func (b *bot) decide(f *frame) (string, int, bool) {
	var me pos
	found, carrying := false, false
	occupied := map[pos]bool{}
	var enemies []pos
	for _, p := range f.Players {
		at := pos{p.X, p.Y}
		if p.ID == b.id {
			if p.Out {
				return "", 0, false
			}
			me, found, carrying = at, true, p.HasFlag
			continue
		}
		if p.Out {
			continue
		}
		occupied[at] = true
		if p.Team != b.team {
			enemies = append(enemies, at)
		}
	}
	if !found {
		return "", 0, false
	}
	for _, w := range f.Walls {
		occupied[pos{w.X, w.Y}] = true
	}
	blocked := func(p pos) bool {
		if p.y < 0 || p.y >= len(b.rows) || p.x < 0 || p.x >= len(b.rows[p.y]) {
			return true
		}
		return b.rows[p.y][p.x] == '#' || occupied[p]
	}
	if b.ammo > 0 && b.cd == 0 {
		for _, e := range enemies {
			if d, ok := aim(me, e, 14); ok {
				return "throw", d, true
			}
		}
	}
	if b.attacker {
		target := b.homeFlag
		if !carrying {
			target = me
			for _, fl := range f.Flags {
				if fl.Team != b.team {
					target = pos{fl.X, fl.Y}
				}
			}
		}
		// commit to a sidestep for a few ticks so greedy steps do not oscillate at obstacles
		if b.detourLeft > 0 {
			b.detourLeft--
			if !blocked(pos{me.x + deltas[b.detour].x, me.y + deltas[b.detour].y}) {
				return "move", b.detour, true
			}
		}
		d, ok := pickMove(b.rng, me, target, blocked, 0.2)
		if direct, has := stepToward(me, target); ok && has && blocked(pos{me.x + deltas[direct].x, me.y + deltas[direct].y}) {
			b.detour, b.detourLeft = d, 4
		}
		return "move", d, ok
	}
	// defender: hover near the home flag (never on it), now and then wall up the base
	inner := blocked
	blocked = func(p pos) bool { return p == b.homeFlag || inner(p) }
	if max(abs(me.x-b.homeFlag.x), abs(me.y-b.homeFlag.y)) > 3 {
		d, ok := pickMove(b.rng, me, b.homeFlag, blocked, 0)
		return "move", d, ok
	}
	if b.rng.Float64() < 0.1 && b.walls < 3 {
		var opts []int
		for d := 0; d < 8; d++ {
			t := pos{me.x + deltas[d].x, me.y + deltas[d].y}
			if b.base.has(t) && t != b.homeFlag && !blocked(t) {
				opts = append(opts, d)
			}
		}
		if len(opts) > 0 {
			return "wall", opts[b.rng.Intn(len(opts))], true
		}
	}
	if b.rng.Float64() < 0.4 {
		d, ok := pickMove(b.rng, me, b.homeFlag, blocked, 1)
		if ok {
			nx, ny := me.x+deltas[d].x, me.y+deltas[d].y
			if max(abs(nx-b.homeFlag.x), abs(ny-b.homeFlag.y)) <= 3 {
				return "move", d, true
			}
		}
	}
	return "", 0, false
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

	m := &metrics{captures: map[string]bool{}}
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
		caps := len(m.captures)
		m.mu.Unlock()
		el := time.Since(start).Seconds()
		fmt.Printf("[%s %5.0fs] connected=%d frames/s=%.0f intents=%d captures=%d %s\n", label, el,
			m.connected.Load(), float64(m.frames.Load())/el, m.intents.Load(), caps, m.gapStats())
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
