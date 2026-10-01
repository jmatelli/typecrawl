// Package game implements the leveling, XP, and HP mechanics layered on
// top of a typing test result.
package game

import (
	"math"
	"time"

	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/words"
)

const (
	// BaseMaxHP is the max HP of a level 1 profile.
	BaseMaxHP = 100
	// HPPerLevel is how much max HP increases for each level gained, on
	// top of the one-time TierTransitionHPBonus applied when crossing into
	// a new word-difficulty tier (see MaxHP).
	HPPerLevel = 10
	// TierTransitionHPBonus is a one-time HP bonus applied for every
	// difficulty tier a level has unlocked (see words.Tier), stacking:
	// +1x at level 10, +2x at level 20, +3x at level 30 and beyond. Average
	// word length roughly doubles at Easy->Medium and jumps sharply at the
	// other transitions too, so mistake exposure spikes sharply right at
	// those levels while smooth HPPerLevel growth alone barely moves --
	// measured, the "words survivable" ratio (MaxHP / average word length
	// in the new tier) fell by as much as 48% at a transition before this
	// bonus existed. This softens that cliff without erasing it: leveling
	// into a new tier is still meant to feel harder, just not like an
	// unheralded gut-punch the instant you arrive.
	TierTransitionHPBonus = 50
	// ComboWindow is how long a mistake stays "hot": another mistake
	// inside this window increases the combo (and its damage) instead
	// of starting over, and refreshes the window again.
	ComboWindow = 2 * time.Second
	// BaseMistakeDamage is the HP lost for a single isolated mistake on an
	// Easy-tier word, before the per-tier multiplier below is applied. It
	// does NOT scale with level on its own -- only MaxHP does -- so in
	// isolation it would make isolated mistakes matter less and less as
	// you level. MistakeDamageMultiplier is what keeps mistakes on harder
	// words meaningful instead: 5 spaced-out mistakes exactly drain a
	// level-1 pool (5 * 20 == BaseMaxHP, tier Easy, multiplier 1.0), and
	// with both MaxHP's tier bonus and the damage multiplier scaling
	// together, it takes 10 isolated mistakes at level 10 (Medium), 13 at
	// level 20 (Hard), and 16 at level 30 (Expert) -- still more forgiving
	// overall as you level, just not by nearly as much as MaxHP growth
	// alone would suggest, because harder words are deliberately meant to
	// sting more per slip.
	BaseMistakeDamage = 20
	// StreakBonusCap limits the per-word XP bonus from a mistake-free
	// word streak so an extremely long streak keeps paying out at a
	// steady rate instead of growing without bound.
	StreakBonusCap = 10
	// StreakPauseLimit is the longest gap allowed between two consecutive
	// keystrokes before the mistake-free streak breaks from hesitation
	// alone. 400ms keeps genuine pauses (thinking, distraction) from
	// counting as "flow" while staying above normal keystroke intervals
	// even at moderate speed (60 WPM averages ~200ms/char, 40 WPM ~300ms,
	// with natural variance on top) -- a tighter number like 250ms would
	// break the streak constantly for anyone under ~70-80 WPM.
	StreakPauseLimit = 400 * time.Millisecond
	// DailyStreakBonusPerDay is the extra XP fraction awarded per
	// consecutive day of practice (including today), rewarding showing up
	// every day rather than any single day's performance.
	DailyStreakBonusPerDay = 0.05
	// DailyStreakBonusCap limits the daily-streak multiplier so it keeps
	// paying out at a steady rate instead of growing without bound.
	DailyStreakBonusCap = 10
	// XPCurveBase and XPCurveExponent define the total XP needed to have
	// reached a given level, as base * level^exponent -- the standard RPG
	// power-curve progression. A per-level cost is then the difference
	// between consecutive totals, which grows on its own (steeper at level
	// 30 than level 5) without compounding as explosively as a pure
	// geometric (fixed-percentage-per-level) curve would at very high
	// levels. Exponent 2.2 is deliberate, not the commonly-cited 1.5-2.2
	// "gentle" range's midpoint: that range is calibrated against a flat
	// per-level baseline, but this game's original formula already grew
	// its per-level cost linearly (+50/level), which itself implies
	// quadratic (exponent-2) cumulative growth -- so exponent needs to
	// exceed 2 here to actually be harder than the level it's replacing,
	// not just "look exponential" while being easier in practice.
	XPCurveBase     = 28
	XPCurveExponent = 2.2

	// MaxPlausibleWPM is a hard ceiling above human capability. Competitive
	// sustained typing tops out around 200-216 WPM at the extreme elite
	// end; anything at or above this is far more likely to be
	// scripted/injected input than a person typing.
	MaxPlausibleWPM = 220.0
	// MinPlausibleKeystrokeCV is the minimum coefficient of variation
	// (stddev/mean of inter-keystroke intervals) expected from real human
	// typing rhythm, which always carries meaningfully more relative
	// spread than this even for fast, consistent typists. A run at or
	// below it, with enough samples to trust (see
	// MinKeystrokeSamplesForCVCheck), reads as suspiciously uniform --
	// keystrokes landing at near-fixed intervals, the way a script (or an
	// automated agent driving the terminal) would inject them.
	MinPlausibleKeystrokeCV = 0.15
	// MinKeystrokeSamplesForCVCheck is how many inter-keystroke intervals
	// stats.Result.KeystrokeSamples must have before the CV check in
	// IsSuspiciousRun is trusted. Short runs don't have enough samples to
	// tell natural variance from noise, so they get the benefit of the
	// doubt on this check (the WPM ceiling still applies regardless).
	MinKeystrokeSamplesForCVCheck = 30

	// GoldenWordChance is the probability a given exercise gets one rare
	// "golden word" at all -- most exercises have none, by design (a
	// guaranteed one every time would stop feeling like a lucky find).
	GoldenWordChance = 0.15
	// GoldenWordMinIndex and GoldenWordMaxIndex bound where in the word
	// list a golden word can land: never the very first couple of words
	// (so there's a moment to notice it coming), and not so deep into a
	// long time-mode buffer that a typical short test would never reach
	// it.
	GoldenWordMinIndex = 3
	GoldenWordMaxIndex = 60
	// GreenWordStreakInterval marks the word right after every Nth
	// consecutive mistake-free word as a "green word": land it clean too
	// and it heals you. Unlike the golden word, this is fully earned, not
	// random, and can repeat multiple times in one long streak.
	GreenWordStreakInterval = 10
	// GreenWordHealAmount is the flat HP restored by landing a green word
	// -- deliberately level-independent (unlike MaxHP), so it's a
	// meaningful save early on and a smaller but still real top-up later.
	GreenWordHealAmount = 25
)

// IsSuspiciousRun reports whether result looks more like scripted or
// injected input than genuine human typing: either its speed exceeds what
// a person can achieve, or -- given enough keystrokes to judge -- its
// timing is suspiciously uniform. A suspicious run still completes and its
// numbers are still shown, but callers should not let it touch XP,
// personal bests, streaks, or achievements.
func IsSuspiciousRun(result stats.Result) bool {
	if result.WPM >= MaxPlausibleWPM {
		return true
	}
	return result.KeystrokeSamples >= MinKeystrokeSamplesForCVCheck && result.KeystrokeCV < MinPlausibleKeystrokeCV
}

// MaxHP returns the max HP for a given level: smooth per-level growth plus
// a one-time bonus for each difficulty tier that level has unlocked, to
// cushion the sharp jump in word length (and so mistake exposure) at each
// tier transition -- see TierTransitionHPBonus.
func MaxHP(level int) int {
	return BaseMaxHP + (level-1)*HPPerLevel + words.Tier(level)*TierTransitionHPBonus
}

// xpCumulativeTotal returns the total XP needed to have reached level
// (from a standing start), using the base*level^exponent power curve.
func xpCumulativeTotal(level float64) float64 {
	return XPCurveBase * math.Pow(level, XPCurveExponent)
}

// XPToNextLevel returns how much XP is needed to go from level to level+1:
// the difference between consecutive points on the power curve, so the
// per-level cost keeps growing on its own (steeper at level 20 than at
// level 5) without needing a separate fixed growth-rate constant.
func XPToNextLevel(level int) int {
	return int(math.Round(xpCumulativeTotal(float64(level+1)) - xpCumulativeTotal(float64(level))))
}

// MistakeDamageMultiplier scales a mistake's damage by the word-difficulty
// tier it happened in (index matches words.Tier's 0-3), so a slip on a
// longer, less common word costs more than one on a short common word.
// Tuned at +0.25 per tier so it doesn't cancel out TierTransitionHPBonus's
// cushioning: combined with that bonus, isolated-mistakes-to-KO still goes
// 5 (level 1) -> 10 (level 10) -> 13 (level 20) -> 16 (level 30), i.e.
// still more forgiving overall as you level, just not as generously as
// MaxHP growth alone would give you.
var MistakeDamageMultiplier = []float64{1.0, 1.25, 1.5, 1.75}

// ComboDamage returns the HP lost for the n-th consecutive mistake within
// the combo window (n starts at 1, so a single isolated mistake costs
// BaseMistakeDamage HP times the given word-difficulty tier's multiplier,
// and each mistake landing inside the still-open window from the previous
// one multiplies the damage further).
func ComboDamage(comboCount, tier int) int {
	tier = min(max(tier, 0), len(MistakeDamageMultiplier)-1)
	return int(math.Round(float64(comboCount) * float64(BaseMistakeDamage) * MistakeDamageMultiplier[tier]))
}

// StreakBonus returns the bonus XP awarded for completing the streak-th
// consecutive mistake-free word (streak starts at 1), mirroring
// ComboDamage but rewarding sustained accuracy instead of punishing
// clustered mistakes.
func StreakBonus(streak int) int {
	return min(streak, StreakBonusCap)
}

// DailyStreakMultiplier returns the XP multiplier for a daily practice
// streak of the given length (consecutive days with at least one completed
// exercise, including today). It's 1.0 with no streak, rising by
// DailyStreakBonusPerDay per day up to DailyStreakBonusCap days.
func DailyStreakMultiplier(streakDays int) float64 {
	days := min(max(streakDays, 0), DailyStreakBonusCap)
	return 1 + float64(days)*DailyStreakBonusPerDay
}

// XPForResult computes the XP awarded for a completed typing test, scaled
// by speed (WPM), accuracy, and how long the test ran.
func XPForResult(wpm, accuracy float64, duration time.Duration) int {
	minutes := duration.Minutes()
	if minutes <= 0 {
		minutes = 1.0 / 60
	}
	xp := wpm * (accuracy / 100) * minutes * 2
	if xp < 1 {
		xp = 1
	}
	return int(xp + 0.5)
}

// AddXP applies gained XP to (level, xp), rolling over as many level-ups as
// needed. It returns the resulting level, the XP progress into that level,
// and how many levels were gained.
func AddXP(level, xp, gained int) (newLevel, newXP, levelsGained int) {
	xp += gained
	for xp >= XPToNextLevel(level) {
		xp -= XPToNextLevel(level)
		level++
		levelsGained++
	}
	return level, xp, levelsGained
}
