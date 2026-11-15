package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func pressKey(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(Model), cmd
}

// runCmd delivers cmd's message to m, as the program would.
func runCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestPrestigeFromTheProfileScreenSticksAcrossARun(t *testing.T) {
	m, profile := newTestApp(t)
	profile.Level, profile.XP = storage.PrestigeLevel+5, 1234
	if err := storage.Save(m.db, profile); err != nil {
		t.Fatal(err)
	}
	m = runCmd(m, func() tea.Msg { return viewProfileMsg{} })
	if m.state != stateProfileView || !containsSubstring(m.profileView.View(), "p: prestige") {
		t.Fatalf("expected the profile screen offering prestige, got state %v", m.state)
	}

	m, _ = pressKey(t, m, "p")
	if !containsSubstring(m.profileView.View(), "press y to prestige") {
		t.Fatal("expected a confirmation prompt")
	}
	var cmd tea.Cmd
	m, cmd = pressKey(t, m, "y")
	if profile.Level != 1 || profile.XP != 0 || profile.Prestige != 1 {
		t.Fatalf("expected the shared profile at level 1 with prestige 1, got %+v", profile)
	}

	// The new prestige achievement pops up, then returns to the profile.
	m = runCmd(m, cmd)
	if m.state != stateAchievementUnlock || !containsSubstring(m.achievementUnlock.View(), "Born Again") {
		t.Fatalf("expected the prestige achievement shown, got state %v", m.state)
	}
	m, cmd = pressKey(t, m, "x")
	m = runCmd(m, cmd)
	if m.state != stateProfileView {
		t.Fatalf("expected to return to the profile screen, got %v", m.state)
	}

	// A run saves the profile from memory; that must not bring the old
	// level back.
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 95, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100, wordsTyped: 20,
	})
	m = next.(Model)
	profiles, _ := storage.ListProfiles(m.db)
	if got := profiles[0]; got.Level >= storage.PrestigeLevel || got.Prestige != 1 {
		t.Fatalf("expected the prestige to survive a run in the database, got level %d prestige %d", got.Level, got.Prestige)
	}
	if m.profile.Level >= storage.PrestigeLevel || m.profile.Prestige != 1 {
		t.Fatalf("expected the prestige to survive a run in memory, got %+v", m.profile)
	}
}

func TestPrestigeIsRefusedBelowTheLevel(t *testing.T) {
	m, profile := newTestApp(t)
	profile.Level = storage.PrestigeLevel - 1
	if err := storage.Save(m.db, profile); err != nil {
		t.Fatal(err)
	}
	m = runCmd(m, func() tea.Msg { return viewProfileMsg{} })
	if containsSubstring(m.profileView.View(), "p: prestige") {
		t.Fatal("expected no prestige key offered below the prestige level")
	}
	m, _ = pressKey(t, m, "p")
	m, _ = pressKey(t, m, "y")
	if profile.Prestige != 0 || profile.Level != storage.PrestigeLevel-1 {
		t.Fatalf("expected nothing to change, got %+v", profile)
	}
	if !containsSubstring(m.profileView.View(), "prestige unlocks at level 30") {
		t.Fatal("expected a note saying when prestige unlocks")
	}
}

// A prestiged profile plays harder words, with the matching damage and HP,
// and the menu says which tier it's on.
func TestPrestigeRaisesTheWordTierInPlay(t *testing.T) {
	m, profile := newTestApp(t)
	profile.Prestige = 1 // level 1
	m, _ = m.startTyping(startTypingMsg{mode: modeTime, target: 30})
	if m.typing.level != game.DifficultyLevel(1, 1) || m.typing.maxHP != game.MaxHP(1, 1) {
		t.Fatalf("expected difficulty level %d and %d HP, got %d and %d",
			game.DifficultyLevel(1, 1), game.MaxHP(1, 1), m.typing.level, m.typing.maxHP)
	}
	menu := newMenuModel(profile, m.db)
	if !containsSubstring(menu.View(), "Difficulty: Medium") || !containsSubstring(menu.View(), "Prestige: ") {
		t.Fatalf("expected the menu to show Medium words and the prestige rank, got:\n%s", menu.View())
	}
}

// p opens the profile from the menu and arms prestige there, so a held or
// repeated p must never get as far as prestiging.
func TestRepeatedPDoesNotPrestige(t *testing.T) {
	m, profile := newTestApp(t)
	profile.Level = storage.PrestigeLevel
	if err := storage.Save(m.db, profile); err != nil {
		t.Fatal(err)
	}
	m.menu = newMenuModel(profile, m.db)
	for range 6 {
		var cmd tea.Cmd
		m, cmd = pressKey(t, m, "p")
		m = runCmd(m, cmd)
	}
	if profile.Prestige != 0 || profile.Level != storage.PrestigeLevel {
		t.Fatalf("expected repeated p not to prestige, got %+v", profile)
	}
}

// Another running copy of the game prestiges the profile. This copy's
// next run must not put the old level back, and it then plays on at the
// real level and rank.
func TestStaleProfileIsReloadedNotSaved(t *testing.T) {
	m, profile := newTestApp(t)
	profile.Level = storage.PrestigeLevel + 5
	if err := storage.Save(m.db, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.PrestigeProfile(m.db, profile.ID); err != nil { // "elsewhere"
		t.Fatal(err)
	}
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 95, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100, wordsTyped: 20,
	})
	m = next.(Model)
	stored, _ := storage.LoadProfile(m.db, profile.ID)
	if stored.Level != 1 || stored.Prestige != 1 {
		t.Fatalf("expected the database untouched by the stale save, got level %d prestige %d", stored.Level, stored.Prestige)
	}
	if m.profile.Level != 1 || m.profile.Prestige != 1 {
		t.Fatalf("expected the stale profile reloaded in memory, got %+v", m.profile)
	}
}
