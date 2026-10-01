package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/stats"
)

// newTestTypingModel builds a typingModel with a fixed, known word list
// (bypassing the random generator) so tests can type exact, predictable
// input.
func newTestTypingModel(mode testMode, wordList []string, maxHP int) typingModel {
	return typingModel{
		mode:       mode,
		target:     100,
		wordList:   wordList,
		typedWords: make([]string, 0, len(wordList)),
		tracker:    stats.NewTracker(),
		hp:         maxHP,
		maxHP:      maxHP,
	}
}

func typeString(m typingModel, s string) typingModel {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return next.(typingModel)
}

func pressSpace(m typingModel) typingModel {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	return next.(typingModel)
}

func pressBackspace(m typingModel) typingModel {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	return next.(typingModel)
}

func TestNewTypingModelInitializesHPAndWordList(t *testing.T) {
	m := newTypingModel(modeTime, 30, 80, 150, 1, false, false, false, nil, nil, nil)
	if m.hp != 150 || m.maxHP != 150 {
		t.Fatalf("expected hp=maxHP=150, got hp=%d maxHP=%d", m.hp, m.maxHP)
	}
	if len(m.wordList) == 0 {
		t.Fatal("expected a non-empty generated word list")
	}
}

func TestTypingCorrectCharactersAdvanceCurrentWord(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat", "dog"}, 100)
	m = typeString(m, "ca")
	if m.current != "ca" {
		t.Fatalf("expected current=%q, got %q", "ca", m.current)
	}
	if !m.started {
		t.Fatal("expected started=true after the first keystroke")
	}
	if m.hp != 100 {
		t.Fatalf("expected no HP loss for correct characters, got %d", m.hp)
	}
}

func TestMistypedCharacterDealsHPDamageAndTracksWeakChar(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat"}, 100)
	m = typeString(m, "x") // wrong: expected 'c'
	if m.hp >= 100 {
		t.Fatalf("expected HP loss after a mistake, got %d", m.hp)
	}
	if m.hitCount != 1 {
		t.Fatalf("expected hitCount=1, got %d", m.hitCount)
	}
	if m.charMistakes['c'] != 1 {
		t.Fatalf("expected the mistake attributed to the expected char 'c', got %+v", m.charMistakes)
	}
}

func TestZenModeDisablesHPLoss(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat"}, 100)
	m.zenMode = true
	m = typeString(m, "xxx") // all wrong
	if m.hp != 100 {
		t.Fatalf("expected no HP loss in zen mode, got %d", m.hp)
	}
	if m.hitCount != 3 {
		t.Fatalf("expected hitCount to still track (for stats) even in zen mode, got %d", m.hitCount)
	}
}

func TestConsecutiveMistakesWithinComboWindowDealEscalatingDamage(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"aaaa"}, 1000)
	m = typeString(m, "x") // mistake 1: combo=1
	afterFirst := 1000 - m.hp
	m = typeString(m, "x") // mistake 2, same word, within combo window: combo=2
	afterSecond := 1000 - m.hp
	secondHit := afterSecond - afterFirst
	if secondHit <= afterFirst {
		t.Fatalf("expected the second consecutive mistake to deal more damage than the first (combo escalation): first=%d second=%d", afterFirst, secondHit)
	}
}

func TestKOTriggersWhenHPReachesZero(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"aaaaaaaaaaaaaaaaaaaa"}, 5)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xxxxx")})
	m = next.(typingModel)
	if !m.ko {
		t.Fatalf("expected KO once HP reaches 0, hp=%d", m.hp)
	}
	if !m.finished {
		t.Fatal("expected finished=true after a KO")
	}
	if cmd == nil {
		t.Fatal("expected finish() to return a command")
	}
	msg := cmd()
	ftm, ok := msg.(finishTypingMsg)
	if !ok {
		t.Fatalf("expected finishTypingMsg, got %T", msg)
	}
	if !ftm.ko {
		t.Fatal("expected finishTypingMsg.ko = true")
	}
}

func TestCompleteWordTracksStreakAndResetsOnMistake(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat", "dog", "fox"}, 100)
	m = typeString(m, "cat")
	m = pressSpace(m)
	if m.successCombo != 1 || m.bestCombo != 1 {
		t.Fatalf("expected successCombo=bestCombo=1 after one clean word, got %d/%d", m.successCombo, m.bestCombo)
	}

	m = typeString(m, "dXg") // mistake
	m = pressSpace(m)
	if m.successCombo != 0 {
		t.Fatalf("expected successCombo reset to 0 after a word with a mistake, got %d", m.successCombo)
	}
	if m.bestCombo != 1 {
		t.Fatalf("expected bestCombo to remain at its peak (1), got %d", m.bestCombo)
	}
}

func TestBackspaceWithinWordClearsMistakeFlagOnFullClear(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat"}, 100)
	m = typeString(m, "x") // mistake, sets wordHasMistake
	if !m.wordHasMistake {
		t.Fatal("expected wordHasMistake=true after a mistake")
	}
	m = pressBackspace(m) // clears current back to ""
	if m.current != "" {
		t.Fatalf("expected current cleared, got %q", m.current)
	}
	if m.wordHasMistake {
		t.Fatal("expected wordHasMistake reset to false once the word is fully cleared (streak gets a clean slate)")
	}
}

func TestBackspaceAcrossWordBoundaryUndoesCompletion(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat", "dog"}, 100)
	m = typeString(m, "cat")
	m = pressSpace(m) // completes "cat", moves to "dog"
	if m.wordIndex != 1 {
		t.Fatalf("expected wordIndex=1 after completing one word, got %d", m.wordIndex)
	}

	m = pressBackspace(m) // nothing in current -> undo "cat"'s completion
	if m.wordIndex != 0 {
		t.Fatalf("expected wordIndex=0 after undoing the completion, got %d", m.wordIndex)
	}
	if m.current != "cat" {
		t.Fatalf("expected current restored to 'cat', got %q", m.current)
	}
	if len(m.typedWords) != 0 {
		t.Fatalf("expected typedWords to shrink back, got %v", m.typedWords)
	}
}

func TestBackspaceAtTheVeryStartIsANoOp(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat"}, 100)
	before := m
	m = pressBackspace(m)
	if m.wordIndex != before.wordIndex || m.current != before.current {
		t.Fatal("expected backspace with nothing to undo to be a no-op")
	}
}

func TestPastedInputIsFullyRejected(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"the", "quick", "brown"}, 100)
	m = typeString(m, "t")
	if m.current != "t" || !m.started {
		t.Fatalf("expected a real keystroke to register and start the run, got current=%q started=%v", m.current, m.started)
	}

	before := m
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("the quick brown fox"), Paste: true})
	m = next.(typingModel)

	if m.current != before.current {
		t.Fatalf("expected paste to be ignored, current changed from %q to %q", before.current, m.current)
	}
	if m.wordIndex != before.wordIndex {
		t.Fatalf("expected paste to not advance any words, wordIndex changed from %d to %d", before.wordIndex, m.wordIndex)
	}
	if m.hp != before.hp {
		t.Fatalf("expected paste to not affect HP, hp changed from %d to %d", before.hp, m.hp)
	}
	if len(m.typedWords) != len(before.typedWords) {
		t.Fatalf("expected paste to not complete any words, typedWords len changed from %d to %d", len(before.typedWords), len(m.typedWords))
	}
	if m.pasteAttempts != 1 {
		t.Fatalf("expected pasteAttempts=1, got %d", m.pasteAttempts)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = next.(typingModel)
	if m.current != "th" {
		t.Fatalf("expected keystrokes after a blocked paste to still register, got current=%q", m.current)
	}
}

func TestPasteWarningAppearsInView(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"the"}, 100)
	if containsSubstring(m.View(), "paste blocked") {
		t.Fatal("expected no paste warning before any paste attempt")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi"), Paste: true})
	m = next.(typingModel)
	if !containsSubstring(m.View(), "paste blocked (1)") {
		t.Fatal("expected the view to show a paste-blocked warning after a paste attempt")
	}
}

func TestWordsModeFinishesAtTarget(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"a", "b", "c"}, 100)
	m.target = 2
	m = typeString(m, "a")
	m = pressSpace(m)
	if m.finished {
		t.Fatal("expected not finished after 1 of 2 target words")
	}
	m = typeString(m, "b")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(typingModel)
	if !m.finished {
		t.Fatal("expected finished after reaching the word-count target")
	}
	if cmd == nil {
		t.Fatal("expected a finish command")
	}
}

func TestTimeModeFinishesWhenElapsedReachesTarget(t *testing.T) {
	m := newTestTypingModel(modeTime, []string{"a", "b", "c"}, 100)
	m.target = 1 // 1 second
	m.started = true
	m.tracker.Start = time.Now().Add(-2 * time.Second) // already over target

	next, cmd := m.Update(tickMsg(time.Now()))
	m = next.(typingModel)
	if !m.finished {
		t.Fatal("expected the tick to finish a time-mode test once elapsed >= target")
	}
	if cmd == nil {
		t.Fatal("expected a finish command")
	}
}

func TestGhostStatusInactiveBeforeStartOrWithoutGhostData(t *testing.T) {
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, nil, nil, nil)
	if _, active := m.ghostStatus(); active {
		t.Fatal("expected inactive with no ghost pace at all")
	}

	m2 := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, nil, []int{500, 1000}, nil)
	if _, active := m2.ghostStatus(); active {
		t.Fatal("expected inactive before the run has started (wordIndex still 0)")
	}
}

func TestGhostStatusComputesAheadAndBehind(t *testing.T) {
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, nil, []int{1000, 2000, 3000}, nil)
	m.started = true
	m.wordIndex = 1

	m.tracker.Start = time.Now().Add(-600 * time.Millisecond)
	delta, active := m.ghostStatus()
	if !active {
		t.Fatal("expected active with ghost data and wordIndex=1")
	}
	if delta < 350 || delta > 450 {
		t.Fatalf("expected delta near +400ms (ahead), got %dms", delta)
	}

	m.tracker.Start = time.Now().Add(-1500 * time.Millisecond)
	delta, active = m.ghostStatus()
	if !active {
		t.Fatal("expected active")
	}
	if delta > -450 || delta < -550 {
		t.Fatalf("expected delta near -500ms (behind), got %dms", delta)
	}

	m.wordIndex = 10
	if _, active := m.ghostStatus(); active {
		t.Fatal("expected inactive once past the ghost's recorded word count")
	}
}

func TestFinishCarriesGhostWPMSeriesThrough(t *testing.T) {
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, nil, nil, []float64{40, 55, 60})
	m.started = true
	m.tracker.Start = time.Now().Add(-time.Second)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // not finish; just confirm field is set on the model
	_ = next
	_ = cmd
	if len(m.ghostWPMSeries) != 3 {
		t.Fatalf("expected ghostWPMSeries to be set on the model, got %v", m.ghostWPMSeries)
	}
	_, cmd2 := m.finish()
	msg := cmd2().(finishTypingMsg)
	if len(msg.ghostWPMSeries) != 3 {
		t.Fatalf("expected finishTypingMsg to carry the ghost WPM series through, got %v", msg.ghostWPMSeries)
	}
}

func TestWrapWordsRespectsWidthAndMaxLines(t *testing.T) {
	list := []string{"aaa", "bbb", "ccc", "ddd", "eee", "fff"}
	lines, next := wrapWords(list, 0, 8, 2) // "aaa bbb" fits in 8 chars; "ccc" wouldn't
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	if len(lines) > 2 {
		t.Fatalf("expected at most 2 lines (maxLines), got %d", len(lines))
	}
	if next <= 0 {
		t.Fatalf("expected a positive nextStart when more words remain, got %d", next)
	}
}

func TestWrapWordsHandlesAllWordsFittingInFewerLinesThanMax(t *testing.T) {
	list := []string{"a", "b"}
	lines, next := wrapWords(list, 0, 80, 3)
	if next != len(list) {
		t.Fatalf("expected nextStart to reach the end of the list (%d), got %d", len(list), next)
	}
	total := 0
	for _, l := range lines {
		total += len(l)
	}
	if total != len(list) {
		t.Fatalf("expected all %d words placed across lines, got %d", len(list), total)
	}
}

func TestRenderWordHighlightsCorrectAndWrongCharacters(t *testing.T) {
	out := renderWord("cat", "cXt")
	if !strings.Contains(out, "c") || !strings.Contains(out, "t") {
		t.Fatalf("expected rendered word to still contain correct characters, got %q", out)
	}
}

func TestRenderHPBarClampsToValidRange(t *testing.T) {
	// Must not panic or produce a negative-width bar for edge-case inputs.
	for _, c := range []struct{ hp, maxHP int }{
		{0, 100}, {100, 100}, {150, 100}, {-10, 100}, {50, 0},
	} {
		out := renderHPBar(c.hp, c.maxHP)
		if out == "" {
			t.Fatalf("hp=%d maxHP=%d: expected non-empty output", c.hp, c.maxHP)
		}
	}
}
