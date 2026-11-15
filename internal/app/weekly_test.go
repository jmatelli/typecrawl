package app

import (
	"testing"
	"time"

	"github.com/jmatelli/typecrawl/internal/challenge"
	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// seedWeek records enough runs earlier this week to meet any weekly
// challenge except the one asking for several different days, which
// can't be seeded on the first days of a week. It skips the test then.
func seedWeek(t *testing.T, m Model, profile *storage.Profile) challenge.Weekly {
	t.Helper()
	now := time.Now()
	weekly := challenge.ThisWeek(now)
	start, _ := challenge.WeekBounds(now)
	days := int(now.Sub(start).Hours()/24) + 1
	if weekly.ID == "week_days_4" && days < weekly.Goal {
		t.Skipf("this week's challenge needs %d days and only %d have started", weekly.Goal, days)
	}
	for i := range 12 {
		// Spread over the days so far, oldest first, all before now.
		when := start.Add(time.Duration(i%days) * 24 * time.Hour).Add(time.Minute)
		if when.After(now) {
			when = now.Add(-time.Minute)
		}
		if err := storage.RecordTestResult(m.db, profile.ID, 60, 99, "time", 30, false, false, false, "quotes", 100, 200*time.Second, when); err != nil {
			t.Fatal(err)
		}
	}
	return weekly
}

func finishRun(m Model, ko bool) Model {
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 90, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100, ko: ko, wordsTyped: 20,
	})
	return next.(Model)
}

func TestWeeklyChallengeIsClaimedOnceAndPaidOnce(t *testing.T) {
	m, profile := newTestApp(t)
	weekly := seedWeek(t, m, profile)

	m = finishRun(m, true) // a knockout can't claim it
	if m.results.extra.weeklyBonusXP != 0 {
		t.Fatal("expected a knocked-out run not to claim the weekly challenge")
	}
	if claimed, _ := storage.HasClaimedWeeklyChallenge(m.db, profile.ID, challenge.WeekKey(time.Now())); claimed {
		t.Fatal("expected no claim recorded after a knockout")
	}

	levelBefore, xpBefore := m.profile.Level, m.profile.XP
	m = finishRun(m, false)
	extra := m.results.extra
	if extra.weeklyBonusXP != weekly.XPBonus {
		t.Fatalf("expected the weekly bonus %d, got %d", weekly.XPBonus, extra.weeklyBonusXP)
	}
	if extra.streakBonusXP >= weekly.XPBonus {
		t.Fatalf("the weekly bonus leaked into the daily-streak XP: %d", extra.streakBonusXP)
	}
	if got := extra.xpBase + extra.xpBonus + extra.streakBonusXP + extra.challengeBonusXP + extra.weeklyBonusXP; got != extra.xpGained {
		t.Fatalf("XP parts add up to %d but xpGained is %d", got, extra.xpGained)
	}
	if level, xp, _ := game.AddXP(levelBefore, xpBefore, extra.xpGained); m.profile.Level != level || m.profile.XP != xp {
		t.Fatalf("the profile is at level %d with %d XP, but %d XP from level %d with %d should give level %d with %d",
			m.profile.Level, m.profile.XP, extra.xpGained, levelBefore, xpBefore, level, xp)
	}
	if runs := m.Runs(); runs[len(runs)-1].XPGained != extra.xpGained {
		t.Fatalf("--json would report %d XP, the results say %d", runs[len(runs)-1].XPGained, extra.xpGained)
	}
	if !containsSubstring(m.results.View(), "Weekly challenge complete!") {
		t.Fatal("expected the results screen to say the weekly challenge is complete")
	}

	m = finishRun(m, false)
	if m.results.extra.weeklyBonusXP != 0 {
		t.Fatal("expected the weekly bonus paid only once")
	}
}

func TestMenuShowsWeeklyProgress(t *testing.T) {
	m, profile := newTestApp(t)
	menu := newMenuModel(profile, m.db)
	weekly := challenge.ThisWeek(time.Now())
	if !containsSubstring(menu.View(), "Weekly challenge: "+weekly.Description) ||
		!containsSubstring(menu.View(), "0/") {
		t.Fatalf("expected the weekly challenge with no progress in the menu, got:\n%s", menu.View())
	}

	seedWeek(t, m, profile)
	m = finishRun(m, false)
	menu = newMenuModel(profile, m.db)
	if !containsSubstring(menu.View(), "Weekly challenge (complete)") {
		t.Fatalf("expected the claimed weekly challenge shown as complete, got:\n%s", menu.View())
	}
}
