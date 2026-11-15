package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/quotes"
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
		// -1 means "none" for all three -- the zero value (0) would
		// otherwise silently mark word index 0 as golden, green and blue
		// in every test that doesn't care about that mechanic.
		goldenWordIdx: -1,
		greenWordIdx:  -1,
		blueWordIdx:   -1,
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
	m := newTypingModel(modeTime, 30, 80, 150, 1, false, false, false, false, nil, nil, nil, nil)
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
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, false, nil, nil, nil, nil)
	if _, active := m.ghostStatus(); active {
		t.Fatal("expected inactive with no ghost pace at all")
	}

	m2 := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, false, nil, nil, []int{500, 1000}, nil)
	if _, active := m2.ghostStatus(); active {
		t.Fatal("expected inactive before the run has started (wordIndex still 0)")
	}
}

func TestGhostStatusComputesAheadAndBehind(t *testing.T) {
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, false, nil, nil, []int{1000, 2000, 3000}, nil)
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
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, false, nil, nil, nil, []float64{40, 55, 60})
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

// --- Golden word (shield) and green word (heal) ---

func TestRollGoldenWordIndexStaysWithinBoundsAndRespectsChance(t *testing.T) {
	const trials = 5000
	hits := 0
	for range trials {
		idx := rollGoldenWordIndex(80)
		if idx == -1 {
			continue
		}
		hits++
		if idx < game.GoldenWordMinIndex || idx > game.GoldenWordMaxIndex {
			t.Fatalf("golden word index %d out of bounds [%d, %d]", idx, game.GoldenWordMinIndex, game.GoldenWordMaxIndex)
		}
	}
	rate := float64(hits) / trials
	// Loose statistical bound -- just confirms it's in the right
	// ballpark, not an exact match.
	if rate < game.GoldenWordChance*0.7 || rate > game.GoldenWordChance*1.3 {
		t.Fatalf("empirical golden word rate %.3f too far from configured chance %.3f", rate, game.GoldenWordChance)
	}
}

func TestRollGoldenWordIndexNoneForShortWordLists(t *testing.T) {
	if got := rollGoldenWordIndex(0); got != -1 {
		t.Fatalf("expected -1 for an empty word list, got %d", got)
	}
	if got := rollGoldenWordIndex(game.GoldenWordMinIndex); got != -1 {
		t.Fatalf("expected -1 when the word list isn't long enough to leave room, got %d", got)
	}
}

func TestGoldenWordGrantsShieldOnlyWhenLandedClean(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat", "dog"}, 100)
	m.goldenWordIdx = 0
	m = typeString(m, "cat")
	m = pressSpace(m)
	if !m.shieldActive {
		t.Fatal("expected a clean golden word to activate the shield")
	}
	if m.goldenWordIdx != -1 {
		t.Fatal("expected goldenWordIdx consumed (reset to -1) after landing it")
	}
}

func TestGoldenWordMissedGrantsNoShield(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat", "dog"}, 100)
	m.goldenWordIdx = 0
	m = typeString(m, "cXt") // mistake on the golden word itself
	m = pressSpace(m)
	if m.shieldActive {
		t.Fatal("expected a missed golden word to NOT activate the shield")
	}
	if m.goldenWordIdx != -1 {
		t.Fatal("expected the golden word opportunity consumed even when missed")
	}
}

func TestShieldForgivesExactlyOneMistakeThenExpires(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"aaaa"}, 100)
	m.shieldActive = true

	m = typeString(m, "x") // wrong keystroke, should be absorbed
	if m.hp != 100 {
		t.Fatalf("expected no HP loss while the shield absorbs a mistake, got hp=%d", m.hp)
	}
	if m.shieldActive {
		t.Fatal("expected the shield consumed by that mistake")
	}

	m = typeString(m, "x") // another wrong keystroke, shield is gone now
	if m.hp >= 100 {
		t.Fatalf("expected normal HP loss once the shield is spent, got hp=%d", m.hp)
	}
}

// greenWordTestWordList returns n identical one-character words, enough
// for the green-word tests below regardless of exact streak arithmetic.
func greenWordTestWordList(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "w"
	}
	return out
}

func TestGreenWordAppearsTwoWordsAheadAndHealsOnCleanCompletion(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)
	m.anyMistake = true // a run that already slipped gets green (heal), not blue
	m.hp = 50           // leave room to observe the heal

	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	// Reached the streak milestone at word index
	// GreenWordStreakInterval-1 (the 10th word); the green word is marked
	// two words ahead, not one -- see the comment at the assignment site
	// for why one word of normal "upcoming" lead time matters.
	wantGreenIdx := game.GreenWordStreakInterval + 1
	if m.greenWordIdx != wantGreenIdx {
		t.Fatalf("expected the green word marked at index %d, got %d", wantGreenIdx, m.greenWordIdx)
	}

	m = typeString(m, "w") // the intervening normal word -- no effect
	m = pressSpace(m)
	if m.hp != 50 {
		t.Fatalf("expected the intervening word to have no effect on hp, got %d", m.hp)
	}

	m = typeString(m, "w") // lands the actual green word
	m = pressSpace(m)
	if m.hp != 50+game.GreenWordHealAmount {
		t.Fatalf("expected hp=%d after healing, got %d", 50+game.GreenWordHealAmount, m.hp)
	}
	if m.greenWordIdx != -1 {
		t.Fatal("expected the green word opportunity consumed after landing it")
	}
}

func TestGreenWordIsVisibleAsUpcomingBeforeBecomingCurrent(t *testing.T) {
	// Regression test: the green word must render with its special color
	// for at least one frame while it's still upcoming (idx > wordIndex),
	// not only become reachable once it's already the current word (where
	// the current-word render case would always shadow it).
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)
	m.anyMistake = true // a run that already slipped gets green (heal), not blue

	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	if m.greenWordIdx <= m.wordIndex {
		t.Fatalf("expected the green word (index %d) to still be ahead of the current word (index %d)", m.greenWordIdx, m.wordIndex)
	}
	view := m.View()
	if !containsSubstring(view, "w") {
		t.Fatal("expected the word list rendered at all")
	}
	// The actual color check: render the word list directly and confirm
	// the greenWordStyle-rendered word appears literally in the output.
	wantRendered := greenWordStyle.Render(wordList[m.greenWordIdx])
	if !strings.Contains(view, wantRendered) {
		t.Fatalf("expected the upcoming green word rendered in greenWordStyle, got:\n%s", view)
	}
}

func TestGreenWordMissedGrantsNoHeal(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)
	m.anyMistake = true // a run that already slipped gets green (heal), not blue
	m.hp = 50

	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	m = typeString(m, "w") // intervening normal word
	m = pressSpace(m)

	hpBeforeGreenWord := m.hp
	m = typeString(m, "Z") // one mistake on the green word (matches its length)
	m = pressSpace(m)
	// Missing it still deals normal mistake damage -- it just forfeits
	// the bonus heal, it doesn't grant immunity.
	if m.hp >= hpBeforeGreenWord {
		t.Fatalf("expected normal mistake damage from missing the green word, hp went from %d to %d", hpBeforeGreenWord, m.hp)
	}
	if m.greenWordIdx != -1 {
		t.Fatal("expected the green word opportunity consumed even when missed")
	}
}

func TestGreenWordHealCapsAtMaxHP(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)
	m.anyMistake = true // a run that already slipped gets green (heal), not blue
	m.hp = 100          // already full

	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	m = typeString(m, "w") // intervening normal word
	m = pressSpace(m)
	m = typeString(m, "w") // the green word
	m = pressSpace(m)
	if m.hp != 100 {
		t.Fatalf("expected hp capped at maxHP=100, got %d", m.hp)
	}
}

func TestGreenWordCanRetriggerOnLaterStreakMilestones(t *testing.T) {
	// A long, unbroken streak should offer a new green word at every
	// multiple of the interval, not just the first time.
	wordList := greenWordTestWordList(game.GreenWordStreakInterval*3 + 10)
	m := newTestTypingModel(modeWords, wordList, 100)
	m.anyMistake = true // a run that already slipped gets green (heal), not blue
	m.hp = 10

	typeClean := func(count int) {
		for i := 0; i < count; i++ {
			m = typeString(m, "w")
			m = pressSpace(m)
		}
	}

	typeClean(game.GreenWordStreakInterval) // reach the first milestone (streak=10)
	typeClean(1)                            // the intervening normal word
	typeClean(1)                            // lands the first green word
	afterFirstHeal := m.hp
	if afterFirstHeal <= 10 {
		t.Fatalf("expected the first heal to raise hp above the starting 10, got %d", afterFirstHeal)
	}

	typeClean(game.GreenWordStreakInterval - 2) // reach the second milestone (streak=20)
	if m.greenWordIdx == -1 {
		t.Fatal("expected a second green word opportunity at the next streak milestone")
	}
	typeClean(1) // the intervening normal word
	typeClean(1) // lands the second green word
	if m.hp <= afterFirstHeal {
		t.Fatalf("expected a second heal to raise hp again, had %d, now %d", afterFirstHeal, m.hp)
	}
}

func TestViewRendersGoldenAndGreenWordsDistinctly(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"alpha", "beta", "gamma"}, 100)
	m.goldenWordIdx = 1
	m.greenWordIdx = 2
	view := m.View()
	if !containsSubstring(view, "beta") || !containsSubstring(view, "gamma") {
		t.Fatalf("expected both special words still rendered, got:\n%s", view)
	}
}

func TestViewShowsShieldActiveIndicator(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"alpha"}, 100)
	if containsSubstring(m.View(), "shield active") {
		t.Fatal("expected no shield indicator before one is active")
	}
	m.shieldActive = true
	if !containsSubstring(m.View(), "shield active") {
		t.Fatal("expected a shield indicator once active")
	}
}

// --- Blue word (timed shield, flawless-run variant of the green word) ---

func TestFlawlessRunGetsBlueWordInsteadOfGreen(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)

	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	if m.greenWordIdx != -1 {
		t.Fatalf("expected no green word in a flawless run, got index %d", m.greenWordIdx)
	}
	wantIdx := game.GreenWordStreakInterval + 1
	if m.blueWordIdx != wantIdx {
		t.Fatalf("expected the blue word marked at index %d, got %d", wantIdx, m.blueWordIdx)
	}
	if !strings.Contains(m.View(), blueWordStyle.Render(wordList[m.blueWordIdx])) {
		t.Fatal("expected the upcoming blue word rendered in blueWordStyle")
	}
}

func TestFixedMistakeStillDisqualifiesBlueWord(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)

	// A typo that's corrected keeps the streak, but the run is no longer
	// flawless.
	m = typeString(m, "x")
	m = pressBackspace(m)
	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	if m.blueWordIdx != -1 || m.greenWordIdx == -1 {
		t.Fatalf("expected a green word after any mistake, got green=%d blue=%d", m.greenWordIdx, m.blueWordIdx)
	}
}

func TestBlueWordGrantsTimedShieldThatForgivesEveryMistake(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)

	for i := 0; i < game.GreenWordStreakInterval+2; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	if m.blueWordIdx != -1 {
		t.Fatal("expected the blue word opportunity consumed after landing it")
	}
	if !m.timedShieldActive() {
		t.Fatal("expected a clean blue word to start the timed shield")
	}
	if !containsSubstring(m.View(), "shield ") {
		t.Fatal("expected a timed shield indicator")
	}

	streak := m.successCombo
	m = typeString(m, "xyz") // several mistakes, all forgiven
	if m.hp != 100 || m.hitCount != 0 {
		t.Fatalf("expected no damage under the timed shield, got hp=%d hits=%d", m.hp, m.hitCount)
	}
	if !m.timedShieldActive() {
		t.Fatal("expected the timed shield not to be consumed by mistakes")
	}
	m = pressBackspace(m)
	m = pressBackspace(m)
	m = pressBackspace(m)
	m = typeString(m, "w")
	m = pressSpace(m)
	if m.successCombo != streak+1 {
		t.Fatalf("expected the streak to survive shielded mistakes, got %d want %d", m.successCombo, streak+1)
	}
}

func TestTimedShieldDoesNotCountUncorrectedWordsAsClean(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"cat", "dog"}, 100)
	m.shieldUntil = time.Now().Add(time.Minute)
	m.successCombo = 5
	m = typeString(m, "xyz") // garbage, all forgiven HP-wise
	m = pressSpace(m)
	if m.hp != 100 {
		t.Fatalf("expected no damage under the timed shield, got hp=%d", m.hp)
	}
	if m.successCombo != 0 {
		t.Fatalf("expected an uncorrected shielded word to break the streak, got %d", m.successCombo)
	}
}

func TestGoldenAbsorbedMistakeDisqualifiesBlueWord(t *testing.T) {
	m := newTestTypingModel(modeWords, greenWordTestWordList(20), 100)
	m.shieldActive = true
	m = typeString(m, "x")
	m = pressBackspace(m)
	if !m.anyMistake {
		t.Fatal("expected a shield-absorbed mistake to still count as a mistake for the blue word")
	}
}

func TestMistakeBeforeBlueWordTurnsItGreen(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)

	for i := 0; i < game.GreenWordStreakInterval; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	blueIdx := m.blueWordIdx
	if blueIdx == -1 {
		t.Fatal("expected a pending blue word after a flawless streak")
	}

	// Slip on the intervening word, then fix it.
	m = typeString(m, "x")
	m = pressBackspace(m)
	if m.blueWordIdx != -1 || m.greenWordIdx != blueIdx {
		t.Fatalf("expected the pending blue word to revert to green at %d, got green=%d blue=%d", blueIdx, m.greenWordIdx, m.blueWordIdx)
	}
	if !strings.Contains(m.View(), greenWordStyle.Render(wordList[blueIdx])) {
		t.Fatal("expected the reverted word rendered green")
	}

	m = typeString(m, "w")
	m = pressSpace(m)
	m = typeString(m, "w") // land the (now green) word
	m = pressSpace(m)
	if m.timedShieldActive() {
		t.Fatal("expected no timed shield once the run had a mistake before the word")
	}
}

func TestBlueWordMissedGrantsNoShield(t *testing.T) {
	wordList := greenWordTestWordList(20)
	m := newTestTypingModel(modeWords, wordList, 100)

	for i := 0; i < game.GreenWordStreakInterval+1; i++ {
		m = typeString(m, "w")
		m = pressSpace(m)
	}
	m = typeString(m, "Z") // miss the blue word
	m = pressSpace(m)
	if m.timedShieldActive() {
		t.Fatal("expected a missed blue word to grant no shield")
	}
	if m.blueWordIdx != -1 {
		t.Fatal("expected the blue word opportunity consumed even when missed")
	}
}

func TestExpiredTimedShieldNoLongerForgives(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"aaaa"}, 100)
	m.shieldUntil = time.Now().Add(-time.Millisecond)
	m = typeString(m, "x")
	if m.hp >= 100 {
		t.Fatalf("expected normal damage once the timed shield expired, got hp=%d", m.hp)
	}
}

func TestTimedShieldPreservesGoldenShield(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"aaaa"}, 100)
	m.shieldActive = true
	m.shieldUntil = time.Now().Add(time.Minute)
	m = typeString(m, "x")
	if !m.shieldActive {
		t.Fatal("expected the golden shield kept while the timed shield absorbs the mistake")
	}
}

var testQuotePool = []quotes.Quote{
	{ID: 1, Text: "Alpha beta gamma delta epsilon."},
	{ID: 2, Text: "Zeta eta theta iota kappa."},
}

func TestNewTypingModelQuotesModeDrawsFromQuotePool(t *testing.T) {
	m := newTypingModel(modeWords, 6, 80, 100, 1, false, false, false, true, testQuotePool, nil, nil, nil)
	if len(m.wordList) != 6 {
		t.Fatalf("expected exactly 6 words, got %d: %v", len(m.wordList), m.wordList)
	}
	poolWords := make(map[string]bool)
	for _, q := range testQuotePool {
		for _, qw := range strings.Fields(q.Text) {
			poolWords[strings.TrimRight(qw, ".")] = true
		}
	}
	for _, w := range m.wordList {
		if !poolWords[strings.TrimRight(w, ".")] {
			t.Errorf("word %q in the generated list doesn't come from testQuotePool", w)
		}
	}
}

func TestNewTypingModelQuotesModeForcesPunctuationOn(t *testing.T) {
	m := newTypingModel(modeWords, 10, 80, 100, 1, false, false, false, true, testQuotePool, nil, nil, nil)
	if !m.punctuation {
		t.Fatal("expected quotes mode to force punctuation on")
	}
}

func TestNewTypingModelRandomModeDoesNotUseQuotePool(t *testing.T) {
	m := newTypingModel(modeWords, 50, 80, 100, 1, false, false, false, false, testQuotePool, nil, nil, nil)
	distinctiveQuoteWords := map[string]bool{"alpha": true, "zeta": true, "kappa": true, "theta": true}
	for _, w := range m.wordList {
		if distinctiveQuoteWords[strings.ToLower(w)] {
			t.Fatalf("expected no quote-sourced words when quotesMode is false, found %q from testQuotePool", w)
		}
	}
}

func TestEnsureWordsTimeModeTopsUpFromQuotePoolWhenQuotesMode(t *testing.T) {
	m := newTypingModel(modeTime, 30, 80, 100, 1, false, false, false, true, testQuotePool, nil, nil, nil)
	initialLen := len(m.wordList)
	m.wordIndex = initialLen - 1 // within ensureWords' lookahead threshold
	m.ensureWords()
	if len(m.wordList) <= initialLen {
		t.Fatalf("expected ensureWords to top up the quote-sourced word list, stayed at %d", initialLen)
	}
}

// --- Non-colour cues for special words ---

// Each special word has to be identifiable with the styling stripped out
// entirely, which is what a player who can't tell the colours apart, or a
// terminal that doesn't show them, is left with.
func TestSpecialWordsAreMarkedWithoutRelyingOnColour(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"alpha", "beta", "gamma", "delta", "omega"}, 100)
	m.goldenWordIdx, m.greenWordIdx, m.blueWordIdx = 1, 2, 3

	plain := stripANSI(m.View())
	for _, want := range []string{" alpha", "*beta", "+gamma", "~delta", " omega"} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected %q in the view, got:\n%s", want, plain)
		}
	}

	// The marker stays while the special word is the one being typed,
	// when the word itself is no longer drawn in its own colour.
	m = typeString(m, "alpha")
	m = pressSpace(m)
	m = typeString(m, "be")
	if plain := stripANSI(m.View()); !strings.Contains(plain, "*beta") {
		t.Errorf("expected the golden marker to stay on the word being typed, got:\n%s", plain)
	}

	// Once the word is behind the player the marker is gone.
	m = typeString(m, "ta")
	m = pressSpace(m)
	if plain := stripANSI(m.View()); strings.Contains(plain, "*beta") || !strings.Contains(plain, " beta") {
		t.Errorf("expected no marker on a completed word, got:\n%s", plain)
	}
}

// A word can turn special while the player is typing (the green and blue
// words are earned mid-run). The marker must not move any other text.
func TestSpecialWordMarkersDoNotShiftTheText(t *testing.T) {
	wordList := strings.Fields(strings.Repeat("some words that wrap over several lines of text ", 6))
	m := newTestTypingModel(modeWords, wordList, 100)
	m.width = 40
	before := stripANSI(m.View())

	lines, _ := wrapWords(m.wordList, m.pageStart, m.pageWidth(), visibleLines)
	// First word of a line, last word of a line, and one in the middle.
	m.goldenWordIdx = lines[1][0]
	m.greenWordIdx = lines[0][len(lines[0])-1]
	m.blueWordIdx = lines[2][1]
	after := stripANSI(m.View())

	if after == before {
		t.Fatal("expected the markers to appear")
	}
	// Blank out only the three cells the markers were put in.
	unmarked := after
	for _, marked := range []string{goldenWordMarker + "of text", greenWordMarker + "lines\n", blueWordMarker + "lines of"} {
		if !strings.Contains(unmarked, marked) {
			t.Fatalf("expected %q in the view, got:\n%s", marked, after)
		}
		unmarked = strings.Replace(unmarked, marked, " "+marked[1:], 1)
	}
	if unmarked != before {
		t.Errorf("markers moved the text:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	for _, line := range strings.Split(after, "\n") {
		if len([]rune(line)) > m.width {
			t.Errorf("line is wider than the %d-column terminal: %q", m.width, line)
		}
	}
}

func TestShieldIndicatorsCarryTheirWordsMarker(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"alpha"}, 100)
	m.shieldActive = true
	m.shieldUntil = time.Now().Add(time.Minute)
	plain := stripANSI(m.View())
	if !strings.Contains(plain, goldenWordMarker+" shield active") {
		t.Errorf("expected the golden word's marker on its shield, got:\n%s", plain)
	}
	if !strings.Contains(plain, blueWordMarker+" shield ") {
		t.Errorf("expected the blue word's marker on the timed shield, got:\n%s", plain)
	}
}
