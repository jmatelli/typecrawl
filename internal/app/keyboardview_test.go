package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
