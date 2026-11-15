package challenge

import (
	"testing"
	"time"
)

func TestTodayIsDeterministicWithinACalendarDay(t *testing.T) {
	d1 := time.Date(2026, 3, 15, 0, 0, 1, 0, time.UTC)
	d2 := time.Date(2026, 3, 15, 23, 59, 59, 0, time.UTC)
	if Today(d1).ID != Today(d2).ID {
		t.Fatal("expected the same challenge for the same calendar day regardless of time of day")
	}
}

func TestTodayVariesAcrossDaysAndCoversWholePool(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seen := make(map[string]bool)
	for i := range 365 {
		seen[Today(base.AddDate(0, 0, i)).ID] = true
	}
	if len(seen) != len(all) {
		t.Fatalf("expected all %d challenges to appear across a year, saw %d distinct", len(all), len(seen))
	}
}

func TestDayFormatsAsCalendarDate(t *testing.T) {
	d := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)
	if got := Day(d); got != "2026-03-05" {
		t.Fatalf("Day(...) = %q, want %q", got, "2026-03-05")
	}
}

func TestNoChallengeIsSatisfiedByAKO(t *testing.T) {
	// A run that maximizes every other field but is KO'd must never
	// satisfy any challenge in the pool.
	maxed := Run{Mode: "time", Target: 120, Punctuation: true, ZenMode: true, Quotes: true, WPM: 999, Accuracy: 100,
		BestWordStreak: 999, Hits: 0, GoldenWords: 3, GreenWords: 3, BlueWords: 3}
	for _, c := range all {
		// Each check must pass on some non-KO run, or this test would
		// pass for the wrong reason.
		met := false
		for _, r := range shapes(maxed) {
			met = met || c.Completed(r)
		}
		if !met {
			t.Fatalf("challenge %q isn't met by any maxed-out run; extend them", c.ID)
		}
		for _, r := range shapes(maxed) {
			r.KO = true
			if c.Completed(r) {
				t.Fatalf("challenge %q was satisfied by a KO'd run: %+v", c.ID, r)
			}
		}
	}
}

// shapes returns r in each mode and target the pool asks for.
func shapes(r Run) []Run {
	var out []Run
	for _, mt := range []struct {
		mode   string
		target int
	}{{"time", 60}, {"time", 120}, {"words", 100}, {"words", 200}} {
		r.Mode, r.Target = mt.mode, mt.target
		out = append(out, r)
	}
	return out
}

func TestEachChallengeChecksItsSpecificCondition(t *testing.T) {
	byID := make(map[string]Challenge)
	for _, c := range all {
		byID[c.ID] = c
	}
	// base fails every challenge: a short run with hits, low numbers and
	// no special words.
	base := Run{Mode: "time", Target: 15, WPM: 10, Accuracy: 50, Hits: 3, BestWordStreak: 2}

	cases := []struct {
		id      string
		satisfy func(Run) Run
	}{
		{"punct_60", func(r Run) Run { r.Mode, r.Target, r.Punctuation = "time", 60, true; return r }},
		{"words_100_zen", func(r Run) Run { r.Mode, r.Target, r.ZenMode = "words", 100, true; return r }},
		{"accuracy_95", func(r Run) Run { r.Accuracy = 95; return r }},
		{"wpm_50", func(r Run) Run { r.WPM = 50; return r }},
		{"words_200", func(r Run) Run { r.Mode, r.Target = "words", 200; return r }},
		{"time_120", func(r Run) Run { r.Mode, r.Target = "time", 120; return r }},
		{"quotes_any", func(r Run) Run { r.Quotes = true; return r }},
		{"quotes_95", func(r Run) Run { r.Quotes, r.Accuracy = true, 95; return r }},
		{"punct_95", func(r Run) Run { r.Punctuation, r.Accuracy = true, 95; return r }},
		{"wpm_70", func(r Run) Run { r.WPM = 70; return r }},
		{"streak_25", func(r Run) Run { r.BestWordStreak = 25; return r }},
		{"streak_50", func(r Run) Run { r.BestWordStreak = 50; return r }},
		{"no_hits_30", func(r Run) Run { r.Target, r.Hits = 30, 0; return r }},
		{"green_word", func(r Run) Run { r.GreenWords = 1; return r }},
		{"blue_word", func(r Run) Run { r.BlueWords = 1; return r }},
		{"special_2", func(r Run) Run { r.GoldenWords, r.BlueWords = 1, 1; return r }},
	}
	if len(cases) != len(all) {
		t.Fatalf("test covers %d challenges but the pool has %d -- update this test when the pool changes", len(cases), len(all))
	}

	for _, c := range cases {
		challenge, ok := byID[c.id]
		if !ok {
			t.Fatalf("challenge %q not found in pool", c.id)
		}
		if !challenge.Completed(c.satisfy(base)) {
			t.Errorf("%s: expected a run satisfying its condition to complete it", c.id)
		}
		if challenge.Completed(base) {
			t.Errorf("%s: expected the unmodified base run to NOT complete it", c.id)
		}
	}
}

func TestAllChallengesHaveUniqueIDsAndPositiveBonus(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range all {
		if seen[c.ID] {
			t.Fatalf("duplicate challenge ID %q", c.ID)
		}
		seen[c.ID] = true
		if c.XPBonus <= 0 {
			t.Errorf("%s: expected a positive XP bonus, got %d", c.ID, c.XPBonus)
		}
		if c.Description == "" {
			t.Errorf("%s: expected a non-empty description", c.ID)
		}
	}
}

// Near misses: each threshold is checked just below its bar.
func TestChallengesRejectNearMisses(t *testing.T) {
	byID := make(map[string]Challenge)
	for _, c := range all {
		byID[c.ID] = c
	}
	cases := map[string]Run{
		"quotes_95":  {Mode: "time", Target: 30, Quotes: true, Accuracy: 94.9},
		"punct_95":   {Mode: "time", Target: 30, Punctuation: true, Accuracy: 94.9},
		"wpm_70":     {Mode: "time", Target: 30, WPM: 69.9},
		"streak_50":  {Mode: "time", Target: 30, BestWordStreak: 49},
		"no_hits_30": {Mode: "time", Target: 30, Hits: 1},
		"special_2":  {Mode: "time", Target: 30, GoldenWords: 1},
	}
	for id, r := range cases {
		if byID[id].Completed(r) {
			t.Errorf("%s: a run just short of the bar completed it: %+v", id, r)
		}
	}
	if byID["no_hits_30"].Completed(Run{Mode: "words", Target: 50}) {
		t.Error("no_hits_30: a words run must not count")
	}
}

func TestRewardsDifferByDifficulty(t *testing.T) {
	if !(Easy.XP() < Medium.XP() && Medium.XP() < Hard.XP()) {
		t.Fatalf("expected easy < medium < hard, got %d, %d, %d", Easy.XP(), Medium.XP(), Hard.XP())
	}
	seen := map[Difficulty]bool{}
	for _, c := range all {
		if c.XPBonus != c.Difficulty.XP() {
			t.Errorf("%s: XPBonus %d doesn't match its difficulty's %d", c.ID, c.XPBonus, c.Difficulty.XP())
		}
		seen[c.Difficulty] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected the pool to use all three difficulties, got %v", seen)
	}
	for d, want := range map[Difficulty]string{Easy: "easy", Medium: "medium", Hard: "hard"} {
		if d.String() != want {
			t.Errorf("Difficulty(%d).String() = %q, want %q", d, d.String(), want)
		}
	}
}

func TestWeekKeyUsesTheISOYear(t *testing.T) {
	cases := map[time.Time]string{
		time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC): "2026-W46",
		time.Date(2026, 12, 28, 12, 0, 0, 0, time.UTC): "2026-W53",
		time.Date(2027, 1, 3, 12, 0, 0, 0, time.UTC):   "2026-W53", // a Sunday still in 2026's last week
		time.Date(2027, 1, 4, 12, 0, 0, 0, time.UTC):   "2027-W01",
		time.Date(2024, 12, 30, 12, 0, 0, 0, time.UTC): "2025-W01", // a Monday already in 2025's first week
	}
	for d, want := range cases {
		if got := WeekKey(d); got != want {
			t.Errorf("WeekKey(%s) = %q, want %q", d.Format("2006-01-02"), got, want)
		}
	}
}

func TestWeekBoundsRunMondayToMonday(t *testing.T) {
	sunday := time.Date(2026, 11, 22, 23, 59, 59, 0, time.UTC)
	monday := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	start, end := WeekBounds(sunday)
	if !start.Equal(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)) || !end.Equal(monday) {
		t.Fatalf("WeekBounds(Sunday 23:59) = %s .. %s", start, end)
	}
	if s, _ := WeekBounds(monday); !s.Equal(monday) {
		t.Fatalf("Monday 00:00 must start its own week, got %s", s)
	}
	if WeekKey(sunday) == WeekKey(monday) {
		t.Fatal("Sunday 23:59 and the next Monday 00:00 must be different weeks")
	}
}

// Across a DST change the week still starts and ends at local midnight,
// even though it's an hour shorter or longer.
func TestWeekBoundsAcrossDST(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	// Clocks went back on Sunday 25 October 2026.
	start, end := WeekBounds(time.Date(2026, 10, 25, 20, 0, 0, 0, paris))
	if start != time.Date(2026, 10, 19, 0, 0, 0, 0, paris) || end != time.Date(2026, 10, 26, 0, 0, 0, 0, paris) {
		t.Fatalf("got %s .. %s", start, end)
	}
	if end.Sub(start) != 7*24*time.Hour+time.Hour {
		t.Fatalf("expected a 169-hour week, got %s", end.Sub(start))
	}
}

func TestThisWeekIsStableWithinAWeekAndCoversThePool(t *testing.T) {
	monday := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	for i := 1; i < 7; i++ {
		if ThisWeek(monday.AddDate(0, 0, i)).ID != ThisWeek(monday).ID {
			t.Fatalf("day %d of the week got a different weekly challenge", i)
		}
	}
	seen := map[string]bool{}
	for i := range 52 * 4 {
		seen[ThisWeek(monday.AddDate(0, 0, 7*i)).ID] = true
	}
	if len(seen) != len(weeklyPool) {
		t.Fatalf("expected all %d weekly challenges over four years, saw %d", len(weeklyPool), len(seen))
	}
}

func TestWeeklyProgressAndCompletion(t *testing.T) {
	ids := map[string]bool{}
	for _, c := range weeklyPool {
		if ids[c.ID] || c.Goal <= 0 || c.XPBonus <= 0 || c.Description == "" {
			t.Errorf("weekly challenge %q has a duplicate ID or an empty field", c.ID)
		}
		ids[c.ID] = true
		if c.Completed(Week{}) || c.Progress(Week{}) != 0 {
			t.Errorf("%s: an empty week must have no progress", c.ID)
		}
	}
	runs := weeklyPool[0]
	if runs.Progress(Week{Runs: 4}) != 4 || runs.Completed(Week{Runs: 9}) || !runs.Completed(Week{Runs: 10}) {
		t.Error("week_runs_10: wrong progress or completion")
	}
	if runs.Progress(Week{Runs: 25}) != 10 {
		t.Error("progress must be capped at the goal")
	}
	minutes := weeklyPool[4]
	if minutes.Progress(Week{Seconds: 29*60 + 59}) != 29 || !minutes.Completed(Week{Seconds: 30 * 60}) {
		t.Error("week_minutes_30: wrong progress or completion")
	}
}
