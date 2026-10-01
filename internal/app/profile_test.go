package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/storage"
)

func newTestProfileModel(t *testing.T) profileModel {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return newProfileModel(db)
}

func TestEmptyNameIsRejected(t *testing.T) {
	m := newTestProfileModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(profileModel)
	if m.errMsg == "" {
		t.Fatal("expected an error message for an empty name")
	}
	if m.created != nil {
		t.Fatal("expected no profile created for an empty name")
	}
}

func TestWhitespaceOnlyNameIsRejected(t *testing.T) {
	m := newTestProfileModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("   ")})
	m = next.(profileModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(profileModel)
	if m.created != nil {
		t.Fatal("expected whitespace-only name to be rejected (trimmed to empty)")
	}
}

func TestDuplicateNameShowsAnError(t *testing.T) {
	m := newTestProfileModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alice")})
	m = next.(profileModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(profileModel)
	if m.created == nil {
		t.Fatal("expected the first creation to succeed")
	}

	// A second profileModel against the same DB trying the same name.
	m2 := profileModel{db: m.db}
	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alice")})
	m2 = next.(profileModel)
	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 = next.(profileModel)
	if m2.created != nil {
		t.Fatal("expected a duplicate name to fail")
	}
	if m2.errMsg == "" {
		t.Fatal("expected an error message for a duplicate name")
	}
}

func TestSuccessfulCreationRevealsAvatarBeforeDismissing(t *testing.T) {
	m := newTestProfileModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bob")})
	m = next.(profileModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(profileModel)
	if m.created == nil || m.created.Name != "bob" {
		t.Fatalf("expected profile 'bob' created, got %+v", m.created)
	}

	// Immediately pressing a key must NOT dismiss the reveal yet.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(profileModel)
	if cmd != nil {
		t.Fatal("expected no dismissal command before revealMinDuration has passed")
	}

	// Simulate the minimum reveal duration having passed.
	m.revealAt = time.Now().Add(-revealMinDuration - time.Millisecond)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if cmd == nil {
		t.Fatal("expected a dismissal command once revealMinDuration has passed")
	}
	msg := cmd()
	created, ok := msg.(profileCreatedMsg)
	if !ok || created.profile.Name != "bob" {
		t.Fatalf("expected profileCreatedMsg for 'bob', got %#v", msg)
	}
}

func TestProfileNameStripsControlCharactersAndEscapeSequences(t *testing.T) {
	m := newTestProfileModel(t)
	malicious := "Eve\x1b]0;PWNED\x07"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(malicious), Paste: true})
	m = next.(profileModel)

	if strings.ContainsAny(m.name, "\x1b\x07") {
		t.Fatalf("expected control characters to be stripped, got %q", m.name)
	}
	// Only the control bytes (ESC, BEL) are stripped -- the OSC syntax
	// characters ("]0;") are themselves ordinary printable runes and pass
	// through as inert text, which is the point: without the ESC byte, a
	// terminal can never parse any of this as a control sequence.
	if m.name != "Eve]0;PWNED" {
		t.Fatalf("expected 'Eve]0;PWNED' (control bytes stripped, rest inert), got %q", m.name)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(profileModel)
	if m.created == nil {
		t.Fatal("expected profile creation to succeed with the sanitized name")
	}
	view := m.View()
	if strings.Contains(view, "\x1b") || strings.Contains(view, "\x07") {
		t.Fatal("expected no raw control bytes anywhere in the rendered view")
	}
}

func TestProfileNameAllowsNormalUnicodeNames(t *testing.T) {
	m := newTestProfileModel(t)
	for _, r := range "Jöel 日本語 🎮" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(profileModel)
	}
	if m.name != "Jöel 日本語 🎮" {
		t.Fatalf("expected normal unicode name to pass through unchanged, got %q", m.name)
	}
}

func TestProfileNameIsCappedAtMaxLength(t *testing.T) {
	m := newTestProfileModel(t)
	huge := strings.Repeat("x", 500)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(huge), Paste: true})
	m = next.(profileModel)
	if len([]rune(m.name)) != maxNameLength {
		t.Fatalf("expected name capped at %d runes, got %d", maxNameLength, len([]rune(m.name)))
	}
}

func TestBackspaceTrimsByRuneNotByte(t *testing.T) {
	m := newTestProfileModel(t)
	// "é" as a single precomposed rune is 2 UTF-8 bytes. A byte-level trim
	// would leave a dangling continuation byte; a rune-level trim removes
	// the whole character cleanly.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("café")})
	m = next.(profileModel)
	if m.name != "café" {
		t.Fatalf("expected 'café', got %q", m.name)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(profileModel)
	if m.name != "caf" {
		t.Fatalf("expected backspace to remove exactly the 'é' rune, leaving 'caf', got %q (valid utf8: %v)", m.name, isValidUTF8(m.name))
	}
}

func TestBackspaceOnEmptyNameIsANoOp(t *testing.T) {
	m := newTestProfileModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(profileModel)
	if m.name != "" {
		t.Fatalf("expected empty name to remain empty, got %q", m.name)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestCtrlCQuitsAtAnyStage(t *testing.T) {
	m := newTestProfileModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected ctrl+c to return a quit command on the name-entry screen")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("carol")})
	m = next.(profileModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(profileModel)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected ctrl+c to return a quit command on the avatar-reveal screen too")
	}
}
