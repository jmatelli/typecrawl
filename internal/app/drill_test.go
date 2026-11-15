package app

import (
	"slices"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/storage"
	"github.com/jmatelli/typecrawl/internal/textsource"
)

// A word counts as missed after any wrong key, even one fixed before
// moving on, and after being sent short. Words typed right, and words
// never reached, don't.
func TestMissedWordsAreEveryWordGotWrong(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"alpha", "beta", "gamma", "delta", "alpha", "omega"}, 100)
	m = typeString(m, "alpha")
	m = pressSpace(m) // right
	m = typeString(m, "bx")
	m = pressBackspace(m)
	m = typeString(m, "eta")
	m = pressSpace(m) // fixed, but missed
	m = typeString(m, "gam")
	m = pressSpace(m) // sent short
	m = typeString(m, "deltaa")
	m = pressSpace(m) // a key too many
	m = typeString(m, "alpxa")
	m = pressSpace(m) // alpha again, missed this time; listed once
	if got, want := m.missedWords(), []string{"beta", "gamma", "delta", "alpha"}; !slices.Equal(got, want) {
		t.Fatalf("missed %q, want %q", got, want)
	}
}

// Mistakes are recorded by key pair too: the key before and the key
// missed, in lowercase. The first key of a word has no pair.
func TestMistakesAreRecordedByKeyPair(t *testing.T) {
	m := newTestTypingModel(modeWords, []string{"The", "cat"}, 100)
	m = typeString(m, "Tj") // h missed after T
	m = pressBackspace(m)
	m = typeString(m, "he")
	m = pressSpace(m)
	m = typeString(m, "x") // c missed, first key: no pair
	if want := map[string]int{"th": 1}; !mapsEqual(m.pairMistakes, want) {
		t.Fatalf("pairs %v, want %v", m.pairMistakes, want)
	}
	if m.charMistakes['h'] != 1 || m.charMistakes['c'] != 1 {
		t.Fatalf("expected both keys counted, got %v", m.charMistakes)
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestBuildDrill(t *testing.T) {
	for _, c := range []struct {
		missed  []string
		repeats int
	}{
		{[]string{"one"}, 12}, // repeated more to make the minimum length
		{[]string{"a", "b"}, 6},
		{[]string{"x", "y", "z", "w"}, 3},
		{[]string{"p", "q", "r", "s", "t", "u"}, 3},
	} {
		missed := c.missed
		for range 50 {
			drill := buildDrill(missed)
			if len(drill) != len(missed)*c.repeats {
				t.Fatalf("%q: expected %d words, got %q", missed, len(missed)*c.repeats, drill)
			}
			counts := map[string]int{}
			for i, w := range drill {
				counts[w]++
				if i > 0 && len(missed) > 1 && drill[i-1] == w {
					t.Fatalf("%q: %q follows itself in %q", missed, w, drill)
				}
			}
			for _, w := range missed {
				if counts[w] != c.repeats {
					t.Fatalf("%q: expected %q %d times, got %q", missed, w, c.repeats, drill)
				}
			}
		}
	}
	many := make([]string, 50)
	for i := range many {
		many[i] = string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	if got := buildDrill(many); len(got) != drillMaxWords*drillRepeats {
		t.Fatalf("expected the drill capped at %d words, got %d", drillMaxWords*drillRepeats, len(got))
	}
	if got := buildDrill([]string{"a", "a", "b"}); len(got) != 12 {
		t.Fatalf("expected a repeated input word counted once, got %q", got)
	}
	m := newResultsModel(stats.Result{}, 80, resultsExtra{missedWords: many})
	if !containsSubstring(m.footer(), "d: drill the first 30 of the 50 words you missed") {
		t.Fatalf("expected the footer to say only 30 are drilled, got %q", m.footer())
	}
}

func finishWithMisses(m Model, ko bool, missed ...string) Model {
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 90, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100, ko: ko, wordsTyped: 20,
		zenMode: true, missedWords: missed,
	})
	m = next.(Model)
	if m.state == stateAchievementUnlock {
		m = runCmd(m, func() tea.Msg { return continueToResultsMsg{} })
	}
	return m
}

// From the results, d drills the words the run missed, three times each.
// The drill earns XP but sets no personal best and counts toward nothing
// else, and its own results offer a drill of what it missed in turn.
func TestDrillFromTheResults(t *testing.T) {
	m, profile := newTestApp(t)
	m = finishWithMisses(m, false, "école", "beta")
	if !containsSubstring(m.results.View(), "d: drill the 2 words you missed") {
		t.Fatalf("expected the drill offered, got:\n%s", m.results.footer())
	}
	successes := profile.SuccessCount

	var cmd tea.Cmd
	m, cmd = pressKey(t, m, "d")
	m = runCmd(m, cmd)
	if m.state != stateTyping {
		t.Fatalf("expected the drill to start, got state %v", m.state)
	}
	drill := m.typing.wordList
	if len(drill) != 12 || m.typing.mode != modeWords || m.typing.target != 12 || !m.typing.zenMode {
		t.Fatalf("expected a 12-word zen drill, got %d words %q, target %d", len(drill), drill, m.typing.target)
	}
	if got := slices.Sorted(slices.Values(drill)); !slices.Equal(got, slices.Concat(slices.Repeat([]string{"beta"}, 6), slices.Repeat([]string{"école"}, 6))) {
		t.Fatalf("expected each missed word six times, got %q", drill)
	}

	for i, w := range drill {
		if i == 0 {
			w = "ecole" // miss the first word, to have something to drill again
		}
		m, _ = pressKey(t, m, w)
		m.typing.tracker.Start = time.Now().Add(-time.Minute) // not scripted-fast
		var next tea.Model
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = next.(Model)
	}
	m = runCmd(m, cmd)
	if m.state == stateAchievementUnlock {
		t.Fatal("expected a drill to unlock no achievement")
	}
	history, _ := storage.ListTestHistory(m.db, profile.ID, 1)
	if len(history) != 1 || history[0].Source != "drill" || history[0].WordsTyped != 12 {
		t.Fatalf("expected the drill in the history, got %+v", history)
	}
	if pb, _ := storage.GetPersonalBest(m.db, profile.ID, "words", 12, "drill"); pb != nil {
		t.Fatalf("expected no personal best from a drill, got %+v", pb)
	}
	if m.results.extra.isNewPB || m.results.extra.xpGained <= 0 {
		t.Fatalf("expected XP and no new best, got %+v", m.results.extra)
	}
	if profile.SuccessCount != successes {
		t.Fatal("expected a drill not to count as an exercise")
	}
	if run := m.Runs()[len(m.Runs())-1]; run.Source != "drill" || !containsSubstring(run.Text(), "drill") {
		t.Fatalf("expected the drill named in what's shared, got %q:\n%s", run.Source, run.Text())
	}
	if !containsSubstring(m.results.footer(), "d: drill the 1 word you missed") {
		t.Fatalf("expected a drill of the word missed in the drill, got:\n%s", m.results.footer())
	}
	if view := newHistoryViewModel(profile, m.db).View(); !containsSubstring(view, "drill") {
		t.Fatalf("expected the history to label the drill, got:\n%s", view)
	}
}

// A run with nothing missed offers no drill, and d does nothing. A
// knockout still offers one; a run that looked scripted doesn't.
func TestDrillOnlyWhenWordsWereMissed(t *testing.T) {
	m, _ := newTestApp(t)
	m = finishWithMisses(m, false)
	if containsSubstring(m.results.footer(), "drill") {
		t.Fatal("expected no drill offered with nothing missed")
	}
	if _, cmd := pressKey(t, m, "d"); cmd != nil {
		t.Fatal("expected d to do nothing with nothing missed")
	}

	m = finishWithMisses(m, true, "gamma")
	if !containsSubstring(m.results.footer(), "d: drill the 1 word you missed") {
		t.Fatal("expected a knockout to offer a drill")
	}

	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 400, Accuracy: 100, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, missedWords: []string{"gamma"},
	})
	m = next.(Model)
	if !m.results.extra.unverified || containsSubstring(m.results.View(), "drill") {
		t.Fatal("expected no drill offered on an unverified run")
	}
}

// Focus-weak loads the weak key pairs as well as the keys, and a run's
// pairs are recorded when it ends.
func TestFocusWeakUsesKeyPairs(t *testing.T) {
	m, profile := newTestApp(t)
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 90, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100,
		charMistakes: map[rune]int{'h': 2}, pairMistakes: map[string]int{"th": 2},
	})
	m = next.(Model)
	m, _ = m.startTyping(startTypingMsg{mode: modeTime, target: 30, focusWeak: true})
	if m.typing.weak.Pairs["th"] != 2 || m.typing.weak.Chars['h'] != 2 {
		t.Fatalf("expected the weak keys and pairs loaded, got %+v", m.typing.weak)
	}
	view := newKeyboardViewModel(profile, m.db).View()
	if !containsSubstring(view, "Weakest key pairs: ") || !containsSubstring(view, "th:2") || !containsSubstring(view, "fade") {
		t.Fatalf("expected the weak pairs and the fading note shown, got:\n%s", view)
	}
}

func TestDrillIsNotAFileSource(t *testing.T) {
	s := textsource.Source{Kind: textsource.Drill}
	if s.Custom() || !s.Practice() || runSource(true, s) != s {
		t.Fatalf("expected a drill to be practice, not a file, and to win over quotes")
	}
}
