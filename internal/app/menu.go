package app

import (
	"database/sql"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/challenge"
	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/storage"
	"github.com/jmatelli/typecrawl/internal/words"
)

type testMode int

const (
	modeTime testMode = iota
	modeWords
)

// modeString converts a testMode to the string form persisted in the DB
// (settings, personal bests, test history).
func modeString(m testMode) string {
	if m == modeWords {
		return "words"
	}
	return "time"
}

// sourceString converts the quotes-mode toggle to the string form persisted
// in the DB (personal bests, test history) -- "random" and "quotes" are
// tracked as separate personal bests at the same mode/target, since typing
// fixed quote text is a meaningfully different challenge from random words.
func sourceString(quotes bool) string {
	if quotes {
		return "quotes"
	}
	return "random"
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	helpStyle    = lipgloss.NewStyle().Faint(true)
	boldStyle    = lipgloss.NewStyle().Bold(true)
	levelUpStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

type menuModel struct {
	db          *sql.DB
	profile     *storage.Profile
	streakDays  int
	mode        testMode
	timeOpts    []int
	wordOpts    []int
	timeIdx     int
	wordIdx     int
	punctuation bool
	zenMode     bool
	focusWeak   bool
	quotes      bool // draws exercise text from cached quotes instead of random words
}

// indexOf finds val in opts, defaulting to 0 if it's not there -- e.g. a
// value saved before the option list changed.
func indexOf(opts []int, val int) int {
	for i, v := range opts {
		if v == val {
			return i
		}
	}
	return 0
}

func newMenuModel(profile *storage.Profile, db *sql.DB) menuModel {
	timeOpts := []int{15, 30, 60, 120}
	wordOpts := []int{50, 100, 200}

	settings, _ := storage.LoadSettings(db, profile.ID) // best-effort; zero value still yields sane defaults
	streakDays, _ := storage.CurrentStreak(db, profile.ID, time.Now())

	m := menuModel{
		db:          db,
		profile:     profile,
		streakDays:  streakDays,
		mode:        modeTime,
		timeOpts:    timeOpts,
		wordOpts:    wordOpts,
		timeIdx:     indexOf(timeOpts, settings.Duration),
		wordIdx:     indexOf(wordOpts, settings.WordCount),
		punctuation: settings.Punctuation,
		zenMode:     settings.ZenMode,
		focusWeak:   settings.FocusWeak,
		quotes:      settings.Quotes,
	}
	if settings.Mode == "words" {
		m.mode = modeWords
	}
	return m
}

// saveSettings persists the current menu choices for this profile so they
// survive into the next exercise and the next session.
func (m menuModel) saveSettings() {
	_ = storage.SaveSettings(m.db, m.profile.ID, storage.Settings{
		Mode:        modeString(m.mode),
		Duration:    m.timeOpts[m.timeIdx],
		WordCount:   m.wordOpts[m.wordIdx],
		Punctuation: m.punctuation,
		ZenMode:     m.zenMode,
		FocusWeak:   m.focusWeak,
		Quotes:      m.quotes,
	})
}

// currentTarget returns the duration or word count currently selected,
// depending on mode.
func (m menuModel) currentTarget() int {
	if m.mode == modeWords {
		return m.wordOpts[m.wordIdx]
	}
	return m.timeOpts[m.timeIdx]
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "tab", "m":
		if m.mode == modeTime {
			m.mode = modeWords
		} else {
			m.mode = modeTime
		}
		m.saveSettings()
	case "left", "h":
		if m.mode == modeTime && m.timeIdx > 0 {
			m.timeIdx--
		} else if m.mode == modeWords && m.wordIdx > 0 {
			m.wordIdx--
		}
		m.saveSettings()
	case "right", "l":
		if m.mode == modeTime && m.timeIdx < len(m.timeOpts)-1 {
			m.timeIdx++
		} else if m.mode == modeWords && m.wordIdx < len(m.wordOpts)-1 {
			m.wordIdx++
		}
		m.saveSettings()
	case "enter":
		mode := m.mode
		target := m.currentTarget()
		punctuation := m.punctuation
		zenMode := m.zenMode
		focusWeak := m.focusWeak
		quotes := m.quotes
		return m, func() tea.Msg {
			return startTypingMsg{mode: mode, target: target, punctuation: punctuation, zenMode: zenMode, focusWeak: focusWeak, quotes: quotes}
		}
	case "c":
		if !m.quotes { // punctuation is implied whenever quotes mode is on
			m.punctuation = !m.punctuation
			m.saveSettings()
		}
	case "z":
		m.zenMode = !m.zenMode
		m.saveSettings()
	case "f":
		if !m.quotes { // weak-key biasing has no meaning over fixed quote text
			m.focusWeak = !m.focusWeak
			m.saveSettings()
		}
	case "s":
		// m.punctuation itself is deliberately left untouched here -- it's
		// the player's own preference, independent of quotes mode. Display
		// and launch behavior already treat punctuation as on whenever
		// quotes is on (see the View below and newTypingModel), so forcing
		// it into m.punctuation too would just leave it stuck "on" after
		// quotes mode is switched back off again.
		m.quotes = !m.quotes
		m.saveSettings()
	case "p":
		return m, func() tea.Msg { return viewProfileMsg{} }
	case "u":
		return m, func() tea.Msg { return switchProfileMsg{} }
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m menuModel) View() string {
	title := titleStyle.Render("Typecrawl")
	profileLine := fmt.Sprintf(
		"%s%s   Level %d   XP %d/%d   HP %s   Difficulty: %s",
		boldStyle.Render(m.profile.Name), titleSuffix(m.profile.EquippedTitle),
		m.profile.Level,
		m.profile.XP,
		game.XPToNextLevel(m.profile.Level),
		renderHPBar(game.MaxHP(m.profile.Level), game.MaxHP(m.profile.Level)),
		boldStyle.Render(words.TierName(words.Tier(m.profile.Level))),
	)
	streakLine := fmt.Sprintf(
		"Daily streak: %s   (current XP multiplier: %s)",
		streakStyle.Render(fmt.Sprintf("%d day(s)", m.streakDays)),
		boldStyle.Render(fmt.Sprintf("x%.2f", game.DailyStreakMultiplier(m.streakDays))),
	)

	today := challenge.Today(time.Now())
	var challengeLine string
	if claimed, _ := storage.HasClaimedDailyChallenge(m.db, m.profile.ID, challenge.Day(time.Now())); claimed {
		challengeLine = dimStyle.Render(fmt.Sprintf("Daily challenge (complete): %s", today.Description))
	} else {
		challengeLine = fmt.Sprintf("Daily challenge: %s   %s", today.Description, streakStyle.Render(fmt.Sprintf("+%d XP", today.XPBonus)))
	}

	modeStr := "Time"
	if m.mode == modeWords {
		modeStr = "Words"
	}
	modeLine := fmt.Sprintf("Mode: %s   (tab to switch)", boldStyle.Render(modeStr))

	var targetLine string
	if m.mode == modeTime {
		targetLine = fmt.Sprintf("Duration: %s   (←/→ or h/l to change)", boldStyle.Render(fmt.Sprintf("%ds", m.timeOpts[m.timeIdx])))
	} else {
		targetLine = fmt.Sprintf("Word count: %s   (←/→ or h/l to change)", boldStyle.Render(fmt.Sprintf("%d", m.wordOpts[m.wordIdx])))
	}

	pbLine := "Your best: " + dimStyle.Render("no record yet for this mode/duration")
	if pb, err := storage.GetPersonalBest(m.db, m.profile.ID, modeString(m.mode), m.currentTarget(), sourceString(m.quotes)); err == nil && pb != nil {
		pbLine = fmt.Sprintf("Your best: %s WPM (%s accuracy, %s)",
			boldStyle.Render(fmt.Sprintf("%.1f", pb.WPM)),
			boldStyle.Render(fmt.Sprintf("%.0f%%", pb.Accuracy)),
			pb.AchievedAt.Format("2006-01-02"),
		)
	}

	sourceStr := "Random words"
	if m.quotes {
		sourceStr = "Quotes"
	}
	sourceLine := fmt.Sprintf("Source: %s (s)", boldStyle.Render(sourceStr))

	punctStr, zenStr, focusStr := "Off", "Off", "Off"
	if m.punctuation || m.quotes {
		punctStr = "On"
	}
	if m.zenMode {
		zenStr = "On"
	}
	focusHint := " (f)"
	if m.quotes {
		focusStr = "N/A"
		focusHint = ""
	} else if m.focusWeak {
		focusStr = "On"
	}
	togglesLine := fmt.Sprintf(
		"Punctuation: %s (c)   Zen mode: %s (z)   Focus weak keys: %s%s",
		boldStyle.Render(punctStr), boldStyle.Render(zenStr), boldStyle.Render(focusStr), focusHint,
	)

	help := helpStyle.Render("enter: start   p: profile   u: switch profile   q: quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title, profileLine, streakLine, challengeLine, "",
		modeLine, targetLine, pbLine, "",
		sourceLine, togglesLine, "",
		help,
	)
}
