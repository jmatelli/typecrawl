package achievements

import "testing"

func TestTierStringNamesAndClamping(t *testing.T) {
	cases := map[Tier]string{
		Bronze: "Bronze", Silver: "Silver", Gold: "Gold",
		Diamond: "Diamond", Master: "Master", Grandmaster: "Grandmaster",
	}
	for tier, want := range cases {
		if got := tier.String(); got != want {
			t.Errorf("Tier(%d).String() = %q, want %q", tier, got, want)
		}
	}
	if got := Tier(99).String(); got != "Bronze" {
		t.Errorf("out-of-range Tier.String() = %q, want fallback %q", got, "Bronze")
	}
}

func TestAllIDsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range All {
		if seen[a.ID] {
			t.Fatalf("duplicate achievement ID %q", a.ID)
		}
		seen[a.ID] = true
	}
}

// progressionCategories lists every multi-threshold category in All, used
// by the tests below to verify the tier-ascension and threshold-ordering
// invariants the achievement system is built on.
var progressionCategories = map[string][]string{
	"exercises":         {"first_exercise", "exercises_10", "exercises_50", "exercises_100", "exercises_250", "exercises_500"},
	"level":             {"level_5", "level_10", "level_20", "level_30"},
	"wpm":               {"wpm_30", "wpm_60", "wpm_100", "wpm_120", "wpm_150", "wpm_200"},
	"combo":             {"combo_10", "combo_25", "combo_50", "combo_100"},
	"daily":             {"daily_3", "daily_7", "daily_30", "daily_100"},
	"ko":                {"ko_1", "ko_5", "ko_10"},
	"word_count":        {"long_hauler", "ultramarathoner"},
	"accuracy_lifetime": {"perfect_run", "sharpshooter", "marksman", "perfect_storm"},
	"accuracy_streak":   {"streak_2", "streak_3", "streak_5", "streak_10"},
	"resilience":        {"comeback_kid", "unbreakable", "phoenix"},
	"wordsmith":         {"wordsmith_10k", "wordsmith_50k", "wordsmith_100k"},
	"time_played":       {"time_1h", "time_5h", "time_20h", "time_50h", "time_100h"},
}

func TestEveryProgressionCategoryUsesUniqueStrictlyAscendingTiers(t *testing.T) {
	byID := make(map[string]Achievement)
	for _, a := range All {
		byID[a.ID] = a
	}

	for label, ids := range progressionCategories {
		seen := make(map[Tier]string)
		prev := Tier(-1)
		for _, id := range ids {
			a, ok := byID[id]
			if !ok {
				t.Fatalf("%s: achievement %q not found in All", label, id)
			}
			if dup, exists := seen[a.Tier]; exists {
				t.Fatalf("%s: tier %s reused by both %q and %q", label, a.Tier, dup, id)
			}
			seen[a.Tier] = id
			if a.Tier <= prev {
				t.Fatalf("%s: tier did not strictly increase at %q (%s after %s)", label, id, a.Tier, prev)
			}
			prev = a.Tier
		}
	}
}

func TestNumericThresholdsStrictlyIncreaseWithinEachCategory(t *testing.T) {
	byID := make(map[string]Achievement)
	for _, a := range All {
		byID[a.ID] = a
	}
	for label, ids := range progressionCategories {
		var prevTarget float64 = -1
		for _, id := range ids {
			a := byID[id]
			if a.progress == nil {
				continue // boolean-only entries (e.g. phoenix) have no numeric threshold
			}
			_, target, ok := a.progress(Progress{})
			if !ok {
				continue
			}
			if target <= prevTarget {
				t.Fatalf("%s: threshold did not increase at %q (%v after %v)", label, id, target, prevTarget)
			}
			prevTarget = target
		}
	}
}

// TestCheckAndProgressAgreeForEveryThresholdAchievement is a property test:
// for every achievement with a numeric progress function, check(p) must
// agree with current>=target for ARBITRARY progress values, not just one
// hand-picked case -- this catches any future drift between the two
// without needing a per-achievement setter.
func TestCheckAndProgressAgreeForEveryThresholdAchievement(t *testing.T) {
	probes := []Progress{
		{},
		{
			Level: 1000, TotalExercises: 1_000_000, BestWordStreak: 1_000_000, KOCount: 1_000_000,
			DailyStreak: 1_000_000, BestWPM: 100_000, PerfectRunCount: 1_000_000, MaxPerfectStreak: 1_000_000,
			ComebackCount: 1_000_000, PunctuationRunCount: 1_000_000, ZenRunCount: 1_000_000,
			TotalWordsTyped: 1_000_000_000, TotalPlaySeconds: 1_000_000_000,
		},
		{TotalExercises: 10, Level: 10, BestWPM: 60, BestWordStreak: 25, DailyStreak: 7, KOCount: 5}, // mid-range, hits several exact thresholds
	}
	for _, a := range All {
		if a.progress == nil {
			continue
		}
		for _, p := range probes {
			current, target, ok := a.progress(p)
			if !ok {
				continue
			}
			want := current >= target
			got := a.Unlocked(p)
			if got != want {
				t.Fatalf("%s: check/progress disagree for probe %+v: check=%v, progress=(%v/%v)", a.ID, p, got, current, target)
			}
		}
	}
}

func TestUnlockedCountMatchesManualTally(t *testing.T) {
	p := Progress{TotalExercises: 100, Level: 20, BestWPM: 80}
	manual := 0
	for _, a := range All {
		if a.Unlocked(p) {
			manual++
		}
	}
	if got := UnlockedCount(p); got != manual {
		t.Fatalf("UnlockedCount = %d, want %d", got, manual)
	}
}

func TestUnlockedCountZeroForZeroProgress(t *testing.T) {
	if got := UnlockedCount(Progress{}); got != 0 {
		t.Fatalf("expected 0 achievements unlocked for zero-value Progress, got %d", got)
	}
}

func TestUnlockedCountAllForMaxedProgress(t *testing.T) {
	maxed := Progress{
		Level: 1000, TotalExercises: 1_000_000, BestWordStreak: 1_000_000, KOCount: 1_000_000,
		DailyStreak: 1_000_000, BestWPM: 100_000, PerfectRunCount: 1_000_000, MaxPerfectStreak: 1_000_000,
		CompletedLongHaul: true, CompletedSprint: true, PracticedLateNight: true, PracticedEarlyMorning: true,
		HasBestInBothModes: true, PracticedAllWeekdays: true, HadBigDay: true, CompletedUltraLongHaul: true,
		ComebackCount: 1_000_000, HadPerfectComeback: true, PunctuationRunCount: 1_000_000, ZenRunCount: 1_000_000,
		QuoteRunCount:   1_000_000,
		TotalWordsTyped: 1_000_000_000, TotalPlaySeconds: 1_000_000_000,
	}
	if got := UnlockedCount(maxed); got != len(All) {
		t.Fatalf("expected all %d achievements unlocked for maxed-out Progress, got %d", len(All), got)
	}
}

func TestNearestRanksByClosestToUnlocking(t *testing.T) {
	p := Progress{
		TotalExercises:  8, // 8/10 exercises_10 -- 80% there
		PerfectRunCount: 9, // 9/10 sharpshooter -- 90% there
	}
	nearest := Nearest(p, 3)
	if len(nearest) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(nearest))
	}
	if nearest[0].Achievement.ID != "sharpshooter" {
		t.Fatalf("expected sharpshooter (90%%) ranked first, got %s", nearest[0].Achievement.ID)
	}
	if nearest[0].Current != 9 || nearest[0].Target != 10 {
		t.Fatalf("expected current=9 target=10, got %v/%v", nearest[0].Current, nearest[0].Target)
	}
	for i := 1; i < len(nearest); i++ {
		if nearest[i].Ratio() > nearest[i-1].Ratio() {
			t.Fatalf("expected descending ratio order, got %v then %v", nearest[i-1].Ratio(), nearest[i].Ratio())
		}
	}
}

func TestNearestExcludesUnlockedAndBooleanOnlyAchievements(t *testing.T) {
	p := Progress{PerfectRunCount: 1} // unlocks perfect_run
	for _, lp := range Nearest(p, len(All)) {
		if lp.Achievement.ID == "perfect_run" {
			t.Fatal("expected an already-unlocked achievement to be excluded from Nearest")
		}
		if lp.Achievement.ID == "phoenix" || lp.Achievement.ID == "well_rounded" || lp.Achievement.ID == "full_week" {
			t.Fatalf("expected boolean-only achievement %s to have no progress function, but it appeared in Nearest", lp.Achievement.ID)
		}
	}
}

func TestNearestAtZeroProgressReturnsRatioZeroCandidatesInAllOrder(t *testing.T) {
	// At zero progress every numeric-threshold achievement sits at ratio 0
	// -- "nearest" doesn't mean "has nonzero progress," just "closest by
	// ratio," so Nearest still returns n candidates, all tied at 0, broken
	// by All's original order (SliceStable).
	var firstNumeric []string
	for _, a := range All {
		if a.progress != nil {
			firstNumeric = append(firstNumeric, a.ID)
		}
		if len(firstNumeric) == 3 {
			break
		}
	}

	got := Nearest(Progress{}, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates even at zero progress, got %d", len(got))
	}
	for i, lp := range got {
		if lp.Ratio() != 0 {
			t.Fatalf("expected ratio 0 at zero progress, got %v for %s", lp.Ratio(), lp.Achievement.ID)
		}
		if lp.Achievement.ID != firstNumeric[i] {
			t.Fatalf("expected tie-break to follow All's order: index %d got %s, want %s", i, lp.Achievement.ID, firstNumeric[i])
		}
	}
}

func TestLockedProgressRatioClampedToZeroAndOne(t *testing.T) {
	cases := []struct {
		lp   LockedProgress
		want float64
	}{
		{LockedProgress{Current: 5, Target: 10}, 0.5},
		{LockedProgress{Current: 15, Target: 10}, 1}, // over target clamps to 1
		{LockedProgress{Current: -5, Target: 10}, 0}, // negative clamps to 0
		{LockedProgress{Current: 5, Target: 0}, 0},   // zero target avoids divide-by-zero, defined as 0
	}
	for _, c := range cases {
		if got := c.lp.Ratio(); got != c.want {
			t.Errorf("Ratio() for %+v = %v, want %v", c.lp, got, c.want)
		}
	}
}

func TestBooleanAchievementsUnlockOnlyOnTheirExactFlag(t *testing.T) {
	byID := make(map[string]Achievement)
	for _, a := range All {
		byID[a.ID] = a
	}
	cases := []struct {
		id  string
		set func(bool) Progress
	}{
		{"quick_draw", func(b bool) Progress { return Progress{CompletedSprint: b} }},
		{"night_owl", func(b bool) Progress { return Progress{PracticedLateNight: b} }},
		{"early_bird", func(b bool) Progress { return Progress{PracticedEarlyMorning: b} }},
		{"long_hauler", func(b bool) Progress { return Progress{CompletedLongHaul: b} }},
		{"ultramarathoner", func(b bool) Progress { return Progress{CompletedUltraLongHaul: b} }},
		{"well_rounded", func(b bool) Progress { return Progress{HasBestInBothModes: b} }},
		{"full_week", func(b bool) Progress { return Progress{PracticedAllWeekdays: b} }},
		{"daily_grind", func(b bool) Progress { return Progress{HadBigDay: b} }},
		{"phoenix", func(b bool) Progress { return Progress{HadPerfectComeback: b} }},
	}
	for _, c := range cases {
		a, ok := byID[c.id]
		if !ok {
			t.Fatalf("%s: not found", c.id)
		}
		if a.Unlocked(c.set(false)) {
			t.Errorf("%s: expected locked when its flag is false", c.id)
		}
		if !a.Unlocked(c.set(true)) {
			t.Errorf("%s: expected unlocked when its flag is true", c.id)
		}
	}
}
