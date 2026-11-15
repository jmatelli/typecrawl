package app

import (
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/avatar"
	"github.com/jmatelli/typecrawl/internal/theme"
)

// The battle strip is drawn below the words in the typing view: the
// profile's avatar on the left, holding a sword, and the current word on
// the right as a monster that loses a letter for every one typed. It is
// only decoration. Nothing in it feeds back into the run, and it hides
// itself when the terminal is too small to fit it (see battleVisible).
//
// Layout, in columns, with the avatar at rest:
//
//	 2  avatar (14)  1  shield (1)  1  sword (10)   gap (8)   monster
//	|--|------------|-|-|-|----------|--------|---------...
//
// Every row is built left to right from those pieces, so nothing in the
// strip ever moves the words above it.
const (
	stripHeight       = avatar.Size
	stripMargin       = 2  // columns left of the avatar at rest
	stripLunge        = 2  // how far the avatar steps in on a correct letter
	stripRestGap      = 8  // columns between the sword tip and the monster at rest
	stripApproach     = 6  // how much further away a new monster starts
	maxMonsterLetters = 12 // longer words are trimmed with an ellipsis

	avatarWidth = avatar.Size * 2
	swordRow    = 3 // the avatar's middle row
	dropRow     = 1 // where a defeated monster's drop is shown
	monsterRow  = 2 // the monster's top row; its letters are on the next

	// The longest the strip gets: the monster at its furthest (walking
	// in) and widest (maxMonsterLetters letters, spaced, plus its sides).
	stripWidth = stripMargin + avatarWidth + 1 + 1 + 1 + swordWidth + stripRestGap + stripApproach + (2*maxMonsterLetters - 1) + 4

	lungeFor    = 150 * time.Millisecond
	hitFor      = 250 * time.Millisecond
	approachFor = 300 * time.Millisecond
	dropFor     = 700 * time.Millisecond
	// koPause keeps the knocked-out frame on screen before the results.
	// The result itself is taken at the moment of the knockout, so the
	// pause doesn't count towards the run's time.
	koPause = 1200 * time.Millisecond
)

const (
	sword      = "──╋━━━━━━▶"
	swordWidth = 10 // columns; checked against sword in the tests
)

var (
	// hitStyle flashes the avatar when the monster lands a hit. Without
	// colour it goes faint instead.
	hitStyle     = theme.Pick(lipgloss.NewStyle().Foreground(theme.Color("1")).Bold(true), lipgloss.NewStyle().Faint(true))
	monsterStyle = lipgloss.NewStyle()
	dropStyle    = lipgloss.NewStyle().Faint(true)
)

type battlePose int

const (
	poseIdle  battlePose = iota
	poseLunge            // a correct letter: the avatar steps in and cuts
	poseHit              // a mistake: the monster hits back
	poseBlock            // a mistake a shield absorbed
	poseKO               // out of HP
)

// battleFrame is everything one frame of the strip shows.
type battleFrame struct {
	avatar      avatar.Avatar
	pose        battlePose
	hitStrength int // 1 for a single mistake, 2 for a combo

	word         string // the whole current word
	cut          int    // how many of its letters are already cut away
	monsterStyle lipgloss.Style
	monsterSide  string // drawn on both sides of the letters
	approach     int    // extra columns while a new monster walks in

	shieldMarker string // "" when there's no shield in front of the avatar
	shieldStyle  lipgloss.Style

	drop      string // shown above the monster after a defeat, "" for none
	dropStyle lipgloss.Style
}

// stripRow assembles one row of the strip from pieces placed at given
// columns, left to right.
type stripRow struct {
	b   strings.Builder
	col int
}

func (r *stripRow) put(col int, s string, width int) {
	if col > r.col {
		r.b.WriteString(strings.Repeat(" ", col-r.col))
		r.col = col
	}
	r.b.WriteString(s)
	r.col += width
}

func monsterLetters(word string, cut int) string {
	rest := []rune(word)[min(cut, len([]rune(word))):]
	if len(rest) == 0 {
		return "x_x" // every letter cut; one press of space finishes it
	}
	if len(rest) > maxMonsterLetters {
		rest = append(rest[:maxMonsterLetters-1], '…')
	}
	parts := make([]string, len(rest))
	for i, r := range rest {
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

func renderBattleStrip(f battleFrame) string {
	rows := make([]stripRow, stripHeight)

	// The avatar, drawn exactly as on the profile page unless it's
	// flashing from a hit or knocked out.
	avatarCol := stripMargin
	avatarStyle := f.avatar.Style()
	switch f.pose {
	case poseLunge:
		avatarCol += stripLunge
	case poseHit:
		avatarCol -= min(max(f.hitStrength, 1), stripMargin)
		avatarStyle = hitStyle
	case poseKO:
		avatarStyle = dropStyle
	}
	for y, line := range f.avatar.Rows(avatarStyle) {
		col := avatarCol
		if f.pose == poseKO && y%2 == 1 {
			col++ // knocked apart
		}
		rows[y].put(col, line, avatarWidth)
	}

	shieldCol := avatarCol + avatarWidth + 1
	if f.shieldMarker != "" && f.pose != poseKO {
		rows[1].put(shieldCol, f.shieldStyle.Render(f.shieldMarker), 1)
		for y := 2; y <= 5; y++ {
			rows[y].put(shieldCol, f.shieldStyle.Render(")"), 1)
		}
	}

	swordCol := shieldCol + 2
	if f.pose == poseKO {
		rows[stripHeight-1].put(swordCol, dropStyle.Render(sword), swordWidth) // dropped
	} else {
		rows[swordRow].put(swordCol, sword, swordWidth)
	}

	// The monster stays put while the avatar lunges, and comes at the
	// avatar when it attacks.
	restCol := stripMargin + avatarWidth + 1 + 1 + 1 + swordWidth + stripRestGap
	monsterCol := restCol + f.approach
	if f.pose == poseHit || f.pose == poseBlock {
		monsterCol -= 2 * max(f.hitStrength, 1)
	}
	letters := monsterLetters(f.word, f.cut)
	w := len([]rune(letters))
	rows[monsterRow].put(monsterCol, f.monsterStyle.Render(" ."+strings.Repeat("-", w)+"."), w+3)
	rows[monsterRow+1].put(monsterCol, f.monsterStyle.Render(f.monsterSide+" "+letters+" "+f.monsterSide), w+4)
	rows[monsterRow+2].put(monsterCol, f.monsterStyle.Render(" /"+strings.Repeat("^", w)+"\\"), w+3)

	if f.drop != "" {
		rows[dropRow].put(restCol, f.dropStyle.Render(f.drop), len([]rune(f.drop)))
	}

	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].b.String()
	}
	return strings.Join(out, "\n")
}

// typingLines is how many lines the typing view takes without the strip:
// header, HP line, blank, the words, blank, help.
const typingLines = 3 + visibleLines + 2

// battleVisible reports whether the strip is drawn: the player hasn't
// turned it off, and the terminal is big enough for the whole view to fit
// with it, so it never pushes the words off screen.
func (m typingModel) battleVisible() bool {
	return m.battle && m.height >= typingLines+1+stripHeight && m.width >= stripWidth
}

// battleFrame works out what the strip shows right now from the run's
// state and the times of the last few events.
func (m typingModel) battleFrame(now time.Time) battleFrame {
	f := battleFrame{
		avatar:       m.avatar,
		monsterStyle: monsterStyle,
		monsterSide:  "|",
	}
	if m.wordIndex < len(m.wordList) {
		f.word = m.wordList[m.wordIndex]
		f.cut = correctPrefix(f.word, m.current)
	}
	switch m.wordIndex {
	case m.goldenWordIdx:
		f.monsterStyle, f.monsterSide = goldenWordStyle, goldenWordMarker
	case m.greenWordIdx:
		f.monsterStyle, f.monsterSide = greenWordStyle, greenWordMarker
	case m.blueWordIdx:
		f.monsterStyle, f.monsterSide = blueWordStyle, blueWordMarker
	}

	switch {
	case m.timedShieldActive():
		f.shieldMarker, f.shieldStyle = blueWordMarker, blueWordStyle
	case m.shieldActive:
		f.shieldMarker, f.shieldStyle = goldenWordMarker, goldenWordStyle
	}

	hitAgo, cutAgo := now.Sub(m.lastHitAt), now.Sub(m.lastCutAt)
	switch {
	case m.ko:
		f.pose = poseKO
	case !m.lastHitAt.IsZero() && hitAgo < hitFor && (!m.lastHitAt.Before(m.lastCutAt) || cutAgo >= lungeFor):
		f.pose, f.hitStrength = poseHit, m.lastHitStrength
		if m.lastHitBlockedBy != "" {
			// Show the shield that took the hit, even if that used it up.
			f.pose = poseBlock
			f.shieldMarker, f.shieldStyle = m.lastHitBlockedBy, m.lastHitBlockedStyle
		}
	case !m.lastCutAt.IsZero() && cutAgo < lungeFor:
		f.pose = poseLunge
	}

	if killAgo := now.Sub(m.lastKillAt); !m.lastKillAt.IsZero() && killAgo < dropFor {
		f.drop, f.dropStyle = m.lastDrop, m.lastDropStyle
		if killAgo < approachFor {
			f.approach = int(math.Ceil(stripApproach * (1 - float64(killAgo)/float64(approachFor))))
		}
	}
	return f
}

// correctPrefix counts the letters of typed that match word from the
// start, which is how many the avatar has cut away. A typo stops the
// count until it is backspaced, and backspacing a correct letter puts it
// back on the monster.
func correctPrefix(word, typed string) int {
	wr, tr := []rune(word), []rune(typed)
	n := 0
	for n < len(wr) && n < len(tr) && wr[n] == tr[n] {
		n++
	}
	return n
}
