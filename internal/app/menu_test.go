package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/storage"
)

func newTestMenuModel(t *testing.T) menuModel {
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
	return newMenuModel(profile, db)
}

func TestModeStringConversion(t *testing.T) {
	if got := modeString(modeTime); got != "time" {
		t.Errorf("modeString(modeTime) = %q, want %q", got, "time")
	}
	if got := modeString(modeWords); got != "words" {
		t.Errorf("modeString(modeWords) = %q, want %q", got, "words")
	}
}

func TestIndexOfFindsValueOrDefaultsToZero(t *testing.T) {
	opts := []int{15, 30, 60, 120}
	if got := indexOf(opts, 60); got != 2 {
		t.Errorf("indexOf(opts, 60) = %d, want 2", got)
	}
	if got := indexOf(opts, 9999); got != 0 {
		t.Errorf("indexOf(opts, 9999) = %d, want 0 (default)", got)
	}
}

func TestNewMenuModelLoadsSavedSettings(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, _ := storage.CreateProfile(db, "tester")
	storage.SaveSettings(db, profile.ID, storage.Settings{
		Mode: "words", Duration: 60, WordCount: 200, Punctuation: true, ZenMode: true,
	})
	m := newMenuModel(profile, db)
	if m.mode != modeWords {
		t.Error("expected mode loaded as words")
	}
	if m.wordOpts[m.wordIdx] != 200 {
		t.Errorf("expected word count 200, got %d", m.wordOpts[m.wordIdx])
	}
	if !m.punctuation || !m.zenMode {
		t.Error("expected punctuation and zenMode loaded as true")
	}
}

func TestTabTogglesModeAndPersists(t *testing.T) {
	m := newTestMenuModel(t)
	if m.mode != modeTime {
		t.Fatal("expected default mode to be time")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(menuModel)
	if m.mode != modeWords {
		t.Fatal("expected tab to switch to words mode")
	}
	settings, _ := storage.LoadSettings(m.db, m.profile.ID)
	if settings.Mode != "words" {
		t.Fatalf("expected the mode change to persist, got %q", settings.Mode)
	}
}

func TestLeftRightAdjustTargetWithinBounds(t *testing.T) {
	m := newTestMenuModel(t)
	initial := m.timeIdx
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(menuModel)
	if m.timeIdx != initial+1 {
		t.Fatalf("expected timeIdx incremented, got %d", m.timeIdx)
	}
	// Can't go below 0.
	for range len(m.timeOpts) + 2 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		m = next.(menuModel)
	}
	if m.timeIdx != 0 {
		t.Fatalf("expected timeIdx clamped at 0, got %d", m.timeIdx)
	}
	// Can't go above the last index.
	for range len(m.timeOpts) + 2 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
		m = next.(menuModel)
	}
	if m.timeIdx != len(m.timeOpts)-1 {
		t.Fatalf("expected timeIdx clamped at the last index, got %d", m.timeIdx)
	}
}

func TestTogglesFlipAndPersist(t *testing.T) {
	m := newTestMenuModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(menuModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m = next.(menuModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = next.(menuModel)
	if !m.punctuation || !m.zenMode || !m.focusWeak {
		t.Fatalf("expected all three toggles on, got punct=%v zen=%v focus=%v", m.punctuation, m.zenMode, m.focusWeak)
	}
	settings, _ := storage.LoadSettings(m.db, m.profile.ID)
	if !settings.Punctuation || !settings.ZenMode || !settings.FocusWeak {
		t.Fatal("expected toggles persisted to settings")
	}
}

func TestEnterStartsTypingWithCurrentSettings(t *testing.T) {
	m := newTestMenuModel(t)
	m.punctuation = true
	m.zenMode = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command from pressing enter")
	}
	msg, ok := cmd().(startTypingMsg)
	if !ok {
		t.Fatalf("expected startTypingMsg, got %T", msg)
	}
	if !msg.punctuation || !msg.zenMode {
		t.Fatalf("expected current toggle state carried into startTypingMsg, got %+v", msg)
	}
	if msg.target != m.timeOpts[m.timeIdx] {
		t.Fatalf("expected target=%d, got %d", m.timeOpts[m.timeIdx], msg.target)
	}
}

func TestPAndUNavigateToProfileAndSwitch(t *testing.T) {
	m := newTestMenuModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if _, ok := cmd().(viewProfileMsg); !ok {
		t.Fatal("expected 'p' to produce viewProfileMsg")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if _, ok := cmd().(switchProfileMsg); !ok {
		t.Fatal("expected 'u' to produce switchProfileMsg")
	}
}

func TestMenuViewShowsDailyChallengeStatus(t *testing.T) {
	m := newTestMenuModel(t)
	view := m.View()
	if !containsSubstring(view, "Daily challenge") {
		t.Fatalf("expected a daily challenge line, got:\n%s", view)
	}
}
