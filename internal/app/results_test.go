package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/stats"
)

func TestResultsViewShowsKOMessageAndNoXP(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 40, Accuracy: 70}, 100, resultsExtra{
		ko: true, koCount: 3, hp: 0, maxHP: 100,
	})
	view := m.View()
	if !containsSubstring(view, "knocked out") {
		t.Fatalf("expected a KO message, got:\n%s", view)
	}
	if !containsSubstring(view, "KO #3") {
		t.Fatalf("expected the KO count shown, got:\n%s", view)
	}
}

func TestResultsViewShowsNewPersonalBest(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 90, Accuracy: 95}, 100, resultsExtra{
		isNewPB: true, newLevel: 5,
	})
	view := m.View()
	if !containsSubstring(view, "NEW PERSONAL BEST") {
		t.Fatalf("expected a new-PB callout, got:\n%s", view)
	}
}

func TestResultsViewShowsExistingPersonalBestWhenNotNew(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 60, Accuracy: 90}, 100, resultsExtra{
		isNewPB: false, personalBest: 75.5, newLevel: 5,
	})
	view := m.View()
	if containsSubstring(view, "NEW PERSONAL BEST") {
		t.Fatal("expected no new-PB callout when isNewPB is false")
	}
	if !containsSubstring(view, "75.5") {
		t.Fatalf("expected the existing personal best shown, got:\n%s", view)
	}
}

func TestResultsViewShowsLevelUp(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 60, Accuracy: 90}, 100, resultsExtra{
		oldLevel: 4, newLevel: 6, levelsGained: 2,
	})
	view := m.View()
	if !containsSubstring(view, "LEVEL UP") || !containsSubstring(view, "4 -> 6") {
		t.Fatalf("expected a level-up callout from 4 to 6, got:\n%s", view)
	}
}

func TestResultsViewBreaksDownXPSources(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 60, Accuracy: 90}, 100, resultsExtra{
		xpGained: 50, xpBase: 30, xpBonus: 10, streakBonusXP: 5, challengeBonusXP: 5, newLevel: 3,
	})
	view := m.View()
	for _, want := range []string{"+50", "30 base", "+10", "daily streak", "daily challenge"} {
		if !containsSubstring(view, want) {
			t.Fatalf("expected %q in the XP breakdown, got:\n%s", want, view)
		}
	}
}

func TestResultsViewShowsNewWordStreakRecord(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 60, Accuracy: 90}, 100, resultsExtra{
		bestCombo: 50, allTimeBestWordStreak: 50, newWordStreakRecord: true, newLevel: 3,
	})
	view := m.View()
	if !containsSubstring(view, "NEW RECORD") {
		t.Fatalf("expected a new word-streak record callout, got:\n%s", view)
	}
}

func TestResultsViewShowsDailyChallengeStatus(t *testing.T) {
	completed := newResultsModel(stats.Result{WPM: 60, Accuracy: 90}, 100, resultsExtra{
		challengeDescription: "Hit 50 WPM", challengeCompleted: true, newLevel: 3,
	})
	if !containsSubstring(completed.View(), "Daily challenge complete!") {
		t.Fatal("expected a completion callout when challengeCompleted is true")
	}

	notYet := newResultsModel(stats.Result{WPM: 60, Accuracy: 90}, 100, resultsExtra{
		challengeDescription: "Hit 50 WPM", challengeCompleted: false, newLevel: 3,
	})
	view := notYet.View()
	if !containsSubstring(view, "Today's challenge") || containsSubstring(view, "complete!") {
		t.Fatalf("expected a neutral reminder, not a completion callout, got:\n%s", view)
	}
}

func TestUnverifiedViewHidesAllGamificationLines(t *testing.T) {
	m := newResultsModel(stats.Result{WPM: 300, Accuracy: 100}, 100, resultsExtra{
		unverified: true,
		// These would normally produce visible callouts -- they must be
		// fully ignored in the unverified view.
		isNewPB: true, levelsGained: 1, newWordStreakRecord: true, xpGained: 999,
	})
	view := m.View()
	if !containsSubstring(view, "Unverified run") {
		t.Fatalf("expected the unverified warning, got:\n%s", view)
	}
	for _, mustNotAppear := range []string{"NEW PERSONAL BEST", "LEVEL UP", "NEW RECORD", "XP:"} {
		if containsSubstring(view, mustNotAppear) {
			t.Fatalf("expected no gamification callout %q in the unverified view, got:\n%s", mustNotAppear, view)
		}
	}
}

func TestResultsViewOverlaysGhostWhenAvailable(t *testing.T) {
	withGhost := newResultsModel(
		stats.Result{WPM: 60, Accuracy: 90, WPMSeries: []float64{40, 50, 60}},
		100,
		resultsExtra{newLevel: 3, ghostWPMSeries: []float64{35, 45, 55}},
	)
	view := withGhost.View()
	if !containsSubstring(view, "ghost (PB)") {
		t.Fatalf("expected a ghost overlay legend when ghostWPMSeries is set, got:\n%s", view)
	}

	withoutGhost := newResultsModel(
		stats.Result{WPM: 60, Accuracy: 90, WPMSeries: []float64{40, 50, 60}},
		100,
		resultsExtra{newLevel: 3},
	)
	if containsSubstring(withoutGhost.View(), "ghost (PB)") {
		t.Fatal("expected no ghost overlay legend without a ghost series")
	}
}

func TestResultsUpdateKeyBindings(t *testing.T) {
	m := newResultsModel(stats.Result{}, 100, resultsExtra{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to return a restart command")
	}
	if _, ok := cmd().(restartMsg); !ok {
		t.Fatal("expected enter to produce restartMsg")
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected esc to return a quit command")
	}
}
