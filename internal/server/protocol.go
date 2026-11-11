// Package server runs the single global game room over WebSockets.
//
// Every frame is a JSON object whose "t" field names its type. The web client
// mirrors the types in this file.
package server

import (
	"hashflag/internal/factions"
	"hashflag/internal/game"
)

// MaxFrameBytes is the largest client frame that is processed; larger frames
// are dropped without closing the connection.
const MaxFrameBytes = 1024

// Client-to-server frame types.
const (
	TypeHello         = "hello"
	TypeSetRole       = "setRole"
	TypeCreateFaction = "createFaction"
	TypeJoinFaction   = "joinFaction"
	TypeLeaveFaction  = "leaveFaction"
	TypeRerollFaction = "rerollFaction"
	TypeIntent        = "intent"
)

// Server-to-client frame types.
const (
	TypeWelcome  = "welcome"
	TypeLobby    = "lobby"
	TypeRound    = "round"
	TypeState    = "state"
	TypeYou      = "you"
	TypeRoundEnd = "roundEnd"
	TypeError    = "error"
)

// ClientMsg is any client-to-server frame. Which fields matter depends on T:
//
//	hello{token?, name}  setRole{role}  createFaction  joinFaction{id}
//	leaveFaction  rerollFaction  intent{kind, dir}
type ClientMsg struct {
	T     string `json:"t"`
	Token string `json:"token,omitempty"`
	Name  string `json:"name,omitempty"`
	Role  string `json:"role,omitempty"` // "attacker" | "defender"
	ID    string `json:"id,omitempty"`   // faction id for joinFaction
	Kind  string `json:"kind,omitempty"` // "move" | "throw" | "wall" | "stay"
	Dir   *int   `json:"dir,omitempty"`  // 0..7 clockwise from north
}

// WelcomeMsg answers hello. Keep the token secret; it restores the slot.
type WelcomeMsg struct {
	T        string `json:"t"`
	Token    string `json:"token"`
	PlayerID string `json:"playerId"`
}

// FactionInfo is one live faction in the lobby list.
type FactionInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Glyph   string `json:"glyph"`
	Team    string `json:"team"` // "A" | "B"
	Members int    `json:"members"`
}

// LobbyMsg is the lobby view for one player.
type LobbyMsg struct {
	T         string           `json:"t"`
	Factions  []FactionInfo    `json:"factions"`
	Board     []factions.Entry `json:"board"`
	Waiting   bool             `json:"waiting"`   // wants in but no room yet
	InWorld   bool             `json:"inWorld"`   // currently playing
	Role      string           `json:"role"`      // "" until chosen
	FactionID string           `json:"factionId"` // "" if none
}

// RectMsg is an inclusive cell rectangle.
type RectMsg struct {
	X0 int `json:"x0"`
	Y0 int `json:"y0"`
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
}

// PosMsg is a cell.
type PosMsg struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// MapMsg is the static map. Rows use '#' for obstacles and '.' for floor.
type MapMsg struct {
	Width  int      `json:"width"`
	Height int      `json:"height"`
	Rows   []string `json:"rows"`
	SafeX0 int      `json:"safeX0"`
	SafeX1 int      `json:"safeX1"`
	BaseA  RectMsg  `json:"baseA"`
	BaseB  RectMsg  `json:"baseB"`
	FlagA  PosMsg   `json:"flagA"`
	FlagB  PosMsg   `json:"flagB"`
}

// YouInfo identifies the receiving player in a round frame.
type YouInfo struct {
	ID   string `json:"id"`
	Team string `json:"team"`
	Role string `json:"role"`
}

// RoundMsg is sent once per round (and on joining or reconnecting). You is
// null for players watching from the lobby.
type RoundMsg struct {
	T     string   `json:"t"`
	Round int      `json:"round"`
	Map   MapMsg   `json:"map"`
	You   *YouInfo `json:"you"`
}

// PlayerState is one player in a state frame.
type PlayerState struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Team    string `json:"team"`
	Role    string `json:"role"`
	Glyph   string `json:"glyph"` // faction emoji, "" if factionless
	Out     bool   `json:"out"`
	HasFlag bool   `json:"hasFlag"`
}

// ProjectileState is one projectile in flight.
type ProjectileState struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Dir  int    `json:"dir"`
	Team string `json:"team"`
}

// WallState is one placed wall.
type WallState struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Team string `json:"team"`
}

// FlagState is a flag; Carrier is a player id or "".
type FlagState struct {
	Team    string `json:"team"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Carrier string `json:"carrier"`
}

// EventMsg is a notable thing that happened during the tick. Kind is one of
// tagged, flagPickedUp, flagDropped, flagReturned, captured, wallPlaced,
// wallBroken. For tagged, Player is the victim and Other the thrower; for
// flag events Team is the flag's team; for wallBroken Other broke it.
type EventMsg struct {
	Kind   string `json:"kind"`
	Player string `json:"player,omitempty"`
	Other  string `json:"other,omitempty"`
	Team   string `json:"team"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

// StateMsg is the full dynamic state after a tick, identical for everyone.
type StateMsg struct {
	T           string            `json:"t"`
	Tick        int               `json:"tick"`
	Players     []PlayerState     `json:"players"`
	Projectiles []ProjectileState `json:"projectiles"`
	Walls       []WallState       `json:"walls"`
	Flags       []FlagState       `json:"flags"`
	Scores      [2]int            `json:"scores"` // [A, B]
	Events      []EventMsg        `json:"events"`
}

// YouMsg is the private per-player status sent after each state frame to
// players in the world.
type YouMsg struct {
	T            string `json:"t"`
	Ammo         int    `json:"ammo"`
	CooldownLeft int    `json:"cooldownLeft"`
	OutLeft      int    `json:"outLeft"`
	HasFlag      bool   `json:"hasFlag"`
	WallsPlaced  int    `json:"wallsPlaced"`
}

// RoundEndMsg closes a round.
type RoundEndMsg struct {
	T                    string           `json:"t"`
	Winner               string           `json:"winner"` // "A" | "B" | "draw"
	Scores               [2]int           `json:"scores"`
	TopFactionsThisRound []factions.Entry `json:"topFactionsThisRound"`
	TopFactionsAllTime   []factions.Entry `json:"topFactionsAllTime"`
}

// ErrorMsg reports a rejected request; the connection stays open.
type ErrorMsg struct {
	T       string `json:"t"`
	Message string `json:"message"`
}

func teamName(t game.Team) string {
	if t == game.TeamB {
		return "B"
	}
	return "A"
}

func roleName(r game.Role) string {
	if r == game.Defender {
		return "defender"
	}
	return "attacker"
}

func parseRole(s string) (game.Role, bool) {
	switch s {
	case "attacker":
		return game.Attacker, true
	case "defender":
		return game.Defender, true
	}
	return 0, false
}

var eventNames = map[game.EventKind]string{
	game.Tagged:       "tagged",
	game.FlagPickedUp: "flagPickedUp",
	game.FlagDropped:  "flagDropped",
	game.FlagReturned: "flagReturned",
	game.Captured:     "captured",
	game.WallPlaced:   "wallPlaced",
	game.WallBroken:   "wallBroken",
}

// parseIntent validates an intent frame.
func parseIntent(m ClientMsg) (game.Intent, bool) {
	var k game.IntentKind
	switch m.Kind {
	case "stay":
		return game.Intent{Kind: game.Stay}, true
	case "move":
		k = game.Move
	case "throw":
		k = game.Throw
	case "wall":
		k = game.PlaceWall
	default:
		return game.Intent{}, false
	}
	if m.Dir == nil || *m.Dir < 0 || *m.Dir > 7 {
		return game.Intent{}, false
	}
	return game.Intent{Kind: k, Dir: game.Dir(*m.Dir)}, true
}

// validClientType reports whether t is a known client frame type.
func validClientType(t string) bool {
	switch t {
	case TypeHello, TypeSetRole, TypeCreateFaction, TypeJoinFaction,
		TypeLeaveFaction, TypeRerollFaction, TypeIntent:
		return true
	}
	return false
}

func mapMsg(m *game.Map) MapMsg {
	rows := make([]string, m.Height)
	buf := make([]byte, m.Width)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.Obstacle[y][x] {
				buf[x] = '#'
			} else {
				buf[x] = '.'
			}
		}
		rows[y] = string(buf)
	}
	rect := func(r game.Rect) RectMsg { return RectMsg{r.X0, r.Y0, r.X1, r.Y1} }
	return MapMsg{
		Width: m.Width, Height: m.Height, Rows: rows,
		SafeX0: m.SafeX0, SafeX1: m.SafeX1,
		BaseA: rect(m.BaseA), BaseB: rect(m.BaseB),
		FlagA: PosMsg{m.FlagA.X, m.FlagA.Y}, FlagB: PosMsg{m.FlagB.X, m.FlagB.Y},
	}
}
