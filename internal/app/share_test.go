package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/stats"
)

var sampleRun = RunSummary{
	Profile:         "alice",
	FinishedAt:      time.Date(2026, 11, 15, 18, 4, 5, 0, time.UTC),
	Mode:            "time",
	Target:          30,
	Source:          "random",
	Punctuation:     true,
	WPM:             62.43,
	Accuracy:        97.1,
	DurationSeconds: 30,
	WordsTyped:      31,
	WordStreak:      14,
	Level:           7,
	XPGained:        58,
	NewPersonalBest: true,
	Verified:        true,
}

// sampleRunJSON is the example line in the README's --json section. If
// this test fails because the format changed on purpose, update both.
const sampleRunJSON = `{"profile":"alice","finished_at":"2026-11-15T18:04:05Z","mode":"time","target":30,"source":"random","punctuation":true,"zen":false,"wpm":62.43,"accuracy":97.1,"duration_seconds":30,"words_typed":31,"word_streak":14,"level":7,"prestige":0,"xp_gained":58,"new_personal_best":true,"ko":false,"verified":true}`

func TestRunSummaryJSONMatchesREADME(t *testing.T) {
	got, err := json.Marshal(sampleRun)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sampleRunJSON {
		t.Fatalf("JSON changed:\n got %s\nwant %s", got, sampleRunJSON)
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), sampleRunJSON) {
		t.Fatal("README.md's --json example no longer matches the JSON format")
	}
}

func TestRunSummaryText(t *testing.T) {
	cases := []struct {
		name string
		edit func(*RunSummary)
		want string
	}{
		{"personal best", func(*RunSummary) {},
			"typecrawl · 62.4 WPM · 97.1% accuracy\ntime 30s · random words · punctuation\nword streak 14 · level 7 · new personal best"},
		{"words mode with quotes and zen", func(s *RunSummary) {
			s.Mode, s.Target, s.Source, s.Zen, s.NewPersonalBest = "words", 50, "quotes", true, false
		}, "typecrawl · 62.4 WPM · 97.1% accuracy\nwords 50 · quotes · zen\nword streak 14 · level 7"},
		{"knockout", func(s *RunSummary) { s.KO, s.NewPersonalBest = true, false },
			"typecrawl · 62.4 WPM · 97.1% accuracy\ntime 30s · random words · punctuation\nword streak 14 · level 7 · knocked out"},
		{"prestiged", func(s *RunSummary) { s.Prestige, s.NewPersonalBest = 2, false },
			"typecrawl · 62.4 WPM · 97.1% accuracy\ntime 30s · random words · punctuation\nword streak 14 · level 7 · prestige 2"},
		{"unverified", func(s *RunSummary) { s.Verified = false },
			"typecrawl · 62.4 WPM · 97.1% accuracy\ntime 30s · random words · punctuation\nunverified run"},
	}
	for _, c := range cases {
		s := sampleRun
		c.edit(&s)
		got := s.Text()
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		if strings.ContainsRune(got, '\x1b') {
			t.Errorf("%s: the text summary must have no escape sequences: %q", c.name, got)
		}
	}
}

func TestFinishedRunsAreRecordedForTheSession(t *testing.T) {
	m, profile := newTestApp(t)

	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 55.555, Accuracy: 96.666, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100, bestCombo: 9, wordsTyped: 28,
	})
	m = next.(Model)
	next, _ = m.update(finishTypingMsg{
		result: stats.Result{WPM: 300, Accuracy: 100, KeystrokeCV: 0.5, KeystrokeSamples: 200},
		mode:   modeWords, target: 50, quotes: true, hp: 100, maxHP: 100,
	})
	m = next.(Model)

	runs := m.Runs()
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	first := runs[0]
	if !first.Verified || first.Profile != profile.Name || first.Mode != "time" || first.Target != 30 ||
		first.WPM != 55.56 || first.Accuracy != 96.67 || first.WordStreak != 9 || first.WordsTyped != 28 ||
		first.XPGained <= 0 || first.Level != m.profile.Level || first.FinishedAt.IsZero() {
		t.Errorf("unexpected first run: %+v", first)
	}
	second := runs[1]
	if second.Verified || second.XPGained != 0 || second.Source != "quotes" || second.Mode != "words" {
		t.Errorf("expected the scripted-looking run marked unverified with no XP, got %+v", second)
	}
}

func TestSharingCopiesTheLatestRunOnce(t *testing.T) {
	// Under GNU screen the sequence is wrapped differently (see
	// copyToClipboard); pin a plain terminal so the check below holds.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TMUX", "")
	m, _ := newTestApp(t)
	var clip bytes.Buffer
	m.clipboard = &clip

	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 60, Accuracy: 98, Duration: 30 * time.Second},
		mode:   modeTime, target: 30, hp: 100, maxHP: 100,
	})
	m = next.(Model)
	m.state = stateResults // skip any achievement-unlock screen

	// The share key on the results screen asks the app to share.
	_, cmd := m.results.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected s to return a command")
	}
	if _, ok := cmd().(shareRunMsg); !ok {
		t.Fatal("expected s to send shareRunMsg")
	}

	for range 2 {
		next, cmd = m.update(shareRunMsg{})
		m = next.(Model)
		if cmd == nil {
			t.Fatal("expected a clipboard command")
		}
		cmd()
	}

	text := m.Runs()[0].Text()
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text))
	if !strings.Contains(clip.String(), want) {
		t.Fatalf("expected an OSC 52 sequence carrying the summary, got %q", clip.String())
	}
	if shared := m.SharedRuns(); len(shared) != 1 {
		t.Fatalf("expected the run listed once however often it's shared, got %d", len(shared))
	}
}

func TestSharingWithNoRunsDoesNothing(t *testing.T) {
	m, _ := newTestApp(t)
	next, cmd := m.update(shareRunMsg{})
	if cmd != nil || len(next.(Model).SharedRuns()) != 0 {
		t.Fatal("expected sharing with no finished run to do nothing")
	}
}

func TestResultsFooterMentionsShare(t *testing.T) {
	r := newResultsModel(stats.Result{WPM: 60, Accuracy: 98}, 80, resultsExtra{})
	if !containsSubstring(r.View(), "s: share") {
		t.Fatal("expected the help line to list s: share")
	}
	unverified := newResultsModel(stats.Result{WPM: 300}, 80, resultsExtra{unverified: true})
	if !containsSubstring(unverified.View(), "s: share") {
		t.Fatal("expected the unverified view's help line to list s: share")
	}

	next, _ := r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !containsSubstring(next.(resultsModel).View(), "printed when you quit") {
		t.Fatal("expected a note saying where the summary went")
	}
}
