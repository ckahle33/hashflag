package server

import (
	"sort"
	"time"
)

// slowTick is the duration above which a single tick is logged.
const slowTick = 100 * time.Millisecond

// tickStats collects per-tick durations (step + marshal + enqueue) for one round.
type tickStats struct{ durs []time.Duration }

func (t *tickStats) add(d time.Duration) { t.durs = append(t.durs, d) }
func (t *tickStats) reset()              { t.durs = t.durs[:0] }

// summary returns the tick count, mean, 95th percentile (nearest rank) and max.
func (t *tickStats) summary() (n int, mean, p95, max time.Duration) {
	n = len(t.durs)
	if n == 0 {
		return
	}
	s := append([]time.Duration(nil), t.durs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var sum time.Duration
	for _, d := range s {
		sum += d
	}
	rank := (n*95 + 99) / 100
	return n, sum / time.Duration(n), s[rank-1], s[n-1]
}

// slowLogLimiter allows at most one slow-tick log line per second.
type slowLogLimiter struct{ last time.Time }

func (l *slowLogLimiter) allow(now time.Time) bool {
	if !l.last.IsZero() && now.Sub(l.last) < time.Second {
		return false
	}
	l.last = now
	return true
}
