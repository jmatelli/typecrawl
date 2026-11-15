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
