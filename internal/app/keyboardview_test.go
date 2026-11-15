package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/keyboard"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func TestKeyboardViewEmptyState(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")

	m := newKeyboardViewModel(profile, db)
	view := m.View()
	if !containsSubstring(view, "not enough mistakes") {
		t.Fatalf("expected empty-state message, got:\n%s", view)
	}
}

func TestKeyboardViewRendersWithMistakeData(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")
	storage.RecordCharMistakes(db, profile.ID, map[rune]int{'j': 12, 'q': 3})

	m := newKeyboardViewModel(profile, db)
	view := m.View()
	if !containsSubstring(view, "j") || !containsSubstring(view, "Most mistakes") {
		t.Fatalf("expected the keyboard render with legend, got:\n%s", view)
	}
}

func TestKeyboardViewNavigatesBackToProfile(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")
	m := newKeyboardViewModel(profile, db)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected esc to return a command")
	}
	if _, ok := cmd().(viewProfileMsg); !ok {
		t.Fatal("expected viewProfileMsg")
	}
}

func TestKeyboardViewDefaultsToQWERTY(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")

	m := newKeyboardViewModel(profile, db)
	if m.layout.ID != keyboard.QWERTY.ID || !containsSubstring(m.View(), "QWERTY") {
		t.Fatalf("expected a new profile to show QWERTY, got %s:\n%s", m.layout.Name, m.View())
	}
}

// The layout can be changed even before any mistakes are recorded, and
// the choice is saved to the profile.
func TestKeyboardViewCyclesAndSavesLayout(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")

	m := newKeyboardViewModel(profile, db)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(keyboardViewModel)
	if m.layout.ID != keyboard.AZERTY.ID || !containsSubstring(m.View(), "AZERTY") {
		t.Fatalf("expected l to switch to AZERTY, got %s", m.layout.Name)
	}
	s, _ := storage.LoadSettings(db, profile.ID)
	if s.KeyboardLayout != keyboard.AZERTY.ID {
		t.Fatalf("expected the layout saved, got %q", s.KeyboardLayout)
	}
	if reopened := newKeyboardViewModel(profile, db); reopened.layout.ID != keyboard.AZERTY.ID {
		t.Fatalf("expected the saved layout on reopening, got %s", reopened.layout.Name)
	}
}

// Saving menu choices must not reset a layout picked on this screen.
func TestMenuSaveKeepsKeyboardLayout(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")
	if err := storage.SaveKeyboardLayout(db, profile.ID, keyboard.Dvorak.ID); err != nil {
		t.Fatal(err)
	}

	menu := newMenuModel(profile, db)
	menu.zenMode = true
	menu.saveSettings()

	s, _ := storage.LoadSettings(db, profile.ID)
	if s.KeyboardLayout != keyboard.Dvorak.ID || !s.ZenMode {
		t.Fatalf("expected the menu save to keep the layout, got %+v", s)
	}
}
