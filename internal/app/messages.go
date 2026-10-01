package app

import "github.com/jmatelli/typecrawl/internal/stats"

type startTypingMsg struct {
	mode        testMode
	target      int
	punctuation bool
	zenMode     bool
	focusWeak   bool
	quotes      bool
}

type finishTypingMsg struct {
	result       stats.Result
	hp           int
	maxHP        int
	ko           bool
	xpBonus      int // bonus XP earned from the mistake-free word streak
	bestCombo    int // longest mistake-free word streak reached
	mode         testMode
	target       int
	charMistakes map[rune]int // this run's per-character mistake counts
	hitCount     int          // how many mistakes actually dealt (or would deal) HP damage
	punctuation  bool         // whether punctuation was enabled this run
	zenMode      bool         // whether zen mode was enabled this run
	wordsTyped   int          // how many words were completed this run

	// ghostWPMSeries is the personal best's WPM curve this run was raced
	// against (loaded at start, before this run could have changed it),
	// for the results screen to overlay on its own graph. nil means there
	// was no ghost to race.
	ghostWPMSeries []float64
}

type restartMsg struct{}

type viewProfileMsg struct{}
type switchProfileMsg struct{}
type viewHistoryMsg struct{}
type viewKeyboardMsg struct{}
