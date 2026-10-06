package audit

import (
	"sync"
	"time"
)

const (
	// statsMinutes is how many per-minute buckets are retained (24 h).
	statsMinutes = 24 * 60
	// rateWindow is the window of the "records per minute" rate.
	rateWindow = 60
)

type minuteBucket struct {
	minute int64 // unix minute this slot currently holds
	count  int64
	errors int64
}

// Stats keeps aggregate counters of the audited requests: per-minute buckets
// for the last 24 hours, per-second buckets for the live rate and today's
// per-operation counts. It is process-local: it counts what this process has
// recorded, which is exactly what Since reports.
type Stats struct {
	mu      sync.Mutex
	now     func() time.Time
	since   time.Time
	minutes [statsMinutes]minuteBucket
	secs    [rateWindow]struct{ sec, count int64 }
	day     int64 // unix day the "today" counters belong to
	today   int64
	todayEr int64
	byOp    map[string]int64
	total   int64
}

// NewStats creates empty Stats. now may be nil (time.Now).
func NewStats(now func() time.Time) *Stats {
	if now == nil {
		now = time.Now
	}
	return &Stats{now: now, since: now().UTC(), byOp: map[string]int64{}}
}

// Record accounts one entry that happened at the current time.
func (s *Stats) Record(e Entry) { s.recordAt(s.now(), e) }

// recordAt accounts e as having happened at t. Records older than the
// retained 24 h (or from the future) are ignored; this is also how the
// counters are repopulated from the log file at startup.
func (s *Stats) recordAt(t time.Time, e Entry) {
	t = t.UTC()
	if n := s.now(); t.After(n.Add(time.Minute)) || t.Before(n.Add(-statsMinutes*time.Minute)) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	min := t.Unix() / 60
	b := &s.minutes[min%statsMinutes]
	if b.minute != min {
		*b = minuteBucket{minute: min}
	}
	b.count++
	if e.Status >= 400 {
		b.errors++
	}

	sec := t.Unix()
	sb := &s.secs[sec%rateWindow]
	if sb.sec != sec {
		sb.sec, sb.count = sec, 0
	}
	sb.count++

	day := sec / 86400
	if day != s.day {
		s.day, s.today, s.todayEr = day, 0, 0
		s.byOp = map[string]int64{}
	}
	s.today++
	if e.Status >= 400 {
		s.todayEr++
	}
	op := e.Operation
	if op == "" {
		op = Operation(e.Method, e.Path)
	}
	s.byOp[op]++
	s.total++
	if t.Before(s.since) {
		s.since = t
	}
}

// MinuteCount is one histogram bar.
type MinuteCount struct {
	Time   string `json:"time"` // RFC3339 start of the minute (UTC)
	Count  int64  `json:"count"`
	Errors int64  `json:"errors"`
}

// StatsSnapshot is a point-in-time view of Stats.
type StatsSnapshot struct {
	Since        string           `json:"since"`
	Total        int64            `json:"total"`
	Today        int64            `json:"today"`
	TodayErrors  int64            `json:"today_errors"`
	LastMinute   int64            `json:"last_minute"` // records in the trailing 60 s
	PerMinute    []MinuteCount    `json:"per_minute"`  // oldest → newest, the current minute last
	ByOperation  map[string]int64 `json:"by_operation"`
	WindowMinute int              `json:"window_minutes"`
}

// Snapshot returns the counters with the last `minutes` per-minute buckets
// (clamped to 1..1440).
func (s *Stats) Snapshot(minutes int) StatsSnapshot {
	if minutes < 1 {
		minutes = 1
	}
	if minutes > statsMinutes {
		minutes = statsMinutes
	}
	t := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := StatsSnapshot{
		Since:        s.since.Format(time.RFC3339),
		Total:        s.total,
		ByOperation:  map[string]int64{},
		PerMinute:    make([]MinuteCount, 0, minutes),
		WindowMinute: minutes,
	}
	nowMin := t.Unix() / 60
	for m := nowMin - int64(minutes) + 1; m <= nowMin; m++ {
		mc := MinuteCount{Time: time.Unix(m*60, 0).UTC().Format(time.RFC3339)}
		if b := s.minutes[((m%statsMinutes)+statsMinutes)%statsMinutes]; b.minute == m {
			mc.Count, mc.Errors = b.count, b.errors
		}
		snap.PerMinute = append(snap.PerMinute, mc)
	}
	nowSec := t.Unix()
	for _, sb := range s.secs {
		if sb.sec > nowSec-rateWindow && sb.sec <= nowSec {
			snap.LastMinute += sb.count
		}
	}
	if nowSec/86400 == s.day {
		snap.Today, snap.TodayErrors = s.today, s.todayEr
		for k, v := range s.byOp {
			snap.ByOperation[k] = v
		}
	}
	return snap
}
