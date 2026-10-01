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
	r := Run{Mode: "time", Target: 999, Punctuation: true, ZenMode: true, WPM: 999, Accuracy: 100, KO: true}
	for _, c := range all {
		if c.Completed(r) {
			t.Fatalf("challenge %q was satisfied by a KO'd run -- every check must require !KO", c.ID)
		}
	}
}

func TestEachChallengeChecksItsSpecificCondition(t *testing.T) {
	byID := make(map[string]Challenge)
	for _, c := range all {
		byID[c.ID] = c
	}
	base := Run{Mode: "time", Target: 30, WPM: 10, Accuracy: 50, KO: false}

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
