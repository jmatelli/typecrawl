package app

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/words"
)

const visibleLines = 3

var (
	headerStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	correctStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	wrongStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Underline(true)
	comboStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	streakStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	// goldenWordStyle marks a rare word that grants a mistake-forgiving
	// shield if typed clean. greenWordStyle marks a streak-earned word
	// that heals if typed clean.
	goldenWordStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	greenWordStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
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

	level       int          // player level at test start; sets the word difficulty tier
	punctuation bool         // whether words get sentence-style capitals/punctuation
	zenMode     bool         // disables HP loss/KO for pure practice
	focusWeak   bool         // whether word choice is biased toward weak characters
	weakChars   map[rune]int // profile's known weak characters, for biasing word choice

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
}

func newTypingModel(mode testMode, target, width, maxHP, level int, punctuation, zenMode, focusWeak bool, weakChars map[rune]int, ghostPaceMS []int, ghostWPMSeries []float64) typingModel {
	n := target
	if mode == modeTime {
		n = 300
	}
	wordList := words.ForLevelWeak(n, level, weakChars)
	if punctuation {
		wordList = words.Punctuate(wordList)
	}
	return typingModel{
		mode:           mode,
		target:         target,
		wordList:       wordList,
		typedWords:     make([]string, 0, n),
		tracker:        stats.NewTracker(),
		width:          width,
		level:          level,
		punctuation:    punctuation,
		zenMode:        zenMode,
		focusWeak:      focusWeak,
		weakChars:      weakChars,
		ghostPaceMS:    ghostPaceMS,
		ghostWPMSeries: ghostWPMSeries,
		hp:             maxHP,
		maxHP:          maxHP,
		goldenWordIdx:  rollGoldenWordIndex(len(wordList)),
		greenWordIdx:   -1,
	}
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
				m.current = m.current[:len(m.current)-1]
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
				if !m.typeRune(r) {
					if m.shieldActive {
						// A landed golden word's shield fully absorbs the
						// very next mistake: no HP loss, no streak break.
						// The keystroke is still wrong in the buffer and
						// needs correcting like any other typo -- the
						// shield forgives the penalty, not the typo
						// itself.
						m.shieldActive = false
					} else {
						m.registerMistake()
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
	expected := m.wordList[m.wordIndex]
	pos := len(m.current)
	correct := pos < len(expected) && rune(expected[pos]) == r
	if !correct && pos < len(expected) {
		if m.charMistakes == nil {
			m.charMistakes = make(map[rune]int)
		}
		m.charMistakes[unicode.ToLower(rune(expected[pos]))]++
	}
	m.tracker.RecordChar(correct, m.wordIndex)
	m.current += string(r)
	return correct
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

	// Resolve this exercise's special words, if the one just finished was
	// one of them. Both are one-shot: landed clean or not, the
	// opportunity is spent either way. Consistent with how backspacing
	// across a word boundary already treats HP damage as a permanent sunk
	// cost rather than something to undo, neither effect is reverted if
	// the player later backspaces back into this word.
	if m.wordIndex == m.goldenWordIdx {
		if !m.wordHasMistake {
			m.shieldActive = true
		}
		m.goldenWordIdx = -1
	}
	if m.wordIndex == m.greenWordIdx {
		if !m.wordHasMistake {
			m.hp = min(m.hp+game.GreenWordHealAmount, m.maxHP)
		}
		m.greenWordIdx = -1
	}

	m.commits = append(m.commits, wordCommit{
		typed:        m.current,
		hadMistake:   m.wordHasMistake,
		successCombo: m.successCombo,
		bestCombo:    m.bestCombo,
		xpBonus:      m.xpBonus,
		pageStart:    m.pageStart,
	})

	m.tracker.RecordWordComplete(m.current, m.wordList[m.wordIndex])
	m.typedWords = append(m.typedWords, m.current)

	if m.wordHasMistake {
		m.successCombo = 0
	} else {
		m.successCombo++
		m.bestCombo = max(m.bestCombo, m.successCombo)
		m.xpBonus += game.StreakBonus(m.successCombo)
		if m.successCombo%game.GreenWordStreakInterval == 0 {
			// Mark the word about to become current -- not this one,
			// which is already done.
			m.greenWordIdx = m.wordIndex + 1
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
}

// ensureWords keeps a healthy lookahead buffer of generated words so the
// page-wrapping logic always has enough words to fill visibleLines.
func (m *typingModel) ensureWords() {
	if m.mode != modeTime {
		return
	}
	const lookahead = 60
	if len(m.wordList)-m.wordIndex < lookahead {
		next := words.ForLevelWeak(lookahead, m.level, m.weakChars, m.wordList...)
		if m.punctuation {
			next = words.Punctuate(next)
		}
		m.wordList = append(m.wordList, next...)
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
	if m.width <= 0 {
		return 80
	}
	return m.width
}

func (m typingModel) finish() (tea.Model, tea.Cmd) {
	m.finished = true
	result := m.tracker.Finish()
	hp, maxHP, ko := m.hp, m.maxHP, m.ko
	xpBonus, bestCombo := m.xpBonus, m.bestCombo
	mode, target := m.mode, m.target
	charMistakes := m.charMistakes
	hitCount := m.hitCount
	punctuation, zenMode := m.punctuation, m.zenMode
	wordsTyped := len(m.typedWords)
	ghostWPMSeries := m.ghostWPMSeries
	return m, func() tea.Msg {
		return finishTypingMsg{
			result: result, hp: hp, maxHP: maxHP, ko: ko,
			xpBonus: xpBonus, bestCombo: bestCombo,
			mode: mode, target: target, charMistakes: charMistakes,
			hitCount:    hitCount,
			punctuation: punctuation, zenMode: zenMode, wordsTyped: wordsTyped,
			ghostWPMSeries: ghostWPMSeries,
		}
	}
}

// wrapWords greedily packs list[start:] into up to maxLines lines of at
// most width characters each. It returns the word indices per line and the
// index of the first word not included (start of the next page).
func wrapWords(list []string, start, width, maxLines int) (lines [][]int, nextStart int) {
	if width <= 0 {
		width = 80
	}
	var currentLine []int
	lineWidth := 0
	i := start
	for i < len(list) {
		addLen := len(list[i])
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
		hpLine += "  " + goldenWordStyle.Render("shield active")
	}

	lines, _ := wrapWords(m.wordList, m.pageStart, m.pageWidth(), visibleLines)
	var body strings.Builder
	for li, line := range lines {
		for _, idx := range line {
			switch {
			case idx < m.wordIndex:
				body.WriteString(renderWord(m.wordList[idx], m.typedWords[idx]))
			case idx == m.wordIndex:
				body.WriteString(renderWord(m.wordList[idx], m.current))
			case idx == m.goldenWordIdx:
				body.WriteString(goldenWordStyle.Render(m.wordList[idx]))
			case idx == m.greenWordIdx:
				body.WriteString(greenWordStyle.Render(m.wordList[idx]))
			default:
				body.WriteString(dimStyle.Render(m.wordList[idx]))
			}
			body.WriteString(" ")
		}
		if li < len(lines)-1 {
			body.WriteString("\n")
		}
	}

	help := helpStyle.Render("esc: back to menu   ctrl+c: quit")

	return lipgloss.JoinVertical(lipgloss.Left, headerStyle.Render(header), hpLine, "", body.String(), "", help)
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
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case ratio > 0.2:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
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
