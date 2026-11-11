package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"hashflag/internal/config"
	"hashflag/internal/factions"
	"hashflag/internal/game"
	"hashflag/internal/stats"
)

// testConfig is a small, obstacle-free config. Tests tweak fields directly.
func testConfig() *config.Config {
	c := config.Defaults()
	c.TickMs = 20
	c.Map.Width = 40
	c.Map.Height = 20
	c.Map.BaseSize = 7
	c.Map.Obstacles.Density = 0
	return &c
}

type harness struct {
	room  *Room
	srv   *httptest.Server
	board *factions.Board
	dir   string
}

func newHarness(t *testing.T, cfg *config.Config, opts RoomOptions) *harness {
	t.Helper()
	dir := t.TempDir()
	board, err := factions.LoadBoard(filepath.Join(dir, "board.json"))
	if err != nil {
		t.Fatal(err)
	}
	log := stats.NewLogger(filepath.Join(dir, "stats.jsonl"))
	if opts.Seed == 0 {
		opts.Seed = 1
	}
	r := NewRoom(cfg, board, log, opts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	srv := httptest.NewServer(NewHub(r))
	t.Cleanup(func() {
		cancel()
		<-done
		srv.Close()
	})
	return &harness{room: r, srv: srv, board: board, dir: dir}
}

// client is a test websocket client that buffers every frame it receives.
type client struct {
	t      *testing.T
	ws     *websocket.Conn
	mu     sync.Mutex
	cond   *sync.Cond
	frames [][]byte
	pos    int
	closed error
}

func (h *harness) dial(t *testing.T) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/ws"
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetReadLimit(1 << 20)
	c := &client{t: t, ws: ws}
	c.cond = sync.NewCond(&c.mu)
	go func() {
		for {
			_, b, err := ws.Read(context.Background())
			c.mu.Lock()
			if err != nil {
				c.closed = err
				c.cond.Broadcast()
				c.mu.Unlock()
				return
			}
			c.frames = append(c.frames, b)
			c.cond.Broadcast()
			c.mu.Unlock()
		}
	}()
	t.Cleanup(func() { ws.CloseNow() })
	return c
}

func (c *client) send(v any) {
	c.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	c.sendRaw(b)
}

func (c *client) sendRaw(b []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// next returns the next unread frame of type typ (skipping others), failing
// after timeout.
func (c *client) next(typ string, timeout time.Duration) []byte {
	c.t.Helper()
	b, ok := c.tryNext(typ, timeout)
	if !ok {
		c.t.Fatalf("no %q frame within %v", typ, timeout)
	}
	return b
}

func (c *client) tryNext(typ string, timeout time.Duration) ([]byte, bool) {
	deadline := time.Now().Add(timeout)
	timer := time.AfterFunc(timeout, func() {
		c.mu.Lock()
		c.cond.Broadcast()
		c.mu.Unlock()
	})
	defer timer.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		for c.pos < len(c.frames) {
			b := c.frames[c.pos]
			c.pos++
			var head struct {
				T string `json:"t"`
			}
			if json.Unmarshal(b, &head) == nil && head.T == typ {
				return b, true
			}
		}
		if c.closed != nil || time.Now().After(deadline) {
			return nil, false
		}
		c.cond.Wait()
	}
}

// waitClosed waits for the server to close the connection.
func (c *client) waitClosed(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		err := c.closed
		c.mu.Unlock()
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

// hello greets and returns the welcome frame.
func (c *client) hello(name, token string) WelcomeMsg {
	c.t.Helper()
	c.send(ClientMsg{T: "hello", Name: name, Token: token})
	return decode[WelcomeMsg](c.t, c.next("welcome", 3*time.Second))
}

// enter greets, picks a role and waits until the player is in the world.
func (c *client) enter(name, role string) WelcomeMsg {
	c.t.Helper()
	w := c.hello(name, "")
	c.send(ClientMsg{T: "setRole", Role: role})
	c.waitLobby(func(l LobbyMsg) bool { return l.InWorld })
	return w
}

func (c *client) waitLobby(pred func(LobbyMsg) bool) LobbyMsg {
	c.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, ok := c.tryNext("lobby", time.Until(deadline))
		if !ok {
			c.t.Fatal("lobby condition not reached")
		}
		if l := decode[LobbyMsg](c.t, b); pred(l) {
			return l
		}
	}
}

// eventually polls cond inside the room goroutine.
func (h *harness) eventually(t *testing.T, timeout time.Duration, cond func(r *Room) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ok := false
		h.room.do(func() { ok = cond(h.room) })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func dirp(d int) *int { return &d }

func TestHelloGetsWelcome(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	c := h.dial(t)
	w := c.hello("  ali\x07ce  ", "")
	if len(w.Token) < 32 || w.PlayerID == "" {
		t.Fatalf("bad welcome %+v", w)
	}
	l := decode[LobbyMsg](t, c.next("lobby", time.Second))
	if l.InWorld || l.Waiting {
		t.Fatalf("fresh player should be idle in lobby: %+v", l)
	}
	var name string
	h.room.do(func() { name = h.room.sessions[game.PlayerID(w.PlayerID)].name })
	if name != "alice" {
		t.Fatalf("name = %q, want sanitized %q", name, "alice")
	}
	c2 := h.dial(t)
	w2 := c2.hello(strings.Repeat("x", 50), "")
	if w2.Token == w.Token || w2.PlayerID == w.PlayerID {
		t.Fatal("sessions must be distinct")
	}
	h.room.do(func() { name = h.room.sessions[game.PlayerID(w2.PlayerID)].name })
	if len([]rune(name)) != 20 {
		t.Fatalf("name not truncated: %q", name)
	}
	c3 := h.dial(t)
	w3 := c3.hello("   ", "")
	h.room.do(func() { name = h.room.sessions[game.PlayerID(w3.PlayerID)].name })
	if !strings.HasPrefix(name, "anon-") {
		t.Fatalf("empty name should become anon-N, got %q", name)
	}
}

func TestLatestIntentWins(t *testing.T) {
	cfg := testConfig()
	cfg.TickMs = 60000 // ticks are driven manually
	h := newHarness(t, cfg, RoomOptions{})
	c := h.dial(t)
	w := c.enter("mover", "attacker")
	pid := game.PlayerID(w.PlayerID)

	var start game.Pos
	h.room.do(func() { start = h.room.world.Players[pid].Pos })
	c.send(ClientMsg{T: "intent", Kind: "move", Dir: dirp(2)})
	c.send(ClientMsg{T: "intent", Kind: "move", Dir: dirp(4)})
	h.eventually(t, 2*time.Second, func(r *Room) bool {
		in, ok := r.pending[pid]
		return ok && in.Kind == game.Move && in.Dir == 4
	})
	var after game.Pos
	var left int
	h.room.do(func() {
		h.room.tick(time.Now())
		after = h.room.world.Players[pid].Pos
		left = len(h.room.pending)
	})
	if after != (game.Pos{X: start.X, Y: start.Y + 1}) {
		t.Fatalf("moved %v -> %v, want one step south", start, after)
	}
	if left != 0 {
		t.Fatal("pending intents must be cleared after a tick")
	}
	// A missed tick means Stay.
	h.room.do(func() { h.room.tick(time.Now()); after = h.room.world.Players[pid].Pos })
	if after != (game.Pos{X: start.X, Y: start.Y + 1}) {
		t.Fatal("player moved without an intent")
	}
	// Invalid direction is ignored.
	c.send(ClientMsg{T: "intent", Kind: "move", Dir: dirp(9)})
	c.send(ClientMsg{T: "hello", Name: "x"}) // processed after the bad intent
	time.Sleep(50 * time.Millisecond)
	h.room.do(func() { left = len(h.room.pending) })
	if left != 0 {
		t.Fatal("invalid dir must be rejected")
	}
}

func TestGarbageFrameDoesNotCloseConnection(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	c := h.dial(t)
	c.sendRaw([]byte("not json at all"))
	c.sendRaw([]byte(`{"t":"bogus"}`))
	c.sendRaw([]byte(`{"t":"hello","name":"` + strings.Repeat("a", 2000) + `"}`))
	c.sendRaw([]byte(`{"t":"intent","kind":"dance","dir":1}`))
	w := c.hello("still-here", "")
	if w.PlayerID == "" {
		t.Fatal("no welcome after garbage")
	}
	var n int
	h.room.do(func() { n = len(h.room.sessions) })
	if n != 1 {
		t.Fatalf("oversized hello must be dropped; sessions = %d", n)
	}
}

func TestFloodDisconnects(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	c := h.dial(t)
	c.hello("flooder", "")
	// A flood at ten times the limit (150/s at a 20 ms tick) for 2 s.
	stop := time.Now().Add(2 * time.Second)
	for time.Now().Before(stop) {
		if c.sendRaw([]byte(`{"t":"intent","kind":"stay"}`)) != nil {
			break
		}
	}
	err := c.waitClosed(3 * time.Second)
	if err == nil {
		t.Fatal("flooding connection was not closed")
	}
	if s := websocket.CloseStatus(err); s != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %v (%v), want policy violation", s, err)
	}
}

func TestRateLimitScalesWithTick(t *testing.T) {
	for tick, want := range map[int]float64{20: 150, 50: 60, 100: 30, 150: 20, 1000: 20, 0: 20} {
		if got := rateLimitFor(tick); got != want {
			t.Errorf("rateLimitFor(%d) = %v, want %v", tick, got, want)
		}
	}
}

// A hold-to-repeat client sends about one intent per tick; at the 20 ms tick
// floor that is 50/s, and even 140/s must not trip the limiter.
func TestSustainedRateWithinLimitNotClosed(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	c := h.dial(t)
	c.hello("holder", "")
	tk := time.NewTicker(20 * time.Millisecond)
	defer tk.Stop()
	stop := time.Now().Add(2 * time.Second)
	for time.Now().Before(stop) {
		<-tk.C
		for i := 0; i < 3; i++ { // 150/s nominal, a little under after jitter
			if err := c.sendRaw([]byte(`{"t":"intent","kind":"stay"}`)); err != nil {
				t.Fatalf("send failed (closed?): %v", err)
			}
		}
	}
	if err := c.waitClosed(100 * time.Millisecond); err != nil {
		t.Fatalf("connection closed: %v", err)
	}
}

func TestReconnectWithTokenRestoresPlayer(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{ReconnectGrace: 5 * time.Second})
	c1 := h.dial(t)
	w1 := c1.enter("phoenix", "defender")
	c1.ws.Close(websocket.StatusNormalClosure, "")
	pid := game.PlayerID(w1.PlayerID)
	h.eventually(t, 2*time.Second, func(r *Room) bool { return r.sessions[pid].conn == nil })

	c2 := h.dial(t)
	w2 := c2.hello("", w1.Token)
	if w2.PlayerID != w1.PlayerID || w2.Token != w1.Token {
		t.Fatalf("reconnect got %+v, want %+v", w2, w1)
	}
	l := c2.waitLobby(func(LobbyMsg) bool { return true })
	if !l.InWorld || l.Role != "defender" {
		t.Fatalf("slot not restored: %+v", l)
	}
	r := decode[RoundMsg](t, c2.next("round", time.Second))
	if r.You == nil || r.You.ID != w1.PlayerID {
		t.Fatalf("round frame should identify the restored player: %+v", r.You)
	}

	// A new connection with the token replaces a still-open one.
	c3 := h.dial(t)
	if w3 := c3.hello("", w1.Token); w3.PlayerID != w1.PlayerID {
		t.Fatal("second reconnect lost the slot")
	}
	if c2.waitClosed(2*time.Second) == nil {
		t.Fatal("replaced connection should be closed")
	}

	// An unknown token simply makes a new session.
	c4 := h.dial(t)
	if w4 := c4.hello("other", "deadbeef"); w4.PlayerID == w1.PlayerID {
		t.Fatal("bad token must not restore a slot")
	}
}

func TestFullSidesWaitInLobby(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPerTeam = 1
	cfg.TeamBalance.MaxRoleSkewPct = 100
	h := newHarness(t, cfg, RoomOptions{ReconnectGrace: 50 * time.Millisecond})
	a := h.dial(t)
	a.enter("a", "attacker")
	b := h.dial(t)
	b.enter("b", "attacker")
	c := h.dial(t)
	c.hello("c", "")
	c.send(ClientMsg{T: "setRole", Role: "attacker"})
	l := c.waitLobby(func(l LobbyMsg) bool { return l.Waiting })
	if l.InWorld {
		t.Fatal("third player should not be in the world")
	}
	// When a slot frees up the waiting player gets in.
	a.ws.CloseNow()
	c.waitLobby(func(l LobbyMsg) bool { return l.InWorld && !l.Waiting })
}

func TestCarrierDisconnectDropsFlag(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{ReconnectGrace: 60 * time.Millisecond})
	c := h.dial(t)
	w := c.enter("runner", "attacker")
	pid := game.PlayerID(w.PlayerID)
	var at game.Pos
	h.room.do(func() {
		wd := h.room.world
		p := wd.Players[pid]
		p.Pos = game.Pos{X: wd.Map.BaseA.X1 + 3, Y: wd.Map.FlagA.Y}
		p.HasFlag = true
		wd.Flags[game.TeamB].Carrier = pid
		wd.Flags[game.TeamB].Pos = p.Pos
		at = p.Pos
	})
	c.ws.CloseNow()
	h.eventually(t, 3*time.Second, func(r *Room) bool {
		_, inWorld := r.world.Players[pid]
		_, hasSession := r.sessions[pid]
		return !inWorld && !hasSession
	})
	var f game.Flag
	h.room.do(func() { f = h.room.world.Flags[game.TeamB] })
	if f.Carrier != "" || f.Pos != at {
		t.Fatalf("flag = %+v, want dropped at %v", f, at)
	}
}

func TestRoundEndsAndStarts(t *testing.T) {
	cfg := testConfig()
	cfg.TickMs = 10
	cfg.RoundTicks = 20
	h := newHarness(t, cfg, RoomOptions{Intermission: 50 * time.Millisecond})
	c := h.dial(t)
	c.enter("looper", "attacker")
	c.next("round", time.Second)
	re := decode[RoundEndMsg](t, c.next("roundEnd", 3*time.Second))
	if re.Winner != "draw" {
		t.Fatalf("winner = %q, want draw", re.Winner)
	}
	r := decode[RoundMsg](t, c.next("round", 3*time.Second))
	if r.You == nil || r.You.Role != "attacker" {
		t.Fatalf("player should re-enter the next round: %+v", r.You)
	}
	if len(r.Map.Rows) != cfg.Map.Height || len(r.Map.Rows[0]) != cfg.Map.Width {
		t.Fatal("bad map rows")
	}
	st := decode[StateMsg](t, c.next("state", time.Second))
	if len(st.Players) != 1 {
		t.Fatalf("state players = %d", len(st.Players))
	}
	h.eventually(t, time.Second, func(r *Room) bool { return r.round >= 2 })
}

func TestMidRoundRoleChangeRejected(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	c := h.dial(t)
	c.enter("stubborn", "attacker")
	c.send(ClientMsg{T: "setRole", Role: "defender"})
	c.next("error", time.Second)
	c.send(ClientMsg{T: "createFaction"})
	c.next("error", time.Second)
	var role string
	h.room.do(func() {
		for _, p := range h.room.world.Players {
			role = roleName(p.Role)
		}
	})
	if role != "attacker" {
		t.Fatalf("role changed to %q", role)
	}
}

func TestCaptureCreditsCapturersFaction(t *testing.T) {
	cfg := testConfig()
	cfg.TickMs = 60000
	h := newHarness(t, cfg, RoomOptions{})
	a := h.dial(t)
	wa := a.hello("scorer", "")
	a.send(ClientMsg{T: "createFaction"})
	la := a.waitLobby(func(l LobbyMsg) bool { return l.FactionID != "" })
	a.send(ClientMsg{T: "setRole", Role: "attacker"})
	a.waitLobby(func(l LobbyMsg) bool { return l.InWorld })

	b := h.dial(t)
	b.hello("victim", "")
	b.send(ClientMsg{T: "createFaction"})
	lb := b.waitLobby(func(l LobbyMsg) bool { return l.FactionID != "" })

	teamOf := func(l LobbyMsg, id string) string {
		for _, f := range l.Factions {
			if f.ID == id {
				return f.Team
			}
		}
		return ""
	}
	if teamOf(la, la.FactionID) != "A" || teamOf(lb, lb.FactionID) != "B" {
		t.Fatal("factions should land on opposite sides")
	}

	pid := game.PlayerID(wa.PlayerID)
	var nameA, nameB string
	h.room.do(func() {
		nameA = h.room.reg.Get(la.FactionID).Name
		nameB = h.room.reg.Get(lb.FactionID).Name
		wd := h.room.world
		p := wd.Players[pid]
		p.HasFlag = true
		wd.Flags[game.TeamB].Carrier = pid
		wd.Flags[game.TeamB].Pos = p.Pos
		h.room.tick(time.Now())
	})
	var tally map[string]int
	h.room.do(func() { tally = h.room.rs.captures })
	if tally[nameA] != 1 || tally[nameB] != 0 {
		t.Fatalf("round tally = %v, want 1 for %q", tally, nameA)
	}
	top := h.board.Top(5)
	if len(top) != 1 || top[0].Name != nameA || top[0].Captures != 1 {
		t.Fatalf("board = %v, want %q credited", top, nameA)
	}
}

func TestTwoHundredBotsOneTick(t *testing.T) {
	cfg := config.Defaults()
	cfg.TickMs = 100
	h := newHarness(t, &cfg, RoomOptions{MaxConns: 300, MaxConnsPerIP: 300})
	clients := make([]*client, 200)
	var wg sync.WaitGroup
	for i := range clients {
		clients[i] = h.dial(t)
	}
	// Join sequentially so team assignment alternates and roles stay
	// balanced: A gets i%4==0 attackers and i%4==2 defenders, etc.
	for i, c := range clients {
		role := "attacker"
		if (i/2)%2 == 1 {
			role = "defender"
		}
		c.hello(fmt.Sprintf("bot%d", i), "")
		c.send(ClientMsg{T: "setRole", Role: role})
		c.waitLobby(func(l LobbyMsg) bool { return l.InWorld })
	}
	var inWorld int
	h.room.do(func() { inWorld = len(h.room.world.Players) })
	if inWorld != 200 {
		t.Fatalf("in world = %d, want 200", inWorld)
	}
	var tick int
	h.room.do(func() { tick = h.room.world.Tick })
	for _, c := range clients {
		wg.Add(1)
		go func(c *client) {
			defer wg.Done()
			for {
				b, ok := c.tryNext("state", 5*time.Second)
				if !ok {
					t.Error("client missed state")
					return
				}
				if st := decode[StateMsg](t, b); st.Tick > tick+1 && len(st.Players) == 200 {
					return
				}
			}
		}(c)
	}
	wg.Wait()
	var d time.Duration
	h.room.do(func() { d = h.room.lastBroadcast })
	t.Logf("step + broadcast to 200 clients: %v", d)
	if d <= 0 || d > 200*time.Millisecond {
		t.Fatalf("tick broadcast took %v, want <= 200ms", d)
	}
	var dropped int
	h.room.do(func() { dropped = 200 - len(h.room.conns) })
	if dropped != 0 {
		t.Fatalf("%d connections dropped", dropped)
	}
}

func TestForeignOriginRejected(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/ws"
	_, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"https://evil.example"}}})
	if err == nil {
		t.Fatal("foreign origin accepted")
	}
	ws, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"http://localhost:5173"}}})
	if err != nil {
		t.Fatalf("localhost origin rejected: %v", err)
	}
	ws.CloseNow()
}

func TestCarrierDisconnectDropsFlagImmediately(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{ReconnectGrace: time.Minute})
	c := h.dial(t)
	w := c.enter("runner", "attacker")
	pid := game.PlayerID(w.PlayerID)
	var at game.Pos
	h.room.do(func() {
		wd := h.room.world
		p := wd.Players[pid]
		p.Pos = game.Pos{X: wd.Map.BaseA.X1 + 3, Y: wd.Map.FlagA.Y}
		p.HasFlag = true
		wd.Flags[game.TeamB].Carrier = pid
		wd.Flags[game.TeamB].Pos = p.Pos
		h.room.rs.carryStart[pid] = wd.Tick
		at = p.Pos
	})
	c.ws.CloseNow()
	h.eventually(t, 2*time.Second, func(r *Room) bool { return r.sessions[pid].conn == nil })
	time.Sleep(100 * time.Millisecond) // a few ticks: the dropper must not re-pick it up
	h.room.do(func() {
		r := h.room
		f := r.world.Flags[game.TeamB]
		if f.Carrier != "" || f.Pos != at {
			t.Errorf("flag = %+v, want dropped at %v", f, at)
		}
		p := r.world.Players[pid]
		if p == nil || p.HasFlag {
			t.Errorf("player should keep the slot without the flag: %+v", p)
		}
		if _, open := r.rs.carryStart[pid]; open || r.rs.carryCount != 1 {
			t.Errorf("carry should be closed: open=%v count=%d", open, r.rs.carryCount)
		}
	})
}

func TestGraceDisconnectedPlayerSkippedForNewRound(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPerTeam = 1
	cfg.TeamBalance.MaxRoleSkewPct = 100
	h := newHarness(t, cfg, RoomOptions{ReconnectGrace: time.Minute})
	a := h.dial(t)
	wa := a.enter("a", "attacker")
	pa := game.PlayerID(wa.PlayerID)
	a.ws.CloseNow()
	h.eventually(t, 2*time.Second, func(r *Room) bool { return r.sessions[pa].conn == nil })

	h.room.do(func() { h.room.startRound(time.Now()) })
	h.room.do(func() {
		r := h.room
		if r.world.Players[pa] != nil {
			t.Error("disconnected player was admitted to the new round")
		}
		if !r.sessions[pa].waiting {
			t.Error("disconnected player should wait for their return")
		}
		r.admitWaiting()
		if r.world.Players[pa] != nil {
			t.Error("admitWaiting took a disconnected player")
		}
	})
	// With one place per side, both live players still get in.
	b, c := h.dial(t), h.dial(t)
	b.enter("b", "attacker")
	c.enter("c", "attacker")
}

func rawDial(h *harness) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.srv.URL, "http")+"/ws", nil)
}

func (h *harness) connCounts() (int, int) {
	h.room.connMu.Lock()
	defer h.room.connMu.Unlock()
	return h.room.nConns, len(h.room.ipConns)
}

func (h *harness) waitNoConns(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		n, ips := h.connCounts()
		if n == 0 && ips == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("connection counters not back to zero: conns=%d ips=%d", n, ips)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestMaxConnsRejectsWith503(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{MaxConns: 2, MaxConnsPerIP: 10})
	a, b := h.dial(t), h.dial(t)
	a.hello("a", "")
	b.hello("b", "")
	_, resp, err := rawDial(h)
	if err == nil || resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("third dial: err=%v resp=%v, want 503", err, resp)
	}
	a.ws.Close(websocket.StatusNormalClosure, "")
	b.ws.Close(websocket.StatusNormalClosure, "")
	h.waitNoConns(t)
	c := h.dial(t) // room again
	c.hello("c", "")
}

func TestMaxConnsPerIPRejectsWith429(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{MaxConns: 100, MaxConnsPerIP: 2})
	a, b := h.dial(t), h.dial(t)
	a.hello("a", "")
	b.hello("b", "")
	_, resp, err := rawDial(h)
	if err == nil || resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third dial: err=%v resp=%v, want 429", err, resp)
	}
	if n, ips := h.connCounts(); n != 2 || ips != 1 {
		t.Fatalf("counts = %d conns, %d ips; want 2, 1", n, ips)
	}
	a.ws.CloseNow()
	b.ws.CloseNow()
	h.waitNoConns(t)
}

func TestRejectedUpgradeDoesNotLeakCount(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	// A plain GET is not a WebSocket upgrade; Accept fails after admission.
	for i := 0; i < 30; i++ {
		resp, err := http.Get(h.srv.URL + "/ws")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	h.waitNoConns(t)
}

func TestHelloTimeoutClosesSilentSocket(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{HelloTimeout: 150 * time.Millisecond})
	silent := h.dial(t)
	greeted := h.dial(t)
	greeted.hello("talker", "")
	err := silent.waitClosed(2 * time.Second)
	if s := websocket.CloseStatus(err); s != websocket.StatusPolicyViolation {
		t.Fatalf("silent socket close = %v (%v), want policy violation", s, err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := greeted.waitClosed(50 * time.Millisecond); err != nil {
		t.Fatalf("greeted socket was closed: %v", err)
	}
	greeted.ws.CloseNow()
	h.waitNoConns(t)
}

// A peer that never reads cannot answer pings and is dropped.
func TestUnansweredPingClosesConnection(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{
		PingInterval: 50 * time.Millisecond, PingTimeout: 100 * time.Millisecond, HelloTimeout: time.Minute})
	nc, err := net.Dial("tcp", h.srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	fmt.Fprintf(nc, "GET /ws HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	h.eventually(t, 2*time.Second, func(r *Room) bool { return len(r.conns) == 1 })
	// Never read or write again: the server's ping gets no pong.
	h.eventually(t, 3*time.Second, func(r *Room) bool { return len(r.conns) == 0 })
	h.waitNoConns(t)
}

func TestCreateFactionRateLimitedPerPlayer(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{})
	a := h.dial(t)
	a.enterLobby("a")
	a.send(ClientMsg{T: "createFaction"})
	a.waitLobby(func(l LobbyMsg) bool { return l.FactionID != "" })
	a.send(ClientMsg{T: "createFaction"})
	e := decode[ErrorMsg](t, a.next("error", 2*time.Second))
	if !strings.Contains(e.Message, "slow down") {
		t.Fatalf("error = %q", e.Message)
	}
	// Another player is not affected.
	b := h.dial(t)
	b.enterLobby("b")
	b.send(ClientMsg{T: "createFaction"})
	b.waitLobby(func(l LobbyMsg) bool { return l.FactionID != "" })
}

func TestFactionCapReached(t *testing.T) {
	h := newHarness(t, testConfig(), RoomOptions{MaxFactions: 2})
	var cs []*client
	for i := 0; i < 3; i++ {
		c := h.dial(t)
		c.enterLobby(fmt.Sprintf("p%d", i))
		cs = append(cs, c)
	}
	for _, c := range cs[:2] {
		c.send(ClientMsg{T: "createFaction"})
		c.waitLobby(func(l LobbyMsg) bool { return l.FactionID != "" })
	}
	cs[2].send(ClientMsg{T: "createFaction"})
	e := decode[ErrorMsg](t, cs[2].next("error", 2*time.Second))
	if !strings.Contains(e.Message, "too many factions") {
		t.Fatalf("error = %q", e.Message)
	}
}

func (c *client) enterLobby(name string) {
	c.t.Helper()
	c.hello(name, "")
}
