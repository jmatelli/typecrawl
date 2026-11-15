package app

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/storage"
)

// RunSummary is one finished run, in the form it leaves the program: as
// the plain-text summary the results screen shares (see Text) and as the
// JSON line --json prints on exit. The JSON field names are documented in
// the README; changing one breaks scripts that read them.
type RunSummary struct {
	Profile         string    `json:"profile"`
	FinishedAt      time.Time `json:"finished_at"`
	Mode            string    `json:"mode"`   // "time" or "words"
	Target          int       `json:"target"` // seconds for "time", words for "words"
	Source          string    `json:"source"` // "random" or "quotes"
	Punctuation     bool      `json:"punctuation"`
	Zen             bool      `json:"zen"`
	WPM             float64   `json:"wpm"`
	Accuracy        float64   `json:"accuracy"` // percent, 0-100
	DurationSeconds float64   `json:"duration_seconds"`
	WordsTyped      int       `json:"words_typed"`
	WordStreak      int       `json:"word_streak"` // longest mistake-free word streak in the run
	Level           int       `json:"level"`       // the profile's level after the run
	Prestige        int       `json:"prestige"`    // the profile's prestige rank
	XPGained        int       `json:"xp_gained"`
	NewPersonalBest bool      `json:"new_personal_best"`
	KO              bool      `json:"ko"`
	// Verified is false for a run that looked scripted (see
	// game.IsSuspiciousRun); it earned nothing and wasn't saved.
	Verified bool `json:"verified"`
}

// round2 keeps two decimals, so the JSON reads 62.43 rather than
// 62.43333333333333.
func round2(f float64) float64 { return math.Round(f*100) / 100 }

func newRunSummary(profile *storage.Profile, msg finishTypingMsg, when time.Time) RunSummary {
	return RunSummary{
		Profile:         profile.Name,
		Prestige:        profile.Prestige,
		FinishedAt:      when.Truncate(time.Second),
		Mode:            modeString(msg.mode),
		Target:          msg.target,
		Source:          sourceString(msg.quotes),
		Punctuation:     msg.punctuation,
		Zen:             msg.zenMode,
		WPM:             round2(msg.result.WPM),
		Accuracy:        round2(msg.result.Accuracy),
		DurationSeconds: round2(msg.result.Duration.Seconds()),
		WordsTyped:      msg.wordsTyped,
		WordStreak:      msg.bestCombo,
		KO:              msg.ko,
		Verified:        true,
	}
}

// Text is the run as a short plain-text summary for pasting elsewhere. It
// has no styling, so it reads the same wherever it lands.
func (s RunSummary) Text() string {
	setup := []string{fmt.Sprintf("%s %d", s.Mode, s.Target)}
	if s.Mode == "time" {
		setup[0] += "s"
	}
	if s.Source == "quotes" {
		setup = append(setup, "quotes")
	} else {
		setup = append(setup, "random words")
		if s.Punctuation {
			setup = append(setup, "punctuation")
		}
	}
	if s.Zen {
		setup = append(setup, "zen")
	}

	outcome := []string{fmt.Sprintf("word streak %d", s.WordStreak), fmt.Sprintf("level %d", s.Level)}
	if s.Prestige > 0 {
		outcome = append(outcome, fmt.Sprintf("prestige %d", s.Prestige))
	}
	switch {
	case !s.Verified:
		outcome = []string{"unverified run"}
	case s.KO:
		outcome = append(outcome, "knocked out")
	case s.NewPersonalBest:
		outcome = append(outcome, "new personal best")
	}

	return fmt.Sprintf("typecrawl · %.1f WPM · %.1f%% accuracy\n%s\n%s",
		s.WPM, s.Accuracy, strings.Join(setup, " · "), strings.Join(outcome, " · "))
}

// shareRunMsg asks for the run on the results screen to be shared.
type shareRunMsg struct{}

// copyToClipboard sends text to the terminal's clipboard with an OSC 52
// escape sequence. That works over SSH, where no local clipboard tool can
// be reached, but many terminals ignore it without saying so; that's why
// shared runs are also printed on exit. GNU screen needs the sequence
// wrapped to pass it on. The sequence goes out in a single write, so it
// can't be split by a frame being drawn at the same time.
func copyToClipboard(w io.Writer, text string) tea.Cmd {
	if w == nil {
		return nil
	}
	seq := osc52.New(text)
	if strings.HasPrefix(os.Getenv("TERM"), "screen") && os.Getenv("TMUX") == "" {
		seq = seq.Screen()
	}
	return func() tea.Msg {
		_, _ = seq.WriteTo(w)
		return nil
	}
}
