package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/coder/websocket"

	"hashflag/internal/factions"
	"hashflag/internal/game"
	"hashflag/internal/lobby"
)

const maxNameRunes = 20

// session is a player identity that outlives individual connections.
type session struct {
	id         game.PlayerID
	seq        int
	token      string
	name       string
	role       game.Role
	hasRole    bool
	conn       *conn     // nil while disconnected
	graceUntil time.Time // when a disconnected slot is released
	waiting    bool      // has a role but no room on a side yet
}

func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return hex.EncodeToString(b)
}

// sanitizeName trims, strips control and format characters, and caps the
// length. It returns "" when nothing printable is left.
func sanitizeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if rs := []rune(s); len(rs) > maxNameRunes {
		s = strings.TrimSpace(string(rs[:maxNameRunes]))
	}
	return s
}

func (r *Room) sessionsBySeq() []*session {
	out := make([]*session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq < out[j].seq })
	return out
}

func (r *Room) sendError(c *conn, msg string) {
	r.sendJSON(c, ErrorMsg{T: TypeError, Message: msg})
}

func (r *Room) handleClient(c *conn, m ClientMsg) {
	if m.T == TypeHello {
		r.hello(c, m)
		return
	}
	s := c.sess
	if s == nil {
		r.sendError(c, "say hello first")
		return
	}
	if m.T == TypeIntent {
		if in, ok := parseIntent(m); ok {
			r.setIntent(s.id, in)
		}
		return
	}
	if r.inWorld(s.id) {
		r.sendError(c, "you are in the round; wait for it to end")
		return
	}
	switch m.T {
	case TypeSetRole:
		r.setRole(s, m.Role)
	case TypeCreateFaction:
		team, _ := lobby.AssignTeam(r.plannedCounts(), r.cfg.MaxPerTeam)
		r.reg.Create(s.id, team)
	case TypeJoinFaction:
		if err := r.reg.Join(m.ID, s.id, r.plannedCounts(), r.cfg.MaxPerTeam); err != nil {
			r.sendError(c, err.Error())
			return
		}
	case TypeLeaveFaction:
		r.reg.Leave(s.id)
	case TypeRerollFaction:
		f := r.reg.FactionOf(s.id)
		if f == nil {
			r.sendError(c, "you are not in a faction")
			return
		}
		if err := r.reg.Reroll(f.ID, s.id); err != nil {
			r.sendError(c, err.Error())
			return
		}
	}
	r.lobbyDirty = true
	if s.conn != nil {
		r.sendLobbyNow(s)
	}
}

// hello creates a session, or restores one from a valid token (replacing any
// connection still attached to it).
func (r *Room) hello(c *conn, m ClientMsg) {
	if c.sess != nil {
		r.sendError(c, "already greeted")
		return
	}
	s := r.byToken[m.Token]
	if m.Token == "" || s == nil {
		r.nextSeq++
		name := sanitizeName(m.Name)
		if name == "" {
			name = fmt.Sprintf("anon-%d", r.nextSeq)
		}
		s = &session{id: game.PlayerID(fmt.Sprintf("p%d", r.nextSeq)), seq: r.nextSeq, token: newToken(), name: name}
		r.sessions[s.id] = s
		r.byToken[s.token] = s
	} else if old := s.conn; old != nil && old != c {
		old.sess = nil
		r.dropConn(old, websocket.StatusNormalClosure, "replaced by a newer connection")
	}
	s.conn = c
	s.graceUntil = time.Time{}
	c.sess = s
	r.sendJSON(c, WelcomeMsg{T: TypeWelcome, Token: s.token, PlayerID: string(s.id)})
	r.sendLobbyNow(s)
	if r.inRound {
		r.sendRound(c)
	}
	r.lobbyDirty = true
}

func (r *Room) setRole(s *session, role string) {
	ro, ok := parseRole(role)
	if !ok {
		r.sendError(s.conn, "unknown role")
		return
	}
	s.role, s.hasRole = ro, true
	entered, reason := r.tryEnter(s)
	r.lobbyDirty = true
	r.sendLobbyNow(s)
	if entered {
		r.sendRound(s.conn)
	} else if reason != "" {
		r.sendError(s.conn, reason)
	}
}

func (r *Room) teamCounts() [2]int {
	var n [2]int
	if r.inRound && r.world != nil {
		for _, p := range r.world.Players {
			n[p.Team]++
		}
	}
	return n
}

// plannedCounts is side sizes for faction decisions: players in the world
// plus faction members who are not (yet) in it. Between rounds nobody is in
// the world, so this keeps new factions balanced during the intermission.
func (r *Room) plannedCounts() [2]int {
	n := r.teamCounts()
	for _, f := range r.reg.List() {
		for _, m := range f.Members {
			if !r.inWorld(m) {
				n[f.Team]++
			}
		}
	}
	return n
}

func (r *Room) roleCounts(t game.Team) map[game.Role]int {
	n := map[game.Role]int{}
	for _, p := range r.world.Players {
		if p.Team == t {
			n[p.Role]++
		}
	}
	return n
}

// tryEnter puts s into the current round. On failure s waits and reason
// explains why ("" when there is simply no round running).
func (r *Room) tryEnter(s *session) (bool, string) {
	if !s.hasRole || !r.inRound || r.world == nil {
		return false, ""
	}
	if r.world.Players[s.id] != nil {
		s.waiting = false
		return true, ""
	}
	maxTeam := r.cfg.MaxPerTeam
	counts := r.teamCounts()
	var team game.Team
	fid := ""
	if f := r.reg.FactionOf(s.id); f != nil {
		team, fid = f.Team, f.ID
		if counts[team] >= maxTeam {
			s.waiting = true
			return false, "your faction's side is full; waiting for a slot"
		}
	} else {
		t, ok := lobby.AssignTeam(counts, maxTeam)
		if !ok {
			s.waiting = true
			return false, "both sides are full; waiting for a slot"
		}
		team = t
	}
	if !lobby.RoleAllowed(r.roleCounts(team), s.role, r.cfg.TeamBalance.MaxRoleSkewPct, maxTeam) {
		s.waiting = true
		return false, "too many " + roleName(s.role) + "s on your side; waiting for a slot"
	}
	p := r.world.AddPlayer(game.Player{ID: s.id, Name: s.name, FactionID: fid, Team: team, Role: s.role})
	if p == nil {
		s.waiting = true
		return false, "no room on the map; waiting for a slot"
	}
	s.waiting = false
	return true, ""
}

// admitWaiting retries waiting players in join order.
func (r *Room) admitWaiting() {
	for _, s := range r.sessionsBySeq() {
		if !s.waiting {
			continue
		}
		if ok, _ := r.tryEnter(s); ok {
			r.lobbyDirty = true
			if s.conn != nil {
				r.sendRound(s.conn)
			}
		}
	}
}

// expireGrace releases slots whose reconnect grace has run out. Removing a
// carrier from the world drops their flag where they stood.
func (r *Room) expireGrace(now time.Time) {
	for _, s := range r.sessions {
		if s.conn != nil || s.graceUntil.IsZero() || now.Before(s.graceUntil) {
			continue
		}
		if r.world != nil && r.world.Players[s.id] != nil {
			r.rs.endCarry(s.id, r.world.Tick)
			r.world.RemovePlayer(s.id)
		}
		r.reg.Leave(s.id)
		delete(r.pending, s.id)
		delete(r.byToken, s.token)
		delete(r.sessions, s.id)
		r.lobbyDirty = true
	}
}

// lobbyShared is the part of the lobby frame common to every player.
func (r *Room) lobbyShared() ([]FactionInfo, []factions.Entry) {
	list := r.reg.List()
	fs := make([]FactionInfo, 0, len(list))
	for _, f := range list {
		fs = append(fs, FactionInfo{ID: f.ID, Name: f.Name, Glyph: f.Glyph, Team: teamName(f.Team), Members: len(f.Members)})
	}
	board := []factions.Entry{}
	if r.board != nil {
		board = r.board.Top(boardTopN)
	}
	return fs, board
}

func (r *Room) sendLobbyNow(s *session) {
	fs, board := r.lobbyShared()
	r.sendLobby(s, fs, board)
}

func (r *Room) sendLobby(s *session, fs []FactionInfo, board []factions.Entry) {
	if s.conn == nil {
		return
	}
	msg := LobbyMsg{T: TypeLobby, Factions: fs, Board: board, Waiting: s.waiting,
		InWorld: r.inWorld(s.id), FactionID: ""}
	if s.hasRole {
		msg.Role = roleName(s.role)
	}
	if f := r.reg.FactionOf(s.id); f != nil {
		msg.FactionID = f.ID
	}
	r.sendJSON(s.conn, msg)
}

// flushLobby sends lobby frames to every connected player if anything
// changed since the last flush.
func (r *Room) flushLobby() {
	if !r.lobbyDirty {
		return
	}
	r.lobbyDirty = false
	fs, board := r.lobbyShared()
	for _, s := range r.sessions {
		r.sendLobby(s, fs, board)
	}
}
