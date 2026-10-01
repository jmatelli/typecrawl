// Package app implements the Typecrawl Bubble Tea application: profile
// creation and management, a menu to pick a test mode, the typing test
// itself (with HP/combo mechanics), and a results screen with XP/leveling.
package app

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/challenge"
	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/quotes"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// scrollKeyMap restricts the viewport to PgUp/PgDn and Ctrl+U/Ctrl+D
// (half-page up/down, for keyboards without dedicated page keys). The
// viewport's own defaults also bind plain "u"/"d"/"f"/"b"/space/arrows/j/k,
// but every one of those is already meaningfully bound elsewhere in this
// app (space completes a word, "d"/"r" delete/reset a profile, "u" switches
// profile, arrows/j/k move a cursor, etc.), so using the defaults as-is
// would silently steal those keys from whichever screen is active.
func scrollKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		// Up/Down intentionally left unbound (zero value).
	}
}

// isScrollKey reports whether msg is one of scrollKeyMap's bindings --
// used to decide whether to route a key to the viewport at all before
// anything else sees it.
func isScrollKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyCtrlU, tea.KeyCtrlD:
		return true
	default:
		return false
	}
}

type state int

const (
	stateProfileSelect state = iota
	stateProfileCreate
	stateProfileView
	stateAchievements
	stateAchievementUnlock
	stateHistory
	stateKeyboard
	stateMenu
	stateTyping
	stateResults
)

type Model struct {
	state    state
	db       *sql.DB
	profile  *storage.Profile
	profiles []*storage.Profile

	profileSelect     profileSelectModel
	profileForm       profileModel
	profileView       profileViewModel
	achievements      achievementsViewModel
	achievementUnlock achievementUnlockModel
	history           historyViewModel
	keyboard          keyboardViewModel
	menu              menuModel
	typing            typingModel
	results           resultsModel

	width  int
	height int

	// viewport lets tall screens (long achievement lists, the profile page,
	// results with graphs) scroll on small terminals instead of just
	// getting clipped. It only engages -- and only its own footer appears
	// -- when the active screen's content actually exceeds the terminal
	// height; a screen that already fits renders exactly as before.
	viewport      viewport.Model
	viewportReady bool

	// pendingAutoStart holds a CLI quick-launch's requested test config
	// until the first WindowSizeMsg arrives -- newTypingModel needs a
	// known terminal width, which isn't available yet at NewWithOptions
	// time, so the transition is deferred rather than applied immediately.
	pendingAutoStart *startTypingMsg
}

// LaunchOptions carries CLI quick-launch parameters (see NewWithOptions),
// letting a session skip menus it already has answers for.
type LaunchOptions struct {
	ProfileName string // empty: no auto-select, start at the normal profile screen
	Mode        string // "time" or "words"; empty: select the profile but stop at the menu
	Target      int    // seconds for "time", word count for "words"
	Punctuation bool
	ZenMode     bool
	FocusWeak   bool
	Quotes      bool // draws exercise text from cached quotes instead of random words
}

// findProfileByName looks up a profile case-insensitively, since CLI flags
// are typically typed by hand.
func findProfileByName(profiles []*storage.Profile, name string) *storage.Profile {
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// NewWithOptions builds the root model like New, then optionally fast-forwards
// past the profile-select and/or menu screens using CLI-provided answers. A
// non-empty ProfileName selects that profile immediately; adding a non-empty
// Mode additionally queues an auto-start of the typing test, applied as soon
// as the terminal size is known. An unrecognized ProfileName is an error
// rather than a silent fallback, since the intent was clearly to skip
// straight past profile selection.
func NewWithOptions(db *sql.DB, profiles []*storage.Profile, opts LaunchOptions) (Model, error) {
	m := New(db, profiles)
	if opts.ProfileName == "" {
		return m, nil
	}

	profile := findProfileByName(profiles, opts.ProfileName)
	if profile == nil {
		names := make([]string, len(profiles))
		for i, p := range profiles {
			names[i] = p.Name
		}
		return Model{}, fmt.Errorf("no profile named %q (available: %s)", opts.ProfileName, strings.Join(names, ", "))
	}
	m.profile = profile
	m.state = stateMenu
	m.menu = newMenuModel(profile, db)

	if opts.Mode == "" {
		return m, nil
	}
	mode := modeTime
	if opts.Mode == "words" {
		mode = modeWords
	}
	m.pendingAutoStart = &startTypingMsg{
		mode: mode, target: opts.Target,
		punctuation: opts.Punctuation, zenMode: opts.ZenMode, focusWeak: opts.FocusWeak,
		quotes: opts.Quotes,
	}
	return m, nil
}

// New builds the root model. profiles is the full profile list at startup;
// an empty list routes straight to profile creation (first-run), otherwise
// the player picks one from the selection screen.
func New(db *sql.DB, profiles []*storage.Profile) Model {
	m := Model{db: db, profiles: profiles}
	if len(profiles) == 0 {
		m.state = stateProfileCreate
		m.profileForm = newProfileModel(db)
	} else {
		m.state = stateProfileSelect
		m.profileSelect = newProfileSelectModel(db, profiles)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	switch m.state {
	case stateProfileCreate:
		return m.profileForm.Init()
	case stateProfileSelect:
		return m.profileSelect.Init()
	default:
		return m.menu.Init()
	}
}

// Update satisfies tea.Model. It delegates to update for all the actual
// message handling, then (a) resets the scroll position whenever the
// active screen changed, so a new screen always starts scrolled to the
// top, and (b) refreshes the viewport's content to match the just-updated
// screen. (b) matters even though View also sets the viewport's content:
// View has a value receiver, so its mutations never reach the persisted
// model, meaning PgUp/PgDn would otherwise scroll through a permanently
// empty viewport. Refreshing here, after every message, keeps the
// persisted viewport's content in sync with whatever will be rendered.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevState := m.state
	next, cmd := m.update(msg)
	nm := next.(Model)
	if nm.state != prevState {
		nm.viewport.SetYOffset(0)
	}
	if nm.viewportReady {
		nm.viewport.Width = nm.width
		nm.viewport.Height = max(nm.height-1, 1)
		nm.viewport.SetContent(nm.activeScreenView())
	}
	return nm, cmd
}

// startTyping builds the typing screen for msg's requested config. Split out
// from update's startTypingMsg case so NewWithOptions's deferred auto-start
// (see pendingAutoStart) can reuse the exact same logic once the terminal
// width is known, instead of duplicating it.
func (m Model) startTyping(msg startTypingMsg) (Model, tea.Cmd) {
	var weakChars map[rune]int
	if msg.focusWeak {
		weakChars, _ = storage.WeakCharWeights(m.db, m.profile.ID) // best-effort; nil falls back to plain selection
	}
	var ghostPaceMS []int
	var ghostWPMSeries []float64
	if pb, err := storage.GetPersonalBest(m.db, m.profile.ID, modeString(msg.mode), msg.target, sourceString(msg.quotes)); err == nil && pb != nil {
		ghostPaceMS = pb.GhostPace         // best-effort; nil just means no ghost to race
		ghostWPMSeries = pb.GhostWPMSeries // best-effort; nil just means no ghost curve to overlay on the results graph
	}
	var quotePool []quotes.Quote
	if msg.quotes {
		quotePool = loadQuotePool(m.db)
	}
	m.typing = newTypingModel(
		msg.mode, msg.target, m.width, game.MaxHP(m.profile.Level), m.profile.Level,
		msg.punctuation, msg.zenMode, msg.focusWeak, msg.quotes, quotePool, weakChars, ghostPaceMS, ghostWPMSeries,
	)
	m.state = stateTyping
	return m, m.typing.Init()
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Reserved exclusively for scrolling -- not bound anywhere else in
		// the app -- so it's safe to intercept before any sub-screen sees
		// it, regardless of which screen is active.
		if isScrollKey(msg) {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.viewportReady {
			m.viewport = viewport.New(msg.Width, msg.Height)
			m.viewport.KeyMap = scrollKeyMap()
			m.viewportReady = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height
		}
		if m.pendingAutoStart != nil {
			start := *m.pendingAutoStart
			m.pendingAutoStart = nil
			return m.startTyping(start)
		}
		// fall through below so the active sub-model also sees the resize

	case profileCreatedMsg:
		m.profile = msg.profile
		m.state = stateMenu
		m.menu = newMenuModel(m.profile, m.db)
		return m, m.menu.Init()

	case profileSelectedMsg:
		m.profile = msg.profile
		m.state = stateMenu
		m.menu = newMenuModel(m.profile, m.db)
		return m, m.menu.Init()

	case createNewProfileMsg:
		m.state = stateProfileCreate
		m.profileForm = newProfileModel(m.db)
		return m, m.profileForm.Init()

	case switchProfileMsg:
		profiles, err := storage.ListProfiles(m.db)
		if err == nil {
			m.profiles = profiles
		}
		m.state = stateProfileSelect
		m.profileSelect = newProfileSelectModel(m.db, m.profiles)
		return m, m.profileSelect.Init()

	case viewProfileMsg:
		m.state = stateProfileView
		m.profileView = newProfileViewModel(m.profile, m.db, m.width)
		return m, nil

	case viewAchievementsMsg:
		m.state = stateAchievements
		m.achievements = newAchievementsViewModel(m.profile, m.db)
		return m, nil

	case viewHistoryMsg:
		m.state = stateHistory
		m.history = newHistoryViewModel(m.profile, m.db)
		return m, nil

	case viewKeyboardMsg:
		m.state = stateKeyboard
		m.keyboard = newKeyboardViewModel(m.profile, m.db)
		return m, nil

	case profileDeletedMsg:
		profiles, err := storage.ListProfiles(m.db)
		if err == nil {
			m.profiles = profiles
		}
		m.profile = nil
		m.state = stateProfileSelect
		m.profileSelect = newProfileSelectModel(m.db, m.profiles)
		return m, m.profileSelect.Init()

	case startTypingMsg:
		return m.startTyping(msg)

	case finishTypingMsg:
		if game.IsSuspiciousRun(msg.result) {
			// Looks scripted/injected rather than hand-typed (see
			// game.IsSuspiciousRun) -- show the raw numbers but touch
			// nothing persisted: no XP, no personal best, no streak or
			// achievement progress.
			m.results = newResultsModel(msg.result, m.width, resultsExtra{unverified: true})
			m.state = stateResults
			return m, nil
		}

		now := time.Now()
		modeStr := modeString(msg.mode)
		sourceStr := sourceString(msg.quotes)

		// Snapshot "before" state for the achievement-unlock diff below,
		// ahead of any mutation this run causes.
		beforeDailyStreak, _ := storage.CurrentStreak(m.db, m.profile.ID, now)
		beforeBestOverall, _ := storage.BestOverallWPM(m.db, m.profile.ID)
		var beforeBestWPM float64
		if beforeBestOverall != nil {
			beforeBestWPM = beforeBestOverall.WPM
		}
		beforeProgress := buildAchievementProgress(m.db, m.profile, beforeDailyStreak, beforeBestWPM)

		_ = storage.RecordActivity(m.db, m.profile.ID, now) // best-effort; shows up on the profile's activity graph
		_ = storage.RecordCharMistakes(m.db, m.profile.ID, msg.charMistakes)
		_ = storage.RecordTestResult(m.db, m.profile.ID, msg.result.WPM, msg.result.Accuracy, modeStr, msg.target, msg.ko, msg.punctuation, msg.zenMode, sourceStr, msg.wordsTyped, msg.result.Duration, now)
		streakDays, _ := storage.CurrentStreak(m.db, m.profile.ID, now)

		var isNewPB bool
		personalBest := beforeBestWPM
		if !msg.ko {
			var err error
			isNewPB, err = storage.RecordPersonalBest(m.db, m.profile.ID, modeStr, msg.target, sourceStr, msg.result.WPM, msg.result.Accuracy, msg.result.WordEndOffsetsMS, msg.result.WPMSeries, now)
			if err == nil {
				if pb, err := storage.GetPersonalBest(m.db, m.profile.ID, modeStr, msg.target, sourceStr); err == nil && pb != nil {
					personalBest = pb.WPM
				}
			}
		}

		var baseXP, xpBonus, streakBonusXP int
		oldLevel := m.profile.Level
		newLevel, newXP, levelsGained := oldLevel, m.profile.XP, 0

		// The daily challenge is a flat one-time-per-day bonus, not part of
		// the streak-multiplied XP below -- it's a separate, simpler
		// reward for hitting today's specific objective at all.
		var challengeBonusXP int
		var challengeCompleted bool
		var todayChallenge = challenge.Today(now)
		if !msg.ko {
			run := challenge.Run{
				Mode: modeStr, Target: msg.target, Punctuation: msg.punctuation, ZenMode: msg.zenMode,
				WPM: msg.result.WPM, Accuracy: msg.result.Accuracy, KO: msg.ko,
			}
			if todayChallenge.Completed(run) {
				day := challenge.Day(now)
				if claimed, _ := storage.HasClaimedDailyChallenge(m.db, m.profile.ID, day); !claimed {
					challengeBonusXP = todayChallenge.XPBonus
					challengeCompleted = true
					_ = storage.ClaimDailyChallenge(m.db, m.profile.ID, day, now)
				}
			}
		}

		if !msg.ko {
			// A knockout earns nothing -- getting overwhelmed by mistakes
			// shouldn't still pay out.
			baseXP = game.XPForResult(msg.result.WPM, msg.result.Accuracy, msg.result.Duration)
			xpBonus = msg.xpBonus

			preStreak := baseXP + xpBonus
			totalXP := int(float64(preStreak)*game.DailyStreakMultiplier(streakDays)+0.5) + challengeBonusXP
			streakBonusXP = totalXP - preStreak - challengeBonusXP

			newLevel, newXP, levelsGained = game.AddXP(oldLevel, m.profile.XP, totalXP)
			m.profile.Level = newLevel
			m.profile.XP = newXP
		}

		// A run that ends in KO doesn't get to set a new word-streak record
		// -- same reasoning as XP: getting overwhelmed by mistakes shouldn't
		// still pay out, even for an achievement earned earlier in the run.
		newWordStreakRecord := !msg.ko && msg.bestCombo > m.profile.BestWordStreak
		if newWordStreakRecord {
			m.profile.BestWordStreak = msg.bestCombo
		}
		if msg.ko {
			m.profile.KOCount++
		} else {
			m.profile.SuccessCount++
		}
		_ = storage.Save(m.db, m.profile) // best-effort; gameplay continues even if the write fails

		m.results = newResultsModel(msg.result, m.width, resultsExtra{
			xpGained:              baseXP + xpBonus + streakBonusXP + challengeBonusXP,
			xpBase:                baseXP,
			xpBonus:               xpBonus,
			streakDays:            streakDays,
			streakBonusXP:         streakBonusXP,
			bestCombo:             msg.bestCombo,
			allTimeBestWordStreak: m.profile.BestWordStreak,
			newWordStreakRecord:   newWordStreakRecord,
			koCount:               m.profile.KOCount,
			isNewPB:               isNewPB,
			personalBest:          personalBest,
			oldLevel:              oldLevel,
			newLevel:              newLevel,
			levelsGained:          levelsGained,
			hp:                    msg.hp,
			maxHP:                 msg.maxHP,
			ko:                    msg.ko,
			hitCount:              msg.hitCount,
			challengeCompleted:    challengeCompleted,
			challengeBonusXP:      challengeBonusXP,
			challengeDescription:  todayChallenge.Description,
			ghostWPMSeries:        msg.ghostWPMSeries,
		})

		afterProgress := buildAchievementProgress(m.db, m.profile, streakDays, personalBest)
		if unlocked := newlyUnlocked(beforeProgress, afterProgress); len(unlocked) > 0 {
			m.achievementUnlock = newAchievementUnlockModel(unlocked)
			m.state = stateAchievementUnlock
		} else {
			m.state = stateResults
		}
		return m, nil

	case continueToResultsMsg:
		m.state = stateResults
		return m, nil

	case restartMsg:
		m.menu = newMenuModel(m.profile, m.db)
		m.state = stateMenu
		return m, m.menu.Init()
	}

	var cmd tea.Cmd
	switch m.state {
	case stateProfileSelect:
		var next tea.Model
		next, cmd = m.profileSelect.Update(msg)
		m.profileSelect = next.(profileSelectModel)
	case stateProfileCreate:
		var next tea.Model
		next, cmd = m.profileForm.Update(msg)
		m.profileForm = next.(profileModel)
	case stateProfileView:
		var next tea.Model
		next, cmd = m.profileView.Update(msg)
		m.profileView = next.(profileViewModel)
	case stateAchievements:
		var next tea.Model
		next, cmd = m.achievements.Update(msg)
		m.achievements = next.(achievementsViewModel)
	case stateAchievementUnlock:
		var next tea.Model
		next, cmd = m.achievementUnlock.Update(msg)
		m.achievementUnlock = next.(achievementUnlockModel)
	case stateHistory:
		var next tea.Model
		next, cmd = m.history.Update(msg)
		m.history = next.(historyViewModel)
	case stateKeyboard:
		var next tea.Model
		next, cmd = m.keyboard.Update(msg)
		m.keyboard = next.(keyboardViewModel)
	case stateMenu:
		var next tea.Model
		next, cmd = m.menu.Update(msg)
		m.menu = next.(menuModel)
	case stateTyping:
		var next tea.Model
		next, cmd = m.typing.Update(msg)
		m.typing = next.(typingModel)
	case stateResults:
		var next tea.Model
		next, cmd = m.results.Update(msg)
		m.results = next.(resultsModel)
	}
	return m, cmd
}

// View satisfies tea.Model. If the active screen's content fits the
// terminal, it's returned as-is; otherwise it's rendered inside a
// scrollable viewport with a footer showing how to scroll and how far
// through the content the view currently is.
func (m Model) View() string {
	content := m.activeScreenView()

	if !m.viewportReady || strings.Count(content, "\n")+1 <= m.height {
		return content
	}

	m.viewport.Width = m.width
	m.viewport.Height = max(m.height-1, 1) // reserve the last line for the scroll footer
	m.viewport.SetContent(content)

	footer := helpStyle.Render(fmt.Sprintf("-- scroll: PgUp/PgDn or Ctrl+U/Ctrl+D (%.0f%%) --", m.viewport.ScrollPercent()*100))
	return m.viewport.View() + "\n" + footer
}

func (m Model) activeScreenView() string {
	switch m.state {
	case stateProfileSelect:
		return m.profileSelect.View()
	case stateProfileCreate:
		return m.profileForm.View()
	case stateProfileView:
		return m.profileView.View()
	case stateAchievements:
		return m.achievements.View()
	case stateAchievementUnlock:
		return m.achievementUnlock.View()
	case stateHistory:
		return m.history.View()
	case stateKeyboard:
		return m.keyboard.View()
	case stateTyping:
		return m.typing.View()
	case stateResults:
		return m.results.View()
	default:
		return m.menu.View()
	}
}
