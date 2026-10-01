// Package achievements defines unlockable milestones derived entirely from
// data already tracked elsewhere (profile records, daily streak, personal
// bests) -- there's no separate "unlocked" state to persist or drift out
// of sync; an achievement's status is just a pure function of current
// progress, recomputed whenever it's displayed.
package achievements

import "sort"

// Progress bundles the stats achievements are evaluated against.
type Progress struct {
	Level          int
	TotalExercises int // successful (non-KO) exercises completed -- a knockout doesn't count as "complete"
	BestWordStreak int
	KOCount        int
	DailyStreak    int
	BestWPM        float64

	PerfectRunCount       int  // non-KO exercises finished with 100% accuracy
	MaxPerfectStreak      int  // longest run of back-to-back 100%-accuracy exercises
	CompletedLongHaul     bool // finished a 200-word exercise
	CompletedSprint       bool // finished a 15-second exercise
	PracticedLateNight    bool // completed an exercise between midnight and 4am
	PracticedEarlyMorning bool // completed an exercise between 4am and 7am

	HasBestInBothModes     bool // recorded a personal best in both time mode and words mode
	PracticedAllWeekdays   bool // completed an exercise on all 7 days of the week
	HadBigDay              bool // completed at least 10 exercises in a single day
	CompletedUltraLongHaul bool // finished a 500-word exercise
	ComebackCount          int  // times the exercise right after a knockout was completed
	HadPerfectComeback     bool // scored 100% accuracy on the exercise right after a knockout
	PunctuationRunCount    int  // exercises completed with punctuation enabled
	ZenRunCount            int  // exercises completed in zen mode
	TotalWordsTyped        int  // lifetime words typed across all exercises
	TotalPlaySeconds       int  // lifetime exercise duration, across all exercises
}

// Tier ranks how impressive an achievement is, independent of its
// category -- Bronze is an easy first step, Grandmaster is the pinnacle.
// Within each themed progression in All (e.g. total exercises, WPM), every
// distinct threshold gets its own tier, in ascending order, rather than
// reusing one across multiple thresholds -- a category with 6 thresholds
// (the most of any so far) uses all 6 tiers, one each.
type Tier int

const (
	Bronze Tier = iota
	Silver
	Gold
	Diamond
	Master
	Grandmaster
)

func (t Tier) String() string {
	switch t {
	case Bronze:
		return "Bronze"
	case Silver:
		return "Silver"
	case Gold:
		return "Gold"
	case Diamond:
		return "Diamond"
	case Master:
		return "Master"
	case Grandmaster:
		return "Grandmaster"
	default:
		return "Bronze"
	}
}

// Achievement is one unlockable milestone.
type Achievement struct {
	ID          string
	Name        string
	Description string
	Tier        Tier
	check       func(Progress) bool
	// progress reports (current, target, true) for a straightforward
	// numeric-threshold achievement, letting callers show "7/10" style
	// nudges -- see Nearest. It's nil for achievements with no meaningful
	// partial credit (one-off booleans like a time-of-day or word-count
	// milestone that's either done or not).
	progress func(Progress) (current, target float64, ok bool)
}

// Unlocked reports whether p satisfies this achievement.
func (a Achievement) Unlocked(p Progress) bool {
	return a.check(p)
}

// numericProgress builds a progress function for a plain "value >= min"
// achievement, given an accessor into Progress.
func numericProgress(get func(Progress) float64, min float64) func(Progress) (float64, float64, bool) {
	return func(p Progress) (float64, float64, bool) { return get(p), min, true }
}

// All is the full, fixed list of achievements, grouped by theme with tiers
// rising within each group.
var All = []Achievement{
	{ID: "first_exercise", Name: "First Steps", Description: "Complete your first exercise", Tier: Bronze,
		check:    func(p Progress) bool { return p.TotalExercises >= 1 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalExercises) }, 1)},
	{ID: "exercises_10", Name: "Warming Up", Description: "Complete 10 exercises", Tier: Silver,
		check:    func(p Progress) bool { return p.TotalExercises >= 10 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalExercises) }, 10)},
	{ID: "exercises_50", Name: "Regular", Description: "Complete 50 exercises", Tier: Gold,
		check:    func(p Progress) bool { return p.TotalExercises >= 50 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalExercises) }, 50)},
	{ID: "exercises_100", Name: "Centurion", Description: "Complete 100 exercises", Tier: Diamond,
		check:    func(p Progress) bool { return p.TotalExercises >= 100 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalExercises) }, 100)},
	{ID: "exercises_250", Name: "Marathoner", Description: "Complete 250 exercises", Tier: Master,
		check:    func(p Progress) bool { return p.TotalExercises >= 250 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalExercises) }, 250)},
	{ID: "exercises_500", Name: "Veteran", Description: "Complete 500 exercises", Tier: Grandmaster,
		check:    func(p Progress) bool { return p.TotalExercises >= 500 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalExercises) }, 500)},

	{ID: "level_5", Name: "Getting Somewhere", Description: "Reach level 5", Tier: Bronze,
		check:    func(p Progress) bool { return p.Level >= 5 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.Level) }, 5)},
	{ID: "level_10", Name: "Medium Difficulty", Description: "Reach level 10 and unlock Medium words", Tier: Silver,
		check:    func(p Progress) bool { return p.Level >= 10 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.Level) }, 10)},
	{ID: "level_20", Name: "Hard Difficulty", Description: "Reach level 20 and unlock Hard words", Tier: Gold,
		check:    func(p Progress) bool { return p.Level >= 20 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.Level) }, 20)},
	{ID: "level_30", Name: "Expert Difficulty", Description: "Reach level 30 and unlock Expert words", Tier: Diamond,
		check:    func(p Progress) bool { return p.Level >= 30 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.Level) }, 30)},

	{ID: "wpm_30", Name: "30 WPM Club", Description: "Hit 30 WPM in a test", Tier: Bronze,
		check:    func(p Progress) bool { return p.BestWPM >= 30 },
		progress: numericProgress(func(p Progress) float64 { return p.BestWPM }, 30)},
	{ID: "wpm_60", Name: "60 WPM Club", Description: "Hit 60 WPM in a test", Tier: Silver,
		check:    func(p Progress) bool { return p.BestWPM >= 60 },
		progress: numericProgress(func(p Progress) float64 { return p.BestWPM }, 60)},
	{ID: "wpm_100", Name: "Triple Digits", Description: "Hit 100 WPM in a test", Tier: Gold,
		check:    func(p Progress) bool { return p.BestWPM >= 100 },
		progress: numericProgress(func(p Progress) float64 { return p.BestWPM }, 100)},
	{ID: "wpm_120", Name: "Blazing Fast", Description: "Hit 120 WPM in a test", Tier: Diamond,
		check:    func(p Progress) bool { return p.BestWPM >= 120 },
		progress: numericProgress(func(p Progress) float64 { return p.BestWPM }, 120)},
	{ID: "wpm_150", Name: "Superhuman", Description: "Hit 150 WPM in a test", Tier: Master,
		check:    func(p Progress) bool { return p.BestWPM >= 150 },
		progress: numericProgress(func(p Progress) float64 { return p.BestWPM }, 150)},
	{ID: "wpm_200", Name: "Inhuman", Description: "Hit 200 WPM in a test", Tier: Grandmaster,
		check:    func(p Progress) bool { return p.BestWPM >= 200 },
		progress: numericProgress(func(p Progress) float64 { return p.BestWPM }, 200)},

	{ID: "combo_10", Name: "In The Zone", Description: "Reach a 10-word mistake-free streak", Tier: Bronze,
		check:    func(p Progress) bool { return p.BestWordStreak >= 10 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.BestWordStreak) }, 10)},
	{ID: "combo_25", Name: "Flawless", Description: "Reach a 25-word mistake-free streak", Tier: Silver,
		check:    func(p Progress) bool { return p.BestWordStreak >= 25 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.BestWordStreak) }, 25)},
	{ID: "combo_50", Name: "Perfectionist", Description: "Reach a 50-word mistake-free streak", Tier: Gold,
		check:    func(p Progress) bool { return p.BestWordStreak >= 50 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.BestWordStreak) }, 50)},
	{ID: "combo_100", Name: "Untouchable", Description: "Reach a 100-word mistake-free streak", Tier: Diamond,
		check:    func(p Progress) bool { return p.BestWordStreak >= 100 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.BestWordStreak) }, 100)},

	{ID: "daily_3", Name: "Habit Forming", Description: "Practice 3 days in a row", Tier: Bronze,
		check:    func(p Progress) bool { return p.DailyStreak >= 3 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.DailyStreak) }, 3)},
	{ID: "daily_7", Name: "One Week Strong", Description: "Practice 7 days in a row", Tier: Silver,
		check:    func(p Progress) bool { return p.DailyStreak >= 7 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.DailyStreak) }, 7)},
	{ID: "daily_30", Name: "Unstoppable", Description: "Practice 30 days in a row", Tier: Gold,
		check:    func(p Progress) bool { return p.DailyStreak >= 30 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.DailyStreak) }, 30)},
	{ID: "daily_100", Name: "Iron Habit", Description: "Practice 100 days in a row", Tier: Diamond,
		check:    func(p Progress) bool { return p.DailyStreak >= 100 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.DailyStreak) }, 100)},

	{ID: "ko_1", Name: "Down But Not Out", Description: "Survive your first knockout", Tier: Bronze,
		check:    func(p Progress) bool { return p.KOCount >= 1 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.KOCount) }, 1)},
	{ID: "ko_5", Name: "Glutton For Punishment", Description: "Get knocked out 5 times", Tier: Silver,
		check:    func(p Progress) bool { return p.KOCount >= 5 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.KOCount) }, 5)},
	{ID: "ko_10", Name: "Never Say Die", Description: "Get knocked out 10 times", Tier: Gold,
		check:    func(p Progress) bool { return p.KOCount >= 10 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.KOCount) }, 10)},

	{ID: "quick_draw", Name: "Quick Draw", Description: "Complete a 15-second exercise", Tier: Bronze,
		check: func(p Progress) bool { return p.CompletedSprint }},
	{ID: "night_owl", Name: "Night Owl", Description: "Complete an exercise between midnight and 4am", Tier: Silver,
		check: func(p Progress) bool { return p.PracticedLateNight }},
	{ID: "early_bird", Name: "Early Bird", Description: "Complete an exercise between 4am and 7am", Tier: Silver,
		check: func(p Progress) bool { return p.PracticedEarlyMorning }},

	// Single-exercise word count -- how long a single sitting can go.
	{ID: "long_hauler", Name: "Long Hauler", Description: "Complete a 200-word exercise", Tier: Bronze,
		check: func(p Progress) bool { return p.CompletedLongHaul }},
	{ID: "ultramarathoner", Name: "Ultramarathoner", Description: "Complete a 500-word exercise", Tier: Silver,
		check: func(p Progress) bool { return p.CompletedUltraLongHaul }},

	// Accuracy, across a lifetime -- one flawless run, then repeated
	// flawless runs, however far apart.
	{ID: "perfect_run", Name: "Immaculate", Description: "Finish an exercise with 100% accuracy", Tier: Bronze,
		check:    func(p Progress) bool { return p.PerfectRunCount >= 1 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.PerfectRunCount) }, 1)},
	{ID: "sharpshooter", Name: "Sharpshooter", Description: "Finish 10 exercises with 100% accuracy", Tier: Silver,
		check:    func(p Progress) bool { return p.PerfectRunCount >= 10 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.PerfectRunCount) }, 10)},
	{ID: "marksman", Name: "Marksman", Description: "Finish 50 exercises with 100% accuracy", Tier: Gold,
		check:    func(p Progress) bool { return p.PerfectRunCount >= 50 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.PerfectRunCount) }, 50)},
	{ID: "perfect_storm", Name: "Perfect Storm", Description: "Finish 100 exercises with 100% accuracy", Tier: Diamond,
		check:    func(p Progress) bool { return p.PerfectRunCount >= 100 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.PerfectRunCount) }, 100)},

	// Accuracy, back-to-back -- flawless runs strung together with no
	// mistakes (or KOs) in between, which resets the streak to zero.
	{ID: "streak_2", Name: "Back-to-Back", Description: "Score 100% accuracy on 2 exercises in a row", Tier: Bronze,
		check:    func(p Progress) bool { return p.MaxPerfectStreak >= 2 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.MaxPerfectStreak) }, 2)},
	{ID: "streak_3", Name: "Hat Trick", Description: "Score 100% accuracy on 3 exercises in a row", Tier: Silver,
		check:    func(p Progress) bool { return p.MaxPerfectStreak >= 3 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.MaxPerfectStreak) }, 3)},
	{ID: "streak_5", Name: "On Fire", Description: "Score 100% accuracy on 5 exercises in a row", Tier: Gold,
		check:    func(p Progress) bool { return p.MaxPerfectStreak >= 5 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.MaxPerfectStreak) }, 5)},
	{ID: "streak_10", Name: "Perfect Ten", Description: "Score 100% accuracy on 10 exercises in a row", Tier: Diamond,
		check:    func(p Progress) bool { return p.MaxPerfectStreak >= 10 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.MaxPerfectStreak) }, 10)},

	// Resilience -- bouncing back from a knockout, doing it repeatedly, then
	// bouncing back flawlessly.
	{ID: "comeback_kid", Name: "Comeback Kid", Description: "Complete an exercise right after a knockout", Tier: Bronze,
		check:    func(p Progress) bool { return p.ComebackCount >= 1 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.ComebackCount) }, 1)},
	{ID: "unbreakable", Name: "Unbreakable", Description: "Bounce back from a knockout 5 times", Tier: Silver,
		check:    func(p Progress) bool { return p.ComebackCount >= 5 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.ComebackCount) }, 5)},
	{ID: "phoenix", Name: "Phoenix", Description: "Score 100% accuracy on the exercise right after a knockout", Tier: Gold,
		check: func(p Progress) bool { return p.HadPerfectComeback }},

	// Lifetime words typed, across every exercise.
	{ID: "wordsmith_10k", Name: "Wordsmith", Description: "Type 10,000 words lifetime", Tier: Bronze,
		check:    func(p Progress) bool { return p.TotalWordsTyped >= 10000 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalWordsTyped) }, 10000)},
	{ID: "wordsmith_50k", Name: "Prolific", Description: "Type 50,000 words lifetime", Tier: Silver,
		check:    func(p Progress) bool { return p.TotalWordsTyped >= 50000 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalWordsTyped) }, 50000)},
	{ID: "wordsmith_100k", Name: "Logophile", Description: "Type 100,000 words lifetime", Tier: Gold,
		check:    func(p Progress) bool { return p.TotalWordsTyped >= 100000 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalWordsTyped) }, 100000)},

	// Lifetime time played, across every exercise.
	{ID: "time_1h", Name: "First Hour", Description: "Log 1 hour of practice", Tier: Bronze,
		check:    func(p Progress) bool { return p.TotalPlaySeconds >= 3600 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalPlaySeconds) }, 3600)},
	{ID: "time_5h", Name: "Getting Serious", Description: "Log 5 hours of practice", Tier: Silver,
		check:    func(p Progress) bool { return p.TotalPlaySeconds >= 5*3600 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalPlaySeconds) }, 5*3600)},
	{ID: "time_20h", Name: "Dedicated", Description: "Log 20 hours of practice", Tier: Gold,
		check:    func(p Progress) bool { return p.TotalPlaySeconds >= 20*3600 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalPlaySeconds) }, 20*3600)},
	{ID: "time_50h", Name: "Time Well Spent", Description: "Log 50 hours of practice", Tier: Diamond,
		check:    func(p Progress) bool { return p.TotalPlaySeconds >= 50*3600 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalPlaySeconds) }, 50*3600)},
	{ID: "time_100h", Name: "Lifer", Description: "Log 100 hours of practice", Tier: Master,
		check:    func(p Progress) bool { return p.TotalPlaySeconds >= 100*3600 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.TotalPlaySeconds) }, 100*3600)},

	{ID: "well_rounded", Name: "Well Rounded", Description: "Set a personal best in both time mode and words mode", Tier: Bronze,
		check: func(p Progress) bool { return p.HasBestInBothModes }},
	{ID: "full_week", Name: "Full Week", Description: "Practice on all 7 days of the week", Tier: Gold,
		check: func(p Progress) bool { return p.PracticedAllWeekdays }},
	{ID: "daily_grind", Name: "Daily Grind", Description: "Complete 10 exercises in a single day", Tier: Silver,
		check: func(p Progress) bool { return p.HadBigDay }},
	{ID: "grammarian", Name: "Grammarian", Description: "Complete 25 exercises with punctuation enabled", Tier: Silver,
		check:    func(p Progress) bool { return p.PunctuationRunCount >= 25 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.PunctuationRunCount) }, 25)},
	{ID: "zen_garden", Name: "Zen Garden", Description: "Complete 25 exercises in zen mode", Tier: Silver,
		check:    func(p Progress) bool { return p.ZenRunCount >= 25 },
		progress: numericProgress(func(p Progress) float64 { return float64(p.ZenRunCount) }, 25)},
}

// LockedProgress pairs a still-locked achievement with how close it is to
// unlocking.
type LockedProgress struct {
	Achievement Achievement
	Current     float64
	Target      float64
}

// Ratio returns how close this achievement is to unlocking, clamped to
// [0, 1).
func (lp LockedProgress) Ratio() float64 {
	if lp.Target <= 0 {
		return 0
	}
	r := lp.Current / lp.Target
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

// Nearest returns up to n achievements not yet unlocked by p, ranked by how
// close they are to unlocking -- only among achievements with trackable
// numeric progress (see Achievement.progress); one-off boolean
// achievements have no meaningful "how close" and are left out.
func Nearest(p Progress, n int) []LockedProgress {
	var candidates []LockedProgress
	for _, a := range All {
		if a.progress == nil || a.Unlocked(p) {
			continue
		}
		current, target, ok := a.progress(p)
		if !ok {
			continue
		}
		candidates = append(candidates, LockedProgress{Achievement: a, Current: current, Target: target})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Ratio() > candidates[j].Ratio() })
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	return candidates
}

// UnlockedCount returns how many of All are satisfied by p.
func UnlockedCount(p Progress) int {
	n := 0
	for _, a := range All {
		if a.Unlocked(p) {
			n++
		}
	}
	return n
}
