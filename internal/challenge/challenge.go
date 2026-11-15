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
	Quotes      bool
	WPM         float64
	Accuracy    float64
	KO          bool
	// BestWordStreak is the run's longest mistake-free word streak.
	BestWordStreak int
	// Hits is how many mistakes dealt HP damage (or would have, in zen
	// mode).
	Hits int
	// GoldenWords, GreenWords and BlueWords count the special words of
	// each kind landed clean.
	GoldenWords, GreenWords, BlueWords int
}

// Difficulty sets a challenge's XP reward.
type Difficulty int

const (
	Easy Difficulty = iota
	Medium
	Hard
)

func (d Difficulty) String() string {
	switch d {
	case Easy:
		return "easy"
	case Medium:
		return "medium"
	default:
		return "hard"
	}
}

// XP is the daily-challenge reward for d.
func (d Difficulty) XP() int {
	switch d {
	case Easy:
		return 30
	case Medium:
		return 50
	default:
		return 80
	}
}

// Challenge is one daily objective.
type Challenge struct {
	ID          string
	Description string
	Difficulty  Difficulty
	XPBonus     int // Difficulty.XP(), filled in by the pool
	check       func(Run) bool
}

func daily(id string, d Difficulty, description string, check func(Run) bool) Challenge {
	return Challenge{ID: id, Description: description, Difficulty: d, XPBonus: d.XP(), check: check}
}

// Completed reports whether r satisfies c. A knocked-out run never does.
func (c Challenge) Completed(r Run) bool {
	return !r.KO && c.check(r)
}

// all is the fixed pool Today picks from. Every entry is checkable from a
// single run's own fields -- no objective needs today's exercise count or
// other cross-run state, so completion can be judged the instant a run
// finishes. Cross-run goals are the weekly challenge's job (see Weekly).
//
// Today picks by hashing the date over len(all), so adding to the pool
// changes which challenge a given day shows. That can't hand out a second
// bonus: claims are recorded per day, not per challenge.
//
// Nothing relies on the golden word alone, since most runs don't have one.
var all = []Challenge{
	daily("punct_60", Medium, "Complete a 60-second exercise with punctuation enabled",
		func(r Run) bool { return r.Mode == "time" && r.Target == 60 && r.Punctuation }),
	daily("words_100_zen", Easy, "Complete a 100-word exercise in zen mode",
		func(r Run) bool { return r.Mode == "words" && r.Target == 100 && r.ZenMode }),
	daily("accuracy_95", Medium, "Score at least 95% accuracy on any exercise",
		func(r Run) bool { return r.Accuracy >= 95 }),
	daily("wpm_50", Medium, "Hit at least 50 WPM on any exercise",
		func(r Run) bool { return r.WPM >= 50 }),
	daily("words_200", Medium, "Complete a 200-word (or longer) exercise",
		func(r Run) bool { return r.Mode == "words" && r.Target >= 200 }),
	daily("time_120", Medium, "Complete a 2-minute (or longer) exercise",
		func(r Run) bool { return r.Mode == "time" && r.Target >= 120 }),

	daily("quotes_any", Easy, "Complete an exercise in quotes mode",
		func(r Run) bool { return r.Quotes }),
	daily("quotes_95", Hard, "Score at least 95% accuracy on a quotes exercise",
		func(r Run) bool { return r.Quotes && r.Accuracy >= 95 }),
	daily("punct_95", Hard, "Score at least 95% accuracy with punctuation enabled",
		func(r Run) bool { return r.Punctuation && r.Accuracy >= 95 }),
	daily("wpm_70", Hard, "Hit at least 70 WPM on any exercise",
		func(r Run) bool { return r.WPM >= 70 }),
	daily("streak_25", Medium, "Reach a 25-word mistake-free streak",
		func(r Run) bool { return r.BestWordStreak >= 25 }),
	daily("streak_50", Hard, "Reach a 50-word mistake-free streak",
		func(r Run) bool { return r.BestWordStreak >= 50 }),
	daily("no_hits_30", Medium, "Finish a 30-second (or longer) exercise without taking a hit",
		func(r Run) bool { return r.Mode == "time" && r.Target >= 30 && r.Hits == 0 }),
	daily("green_word", Medium, "Land a green (+) word to heal",
		func(r Run) bool { return r.GreenWords >= 1 }),
	daily("blue_word", Medium, "Land a blue (~) word for a timed shield",
		func(r Run) bool { return r.BlueWords >= 1 }),
	daily("special_2", Hard, "Land two special words (*, + or ~) in one exercise",
		func(r Run) bool { return r.GoldenWords+r.GreenWords+r.BlueWords >= 2 }),
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
