package game

import (
	"github.com/jmatelli/typecrawl/internal/words"
	"testing"
	"time"

	"github.com/jmatelli/typecrawl/internal/stats"
)

func TestMaxHPGrowsPerLevelAndJumpsAtTierTransitions(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, BaseMaxHP},                // level 1, tier 0, no bonus yet
		{9, BaseMaxHP + 8*HPPerLevel}, // still tier 0
		{10, BaseMaxHP + 9*HPPerLevel + 1*TierTransitionHPBonus},  // tier 1 unlocked
		{20, BaseMaxHP + 19*HPPerLevel + 2*TierTransitionHPBonus}, // tier 2 unlocked
		{30, BaseMaxHP + 29*HPPerLevel + 3*TierTransitionHPBonus}, // tier 3 (capped) unlocked
		{40, BaseMaxHP + 39*HPPerLevel + 3*TierTransitionHPBonus}, // tier stays capped at 3
	}
	for _, c := range cases {
		if got := MaxHP(c.level, 0); got != c.want {
			t.Errorf("MaxHP(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestMaxHPStrictlyIncreasesWithLevel(t *testing.T) {
	prev := MaxHP(1, 0)
	for level := 2; level <= 60; level++ {
		cur := MaxHP(level, 0)
		if cur <= prev {
			t.Fatalf("MaxHP(%d)=%d did not increase over MaxHP(%d)=%d", level, cur, level-1, prev)
		}
		prev = cur
	}
}

func TestXPToNextLevelIncreasesWithLevel(t *testing.T) {
	prev := XPToNextLevel(1)
	for level := 2; level <= 60; level++ {
		cur := XPToNextLevel(level)
		if cur < prev {
			t.Fatalf("XPToNextLevel(%d)=%d is less than XPToNextLevel(%d)=%d -- per-level cost must not decrease", level, cur, level-1, prev)
		}
		prev = cur
	}
}

func TestXPToNextLevelHarderThanOldLinearFormula(t *testing.T) {
	// Historical context: the original formula was a flat 50 XP/level,
	// which cumulatively implies quadratic growth (50*level^2 total by
	// level N via the standard arithmetic-series sum). The power curve
	// here must cost at least as much as that at every level, or it would
	// be an accidental nerf relative to what shipped before.
	for level := 1; level <= 60; level++ {
		oldLinearCost := 50 * level
		newCost := XPToNextLevel(level)
		if newCost < oldLinearCost {
			t.Errorf("level %d: new curve costs %d, old linear formula cost %d -- new curve must not be easier", level, newCost, oldLinearCost)
		}
	}
}

func TestComboDamageScalesWithComboCountAndTier(t *testing.T) {
	// A single isolated mistake (combo=1) at tier 0 costs exactly
	// BaseMistakeDamage.
	if got := ComboDamage(1, 0); got != BaseMistakeDamage {
		t.Fatalf("ComboDamage(1, 0) = %d, want %d", got, BaseMistakeDamage)
	}
	// Damage increases with combo count, for a fixed tier.
	prev := ComboDamage(1, 0)
	for combo := 2; combo <= 5; combo++ {
		cur := ComboDamage(combo, 0)
		if cur <= prev {
			t.Fatalf("ComboDamage(%d, 0)=%d did not increase over ComboDamage(%d, 0)=%d", combo, cur, combo-1, prev)
		}
		prev = cur
	}
	// Damage increases with tier, for a fixed combo count.
	prevTier := ComboDamage(1, 0)
	for tier := 1; tier < len(MistakeDamageMultiplier); tier++ {
		cur := ComboDamage(1, tier)
		if cur <= prevTier {
			t.Fatalf("ComboDamage(1, %d)=%d did not increase over ComboDamage(1, %d)=%d", tier, cur, tier-1, prevTier)
		}
		prevTier = cur
	}
}

func TestComboDamageClampsOutOfRangeTier(t *testing.T) {
	// A tier beyond the defined multipliers must clamp to the last one,
	// not panic or index out of range.
	highest := ComboDamage(1, len(MistakeDamageMultiplier)-1)
	if got := ComboDamage(1, 999); got != highest {
		t.Errorf("ComboDamage(1, 999) = %d, want clamp to highest tier's %d", got, highest)
	}
	if got := ComboDamage(1, -5); got != ComboDamage(1, 0) {
		t.Errorf("ComboDamage(1, -5) = %d, want clamp to tier 0's %d", got, ComboDamage(1, 0))
	}
}

func TestComboDamagePlateausAtTheMultiplierCap(t *testing.T) {
	// Past the cap, each additional combo hit costs the same as the last
	// one -- damage keeps accruing with a longer chain, but stops
	// accelerating, unlike the uncapped growth this replaced.
	comboAtCap := int(1+(ComboMultiplierCap-1)/ComboMultiplierGrowth) + 1
	atCap := ComboDamage(comboAtCap, 0)
	if got := ComboDamage(comboAtCap+1, 0); got != atCap {
		t.Errorf("ComboDamage(%d, 0) = %d, want it to plateau at the capped %d", comboAtCap+1, got, atCap)
	}
	if got := ComboDamage(comboAtCap+50, 0); got != atCap {
		t.Errorf("ComboDamage(%d, 0) = %d, want it to stay at the capped %d even far past it", comboAtCap+50, got, atCap)
	}
}

func TestComboDamageBurstNoLongerKOsALevelOneCharacterInThreeHits(t *testing.T) {
	// The original uncapped linear growth let 3 rapid-fire mistakes
	// (20+40+60=120) alone exceed a level-1 character's 100 max HP. The
	// flattened, capped growth must no longer do that in 3 hits.
	cumulative := 0
	for combo := 1; combo <= 3; combo++ {
		cumulative += ComboDamage(combo, 0)
	}
	if cumulative >= BaseMaxHP {
		t.Errorf("3-hit combo burst costs %d HP, want it to stay under a level-1 character's %d max HP", cumulative, BaseMaxHP)
	}
}

func TestIsolatedMistakesToKOGetsMoreForgivingWithLevel(t *testing.T) {
	// Documented design invariant: isolated (non-combo) mistakes-to-KO
	// should go 5 -> 10 -> 13 -> 16 across the tier-1/10/20/30 boundary
	// levels, per the comments on BaseMistakeDamage and
	// MistakeDamageMultiplier.
	mistakesToKO := func(level int) int {
		hp := MaxHP(level, 0)
		tier := 0
		switch {
		case level >= 30:
			tier = 3
		case level >= 20:
			tier = 2
		case level >= 10:
			tier = 1
		}
		n := 0
		for hp > 0 {
			hp -= ComboDamage(1, tier)
			n++
		}
		return n
	}
	cases := []struct {
		level int
		want  int
	}{
		{1, 5}, {10, 10}, {20, 13}, {30, 16},
	}
	for _, c := range cases {
		if got := mistakesToKO(c.level); got != c.want {
			t.Errorf("mistakes-to-KO at level %d = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestStreakBonusCappedAtStreakBonusCap(t *testing.T) {
	for streak := 1; streak <= StreakBonusCap; streak++ {
		if got := StreakBonus(streak); got != streak {
			t.Errorf("StreakBonus(%d) = %d, want %d (below cap)", streak, got, streak)
		}
	}
	if got := StreakBonus(StreakBonusCap + 50); got != StreakBonusCap {
		t.Errorf("StreakBonus(cap+50) = %d, want capped at %d", got, StreakBonusCap)
	}
}

func TestDailyStreakMultiplierRangeAndCap(t *testing.T) {
	if got := DailyStreakMultiplier(0); got != 1.0 {
		t.Errorf("DailyStreakMultiplier(0) = %v, want 1.0 (no streak, no bonus)", got)
	}
	if got := DailyStreakMultiplier(-5); got != 1.0 {
		t.Errorf("DailyStreakMultiplier(-5) = %v, want clamped to 1.0", got)
	}
	want := 1 + float64(DailyStreakBonusCap)*DailyStreakBonusPerDay
	if got := DailyStreakMultiplier(DailyStreakBonusCap); got != want {
		t.Errorf("DailyStreakMultiplier(cap) = %v, want %v", got, want)
	}
	if got := DailyStreakMultiplier(DailyStreakBonusCap + 100); got != want {
		t.Errorf("DailyStreakMultiplier(cap+100) = %v, want capped at %v", got, want)
	}
}

func TestXPForResultScalesWithSpeedAccuracyAndDuration(t *testing.T) {
	base := XPForResult(60, 100, time.Minute)
	if faster := XPForResult(120, 100, time.Minute); faster <= base {
		t.Errorf("expected higher WPM to yield more XP: 120wpm=%d, 60wpm=%d", faster, base)
	}
	if lessAccurate := XPForResult(60, 50, time.Minute); lessAccurate >= base {
		t.Errorf("expected lower accuracy to yield less XP: 50%%=%d, 100%%=%d", lessAccurate, base)
	}
	if longer := XPForResult(60, 100, 2*time.Minute); longer <= base {
		t.Errorf("expected a longer test to yield more XP: 2min=%d, 1min=%d", longer, base)
	}
}

func TestXPForResultNeverBelowOne(t *testing.T) {
	if got := XPForResult(0, 0, 0); got < 1 {
		t.Errorf("XPForResult with all-zero input = %d, want at least 1", got)
	}
	if got := XPForResult(0, 0, time.Millisecond); got < 1 {
		t.Errorf("expected a floor of 1 XP even for a near-worthless run, got %d", got)
	}
}

func TestAddXPWithinOneLevelDoesNotLevelUp(t *testing.T) {
	needed := XPToNextLevel(5)
	newLevel, newXP, gained := AddXP(5, 0, needed-1)
	if newLevel != 5 || gained != 0 {
		t.Fatalf("expected to stay at level 5 with no level-up, got level=%d gained=%d", newLevel, gained)
	}
	if newXP != needed-1 {
		t.Fatalf("expected xp=%d, got %d", needed-1, newXP)
	}
}

func TestAddXPLevelsUpExactlyAtThreshold(t *testing.T) {
	needed := XPToNextLevel(5)
	newLevel, newXP, gained := AddXP(5, 0, needed)
	if newLevel != 6 || gained != 1 || newXP != 0 {
		t.Fatalf("expected level=6 gained=1 xp=0 exactly at threshold, got level=%d gained=%d xp=%d", newLevel, gained, newXP)
	}
}

func TestAddXPRollsOverMultipleLevelsInOneCall(t *testing.T) {
	// A huge XP injection should roll over as many levels as it covers,
	// leaving the correct remainder.
	level, xp := 1, 0
	var totalGained int
	hugeXP := 100000
	newLevel, newXP, gained := AddXP(level, xp, hugeXP)
	totalGained = gained

	// Cross-check by manually simulating one level at a time.
	simLevel, simXP := 1, 0
	remaining := hugeXP
	simGained := 0
	for remaining >= XPToNextLevel(simLevel) {
		need := XPToNextLevel(simLevel)
		remaining -= need
		simLevel++
		simGained++
	}
	simXP = remaining

	if newLevel != simLevel || newXP != simXP || totalGained != simGained {
		t.Fatalf("AddXP(1,0,%d) = (%d,%d,%d), want (%d,%d,%d)", hugeXP, newLevel, newXP, totalGained, simLevel, simXP, simGained)
	}
}

func TestIsSuspiciousRunWPMCeiling(t *testing.T) {
	cases := []struct {
		name string
		r    stats.Result
		want bool
	}{
		{"normal human run", stats.Result{WPM: 85, KeystrokeCV: 0.5, KeystrokeSamples: 200}, false},
		{"fast but legit human, high variance", stats.Result{WPM: 180, KeystrokeCV: 0.3, KeystrokeSamples: 300}, false},
		{"exactly at WPM ceiling", stats.Result{WPM: MaxPlausibleWPM, KeystrokeCV: 0.5, KeystrokeSamples: 200}, true},
		{"just under WPM ceiling", stats.Result{WPM: MaxPlausibleWPM - 0.1, KeystrokeCV: 0.5, KeystrokeSamples: 200}, false},
		{"uniform timing with enough samples", stats.Result{WPM: 90, KeystrokeCV: 0.05, KeystrokeSamples: 100}, true},
		{"uniform timing but too few samples -- benefit of the doubt", stats.Result{WPM: 90, KeystrokeCV: 0.05, KeystrokeSamples: 10}, false},
		{"exactly at CV threshold with enough samples -- not below it", stats.Result{WPM: 90, KeystrokeCV: MinPlausibleKeystrokeCV, KeystrokeSamples: 100}, false},
		{"just below CV threshold with enough samples", stats.Result{WPM: 90, KeystrokeCV: MinPlausibleKeystrokeCV - 0.001, KeystrokeSamples: 100}, true},
		{"exactly at sample threshold", stats.Result{WPM: 90, KeystrokeCV: 0.05, KeystrokeSamples: MinKeystrokeSamplesForCVCheck}, true},
		{"one below sample threshold", stats.Result{WPM: 90, KeystrokeCV: 0.05, KeystrokeSamples: MinKeystrokeSamplesForCVCheck - 1}, false},
	}
	for _, c := range cases {
		if got := IsSuspiciousRun(c.r); got != c.want {
			t.Errorf("%s: IsSuspiciousRun(%+v) = %v, want %v", c.name, c.r, got, c.want)
		}
	}
}

func TestDifficultyLevelRaisesTheTierFloorPerPrestigeRank(t *testing.T) {
	cases := []struct{ level, prestige, want int }{
		{1, 0, 1}, {35, 0, 35},
		{1, 1, 10}, {15, 1, 15},
		{1, 2, 20},
		{1, 3, 30}, {1, 9, 30}, {42, 9, 42},
		{5, -1, 5},
	}
	for _, c := range cases {
		if got := DifficultyLevel(c.level, c.prestige); got != c.want {
			t.Errorf("DifficultyLevel(%d, %d) = %d, want %d", c.level, c.prestige, got, c.want)
		}
	}
}

// A prestiged level-1 profile is harder than an unprestiged one: fewer
// isolated mistakes to a knockout, because word damage rises faster than
// the tier bonus adds HP. And it's harder than the level-30 profile it
// came from, which is the point of prestiging.
func TestPrestigeRaisesDifficulty(t *testing.T) {
	mistakesToKO := func(level, prestige int) int {
		tier := words.Tier(DifficultyLevel(level, prestige))
		hit := ComboDamage(1, tier)
		return (MaxHP(level, prestige) + hit - 1) / hit
	}
	if got := mistakesToKO(1, 0); got != 5 {
		t.Fatalf("expected 5 mistakes to KO at level 1 without prestige, got %d", got)
	}
	if a, b := mistakesToKO(1, 1), mistakesToKO(30, 0); a >= b {
		t.Fatalf("expected prestige 1 at level 1 (%d mistakes) to be harder than level 30 (%d)", a, b)
	}
	if MaxHP(1, 1) != BaseMaxHP+TierTransitionHPBonus {
		t.Fatalf("expected the tier bonus at prestige 1, got %d", MaxHP(1, 1))
	}
}

func TestFloorOf(t *testing.T) {
	cases := []struct {
		n, level  int
		words     int
		tierLevel int
		boss      bool
	}{
		{1, 1, 15, 1, false},
		{2, 1, 17, 1, false},
		{3, 1, 19, 1, false},
		{4, 1, 21, 11, false}, // a tier up every 3 floors
		{5, 1, 33, 21, true},  // boss: a tier above floor 4, 10 words longer
		{6, 1, 25, 11, false},
		{10, 1, 43, 41, true},
		{0, 1, 15, 1, false}, // below 1 is floor 1
		{1, 25, 15, 25, false},
	}
	for _, c := range cases {
		f := FloorOf(c.n, c.level)
		if f.Words != c.words || f.Level != c.tierLevel || f.Boss != c.boss {
			t.Errorf("FloorOf(%d, %d) = %+v, want words %d level %d boss %v", c.n, c.level, f, c.words, c.tierLevel, c.boss)
		}
	}
	// Deep floors stay at the hardest tier rather than overflowing.
	if f := FloorOf(1000, 1); words.Tier(f.Level) != words.Tier(1000) {
		t.Fatalf("expected the hardest tier, got level %d", f.Level)
	}
}

func TestCrawlDepthBonus(t *testing.T) {
	if got := CrawlDepthBonus(FloorOf(3, 1)); got != 15 {
		t.Errorf("floor 3 bonus = %d, want 15", got)
	}
	if got := CrawlDepthBonus(FloorOf(5, 1)); got != 50 {
		t.Errorf("boss floor 5 bonus = %d, want 50 (doubled)", got)
	}
}
