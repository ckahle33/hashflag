package game

// Pos is a grid cell. X grows right, Y grows down.
type Pos struct{ X, Y int }

// Team identifies a side. Team A is on the left, Team B on the right.
type Team int

const (
	TeamA Team = iota
	TeamB
)

// Role is a player's current role.
type Role int

const (
	Attacker Role = iota
	Defender
)

// Dir is one of 8 directions, clockwise from north:
// 0=N 1=NE 2=E 3=SE 4=S 5=SW 6=W 7=NW.
type Dir int

var dirDeltas = [8]Pos{
	{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
}

// Delta returns the unit step for d.
func (d Dir) Delta() Pos { return dirDeltas[((int(d)%8)+8)%8] }

// Rect is an inclusive rectangle of cells.
type Rect struct{ X0, Y0, X1, Y1 int }

// Contains reports whether p lies inside r (inclusive).
func (r Rect) Contains(p Pos) bool {
	return p.X >= r.X0 && p.X <= r.X1 && p.Y >= r.Y0 && p.Y <= r.Y1
}

// Map is the static board. Obstacle is indexed [y][x].
type Map struct {
	Width, Height  int
	Obstacle       [][]bool
	SafeX0, SafeX1 int
	BaseA, BaseB   Rect
	FlagA, FlagB   Pos
}

// PlayerID uniquely identifies a player.
type PlayerID string

// IntentKind is what a player asked to do this tick.
type IntentKind int

const (
	Stay IntentKind = iota
	Move
	Throw
	PlaceWall
)

// Intent is one player's queued action for a tick.
type Intent struct {
	Kind IntentKind
	Dir  Dir
}

// Player is a participant on the board. A player with OutLeft > 0 is out
// (tagged): not on the grid for collision, targeting or refill purposes.
type Player struct {
	ID           PlayerID
	Name         string
	FactionID    string
	Team         Team
	Role         Role
	Pos          Pos
	Ammo         int
	CooldownLeft int
	OutLeft      int
	HasFlag      bool
	WallsPlaced  int
}

// Out reports whether the player is currently tagged out.
func (p *Player) Out() bool { return p.OutLeft > 0 }

// Projectile is a thrown object in flight.
type Projectile struct {
	Pos      Pos
	Dir      Dir
	Owner    PlayerID
	Team     Team
	Traveled int

	// Flight parameters captured from the thrower's role at launch. Zero
	// speed means "use the global projectile config".
	speed, rangeCells    int
	pierce, friendlyFire bool
}

// Flag is a team's flag. Carrier is empty when nobody holds it.
type Flag struct {
	Team       Team
	Home, Pos  Pos
	Carrier    PlayerID
	DroppedFor int
}

// Wall is a placed wall cell.
type Wall struct {
	Owner PlayerID
	Team  Team
	Hits  int // remaining hits; set from walls.hitsToBreak at placement
}

// EventKind classifies an Event.
type EventKind int

const (
	Tagged EventKind = iota
	FlagPickedUp
	FlagDropped
	FlagReturned
	Captured
	WallPlaced
	WallBroken
)

// Event is something notable that happened during a tick.
type Event struct {
	Tick   int
	Kind   EventKind
	Player PlayerID
	Other  PlayerID
	Team   Team
	Pos    Pos
}
