package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/storage"
)

func TestHistoryViewEmptyState(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")

	m := newHistoryViewModel(profile, db)
	view := m.View()
	if !containsSubstring(view, "no exercises completed yet") {
		t.Fatalf("expected empty-state message, got:\n%s", view)
	}
}

func TestHistoryViewRendersEntriesNewestFirst(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")
	now := time.Now()
	storage.RecordTestResult(db, profile.ID, 65.5, 97.2, "time", 30, false, false, false, "random", 40, 30*time.Second, now)
	storage.RecordTestResult(db, profile.ID, 40, 80, "words", 50, true, false, false, "random", 20, 25*time.Second, now.Add(time.Minute))

	m := newHistoryViewModel(profile, db)
	if len(m.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.entries))
	}
	view := m.View()
	if !containsSubstring(view, "Test History") || !containsSubstring(view, "65.5") || !containsSubstring(view, "KO") {
		t.Fatalf("expected title, WPM, and KO flag; got:\n%s", view)
	}
}

func TestHistoryViewNavigatesBackToProfile(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")
	m := newHistoryViewModel(profile, db)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected esc to return a command")
	}
	if _, ok := cmd().(viewProfileMsg); !ok {
		t.Fatal("expected viewProfileMsg")
	}
}
