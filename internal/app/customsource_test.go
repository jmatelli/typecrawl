package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/challenge"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/storage"
	"github.com/jmatelli/typecrawl/internal/textsource"
)

// writeSource writes content to a file named name in a fresh directory
// and returns it as a source of the given kind.
func writeSource(t *testing.T, kind textsource.Kind, name, content string) textsource.Source {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return textsource.Source{Kind: kind, Path: path}
}

// bigRun is a run that, on the built-in words, completes several
// achievements at once: a first exercise, a perfect run, a 15-second
// sprint, a long word streak.
func bigRun(custom textsource.Source) finishTypingMsg {
	return finishTypingMsg{
		result: stats.Result{WPM: 120, Accuracy: 100, Duration: 15 * time.Second},
		mode:   modeTime, target: 15, hp: 100, maxHP: 100, wordsTyped: 150,
		bestCombo: 150, punctuation: true, zenMode: true, custom: custom,
	}
}

func TestCustomRunsEarnXPButNoAchievementsOrChallenges(t *testing.T) {
	list := writeSource(t, textsource.WordList, "français.txt", "a")

	// The same run on the built-in words does unlock achievements, so the
	// check below isn't passing just because nothing could unlock.
	builtin, _ := newTestApp(t)
	next, _ := builtin.update(bigRun(textsource.Source{}))
	if next.(Model).state != stateAchievementUnlock {
		t.Fatal("expected the built-in run to unlock achievements")
	}

	m, profile := newTestApp(t)
	seedWeek(t, m, profile)
	before := buildAchievementProgress(m.db, profile, 0, 0)
	next, _ = m.update(bigRun(list))
	m = next.(Model)

	if m.state != stateResults {
		t.Fatalf("expected the results screen with no achievement popup, got state %v", m.state)
	}
	after := buildAchievementProgress(m.db, profile, 0, 0)
	if unlocked := newlyUnlocked(before, after); len(unlocked) > 0 {
		t.Fatalf("expected no achievement from a custom run, got %v", unlocked[0].ID)
	}
	extra := m.results.extra
	if extra.xpGained <= 0 || profile.XP <= 0 {
		t.Fatalf("expected the custom run to earn XP, got %d", extra.xpGained)
	}
	if extra.challengeBonusXP != 0 || extra.weeklyBonusXP != 0 {
		t.Fatalf("expected no challenge bonus, got daily %d weekly %d", extra.challengeBonusXP, extra.weeklyBonusXP)
	}
	if claimed, _ := storage.HasClaimedWeeklyChallenge(m.db, profile.ID, challenge.WeekKey(time.Now())); claimed {
		t.Fatal("expected no weekly claim from a custom run")
	}
	if claimed, _ := storage.HasClaimedDailyChallenge(m.db, profile.ID, challenge.Day(time.Now())); claimed {
		t.Fatal("expected no daily claim from a custom run")
	}
	if profile.SuccessCount != 0 || profile.BestWordStreak != 0 || profile.KOCount != 0 {
		t.Fatalf("expected the profile's counters untouched, got %+v", profile)
	}
	if !extra.isNewPB {
		t.Fatal("expected the custom run to set its own first personal best")
	}
	if pb, _ := storage.GetPersonalBest(m.db, profile.ID, "time", 15, list.Key()); pb == nil || pb.WPM != 120 {
		t.Fatalf("expected the personal best under the word list's key, got %+v", pb)
	}
	if pb, _ := storage.GetPersonalBest(m.db, profile.ID, "time", 15, "random"); pb != nil {
		t.Fatal("expected no random-words personal best from a custom run")
	}

	// A knockout on a custom source doesn't count as a knockout either.
	ko := bigRun(list)
	ko.ko = true
	next, _ = m.update(ko)
	if m = next.(Model); profile.KOCount != 0 {
		t.Fatalf("expected the KO count untouched, got %d", profile.KOCount)
	}
}

// Words with accents, Cyrillic and CJK are typed through to a perfect run,
// and every wrapped line fits the screen.
func TestCustomWordListTypedEndToEnd(t *testing.T) {
	list := writeSource(t, textsource.WordList, "mixed.txt", "école\nпривет\n日本語\nnaïve\n")
	m, profile := newTestApp(t)
	m, _ = m.startTyping(startTypingMsg{mode: modeWords, target: 12, custom: list})
	if m.state != stateTyping {
		t.Fatalf("expected the run to start, got state %v (%s)", m.state, m.menu.errMsg)
	}
	for _, w := range m.typing.wordList {
		if !strings.Contains("école привет 日本語 naïve", w) {
			t.Fatalf("expected only words from the list, got %q", w)
		}
	}
	for _, width := range []int{8, 13, 30} {
		lines, _ := wrapWords(m.typing.wordList, 0, width, 100)
		for _, line := range lines {
			parts := make([]string, len(line))
			for i, idx := range line {
				parts[i] = m.typing.wordList[idx]
			}
			if w := lipgloss.Width(strings.Join(parts, " ")); w > width {
				t.Fatalf("a line is %d columns wide on a %d-column screen: %q", w, width, parts)
			}
		}
	}

	var cmd tea.Cmd
	for i := range 12 {
		word := m.typing.wordList[i]
		m, _ = pressKey(t, m, word)
		// Typed instantly, the run would look scripted and not be saved.
		m.typing.tracker.Start = time.Now().Add(-time.Minute)
		var next tea.Model
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m = next.(Model)
	}
	m = runCmd(m, cmd)
	if m.state != stateResults && m.state != stateAchievementUnlock {
		t.Fatalf("expected the run to finish, got state %v", m.state)
	}
	history, _ := storage.ListTestHistory(m.db, profile.ID, 1)
	if len(history) != 1 || history[0].Accuracy != 100 || history[0].Source != list.Key() || history[0].WordsTyped != 12 {
		t.Fatalf("expected a perfect 12-word run from the word list, got %+v", history)
	}
}

// A text is typed in order, and a time run longer than the text carries
// on from the start again without skipping or restarting mid-way.
func TestCustomTextIsTypedInOrder(t *testing.T) {
	text := writeSource(t, textsource.Text, "book.txt", "Call me Ishmael. Some years ago, never mind how long precisely.")
	want := strings.Fields("Call me Ishmael. Some years ago, never mind how long precisely.")
	m, _ := newTestApp(t)
	m, _ = m.startTyping(startTypingMsg{mode: modeTime, target: 60, custom: text, focusWeak: true})
	if !m.typing.punctuation || m.typing.focusWeak {
		t.Fatal("expected a text to count as punctuation and to ignore weak-key focus")
	}
	m.typing.wordIndex = len(m.typing.wordList) - 1
	m.typing.ensureWords()
	if len(m.typing.wordList) <= 300 {
		t.Fatal("expected the lookahead to be topped up")
	}
	for i, w := range m.typing.wordList {
		if w != want[i%len(want)] {
			t.Fatalf("word %d is %q, want %q", i, w, want[i%len(want)])
		}
	}
}

func TestMissingSavedFileStopsTheStart(t *testing.T) {
	m, _ := newTestApp(t)
	m.menu = newMenuModel(m.profile, m.db)
	gone := textsource.Source{Kind: textsource.WordList, Path: filepath.Join(t.TempDir(), "gone.txt")}
	next, _ := m.update(startTypingMsg{mode: modeTime, target: 30, custom: gone})
	m = next.(Model)
	if m.state != stateMenu || !containsSubstring(m.menu.View(), "Couldn't read word list gone.txt") {
		t.Fatalf("expected the menu with an error, got state %v:\n%s", m.state, m.menu.View())
	}
	m.menu = press(t, m.menu, 'm')
	if containsSubstring(m.menu.View(), "Couldn't read") {
		t.Fatal("expected the next key to clear the error")
	}
}

func TestMenuCyclesThroughSavedSources(t *testing.T) {
	m := newTestMenuModel(t)
	list := writeSource(t, textsource.WordList, "french.txt", "bonjour")
	text := writeSource(t, textsource.Text, "book.txt", "Once upon a time")
	for i, s := range []textsource.Source{list, text} {
		if err := storage.SaveCustomSource(m.db, m.profile.ID, s.Key(), time.Now().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	m = newMenuModel(m.profile, m.db)

	var seen []string
	for range 5 {
		seen = append(seen, m.source().Name())
		m = press(t, m, 's')
	}
	want := []string{"Random words", "Quotes", "word list french.txt", "text book.txt", "Random words"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Fatalf("expected the cycle %q, got %q", want, seen)
	}

	// The loop left it on quotes. Select the text and come back to the
	// menu: it's remembered.
	m = press(t, m, 's')
	m = press(t, m, 's')
	m = newMenuModel(m.profile, m.db)
	if m.source() != text {
		t.Fatalf("expected the text still selected, got %+v", m.source())
	}
	view := m.View()
	if !containsSubstring(view, "Source: text book.txt (s, x: forget)") || !containsSubstring(view, "no achievements or challenges") {
		t.Fatalf("expected the selected text and its note shown, got:\n%s", view)
	}
	if containsSubstring(view, filepath.Dir(text.Path)) {
		t.Fatal("expected the menu to show the file name, not its directory")
	}
	// A text is typed as written: punctuation is on and focus is off.
	m = press(t, m, 'c')
	m = press(t, m, 'f')
	if m.punctuation || m.focusWeak || !containsSubstring(m.View(), "Focus weak keys: N/A") {
		t.Fatal("expected c and f to do nothing while a text is selected")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if start := cmd().(startTypingMsg); start.custom != text {
		t.Fatalf("expected enter to start the text, got %+v", start.custom)
	}

	// x forgets it, back to the random words; the word list stays.
	m = press(t, m, 'x')
	if m.source().Kind != textsource.Random {
		t.Fatalf("expected the random words after forgetting, got %+v", m.source())
	}
	saved, _ := storage.ListCustomSources(m.db, m.profile.ID)
	if len(saved) != 1 || saved[0] != list.Key() {
		t.Fatalf("expected only the word list saved, got %q", saved)
	}
	if settings, _ := storage.LoadSettings(m.db, m.profile.ID); settings.CustomSource != "" {
		t.Fatalf("expected no source selected, got %q", settings.CustomSource)
	}
	// x does nothing on a built-in source.
	m = press(t, m, 'x')
	if saved, _ := storage.ListCustomSources(m.db, m.profile.ID); len(saved) != 1 {
		t.Fatal("expected x on the random words to forget nothing")
	}
}

// The menu's personal best line reads the same key a run writes.
func TestMenuShowsTheSourcesOwnBest(t *testing.T) {
	m := newTestMenuModel(t)
	list := writeSource(t, textsource.WordList, "french.txt", "bonjour")
	if err := storage.SaveCustomSource(m.db, m.profile.ID, list.Key(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.RecordPersonalBest(m.db, m.profile.ID, "time", 30, list.Key(), 77.7, 99, nil, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	m = newMenuModel(m.profile, m.db)
	if containsSubstring(m.View(), "77.7") {
		t.Fatal("expected the word list's best not shown for the random words")
	}
	m = press(t, m, 's')
	m = press(t, m, 's')
	if !containsSubstring(m.View(), "Your best: 77.7 WPM") {
		t.Fatalf("expected the word list's own best, got:\n%s", m.View())
	}
}

func TestQuickLaunchSavesTheSource(t *testing.T) {
	m, profile := newTestApp(t)
	list := writeSource(t, textsource.WordList, "french.txt", "bonjour")
	launched, err := NewWithOptions(m.db, []*storage.Profile{profile}, LaunchOptions{
		ProfileName: profile.Name, Mode: "time", Target: 30, Custom: list,
	})
	if err != nil {
		t.Fatal(err)
	}
	if launched.pendingAutoStart == nil || launched.pendingAutoStart.custom != list {
		t.Fatalf("expected the quick launch to start the word list, got %+v", launched.pendingAutoStart)
	}
	if saved, _ := storage.ListCustomSources(m.db, profile.ID); len(saved) != 1 || saved[0] != list.Key() {
		t.Fatalf("expected the word list saved to the profile, got %q", saved)
	}
}

// What leaves the program -- the shared text and --json -- names the
// file but never the directories leading to it.
func TestSharedCustomRunShowsOnlyTheFileName(t *testing.T) {
	list := writeSource(t, textsource.WordList, "français.txt", "a")
	m, _ := newTestApp(t)
	next, _ := m.update(bigRun(list))
	m = next.(Model)
	run := m.Runs()[0]
	if run.Source != "wordlist" || run.SourceName != "français.txt" {
		t.Fatalf("expected source wordlist and name français.txt, got %q %q", run.Source, run.SourceName)
	}
	buf, _ := json.Marshal(run)
	dir := filepath.Dir(list.Path)
	for name, out := range map[string]string{"text": run.Text(), "json": string(buf)} {
		if strings.Contains(out, dir) || strings.Contains(out, "private") {
			t.Fatalf("expected the %s to leave out the directory, got %s", name, out)
		}
	}
	if !strings.Contains(run.Text(), "word list français.txt · punctuation") {
		t.Fatalf("expected the shared text to name the word list, got:\n%s", run.Text())
	}
}

func TestHistoryShowsEachRunsSource(t *testing.T) {
	m, profile := newTestApp(t)
	now := time.Now()
	for i, source := range []string{"random", "quotes", "text:/home/me/private/book.txt"} {
		if err := storage.RecordTestResult(m.db, profile.ID, 50, 99, "time", 30, false, false, false, source, 20, 30*time.Second, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	view := newHistoryViewModel(profile, m.db).View()
	if !containsSubstring(view, "book.txt") || !containsSubstring(view, "quotes") || containsSubstring(view, "private") {
		t.Fatalf("expected the sources shown by name, got:\n%s", view)
	}
}
