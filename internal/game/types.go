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
