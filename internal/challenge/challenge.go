// Package challenge defines a rotating daily objective, deterministically
// selected by calendar day so every profile sees the same challenge on the
// same day without needing to persist which one was picked.
package challenge

import (
	"hash/fnv"
	"time"
)

// Run is the shape of a just-completed exercise, evaluated against a
// Challenge to see if it satisfies it.
type Run struct {
	Mode        string // "time" or "words"
	Target      int
	Punctuation bool
	ZenMode     bool
	WPM         float64
	Accuracy    float64
	KO          bool
}

// Challenge is one daily objective.
type Challenge struct {
	ID          string
	Description string
	XPBonus     int
	check       func(Run) bool
}

// Completed reports whether r satisfies c.
func (c Challenge) Completed(r Run) bool {
	return c.check(r)
}

// all is the fixed pool Today picks from. Every entry is checkable from a
// single run's own fields -- no objective needs today's exercise count or
// other cross-run state, so completion can be judged the instant a run
// finishes.
var all = []Challenge{
	{ID: "punct_60", Description: "Complete a 60-second exercise with punctuation enabled", XPBonus: 50,
		check: func(r Run) bool { return !r.KO && r.Mode == "time" && r.Target == 60 && r.Punctuation }},
	{ID: "words_100_zen", Description: "Complete a 100-word exercise in zen mode", XPBonus: 50,
		check: func(r Run) bool { return !r.KO && r.Mode == "words" && r.Target == 100 && r.ZenMode }},
	{ID: "accuracy_95", Description: "Score at least 95% accuracy on any exercise", XPBonus: 50,
		check: func(r Run) bool { return !r.KO && r.Accuracy >= 95 }},
	{ID: "wpm_50", Description: "Hit at least 50 WPM on any exercise", XPBonus: 50,
		check: func(r Run) bool { return !r.KO && r.WPM >= 50 }},
	{ID: "words_200", Description: "Complete a 200-word (or longer) exercise", XPBonus: 50,
		check: func(r Run) bool { return !r.KO && r.Mode == "words" && r.Target >= 200 }},
	{ID: "time_120", Description: "Complete a 2-minute (or longer) exercise", XPBonus: 50,
		check: func(r Run) bool { return !r.KO && r.Mode == "time" && r.Target >= 120 }},
}

// Today returns the challenge selected for date's calendar day (local,
// "2006-01-02" format) -- the same day always maps to the same challenge,
// and it changes at local midnight.
func Today(date time.Time) Challenge {
	h := fnv.New32a()
	h.Write([]byte(Day(date)))
	return all[int(h.Sum32())%len(all)]
}

// Day returns date's calendar day as the stable key used both to pick
// today's challenge and to record whether it's been claimed.
func Day(date time.Time) string {
	return date.Format("2006-01-02")
}
