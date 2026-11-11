// Package lobby holds team and role assignment rules. Functions are pure.
package lobby

import "hashflag/internal/game"

// AssignTeam returns the side with fewer players (TeamA on a tie), or false
// if both sides have reached max.
func AssignTeam(counts [2]int, max int) (game.Team, bool) {
	if counts[0] >= max && counts[1] >= max {
		return game.TeamA, false
	}
	if counts[0] <= counts[1] {
		if counts[0] < max {
			return game.TeamA, true
		}
	}
	if counts[1] < max {
		return game.TeamB, true
	}
	return game.TeamA, true
}

// RoleAllowed reports whether one more player may take role without the role
// exceeding skewPct percent of maxPerTeam.
func RoleAllowed(counts map[game.Role]int, role game.Role, skewPct, maxPerTeam int) bool {
	return counts[role]+1 <= skewPct*maxPerTeam/100
}
