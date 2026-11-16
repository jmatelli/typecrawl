package app

import (
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/avatar"
	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/quotes"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/textsource"
	"github.com/jmatelli/typecrawl/internal/theme"
	"github.com/jmatelli/typecrawl/internal/words"
)

const visibleLines = 3

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	// correctStyle and wrongStyle mark typed characters. Without colour
	// (see package theme) they fall back to attributes that still set
	// three kinds of text apart: typed text is bold, a mistake is in
	// reverse video, and text still to come is faint.
	correctStyle = theme.Pick(lipgloss.NewStyle().Foreground(theme.Color("2")), lipgloss.NewStyle().Bold(true))
	wrongStyle   = theme.Pick(lipgloss.NewStyle().Foreground(theme.Color("1")).Underline(true), lipgloss.NewStyle().Underline(true).Reverse(true))
	comboStyle   = lipgloss.NewStyle().Foreground(theme.Color("208")).Bold(true)
	streakStyle  = lipgloss.NewStyle().Foreground(theme.Color("10")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(theme.Color("1")).Bold(true)
	// goldenWordStyle marks a rare word that grants a mistake-forgiving
	// shield if typed clean. greenWordStyle marks a streak-earned word
	// that heals if typed clean. blueWordStyle marks the green word's
	// flawless-run variant, which grants a timed shield instead.
	goldenWordStyle = lipgloss.NewStyle().Foreground(theme.Color("220")).Bold(true)
	greenWordStyle  = lipgloss.NewStyle().Foreground(theme.Color("82")).Bold(true)
	blueWordStyle   = lipgloss.NewStyle().Foreground(theme.Color("39")).Bold(true)
	hpHighStyle     = lipgloss.NewStyle().Foreground(theme.Color("2"))
	hpMidStyle      = lipgloss.NewStyle().Foreground(theme.Color("3"))
	hpLowStyle      = lipgloss.NewStyle().Foreground(theme.Color("1"))
)

// Each special word carries a marker as well as a colour, so the three can
// be told apart by players who can't distinguish the colours and on
// terminals that don't show them. The same marker labels the shield its
// word grants. They are plain ASCII so they are one cell wide everywhere.
const (
	goldenWordMarker = "*" // grants a shield
	greenWordMarker  = "+" // heals
	blueWordMarker   = "~" // grants a timed shield
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// wordCommit snapshots the state right before a word's completion was
// applied, plus what was typed, so backspacing back into that word can
// undo the completion exactly rather than leaving stale bookkeeping.
type wordCommit struct {
	typed        string
	hadMistake   bool
	successCombo int
	bestCombo    int
	xpBonus      int
	pageStart    int
}

type typingModel struct {
	mode       testMode
	target     int // seconds for time mode, word count for words mode
	wordList   []string
	typedWords []string // what the user actually typed, per completed word
	current    string
	wordIndex  int
	pageStart  int // index of the first word on the currently visible page
	tracker    *stats.Tracker
	started    bool
	finished   bool
	width      int
	commits    []wordCommit // undo stack for backspacing across word boundaries

	level       int        // game.DifficultyLevel at test start; sets the word difficulty tier
	punctuation bool       // whether words get sentence-style capitals/punctuation
	zenMode     bool       // disables HP loss/KO for pure practice
	focusWeak   bool       // whether word choice is biased toward weak characters
	weak        words.Weak // profile's weak keys and key pairs, for biasing word choice

	// quotesMode draws exercise text from quotePool (real quotations)
	// instead of internal/words' random word lists -- see genWords. It has
	// no interaction with focusWeak (weak-key biasing has no meaning over
	// fixed quote text) and always implies punctuation, since a quote's own
	// punctuation is part of the text, not an optional flourish.
	quotesMode bool
	quotePool  []quotes.Quote

	// custom is the player's own word list or text, when the run uses
	// one (see withCustomSource), and customWords its words as loaded.
	custom      textsource.Source
	customWords []string
	// customWeights is words.Weights for a word list, kept so each top-up
	// doesn't walk the whole list again.
	customWeights []int

	// floor is the crawl floor this run is, or Number 0 outside a crawl
	// (see withCrawlFloor).
	floor game.CrawlFloor

	// ghostPaceMS is the personal best's per-word cumulative elapsed
	// milliseconds for this exact mode/target, if one exists -- raced live
	// against via ghostStatus. nil means no PB to race yet.
	ghostPaceMS []int
	// ghostWPMSeries is that same personal best's cumulative-WPM-per-word
	// curve, carried through to finishTypingMsg so the results screen can
	// overlay it on this run's own graph. nil means no ghost to overlay.
	ghostWPMSeries []float64

	hp         int
	maxHP      int
	comboCount int
	comboUntil time.Time
	ko         bool
	hitCount   int // how many mistakes actually dealt (or would deal) HP damage this run

	wordHasMistake bool         // whether the word in progress has had any mistake
	successCombo   int          // consecutive mistake-free completed words
	bestCombo      int          // best successCombo reached this test
	xpBonus        int          // accumulated XP from the success streak
	lastKeystroke  time.Time    // when the last rune/space was processed, for the streak's pause check
	charMistakes   map[rune]int // this run's per-character mistake counts, for weak-key tracking
	// pairMistakes counts this run's mistakes by key pair: the character
	// before the mistyped one and the mistyped one, lowercase.
	pairMistakes map[string]int
	// missed holds the positions of words this run got wrong at any
	// point, for a drill of them afterwards. Unlike wordHasMistake it's
	// never cleared: a word mistyped and then fixed was still missed.
	missed map[int]bool
	// specialLanded counts the special words landed clean this run, for
	// the daily challenges that ask for them. specialLandedAt holds the
	// word positions already counted: backspacing across a streak
	// milestone and retyping it marks the same position special again,
	// and that must not count twice.
	specialLanded   specialWords
	specialLandedAt map[int]bool

	pasteAttempts int // pasted input events rejected this run -- see the tea.KeyRunes case

	// goldenWordIdx is the word index (if any) this exercise's rare golden
	// word landed on, fixed once at construction; -1 means this exercise
	// has none. greenWordIdx is the word index earning a heal if typed
	// clean, set dynamically whenever the streak hits a multiple of
	// game.GreenWordStreakInterval; -1 means none currently pending.
	// shieldActive is consumed by the next mistake, fully forgiving it
	// (no HP loss, no streak break) once the golden word is landed clean.
	goldenWordIdx int
	greenWordIdx  int
	shieldActive  bool

	// blueWordIdx replaces greenWordIdx when the streak milestone is hit
	// in a run with no mistake at all so far (anyMistake false): landing
	// it clean sets shieldUntil, and every mistake before that deadline is
	// forgiven the same way the golden shield forgives one. -1 means none
	// pending.
	blueWordIdx int
	shieldUntil time.Time
	anyMistake  bool // whether any wrong keystroke happened this run, even if fixed or shielded

	// The battle strip (see battle.go). battle is the profile's setting;
	// the strip is also hidden when the terminal is too small. The times
	// below are when the last letter was cut, the last hit landed and the
	// last monster fell, which the strip animates from.
	battle              bool
	avatar              avatar.Avatar
	height              int
	lastCutAt           time.Time
	lastHitAt           time.Time
	lastHitStrength     int
	lastHitBlockedBy    string // the marker of the shield that absorbed the last hit, "" if none did
	lastHitBlockedStyle lipgloss.Style
	lastKillAt          time.Time
	lastDrop            string
	lastDropStyle       lipgloss.Style
	// pendingFinish delivers the result during the pause after a
	// knockout (see finish). It returns nil once the result has been
	// delivered, so it can't be recorded twice.
	pendingFinish tea.Cmd
}

func newTypingModel(mode testMode, target, width, maxHP, level int, punctuation, zenMode, focusWeak, quotesMode bool, quotePool []quotes.Quote, weak words.Weak, ghostPaceMS []int, ghostWPMSeries []float64) typingModel {
	// A quote's punctuation is part of its text, not an optional flourish
	// layered on top the way words.Punctuate dresses up random words -- so
	// quotes mode always counts as a punctuation-enabled run (this is what
	// feeds the "Grammarian" achievement and the punctuation daily
	// challenge, among other things).
	punctuation = punctuation || quotesMode

	n := target
	if mode == modeTime {
		n = 300
	}
	m := typingModel{
		mode:           mode,
		target:         target,
		typedWords:     make([]string, 0, n),
		tracker:        stats.NewTracker(),
		width:          width,
		level:          level,
		punctuation:    punctuation,
		zenMode:        zenMode,
		focusWeak:      focusWeak,
		quotesMode:     quotesMode,
		quotePool:      quotePool,
		weak:           weak,
		ghostPaceMS:    ghostPaceMS,
		ghostWPMSeries: ghostWPMSeries,
		hp:             maxHP,
		maxHP:          maxHP,
		greenWordIdx:   -1,
		blueWordIdx:    -1,
	}
	m.wordList = m.genWords(n)
	m.goldenWordIdx = rollGoldenWordIndex(len(m.wordList))
	return m
}

// withCustomSource switches the run to the player's own file or a drill:
// loaded is a word list's or a text's words as textsource.Load returned
// them, or a drill's words in order. It
// replaces the words the constructor generated. Like a quote, a text
// carries its own punctuation, so it counts as a punctuation run and has
// nothing for weak-key focus to bias.
func (m typingModel) withCustomSource(src textsource.Source, loaded []string) typingModel {
	m.custom, m.customWords = src, loaded
	m.quotesMode, m.quotePool = false, nil
	switch src.Kind {
	case textsource.Text:
		m.punctuation = true
		m.focusWeak, m.weak = false, words.Weak{}
	case textsource.Drill:
		// Typed as given: the drill's words keep whatever punctuation
		// they had, and there's no choice for weak-key focus to bias.
		m.focusWeak, m.weak = false, words.Weak{}
	default:
		m.customWeights = words.Weights(loaded, m.weak)
	}
	n := m.target
	if m.mode == modeTime {
		n = 300
	}
	m.wordList = m.genWords(n)
	m.goldenWordIdx = rollGoldenWordIndex(len(m.wordList))
	return m
}

// withCrawlFloor makes the run floor f of a crawl, starting with the hp
// and golden shield the floor before it ended with: they carry from floor
// to floor. The constructor already drew f's words at f's level; that
// level also sets how hard mistakes hit. A crawl has no zen mode, which
// would make the HP pointless.
func (m typingModel) withCrawlFloor(f game.CrawlFloor, hp int, shield bool) typingModel {
	m.floor = f
	m.hp = min(max(hp, 1), m.maxHP)
	m.shieldActive = shield
	m.zenMode = false
	return m
}

// genWords produces n more exercise words: from the player's own text in
// order, drawn from their word list, from quotePool (whole quotes
// concatenated end to end), or from internal/words' tiered random lists.
// Drawn words can be dressed up with sentence-style punctuation. tail,
// when given, is the already-generated word list: drawn words use its end
// to avoid repeats (see words.ForLevel), and a text carries on from
// however many words it already holds. Quotes carry their own punctuation
// and have no equivalent immediate-repeat concern worth tracking across
// calls (see quotes.Words).
func (m typingModel) genWords(n int, tail ...string) []string {
	var list []string
	switch {
	case m.custom.Kind == textsource.Text || m.custom.Kind == textsource.Drill:
		return textsource.InOrder(m.customWords, len(tail), n)
	case m.custom.Kind == textsource.WordList:
		list = words.FromWeighted(m.customWords, m.customWeights, n, tail...)
	case m.quotesMode:
		return quotes.Words(m.quotePool, n)
	default:
		list = words.ForLevelWeak(n, m.level, m.weak, tail...)
	}
	if m.punctuation {
		list = words.Punctuate(list)
	}
	return list
}

// rollGoldenWordIndex decides whether this exercise gets a rare golden
// word and, if so, where: most exercises get none at all (see
// game.GoldenWordChance), and when one does land it's within an early,
// likely-to-be-reached window rather than anywhere in a long time-mode
// buffer. Returns -1 for "no golden word this exercise."
func rollGoldenWordIndex(wordCount int) int {
	if wordCount <= game.GoldenWordMinIndex || rand.Float64() >= game.GoldenWordChance {
		return -1
	}
	hi := min(wordCount-1, game.GoldenWordMaxIndex)
	if hi <= game.GoldenWordMinIndex {
		return -1
	}
	return game.GoldenWordMinIndex + rand.Intn(hi-game.GoldenWordMinIndex+1)
}

// ghostStatus reports how far ahead (positive) or behind (negative) of the
// personal best's pace the current run is, in milliseconds, once at least
// one word has been completed and a ghost pace exists for this mode/target.
func (m typingModel) ghostStatus() (deltaMS int, active bool) {
	if !m.started || len(m.ghostPaceMS) == 0 || m.wordIndex == 0 || m.wordIndex > len(m.ghostPaceMS) {
		return 0, false
	}
	ghostElapsed := m.ghostPaceMS[m.wordIndex-1]
	actualElapsed := int(time.Since(m.tracker.Start).Milliseconds())
	return ghostElapsed - actualElapsed, true
}

func (m typingModel) Init() tea.Cmd {
	return tick()
}

func (m typingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.finished {
			return m, nil
		}
		if m.started && m.mode == modeTime {
			elapsed := time.Since(m.tracker.Start)
			if elapsed >= time.Duration(m.target)*time.Second {
				return m.finish()
			}
		}
		return m, tick()

	case tea.KeyMsg:
		if m.finished {
			// Only reachable during the pause on a knockout, before the
			// results take over. Quitting there must still record the
			// knockout, so the result goes first.
			if msg.Type == tea.KeyCtrlC {
				return m, tea.Sequence(m.pendingFinish, tea.Quit)
			}
			return m, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			return m, func() tea.Msg { return restartMsg{} }
		case tea.KeySpace:
			m.checkPause()
			m.completeWord()
			if m.mode == modeWords && m.wordIndex >= m.target {
				return m.finish()
			}
			m.advancePage()
		case tea.KeyBackspace:
			if len(m.current) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.current)
				m.current = m.current[:len(m.current)-size] // one letter, not one byte
				if m.current == "" {
					// A full clear is a fresh restart of this word's streak
					// eligibility: fixing every character earns the streak
					// back. HP damage and accuracy from the original wrong
					// keystrokes already happened and stay permanent --
					// only the streak, which is about the final attempt,
					// gets a clean slate.
					m.wordHasMistake = false
				}
			} else {
				m.backspaceIntoPreviousWord()
			}
		case tea.KeyRunes:
			if msg.Paste {
				// Bracketed paste would otherwise let pasted text complete
				// words instantly with no real typing -- an easy way to
				// fake a flawless, superhuman-WPM run and inflate personal
				// bests and achievements. Reject it outright: no characters
				// recorded, no mistake, no progress.
				m.pasteAttempts++
				break
			}
			m.checkPause()
			if !m.started {
				m.started = true
				m.tracker.Start = time.Now()
			}
			for _, r := range msg.Runes {
				cutBefore := m.lettersCut()
				if m.typeRune(r) {
					if m.lettersCut() > cutBefore {
						m.lastCutAt = time.Now()
					}
				} else {
					m.anyMistake = true
					if m.blueWordIdx != -1 {
						// The run just stopped being flawless, so a pending
						// blue word reverts to the regular green heal.
						m.greenWordIdx = m.blueWordIdx
						m.blueWordIdx = -1
					}
					if m.timedShieldActive() {
						// A landed blue word's timed shield forgives every
						// mistake until it runs out, without being spent.
						// Checked before the golden shield so that one is
						// saved for after the timer expires.
						m.noteHit(1, blueWordMarker, blueWordStyle)
						continue
					}
					if m.shieldActive {
						m.noteHit(1, goldenWordMarker, goldenWordStyle)
						// A landed golden word's shield fully absorbs the
						// very next mistake: no HP loss, no streak break.
						// The keystroke is still wrong in the buffer and
						// needs correcting like any other typo -- the
						// shield forgives the penalty, not the typo
						// itself.
						m.shieldActive = false
					} else {
						m.registerMistake()
						m.noteHit(m.comboCount, "", lipgloss.Style{})
						if m.hp <= 0 {
							m.ko = true
							break
						}
					}
				}
			}
			if m.ko {
				return m.finish()
			}
		}
	}
	return m, nil
}

// typeRune records one keystroke against the current word and returns
// whether it was correct. A wrong keystroke (when there's an actual target
// character at that position) is attributed to the character the player
// was trying to hit, feeding the weak-key tracking.
func (m *typingModel) typeRune(r rune) bool {
	if m.wordIndex >= len(m.wordList) {
		return true
	}
	// Positions are in runes, not bytes, so a letter like "é" is one
	// keystroke like any other.
	expected := []rune(m.wordList[m.wordIndex])
	pos := utf8.RuneCountInString(m.current)
	correct := pos < len(expected) && expected[pos] == r
	if !correct {
		m.markMissed()
	}
	if !correct && pos < len(expected) {
		if m.charMistakes == nil {
			m.charMistakes = make(map[rune]int)
		}
		m.charMistakes[unicode.ToLower(expected[pos])]++
		if pos > 0 {
			if m.pairMistakes == nil {
				m.pairMistakes = make(map[string]int)
			}
			m.pairMistakes[string([]rune{unicode.ToLower(expected[pos-1]), unicode.ToLower(expected[pos])})]++
		}
	}
	m.tracker.RecordChar(correct, m.wordIndex)
	m.current += string(r)
	return correct
}

// markMissed notes the current word as missed.
func (m *typingModel) markMissed() {
	if m.missed == nil {
		m.missed = make(map[int]bool)
	}
	m.missed[m.wordIndex] = true
}

// missedWords returns the words this run missed, each once, in the order
// they came up. Only words actually typed into can be missed, so the
// unreached lookahead never is.
func (m typingModel) missedWords() []string {
	var out []string
	seen := map[string]bool{}
	for i, w := range m.wordList {
		if m.missed[i] && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// lettersCut is how many letters of the current word the battle strip's
// avatar has cut away.
func (m typingModel) lettersCut() int {
	if m.wordIndex >= len(m.wordList) {
		return 0
	}
	return correctPrefix(m.wordList[m.wordIndex], m.current)
}

// noteHit records a hit for the battle strip: the monster attacks, harder
// in a combo, and either the avatar takes it or the shield marked
// blockedBy does.
func (m *typingModel) noteHit(strength int, blockedBy string, blockedStyle lipgloss.Style) {
	if m.zenMode {
		return // no HP loss in zen mode, so nothing hits back
	}
	m.lastHitAt = time.Now()
	m.lastHitStrength = min(strength, 2)
	m.lastHitBlockedBy, m.lastHitBlockedStyle = blockedBy, blockedStyle
}

// timedShieldActive reports whether a blue word's timed shield is still
// running.
func (m typingModel) timedShieldActive() bool {
	return time.Now().Before(m.shieldUntil)
}

// checkPause breaks the mistake-free streak on its own if too long has
// passed since the last keystroke -- hesitating resets the streak just
// like a mistake does, independent of accuracy.
func (m *typingModel) checkPause() {
	now := time.Now()
	if !m.lastKeystroke.IsZero() && now.Sub(m.lastKeystroke) > game.StreakPauseLimit {
		m.successCombo = 0
	}
	m.lastKeystroke = now
}

// registerMistake applies HP damage for a mistake, unless zen mode is on.
// Consecutive mistakes within game.ComboWindow of each other build a combo
// that deals increasing damage and keeps refreshing the window. Damage also
// scales with the current word's difficulty tier -- a slip on a harder
// word stings more.
func (m *typingModel) registerMistake() {
	now := time.Now()
	if now.Before(m.comboUntil) {
		m.comboCount++
	} else {
		m.comboCount = 1
	}
	m.comboUntil = now.Add(game.ComboWindow)
	m.hitCount++
	if !m.zenMode {
		m.hp = max(m.hp-game.ComboDamage(m.comboCount, words.Tier(m.level)), 0)
	}
	m.wordHasMistake = true
}

func (m *typingModel) completeWord() {
	if m.current == "" || m.wordIndex >= len(m.wordList) {
		return
	}

	// The timed shield forgives a typo's HP/combo penalty, not the typo:
	// submitting a word that still has shielded typos in it doesn't count
	// as clean, so the shield window can't be used to mash through words
	// while farming streak XP. Correcting them before space still does.
	if m.timedShieldActive() && m.current != m.wordList[m.wordIndex] {
		m.wordHasMistake = true
	}

	// Resolve this exercise's special words, if the one just finished was
	// one of them. Both are one-shot: landed clean or not, the
	// opportunity is spent either way. Consistent with how backspacing
	// across a word boundary already treats HP damage as a permanent sunk
	// cost rather than something to undo, neither effect is reverted if
	// the player later backspaces back into this word.
	// The monster falls either way; a special one drops its reward only
	// if it was landed clean.
	m.lastKillAt = time.Now()
	m.lastDrop, m.lastDropStyle = "poof!", dropStyle
	if m.wordIndex == m.goldenWordIdx {
		if !m.wordHasMistake {
			m.shieldActive = true
			m.countSpecialLanded(&m.specialLanded.golden)
			m.lastDrop, m.lastDropStyle = goldenWordMarker+" shield!", goldenWordStyle
		}
		m.goldenWordIdx = -1
	}
	if m.wordIndex == m.greenWordIdx {
		if !m.wordHasMistake {
			m.hp = min(m.hp+game.GreenWordHealAmount, m.maxHP)
			m.countSpecialLanded(&m.specialLanded.green)
			m.lastDrop, m.lastDropStyle = greenWordMarker+" heal!", greenWordStyle
		}
		m.greenWordIdx = -1
	}
	if m.wordIndex == m.blueWordIdx {
		if !m.wordHasMistake {
			m.shieldUntil = time.Now().Add(game.BlueWordShieldDuration)
			m.countSpecialLanded(&m.specialLanded.blue)
			m.lastDrop, m.lastDropStyle = fmt.Sprintf("%s %.0fs shield!", blueWordMarker, game.BlueWordShieldDuration.Seconds()), blueWordStyle
		}
		m.blueWordIdx = -1
	}

	m.commits = append(m.commits, wordCommit{
		typed:        m.current,
		hadMistake:   m.wordHasMistake,
		successCombo: m.successCombo,
		bestCombo:    m.bestCombo,
		xpBonus:      m.xpBonus,
		pageStart:    m.pageStart,
	})

	// A word sent short or long, with no wrong key in it, was missed too.
	if m.current != m.wordList[m.wordIndex] {
		m.markMissed()
	}
	m.tracker.RecordWordComplete(m.current, m.wordList[m.wordIndex])
	m.typedWords = append(m.typedWords, m.current)

	if m.wordHasMistake {
		m.successCombo = 0
	} else {
		m.successCombo++
		m.bestCombo = max(m.bestCombo, m.successCombo)
		m.xpBonus += game.StreakBonus(m.successCombo)
		if m.successCombo%game.GreenWordStreakInterval == 0 {
			// +2, not +1: m.wordIndex is about to become wordIndex+1 (the
			// next word), so marking that one green would make it
			// current the instant it's assigned -- with zero frames
			// where it's still "upcoming," the green tint would never
			// actually be reachable in the render switch below (the
			// current-word case is checked first). Marking the word
			// *after* next instead gives it one real word's worth of
			// lead time to be seen coming, same as a golden word.
			//
			// A flawless run so far at full HP, where a heal would be
			// wasted, gets a blue (timed shield) word instead. A crawl
			// floor can start below full HP with no mistake yet, and
			// then a heal is what it needs.
			if m.anyMistake || m.hp < m.maxHP {
				m.greenWordIdx = m.wordIndex + 2
			} else {
				m.blueWordIdx = m.wordIndex + 2
			}
		}
	}
	m.wordHasMistake = false

	m.current = ""
	m.wordIndex++
	m.ensureWords()
}

// backspaceIntoPreviousWord reopens the previous word for editing when
// backspace is pressed with nothing left to delete in the current one. It
// undoes everything that word's completion affected -- its timing record,
// streak/combo bookkeeping, and any page flip it triggered -- so retyping
// it plays out exactly as if it had never been completed. It can cascade
// back multiple words if backspace keeps being pressed.
// countSpecialLanded adds one to counter for the special word just
// landed, unless a special word at this position was already counted.
func (m *typingModel) countSpecialLanded(counter *int) {
	if m.specialLandedAt[m.wordIndex] {
		return
	}
	if m.specialLandedAt == nil {
		m.specialLandedAt = make(map[int]bool)
	}
	m.specialLandedAt[m.wordIndex] = true
	*counter++
}

func (m *typingModel) backspaceIntoPreviousWord() {
	if len(m.commits) == 0 || m.wordIndex == 0 {
		return
	}
	last := m.commits[len(m.commits)-1]
	m.commits = m.commits[:len(m.commits)-1]

	m.tracker.RemoveLastWord()
	m.typedWords = m.typedWords[:len(m.typedWords)-1]
	m.wordIndex--
	m.current = last.typed
	m.wordHasMistake = last.hadMistake
	m.successCombo = last.successCombo
	m.bestCombo = last.bestCombo
	m.xpBonus = last.xpBonus
	m.pageStart = last.pageStart
	m.lastKillAt = time.Time{} // the monster is back, so its drop goes
}

// ensureWords keeps a healthy lookahead buffer of generated words so the
// page-wrapping logic always has enough words to fill visibleLines.
func (m *typingModel) ensureWords() {
	if m.mode != modeTime {
		return
	}
	const lookahead = 60
	if len(m.wordList)-m.wordIndex < lookahead {
		m.wordList = append(m.wordList, m.genWords(lookahead, m.wordList...)...)
	}
}

// advancePage moves pageStart forward once the word just completed was the
// last word on the last visible line, so finished lines never scroll away
// mid-page -- the whole page flips at once.
func (m *typingModel) advancePage() {
	_, nextStart := wrapWords(m.wordList, m.pageStart, m.pageWidth(), visibleLines)
	if m.wordIndex >= nextStart {
		m.pageStart = nextStart
	}
}

func (m typingModel) pageWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	// One column is kept for the marker gutter at the start of each line
	// (see View).
	return max(width-1, 1)
}

func (m typingModel) finish() (tea.Model, tea.Cmd) {
	m.finished = true
	result := m.tracker.Finish()
	hp, maxHP, ko := m.hp, m.maxHP, m.ko
	xpBonus, bestCombo := m.xpBonus, m.bestCombo
	mode, target := m.mode, m.target
	charMistakes := m.charMistakes
	pairMistakes := m.pairMistakes
	missedWords := m.missedWords()
	hitCount := m.hitCount
	punctuation, zenMode := m.punctuation, m.zenMode
	quotesMode := m.quotesMode
	custom := m.custom
	wordsTyped := len(m.typedWords)
	ghostWPMSeries := m.ghostWPMSeries
	msg := finishTypingMsg{
		result: result, hp: hp, maxHP: maxHP, ko: ko,
		xpBonus: xpBonus, bestCombo: bestCombo,
		mode: mode, target: target, charMistakes: charMistakes,
		pairMistakes: pairMistakes, missedWords: missedWords,
		floor: m.floor, shieldLeft: m.shieldActive,
		hitCount:    hitCount,
		punctuation: punctuation, zenMode: zenMode, quotes: quotesMode, custom: custom, wordsTyped: wordsTyped,
		ghostWPMSeries: ghostWPMSeries,
		specialLanded:  m.specialLanded,
	}
	if ko && m.battleVisible() {
		// Leave the knocked-out avatar on screen for a moment. The result
		// was already taken above, so the pause isn't counted as typing
		// time.
		delivered := new(atomic.Bool)
		m.pendingFinish = func() tea.Msg {
			if delivered.Swap(true) {
				return nil
			}
			return msg
		}
		deliver := m.pendingFinish
		return m, tea.Tick(koPause, func(time.Time) tea.Msg { return deliver() })
	}
	return m, func() tea.Msg { return msg }
}

// wrapWords greedily packs list[start:] into up to maxLines lines of at
// most width columns each. It returns the word indices per line and the
// index of the first word not included (start of the next page).
func wrapWords(list []string, start, width, maxLines int) (lines [][]int, nextStart int) {
	if width <= 0 {
		width = 80
	}
	var currentLine []int
	lineWidth := 0
	i := start
	for i < len(list) {
		// Columns, not characters: 日 takes two.
		addLen := lipgloss.Width(list[i])
		if lineWidth > 0 {
			addLen++ // separating space
		}
		if lineWidth > 0 && lineWidth+addLen > width {
			lines = append(lines, currentLine)
			if len(lines) == maxLines {
				return lines, i
			}
			currentLine = nil
			lineWidth = 0
			continue
		}
		currentLine = append(currentLine, i)
		lineWidth += addLen
		i++
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines, i
}

func (m typingModel) View() string {
	var header string
	if m.mode == modeTime {
		var elapsed time.Duration
		if m.started {
			elapsed = time.Since(m.tracker.Start)
		}
		remaining := max(time.Duration(m.target)*time.Second-elapsed, 0)
		header = fmt.Sprintf("Time left: %ds", int(remaining.Seconds()+0.999))
	} else {
		header = fmt.Sprintf("Word %d/%d", m.wordIndex+1, m.target)
		if m.floor.Number > 0 {
			header = fmt.Sprintf("Floor %d   %s", m.floor.Number, header)
			if m.floor.Boss {
				header += "   " + wrongStyle.Render("BOSS")
			}
		}
	}
	if m.focusWeak {
		header += "   " + streakStyle.Render("[focus: weak keys]")
	}

	var hpLine string
	if m.zenMode {
		hpLine = dimStyle.Render("Zen Mode -- no HP loss")
	} else {
		hpLine = "HP: " + renderHPBar(m.hp, m.maxHP)
		if now := time.Now(); now.Before(m.comboUntil) && m.comboCount > 0 {
			remaining := m.comboUntil.Sub(now)
			hpLine += "  " + comboStyle.Render(fmt.Sprintf("combo x%d! (%.1fs)", m.comboCount, remaining.Seconds()))
		}
	}
	if m.successCombo > 0 {
		hpLine += "  " + streakStyle.Render(fmt.Sprintf("streak x%d (+%d xp/word)", m.successCombo, game.StreakBonus(m.successCombo)))
	}
	if delta, active := m.ghostStatus(); active {
		if delta >= 0 {
			hpLine += "  " + correctStyle.Render(fmt.Sprintf("ghost: +%.1fs ahead", float64(delta)/1000))
		} else {
			hpLine += "  " + wrongStyle.Render(fmt.Sprintf("ghost: %.1fs behind", float64(-delta)/1000))
		}
	}
	if m.pasteAttempts > 0 {
		hpLine += "  " + warnStyle.Render(fmt.Sprintf("paste blocked (%d) -- type it out", m.pasteAttempts))
	}
	if m.shieldActive {
		hpLine += "  " + goldenWordStyle.Render(goldenWordMarker+" shield active")
	}
	if remaining := time.Until(m.shieldUntil); remaining > 0 {
		hpLine += "  " + blueWordStyle.Render(fmt.Sprintf("%s shield %.1fs", blueWordMarker, remaining.Seconds()))
	}

	lines, _ := wrapWords(m.wordList, m.pageStart, m.pageWidth(), visibleLines)
	var body strings.Builder
	for li, line := range lines {
		for _, idx := range line {
			// The cell before each word -- the gutter for the first word
			// on a line, the separating space for the others -- holds
			// the word's marker if it is a special word. Using a cell
			// that is already there means a word turning special never
			// shifts the text after it. The marker stays while the word
			// is being typed, when the word itself is drawn in the
			// typed/untyped colours instead of its own.
			switch {
			case idx < m.wordIndex:
				body.WriteString(" ")
			case idx == m.goldenWordIdx:
				body.WriteString(goldenWordStyle.Render(goldenWordMarker))
			case idx == m.greenWordIdx:
				body.WriteString(greenWordStyle.Render(greenWordMarker))
			case idx == m.blueWordIdx:
				body.WriteString(blueWordStyle.Render(blueWordMarker))
			default:
				body.WriteString(" ")
			}
			switch {
			case idx < m.wordIndex:
				body.WriteString(renderWord(m.wordList[idx], m.typedWords[idx]))
			case idx == m.wordIndex:
				body.WriteString(renderWord(m.wordList[idx], m.current))
			case idx == m.goldenWordIdx:
				body.WriteString(goldenWordStyle.Render(m.wordList[idx]))
			case idx == m.greenWordIdx:
				body.WriteString(greenWordStyle.Render(m.wordList[idx]))
			case idx == m.blueWordIdx:
				body.WriteString(blueWordStyle.Render(m.wordList[idx]))
			default:
				body.WriteString(dimStyle.Render(m.wordList[idx]))
			}
		}
		if li < len(lines)-1 {
			body.WriteString("\n")
		}
	}

	help := helpStyle.Render("esc: back to menu   ctrl+c: quit")

	parts := []string{headerStyle.Render(header), hpLine, "", body.String(), ""}
	if m.battleVisible() {
		parts = append(parts, renderBattleStrip(m.battleFrame(time.Now())), "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(parts, help)...)
}

// renderHPBar draws a filled/empty block bar plus the numeric HP, colored
// green/yellow/red depending on remaining ratio.
func renderHPBar(hp, maxHP int) string {
	if maxHP <= 0 {
		maxHP = 1
	}
	const width = 20
	filled := min(max(int(float64(hp)/float64(maxHP)*width), 0), width)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s %d/%d", hpStyleFor(hp, maxHP).Render(bar), hp, maxHP)
}

func hpStyleFor(hp, maxHP int) lipgloss.Style {
	ratio := float64(hp) / float64(max(maxHP, 1))
	switch {
	case ratio > 0.5:
		return hpHighStyle
	case ratio > 0.2:
		return hpMidStyle
	default:
		return hpLowStyle
	}
}

func renderWord(target, typed string) string {
	var b strings.Builder
	tr := []rune(target)
	ty := []rune(typed)
	n := max(len(tr), len(ty))
	for i := range n {
		switch {
		case i < len(ty) && i < len(tr):
			if ty[i] == tr[i] {
				b.WriteString(correctStyle.Render(string(tr[i])))
			} else {
				b.WriteString(wrongStyle.Render(string(tr[i])))
			}
		case i < len(tr):
			b.WriteString(string(tr[i]))
		default:
			b.WriteString(wrongStyle.Render(string(ty[i])))
		}
	}
	return b.String()
}
