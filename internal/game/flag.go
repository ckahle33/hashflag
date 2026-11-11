package game

// dropCarried makes p's flag (if p carries one) a dropped flag at p's
// position. It reports whether a flag was dropped.
func (w *World) dropCarried(p *Player) bool {
	if p == nil || !p.HasFlag {
		return false
	}
	p.HasFlag = false
	for i := range w.Flags {
		f := &w.Flags[i]
		if f.Carrier == p.ID {
			f.Carrier = ""
			f.Pos = p.Pos
			f.DroppedFor = 0
			return true
		}
	}
	return false
}

// DropFlagOf drops the flag pid carries, if any, at pid's position and
// returns the FlagDropped event. The player stays in the world. It is for
// callers outside Step, such as a disconnect.
func (w *World) DropFlagOf(pid PlayerID) []Event {
	p := w.Players[pid]
	if p == nil || !p.HasFlag {
		return nil
	}
	for i := range w.Flags {
		if w.Flags[i].Carrier == pid {
			if !w.dropCarried(p) {
				return nil
			}
			p.noPickup, p.noPickupPos = true, p.Pos
			f := &w.Flags[i]
			return []Event{{Tick: w.Tick, Kind: FlagDropped, Player: pid, Team: f.Team, Pos: f.Pos}}
		}
	}
	return nil
}

func (w *World) returnFlag(f *Flag) {
	f.Pos = f.Home
	f.DroppedFor = 0
}

// resolveFlags is step 6b: flag drop, follow, return, pickup and capture.
// Everything runs in sorted PlayerID order / flag index order.
func (w *World) resolveFlags(events *[]Event) {
	emit := func(k EventKind, p *Player, f *Flag) {
		e := Event{Tick: w.Tick, Kind: k, Team: f.Team, Pos: f.Pos}
		if p != nil {
			e.Player = p.ID
		}
		*events = append(*events, e)
	}

	// A. Age dropped flags and auto-return them.
	for i := range w.Flags {
		f := &w.Flags[i]
		if f.Carrier != "" || f.Pos == f.Home {
			continue
		}
		f.DroppedFor++
		if n := w.Cfg.Flag.AutoReturnTicks; n != nil && f.DroppedFor >= *n {
			w.returnFlag(f)
			emit(FlagReturned, nil, f)
		}
	}

	// B. Out carriers drop their flag in place.
	for _, id := range w.order {
		p := w.Players[id]
		if p.Out() && p.HasFlag {
			for i := range w.Flags {
				if w.Flags[i].Carrier == id {
					if w.dropCarried(p) {
						emit(FlagDropped, p, &w.Flags[i])
					}
					break
				}
			}
			p.HasFlag = false
		}
	}

	// C. Carried flags follow their carrier.
	for i := range w.Flags {
		f := &w.Flags[i]
		if f.Carrier != "" {
			if p := w.Players[f.Carrier]; p != nil {
				f.Pos = p.Pos
			}
		}
	}

	// D. Teammates return their own dropped flag by touching it.
	for i := range w.Flags {
		f := &w.Flags[i]
		if f.Carrier != "" || f.Pos == f.Home {
			continue
		}
		for _, id := range w.order {
			p := w.Players[id]
			if !p.Out() && p.Team == f.Team && p.Pos == f.Pos {
				w.returnFlag(f)
				emit(FlagReturned, p, f)
				break
			}
		}
	}

	// E. Attackers pick up the enemy flag; lowest PlayerID wins a shared cell.
	for i := range w.Flags {
		f := &w.Flags[i]
		if f.Carrier != "" {
			continue
		}
		for _, id := range w.order {
			p := w.Players[id]
			if p.noPickup && p.Pos != p.noPickupPos {
				p.noPickup = false
			}
			if p.Out() || p.HasFlag || p.Role != Attacker || p.Team == f.Team || p.Pos != f.Pos || p.noPickup {
				continue
			}
			f.Carrier = id
			f.DroppedFor = 0
			p.HasFlag = true
			emit(FlagPickedUp, p, f)
			break
		}
	}

	// F. Carriers inside their own base capture.
	for i := range w.Flags {
		f := &w.Flags[i]
		if f.Carrier == "" {
			continue
		}
		p := w.Players[f.Carrier]
		if p == nil || p.Out() || !w.base(p.Team).Contains(p.Pos) {
			continue
		}
		w.scores[p.Team]++
		p.HasFlag = false
		f.Carrier = ""
		w.returnFlag(f)
		emit(Captured, p, f)
	}
}

// Scores returns captures per team.
func (w *World) Scores() [2]int { return w.scores }

// Winner reports whether the round is over and who won (NoTeam on a tie).
func (w *World) Winner() (team Team, done bool) {
	s := w.scores
	lead := func() Team {
		switch {
		case s[TeamA] > s[TeamB]:
			return TeamA
		case s[TeamB] > s[TeamA]:
			return TeamB
		}
		return NoTeam
	}
	if w.Cfg.Win.Mode == "firstToCaptures" {
		if s[TeamA] >= w.Cfg.Win.Captures || s[TeamB] >= w.Cfg.Win.Captures {
			return lead(), true
		}
	}
	if w.Tick >= w.Cfg.RoundTicks {
		return lead(), true
	}
	return NoTeam, false
}
