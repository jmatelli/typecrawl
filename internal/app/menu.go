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
	"github.com/jmatelli/typecrawl/internal/textsource"
	"github.com/jmatelli/typecrawl/internal/theme"
	"github.com/jmatelli/typecrawl/internal/words"
)

type testMode int

const (
	modeTime testMode = iota
	modeWords
	// modeCrawl is crawl mode: floors of words, HP carried between them.
	// It isn't saved as the menu's mode (older versions wouldn't know
	// it); the menu comes back to it after a crawl instead.
	modeCrawl
)

// modeString converts a testMode to the string form persisted in the DB
// (settings, personal bests, test history). A crawl floor is a words
// run.
func modeString(m testMode) string {
	if m == modeWords || m == modeCrawl {
		return "words"
	}
	return "time"
}

// runSource is the source a run with these choices types from: the
// player's own file or a drill when one is set, otherwise quotes or
// random words.
// Every place that stores or looks up a run's source goes through it, so
// a personal best is always written and read under the same key.
func runSource(quotes bool, custom textsource.Source) textsource.Source {
	if custom.Practice() {
		return custom
	}
	return textsource.Builtin(quotes)
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(theme.Color("6"))
	helpStyle    = lipgloss.NewStyle().Faint(true)
	boldStyle    = lipgloss.NewStyle().Bold(true)
	levelUpStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Color("11"))
)

type menuModel struct {
	db         *sql.DB
	profile    *storage.Profile
	streakDays int
	// weekly is this week's challenge, loaded with the menu (which is
	// rebuilt after every run) rather than on every redraw.
	weekly         challenge.Weekly
	weeklyProgress int
	weeklyClaimed  bool
	mode           testMode
	timeOpts       []int
	wordOpts       []int
	timeIdx        int
	wordIdx        int
	punctuation    bool
	zenMode        bool
	focusWeak      bool
	quotes         bool // draws exercise text from cached quotes instead of random words
	// customSources are the profile's saved word lists and texts, and
	// customIdx the one selected, or -1 when the source is quotes or the
	// random words.
	customSources []textsource.Source
	customIdx     int
	battle        bool // shows the battle strip below the words
	// errMsg explains why the last start didn't happen, such as a saved
	// file that has since been moved. Any key clears it.
	errMsg string
	// keyboardLayout isn't shown or changed here (the Weak Keys screen does
	// that), but it's carried so saveSettings doesn't reset it.
	keyboardLayout string
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
	saved, _ := storage.ListCustomSources(db, profile.ID)
	customIdx := -1
	var customSources []textsource.Source
	for _, key := range saved {
		src, ok := textsource.Parse(key)
		if !ok || !src.Custom() {
			continue
		}
		if key == settings.CustomSource {
			customIdx = len(customSources)
		}
		customSources = append(customSources, src)
	}
	streakDays, _ := storage.CurrentStreak(db, profile.ID, time.Now())
	weekly, week, weeklyClaimed := weeklyStatus(db, profile.ID, time.Now())

	m := menuModel{
		db: db,

		weekly:         weekly,
		weeklyProgress: weekly.Progress(week),
		weeklyClaimed:  weeklyClaimed,
		profile:        profile,
		streakDays:     streakDays,
		mode:           modeTime,
		timeOpts:       timeOpts,
		wordOpts:       wordOpts,
		timeIdx:        indexOf(timeOpts, settings.Duration),
		wordIdx:        indexOf(wordOpts, settings.WordCount),
		punctuation:    settings.Punctuation,
		zenMode:        settings.ZenMode,
		focusWeak:      settings.FocusWeak,
		quotes:         settings.Quotes,
		customSources:  customSources,
		customIdx:      customIdx,
		battle:         !settings.HideBattleStrip,

		keyboardLayout: settings.KeyboardLayout,
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

		HideBattleStrip: !m.battle,
		KeyboardLayout:  m.keyboardLayout,
		CustomSource:    m.customKey(),
	})
}

// custom is the selected saved source, or the zero Source when it's
// quotes or the random words.
func (m menuModel) custom() textsource.Source {
	if m.customIdx < 0 || m.customIdx >= len(m.customSources) {
		return textsource.Source{}
	}
	return m.customSources[m.customIdx]
}

func (m menuModel) customKey() string {
	if c := m.custom(); c.Custom() {
		return c.Key()
	}
	return ""
}

// source is what the next run types from.
func (m menuModel) source() textsource.Source {
	return runSource(m.quotes, m.custom())
}

// fixedText reports whether the source is text typed as written (quotes
// or a player's text): punctuation is part of it, and there's nothing for
// weak-key focus to pick.
func (m menuModel) fixedText() bool {
	k := m.source().Kind
	return k == textsource.Quotes || k == textsource.Text
}

// nextSource moves to the next source in the order random words, quotes,
// then each saved file, and round again.
func (m *menuModel) nextSource() {
	switch {
	case m.customIdx >= 0:
		m.customIdx++
		if m.customIdx >= len(m.customSources) {
			m.customIdx = -1
			m.quotes = false
		}
	case m.quotes && len(m.customSources) > 0:
		m.customIdx = 0
	default:
		m.quotes = !m.quotes
	}
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
	m.errMsg = ""

	switch keyMsg.String() {
	case "tab", "m":
		m.mode = (m.mode + 1) % (modeCrawl + 1)
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
		if m.mode == modeCrawl {
			punctuation := m.punctuation
			return m, func() tea.Msg { return startTypingMsg{crawl: true, punctuation: punctuation} }
		}
		mode := m.mode
		target := m.currentTarget()
		punctuation := m.punctuation
		zenMode := m.zenMode
		focusWeak := m.focusWeak && !m.fixedText()
		quotes := m.quotes
		custom := m.custom()
		return m, func() tea.Msg {
			return startTypingMsg{mode: mode, target: target, punctuation: punctuation, zenMode: zenMode, focusWeak: focusWeak, quotes: quotes, custom: custom}
		}
	case "c":
		if !m.fixedText() || m.mode == modeCrawl { // punctuation is implied whenever the text is typed as written
			m.punctuation = !m.punctuation
			m.saveSettings()
		}
	case "z":
		if m.mode != modeCrawl { // no zen in a crawl: the HP is the point
			m.zenMode = !m.zenMode
			m.saveSettings()
		}
	case "f":
		if !m.fixedText() && m.mode != modeCrawl { // weak-key biasing has no meaning over fixed text
			m.focusWeak = !m.focusWeak
			m.saveSettings()
		}
	case "s":
		if m.mode == modeCrawl {
			break // a crawl is always the built-in words
		}
		// m.punctuation itself is deliberately left untouched here -- it's
		// the player's own preference, independent of quotes mode. Display
		// and launch behavior already treat punctuation as on whenever
		// quotes is on (see the View below and newTypingModel), so forcing
		// it into m.punctuation too would just leave it stuck "on" after
		// quotes mode is switched back off again.
		m.nextSource()
		m.saveSettings()
	case "x":
		// Forget the selected saved file. Its runs and personal bests stay.
		if c := m.custom(); c.Custom() && m.mode != modeCrawl {
			_ = storage.ForgetCustomSource(m.db, m.profile.ID, c.Key())
			m.customSources = append(m.customSources[:m.customIdx:m.customIdx], m.customSources[m.customIdx+1:]...)
			m.customIdx = -1
			m.quotes = false
			m.saveSettings()
		}
	case "b":
		m.battle = !m.battle
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
		"%s%s   Level %d%s   XP %d/%d   HP %s   Difficulty: %s",
		boldStyle.Render(m.profile.Name), titleSuffix(m.profile.EquippedTitle),
		m.profile.Level, prestigeSuffix(m.profile.Prestige),
		m.profile.XP,
		game.XPToNextLevel(m.profile.Level),
		renderHPBar(game.MaxHP(m.profile.Level, m.profile.Prestige), game.MaxHP(m.profile.Level, m.profile.Prestige)),
		boldStyle.Render(words.TierName(words.Tier(game.DifficultyLevel(m.profile.Level, m.profile.Prestige)))),
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
		challengeLine = fmt.Sprintf("Daily challenge (%s): %s   %s", today.Difficulty, today.Description, streakStyle.Render(fmt.Sprintf("+%d XP", today.XPBonus)))
	}
	challengeLine = lipgloss.JoinVertical(lipgloss.Left, challengeLine, m.weeklyLine())

	modeStr := "Time"
	switch m.mode {
	case modeWords:
		modeStr = "Words"
	case modeCrawl:
		modeStr = "Crawl"
	}
	modeLine := fmt.Sprintf("Mode: %s   (tab to switch)", boldStyle.Render(modeStr))

	var targetLine string
	switch m.mode {
	case modeCrawl:
		targetLine = fmt.Sprintf("Floors: %d words, %d more on each; harder words every %d floors, a boss every %d",
			game.CrawlFirstFloorWords, game.CrawlWordsPerFloor, game.CrawlFloorsPerTier, game.CrawlBossEvery)
	case modeTime:
		targetLine = fmt.Sprintf("Duration: %s   (←/→ or h/l to change)", boldStyle.Render(fmt.Sprintf("%ds", m.timeOpts[m.timeIdx])))
	default:
		targetLine = fmt.Sprintf("Word count: %s   (←/→ or h/l to change)", boldStyle.Render(fmt.Sprintf("%d", m.wordOpts[m.wordIdx])))
	}

	pbLine := "Your best: " + dimStyle.Render("no record yet for this mode/duration")
	if m.mode == modeCrawl {
		pbLine = deepestFloorLine(m.profile.DeepestFloor)
	} else if pb, err := storage.GetPersonalBest(m.db, m.profile.ID, modeString(m.mode), m.currentTarget(), m.source().Key()); err == nil && pb != nil {
		pbLine = fmt.Sprintf("Your best: %s WPM (%s accuracy, %s)",
			boldStyle.Render(fmt.Sprintf("%.1f", pb.WPM)),
			boldStyle.Render(fmt.Sprintf("%.0f%%", pb.Accuracy)),
			pb.AchievedAt.Format("2006-01-02"),
		)
	}

	sourceStr := m.source().Name()
	sourceHint := " (s)"
	if m.custom().Custom() {
		sourceHint = " (s, x: forget)"
	}
	battleStr := "Off"
	if m.battle {
		battleStr = "On"
	}
	if m.mode == modeCrawl {
		sourceStr, sourceHint = "Random words", ""
	}
	sourceLine := fmt.Sprintf("Source: %s%s   Battle strip: %s (b)", boldStyle.Render(sourceStr), sourceHint, boldStyle.Render(battleStr))

	punctStr, zenStr, focusStr := "Off", "Off", "Off"
	crawl := m.mode == modeCrawl
	if m.punctuation || (m.fixedText() && !crawl) {
		punctStr = "On"
	}
	zenHint := " (z)"
	if crawl {
		zenStr, zenHint = "N/A", ""
	} else if m.zenMode {
		zenStr = "On"
	}
	focusHint := " (f)"
	if m.fixedText() || crawl {
		focusStr = "N/A"
		focusHint = ""
	} else if m.focusWeak {
		focusStr = "On"
	}
	togglesLine := fmt.Sprintf(
		"Punctuation: %s (c)   Zen mode: %s%s   Focus weak keys: %s%s",
		boldStyle.Render(punctStr), boldStyle.Render(zenStr), zenHint, boldStyle.Render(focusStr), focusHint,
	)

	help := helpStyle.Render("enter: start   p: profile   u: switch profile   q: quit")

	lines := []string{
		title, profileLine, streakLine, challengeLine, "",
		modeLine, targetLine, pbLine, "",
		sourceLine, togglesLine, "",
	}
	switch {
	case crawl:
		lines = append(lines[:len(lines)-1], dimStyle.Render("HP carries from floor to floor, and a knockout ends the crawl."), "")
	case m.source().Custom():
		lines = append(lines[:len(lines)-1], dimStyle.Render("Your own text earns XP and its own bests, but no achievements or challenges."), "")
	}
	if m.errMsg != "" {
		lines = append(lines, wrongStyle.Render(m.errMsg), "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(lines, help)...)
}

// weeklyLine shows this week's challenge and the progress toward it.
func (m menuModel) weeklyLine() string {
	if m.weeklyClaimed {
		return dimStyle.Render(fmt.Sprintf("Weekly challenge (complete): %s", m.weekly.Description))
	}
	return fmt.Sprintf("Weekly challenge: %s   %s   %s",
		m.weekly.Description,
		boldStyle.Render(fmt.Sprintf("%d/%d", m.weeklyProgress, m.weekly.Goal)),
		streakStyle.Render(fmt.Sprintf("+%d XP", m.weekly.XPBonus)))
}
