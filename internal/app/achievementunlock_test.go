package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/achievements"
)

func TestAchievementUnlockViewListsEachUnlockedAchievement(t *testing.T) {
	unlocked := []achievements.Achievement{
		{ID: "first_exercise", Name: "First Steps", Description: "Complete your first exercise", Tier: achievements.Bronze},
		{ID: "wpm_30", Name: "30 WPM Club", Description: "Hit 30 WPM in a test", Tier: achievements.Bronze},
	}
	m := newAchievementUnlockModel(unlocked)
	view := m.View()
	for _, a := range unlocked {
		if !containsSubstring(view, a.Name) {
			t.Errorf("expected %q in the view, got:\n%s", a.Name, view)
		}
	}
	if !containsSubstring(view, "Achievement Unlocked") {
		t.Fatalf("expected the headline, got:\n%s", view)
	}
}

func TestAnyKeyDismissesToResults(t *testing.T) {
	m := newAchievementUnlockModel(nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("expected a command from any keypress")
	}
	if _, ok := cmd().(continueToResultsMsg); !ok {
		t.Fatal("expected continueToResultsMsg")
	}
}

func TestNonKeyMessagesDoNotDismiss(t *testing.T) {
	m := newAchievementUnlockModel(nil)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatal("expected a non-key message to not dismiss the screen")
	}
}
