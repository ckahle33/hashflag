package lobby

import (
	"testing"

	"hashflag/internal/game"
)

func TestAssignTeamBalances(t *testing.T) {
	cases := []struct {
		counts [2]int
		want   game.Team
	}{
		{[2]int{0, 0}, game.TeamA},
		{[2]int{3, 3}, game.TeamA},
		{[2]int{3, 2}, game.TeamB},
		{[2]int{2, 3}, game.TeamA},
	}
	for _, c := range cases {
		got, ok := AssignTeam(c.counts, 10)
		if !ok || got != c.want {
			t.Errorf("AssignTeam(%v) = %v,%v want %v,true", c.counts, got, ok, c.want)
		}
	}
}

func TestBothFullReturnsFalse(t *testing.T) {
	if _, ok := AssignTeam([2]int{5, 5}, 5); ok {
		t.Error("expected false when both full")
	}
	if tm, ok := AssignTeam([2]int{5, 4}, 5); !ok || tm != game.TeamB {
		t.Errorf("got %v,%v want TeamB,true", tm, ok)
	}
}

func TestRoleSkewCap(t *testing.T) {
	counts := map[game.Role]int{game.Attacker: 59}
	if !RoleAllowed(counts, game.Attacker, 60, 100) {
		t.Error("60th attacker should be allowed")
	}
	counts[game.Attacker] = 60
	if RoleAllowed(counts, game.Attacker, 60, 100) {
		t.Error("61st attacker should be refused")
	}
	if !RoleAllowed(map[game.Role]int{}, game.Defender, 60, 100) {
		t.Error("first defender allowed")
	}
	if !RoleAllowed(nil, game.Attacker, 60, 100) {
		t.Error("first attacker allowed")
	}
}
