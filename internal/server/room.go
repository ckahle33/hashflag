package server

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"

	"hashflag/internal/config"
	"hashflag/internal/factions"
	"hashflag/internal/game"
	"hashflag/internal/stats"
)

// Defaults for zero RoomOptions fields.
const (
	DefaultReconnectGrace = 30 * time.Second
	DefaultIntermission   = 10 * time.Second
	DefaultHelloTimeout   = 10 * time.Second
	DefaultPingInterval   = 30 * time.Second
	DefaultPingTimeout    = 10 * time.Second
	DefaultMaxPerIP       = 20
	DefaultMaxFactions    = 500
	minMaxConns           = 50
	// FactionCreateInterval is the minimum gap between one player's new factions.
	FactionCreateInterval = 5 * time.Second
	boardTopN             = 10
	inboxSize             = 1024
)

// DefaultAllowedOrigins are the extra Origin patterns accepted besides the
// request's own host.
var DefaultAllowedOrigins = []string{"localhost:*", "127.0.0.1:*"}

// RoomOptions tune a Room. Zero values take the defaults above.
type RoomOptions struct {
	ReconnectGrace time.Duration // how long a dropped player's slot is kept
	Intermission   time.Duration // pause between rounds
	AllowedOrigins []string      // extra WebSocket Origin patterns
	Seed           int64         // 0 = random; otherwise deterministic maps and names

	MaxConns      int           // open WebSockets in total; default 4*MaxPerTeam, at least 50
	MaxConnsPerIP int           // open WebSockets per remote IP; default 20
	HelloTimeout  time.Duration // a socket must send a valid hello within this; default 10s
	PingInterval  time.Duration // keepalive ping period; default 30s
	PingTimeout   time.Duration // a ping unanswered this long closes the socket; default 10s
	MaxFactions   int           // live factions in total; default 500
}

// roomMsg is one item in the room inbox. Exactly one field group is set.
type roomMsg struct {
	open   *conn // connection registered
	closed *conn // connection ended
	from   *conn // client frame
	msg    ClientMsg
	fn     func() // run inside the room goroutine (tests, bots)
	ack    chan struct{}
}

// roundStats accumulates stats for the current round.
type roundStats struct {
	captures    map[string]int // faction name -> captures
	tags        map[string]int // tagger name -> tags
	carryStart  map[game.PlayerID]int
	carryTotal  int
	carryCount  int
	wallsPlaced int
	wallsBroken int
}

func newRoundStats() *roundStats {
	return &roundStats{
		captures:   map[string]int{},
		tags:       map[string]int{},
		carryStart: map[game.PlayerID]int{},
	}
}

// endCarry closes an open flag carry for pid at tick.
func (s *roundStats) endCarry(pid game.PlayerID, tick int) {
	if start, ok := s.carryStart[pid]; ok {
		s.carryTotal += tick - start
		s.carryCount++
		delete(s.carryStart, pid)
	}
}

// Room is the single global game room. All fields below inbox are owned by
// the Run goroutine; everything else talks to it through the inbox.
type Room struct {
	cfg   *config.Config
	board *factions.Board
	log   *stats.Logger
	opts  RoomOptions

	inbox chan roomMsg
	done  chan struct{} // closed when Run stops accepting messages

	// Connection admission, touched from HTTP handler goroutines.
	connMu  sync.Mutex
	nConns  int
	ipConns map[string]int

	rng      *rand.Rand
	conns    map[*conn]bool
	sessions map[game.PlayerID]*session
	byToken  map[string]*session
	nextSeq  int
	reg      *factions.Registry

	world             *game.World
	mapMsg            MapMsg
	round             int
	inRound           bool
	intermissionUntil time.Time
	pending           map[game.PlayerID]game.Intent
	rs                *roundStats
	lobbyDirty        bool
	lastBroadcast     time.Duration // duration of the last step + broadcast
	tickTimes         tickStats
	slowLog           slowLogLimiter
}

// NewRoom builds a room. board and log may be nil (nothing is persisted).
func NewRoom(cfg *config.Config, board *factions.Board, log *stats.Logger, opts RoomOptions) *Room {
	if opts.ReconnectGrace <= 0 {
		opts.ReconnectGrace = DefaultReconnectGrace
	}
	if opts.Intermission <= 0 {
		opts.Intermission = DefaultIntermission
	}
	if opts.AllowedOrigins == nil {
		opts.AllowedOrigins = DefaultAllowedOrigins
	}
	if opts.MaxConns <= 0 {
		opts.MaxConns = max(minMaxConns, 4*cfg.MaxPerTeam)
	}
	if opts.MaxConnsPerIP <= 0 {
		opts.MaxConnsPerIP = DefaultMaxPerIP
	}
	if opts.HelloTimeout <= 0 {
		opts.HelloTimeout = DefaultHelloTimeout
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = DefaultPingInterval
	}
	if opts.PingTimeout <= 0 {
		opts.PingTimeout = DefaultPingTimeout
	}
	if opts.MaxFactions <= 0 {
		opts.MaxFactions = DefaultMaxFactions
	}
	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	return &Room{
		cfg: cfg, board: board, log: log, opts: opts,
		inbox:    make(chan roomMsg, inboxSize),
		done:     make(chan struct{}),
		ipConns:  map[string]int{},
		rng:      rng,
		conns:    map[*conn]bool{},
		sessions: map[game.PlayerID]*session{},
		byToken:  map[string]*session{},
		reg:      factions.NewRegistry(rand.New(rand.NewSource(rng.Int63()))),
		pending:  map[game.PlayerID]game.Intent{},
		rs:       newRoundStats(),
	}
}

// Run starts the first round and processes ticks and messages until ctx is
// cancelled, then closes every connection and returns.
func (r *Room) Run(ctx context.Context) {
	r.startRound(time.Now())
	ticker := time.NewTicker(time.Duration(r.cfg.TickMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.shutdown()
			return
		case now := <-ticker.C:
			r.tick(now)
		case m := <-r.inbox:
			r.handle(m)
		}
	}
}

// logTickTiming logs the current round's tick timing summary and resets it.
func (r *Room) logTickTiming() {
	if n, mean, p95, max := r.tickTimes.summary(); n > 0 {
		ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
		log.Printf("server: round %d tick timing: ticks=%d mean=%.2fms p95=%.2fms max=%.2fms", r.round, n, ms(mean), ms(p95), ms(max))
	}
	r.tickTimes.reset()
}

func (r *Room) shutdown() {
	close(r.done)
	r.logTickTiming() // partial round, so a short run still reports timing
	if r.board != nil {
		if err := r.board.Save(); err != nil {
			log.Printf("server: saving board: %v", err)
		}
	}
	var wg sync.WaitGroup
	for c := range r.conns {
		c.dead = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.closeGracefully(websocket.StatusGoingAway, "server shutting down")
		}()
	}
	wg.Wait()
	r.conns = map[*conn]bool{}
}

// do runs fn inside the room goroutine and waits for it. It returns without
// running fn if the room has stopped.
func (r *Room) do(fn func()) {
	ack := make(chan struct{})
	if r.post(roomMsg{fn: fn, ack: ack}) {
		select {
		case <-ack:
		case <-r.done:
		}
	}
}

func (r *Room) handle(m roomMsg) {
	switch {
	case m.fn != nil:
		m.fn()
		close(m.ack)
	case m.open != nil:
		r.conns[m.open] = true
		if r.inRound {
			r.sendRound(m.open)
		}
	case m.closed != nil:
		r.dropConn(m.closed, 0, "")
	case m.from != nil:
		if !m.from.dead {
			r.handleClient(m.from, m.msg)
		}
	}
}

// dropConn forgets c and starts its session's reconnect grace.
func (r *Room) dropConn(c *conn, code websocket.StatusCode, reason string) {
	if c.dead {
		return
	}
	c.dead = true
	delete(r.conns, c)
	if s := c.sess; s != nil && s.conn == c {
		s.conn = nil
		s.graceUntil = time.Now().Add(r.opts.ReconnectGrace)
		// The slot is kept for the grace period, but the flag is not.
		if r.inRound && r.world != nil {
			r.record(r.world.DropFlagOf(s.id))
		}
		delete(r.pending, s.id)
		r.lobbyDirty = true
	}
	c.sess = nil
	c.kill(code, reason)
}

// setIntent queues pid's intent for the next tick; later calls in the same
// tick replace earlier ones. Players not in the world are ignored.
func (r *Room) setIntent(pid game.PlayerID, in game.Intent) {
	if r.inWorld(pid) {
		r.pending[pid] = in
	}
}

func (r *Room) inWorld(pid game.PlayerID) bool {
	return r.inRound && r.world != nil && r.world.Players[pid] != nil
}

// tick is one ticker beat: expire graces, then either advance the round or
// wait out the intermission, then flush lobby changes.
func (r *Room) tick(now time.Time) {
	r.expireGrace(now)
	if r.inRound {
		r.admitWaiting()
		r.stepWorld(now)
	} else if !now.Before(r.intermissionUntil) {
		r.startRound(now)
	}
	r.flushLobby()
}

func (r *Room) stepWorld(now time.Time) {
	start := time.Now()
	intents := r.pending
	r.pending = map[game.PlayerID]game.Intent{}
	events := r.world.Step(intents)
	r.record(events)

	b, err := json.Marshal(r.stateMsg(events))
	if err != nil {
		log.Printf("server: marshal state: %v", err)
		return
	}
	for c := range r.conns {
		r.send(c, b)
	}
	for _, s := range r.sessions {
		if s.conn == nil {
			continue
		}
		if p := r.world.Players[s.id]; p != nil {
			r.sendJSON(s.conn, YouMsg{T: TypeYou, Ammo: p.Ammo, CooldownLeft: p.CooldownLeft,
				OutLeft: p.OutLeft, HasFlag: p.HasFlag, WallsPlaced: p.WallsPlaced})
		}
	}
	r.lastBroadcast = time.Since(start)
	r.tickTimes.add(r.lastBroadcast)
	if r.lastBroadcast > slowTick && r.slowLog.allow(time.Now()) {
		log.Printf("server: slow tick %d: %v (limit %v, %d conns)", r.world.Tick, r.lastBroadcast, slowTick, len(r.conns))
	}

	if team, done := r.world.Winner(); done {
		r.endRound(team, now)
	}
}

// record updates round stats and the board from a tick's events.
func (r *Room) record(events []game.Event) {
	for _, e := range events {
		switch e.Kind {
		case game.Tagged:
			r.rs.tags[r.nameOf(e.Other)]++
		case game.FlagPickedUp:
			r.rs.carryStart[e.Player] = e.Tick
		case game.FlagDropped:
			r.rs.endCarry(e.Player, e.Tick)
		case game.Captured:
			// e.Team is the captured flag's team; credit the capturer.
			r.rs.endCarry(e.Player, e.Tick)
			if f := r.reg.FactionOf(e.Player); f != nil {
				r.rs.captures[f.Name]++
				if r.board != nil {
					r.board.AddCapture(f.Name)
				}
				r.lobbyDirty = true
			}
		case game.WallPlaced:
			r.rs.wallsPlaced++
		case game.WallBroken:
			r.rs.wallsBroken++
		}
	}
}

func (r *Room) nameOf(pid game.PlayerID) string {
	if p := r.world.Players[pid]; p != nil {
		return p.Name
	}
	if s := r.sessions[pid]; s != nil {
		return s.name
	}
	return string(pid)
}

func (r *Room) stateMsg(events []game.Event) StateMsg {
	w := r.world
	ids := make([]game.PlayerID, 0, len(w.Players))
	for id := range w.Players {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	st := StateMsg{
		T: TypeState, Tick: w.Tick, Scores: w.Scores(),
		Players:     make([]PlayerState, 0, len(ids)),
		Projectiles: make([]ProjectileState, 0, len(w.Projectiles)),
		Walls:       make([]WallState, 0, len(w.Walls)),
		Flags:       make([]FlagState, 0, 2),
		Events:      make([]EventMsg, 0, len(events)),
	}
	for _, id := range ids {
		p := w.Players[id]
		glyph := ""
		if f := r.reg.FactionOf(id); f != nil {
			glyph = f.Glyph
		}
		st.Players = append(st.Players, PlayerState{ID: string(id), Name: p.Name, X: p.Pos.X, Y: p.Pos.Y,
			Team: teamName(p.Team), Role: roleName(p.Role), Glyph: glyph, Out: p.Out(), HasFlag: p.HasFlag})
	}
	for _, pr := range w.Projectiles {
		st.Projectiles = append(st.Projectiles, ProjectileState{X: pr.Pos.X, Y: pr.Pos.Y, Dir: int(pr.Dir), Team: teamName(pr.Team)})
	}
	for pos, wl := range w.Walls {
		st.Walls = append(st.Walls, WallState{X: pos.X, Y: pos.Y, Team: teamName(wl.Team)})
	}
	sort.Slice(st.Walls, func(i, j int) bool {
		a, b := st.Walls[i], st.Walls[j]
		return a.Y < b.Y || (a.Y == b.Y && a.X < b.X)
	})
	for _, f := range w.Flags {
		st.Flags = append(st.Flags, FlagState{Team: teamName(f.Team), X: f.Pos.X, Y: f.Pos.Y, Carrier: string(f.Carrier)})
	}
	for _, e := range events {
		st.Events = append(st.Events, EventMsg{Kind: eventNames[e.Kind], Player: string(e.Player),
			Other: string(e.Other), Team: teamName(e.Team), X: e.Pos.X, Y: e.Pos.Y})
	}
	return st
}

// endRound broadcasts the result, empties the world, persists the board and
// stats, and schedules the next round.
func (r *Room) endRound(team game.Team, now time.Time) {
	winner := "draw"
	if team != game.NoTeam {
		winner = teamName(team)
	}
	for pid := range r.rs.carryStart {
		r.rs.endCarry(pid, r.world.Tick)
	}
	scores := r.world.Scores()
	thisRound := topEntries(r.rs.captures, boardTopN)
	var allTime []factions.Entry
	if r.board != nil {
		allTime = r.board.Top(boardTopN)
	}
	if allTime == nil {
		allTime = []factions.Entry{}
	}
	b, err := json.Marshal(RoundEndMsg{T: TypeRoundEnd, Winner: winner, Scores: scores,
		TopFactionsThisRound: thisRound, TopFactionsAllTime: allTime})
	if err == nil {
		for c := range r.conns {
			r.send(c, b)
		}
	}

	r.logTickTiming()

	for id := range r.world.Players {
		r.world.RemovePlayer(id)
	}
	for _, s := range r.sessions {
		s.waiting = false
	}
	r.inRound = false
	r.pending = map[game.PlayerID]game.Intent{}
	r.intermissionUntil = now.Add(r.opts.Intermission)
	r.lobbyDirty = true

	if r.board != nil {
		if err := r.board.Save(); err != nil {
			log.Printf("server: saving board: %v", err)
		}
	}
	if r.log != nil {
		mean := 0.0
		if r.rs.carryCount > 0 {
			mean = float64(r.rs.carryTotal) / float64(r.rs.carryCount)
		}
		err := r.log.Append(stats.RoundStats{
			Round: r.round, Winner: winner, Scores: scores,
			CapturesByFaction: r.rs.captures, TagsByPlayer: r.rs.tags,
			MeanCarryTicks: mean, WallsPlaced: r.rs.wallsPlaced, WallsBroken: r.rs.wallsBroken,
		})
		if err != nil {
			log.Printf("server: appending stats: %v", err)
		}
	}
}

// topEntries sorts a faction tally like Board.Top.
func topEntries(m map[string]int, n int) []factions.Entry {
	out := make([]factions.Entry, 0, len(m))
	for name, c := range m {
		out = append(out, factions.Entry{Name: name, Captures: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Captures != out[j].Captures {
			return out[i].Captures > out[j].Captures
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// startRound makes a fresh world and re-admits everyone who has a role:
// faction members first (so sides follow their factions), then factionless
// players in join order, balanced by AssignTeam.
func (r *Room) startRound(now time.Time) {
	seed := r.rng.Int63()
	if r.opts.Seed != 0 {
		seed = r.opts.Seed + int64(r.round)
	}
	w, err := game.NewWorld(r.cfg, seed)
	if err != nil {
		log.Printf("server: new world: %v", err)
		r.intermissionUntil = now.Add(r.opts.Intermission)
		return
	}
	r.world = w
	r.mapMsg = mapMsg(w.Map)
	r.round++
	r.inRound = true
	r.rs = newRoundStats()
	r.pending = map[game.PlayerID]game.Intent{}

	seen := map[game.PlayerID]bool{}
	for _, f := range r.reg.List() {
		for _, m := range f.Members {
			if s := r.sessions[m]; s != nil && s.hasRole {
				seen[m] = true
				r.enterOrWait(s)
			}
		}
	}
	for _, s := range r.sessionsBySeq() {
		if s.hasRole && !seen[s.id] {
			r.enterOrWait(s)
		}
	}
	for c := range r.conns {
		r.sendRound(c)
	}
	r.lobbyDirty = true
}

// sendRound sends the round frame; You is set when c's player is in the world.
func (r *Room) sendRound(c *conn) {
	msg := RoundMsg{T: TypeRound, Round: r.round, Map: r.mapMsg}
	if s := c.sess; s != nil {
		if p := r.world.Players[s.id]; p != nil && r.inRound {
			msg.You = &YouInfo{ID: string(p.ID), Team: teamName(p.Team), Role: roleName(p.Role)}
		}
	}
	r.sendJSON(c, msg)
}
