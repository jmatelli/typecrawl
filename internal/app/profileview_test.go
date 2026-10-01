package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/achievements"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func newTestProfileViewModel(t *testing.T) profileViewModel {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	profile, err := storage.CreateProfile(db, "tester")
	if err != nil {
		t.Fatal(err)
	}
	return newProfileViewModel(profile, db, 100)
}

func TestFormatPeriodSummaryShowsPlaceholderWithNoActivity(t *testing.T) {
	got := formatPeriodSummary("This week", storage.PeriodSummary{})
	if !containsSubstring(got, "no exercises yet") {
		t.Fatalf("expected placeholder text, got %q", got)
	}
}

func TestFormatPeriodSummaryShowsWeekdayForBestDay(t *testing.T) {
	got := formatPeriodSummary("This week", storage.PeriodSummary{
		Exercises: 5, AvgWPM: 68.3, BestDay: "2026-09-29", BestDayCount: 3,
	})
	if !containsSubstring(got, "Tuesday") {
		t.Fatalf("expected the weekday name for 2026-09-29 (a Tuesday), got %q", got)
	}
}

func TestFormatNudgeProgressShowsHoursForTimeAchievements(t *testing.T) {
	lp := achievements.LockedProgress{
		Achievement: achievements.Achievement{ID: "time_5h"},
		Current:     1800, Target: 18000,
	}
	if got := formatNudgeProgress(lp); got != "0.5h/5h" {
		t.Fatalf("expected '0.5h/5h', got %q", got)
	}
}

func TestFormatNudgeProgressShowsPlainNumbersOtherwise(t *testing.T) {
	lp := achievements.LockedProgress{
		Achievement: achievements.Achievement{ID: "exercises_10"},
		Current:     7, Target: 10,
	}
	if got := formatNudgeProgress(lp); got != "7/10" {
		t.Fatalf("expected '7/10', got %q", got)
	}
}

func TestSuccessRateSuffixEmptyWithNoExercises(t *testing.T) {
	if got := successRateSuffix(0, 0); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestSuccessRateSuffixComputesPercentage(t *testing.T) {
	got := successRateSuffix(8, 2)
	if !containsSubstring(got, "80%") {
		t.Fatalf("expected 80%% success rate, got %q", got)
	}
}

func TestProfileViewDeleteRequiresConfirmation(t *testing.T) {
	m := newTestProfileViewModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(profileViewModel)
	if m.confirm != "d" {
		t.Fatal("expected delete armed")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(profileViewModel)
	if cmd == nil {
		t.Fatal("expected a profileDeletedMsg command after confirming delete")
	}
	if _, ok := cmd().(profileDeletedMsg); !ok {
		t.Fatal("expected profileDeletedMsg")
	}
}

func TestProfileViewResetClearsStatsButKeepsName(t *testing.T) {
	m := newTestProfileViewModel(t)
	m.profile.Level = 10
	storage.Save(m.db, m.profile)
	storage.RecordActivity(m.db, m.profile.ID, time.Now())

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(profileViewModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(profileViewModel)

	if m.profile.Level != 1 {
		t.Fatalf("expected level reset to 1, got %d", m.profile.Level)
	}
	if m.profile.Name != "tester" {
		t.Fatalf("expected name preserved, got %q", m.profile.Name)
	}
	if len(m.activity) != 0 {
		t.Fatal("expected activity cleared in the model after reset")
	}
}

func TestProfileViewNonMatchingKeyCancelsConfirmation(t *testing.T) {
	m := newTestProfileViewModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(profileViewModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(profileViewModel)
	if m.confirm != "" {
		t.Fatal("expected confirmation cancelled by a non-matching key")
	}
	// Profile must still exist.
	profiles, _ := storage.ListProfiles(m.db)
	if len(profiles) != 1 {
		t.Fatal("expected the profile to still exist")
	}
}

func TestProfileViewTitleCycleKeyEquipsAndPersists(t *testing.T) {
	m := newTestProfileViewModel(t)
	m.profile.SuccessCount = 1
	storage.Save(m.db, m.profile)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = next.(profileViewModel)
	if m.profile.EquippedTitle == "" {
		t.Fatal("expected a title equipped after pressing 't' with an unlocked achievement")
	}
	profiles, _ := storage.ListProfiles(m.db)
	if profiles[0].EquippedTitle != m.profile.EquippedTitle {
		t.Fatal("expected the equipped title persisted to storage")
	}
}

func TestProfileViewNavigationKeys(t *testing.T) {
	m := newTestProfileViewModel(t)
	cases := map[string]interface{}{
		"a": viewAchievementsMsg{},
		"h": viewHistoryMsg{},
		"k": viewKeyboardMsg{},
	}
	for key, wantType := range cases {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd == nil {
			t.Fatalf("%s: expected a command", key)
		}
		msg := cmd()
		switch wantType.(type) {
		case viewAchievementsMsg:
			if _, ok := msg.(viewAchievementsMsg); !ok {
				t.Errorf("%s: expected viewAchievementsMsg, got %T", key, msg)
			}
		case viewHistoryMsg:
			if _, ok := msg.(viewHistoryMsg); !ok {
				t.Errorf("%s: expected viewHistoryMsg, got %T", key, msg)
			}
		case viewKeyboardMsg:
			if _, ok := msg.(viewKeyboardMsg); !ok {
				t.Errorf("%s: expected viewKeyboardMsg, got %T", key, msg)
			}
		}
	}
}

func TestProfileViewRendersWithoutPanicking(t *testing.T) {
	m := newTestProfileViewModel(t)
	view := m.View()
	for _, want := range []string{"Profile", "This week", "This month", "Achievements"} {
		if !containsSubstring(view, want) {
			t.Errorf("expected %q in the rendered view", want)
		}
	}
}
