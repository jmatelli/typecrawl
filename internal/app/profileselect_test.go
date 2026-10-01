package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/storage"
)

func newTestProfileSelectModel(t *testing.T, names ...string) profileSelectModel {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var profiles []*storage.Profile
	for _, n := range names {
		p, err := storage.CreateProfile(db, n)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, p)
	}
	return newProfileSelectModel(db, profiles)
}

func TestCursorMovementClampsAtBothEnds(t *testing.T) {
	m := newTestProfileSelectModel(t, "a", "b", "c")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(profileSelectModel)
	if m.cursor != 0 {
		t.Fatalf("expected cursor clamped at 0, got %d", m.cursor)
	}
	for range 5 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(profileSelectModel)
	}
	if m.cursor != len(m.profiles)-1 {
		t.Fatalf("expected cursor clamped at the last index, got %d", m.cursor)
	}
}

func TestEnterSelectsTheCursoredProfile(t *testing.T) {
	m := newTestProfileSelectModel(t, "alice", "bob")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(profileSelectModel)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg, ok := cmd().(profileSelectedMsg)
	if !ok || msg.profile.Name != "bob" {
		t.Fatalf("expected profileSelectedMsg for bob, got %#v", msg)
	}
}

func TestEnterWithNoProfilesDoesNothing(t *testing.T) {
	m := newTestProfileSelectModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no command when selecting with zero profiles")
	}
}

func TestNKeyRequestsNewProfile(t *testing.T) {
	m := newTestProfileSelectModel(t, "alice")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if _, ok := cmd().(createNewProfileMsg); !ok {
		t.Fatal("expected 'n' to produce createNewProfileMsg")
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	m := newTestProfileSelectModel(t, "alice")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(profileSelectModel)
	if m.confirm != "d" {
		t.Fatal("expected delete armed, awaiting confirmation")
	}
	// Any other key cancels instead of confirming.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(profileSelectModel)
	if len(m.profiles) != 1 {
		t.Fatal("expected a non-matching key to cancel, not delete")
	}

	// Pressing 'd' again confirms and actually deletes.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(profileSelectModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(profileSelectModel)
	if len(m.profiles) != 0 {
		t.Fatalf("expected the profile deleted after confirmation, got %d remaining", len(m.profiles))
	}
}

func TestResetRequiresConfirmationAndKeepsProfileInList(t *testing.T) {
	m := newTestProfileSelectModel(t, "alice")
	m.profiles[0].Level = 10
	storage.Save(m.db, m.profiles[0])

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(profileSelectModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(profileSelectModel)

	if len(m.profiles) != 1 {
		t.Fatal("expected the profile to remain in the list after reset")
	}
	if m.profiles[0].Level != 1 {
		t.Fatalf("expected the profile's level reset to 1, got %d", m.profiles[0].Level)
	}
}

func TestProfileSelectViewShowsEmptyStateHint(t *testing.T) {
	m := newTestProfileSelectModel(t)
	view := m.View()
	if !containsSubstring(view, "no profiles yet") {
		t.Fatalf("expected an empty-state hint, got:\n%s", view)
	}
}
