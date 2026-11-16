package app

import (
	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/textsource"
)

type startTypingMsg struct {
	mode        testMode
	target      int
	punctuation bool
	zenMode     bool
	focusWeak   bool
	quotes      bool
	custom      textsource.Source // a player's word list or text, or a drill; overrides quotes when set
	drillWords  []string          // a drill's words, in order, when custom is a drill
	// crawl starts a new crawl instead, at floor 1 with full HP; of the
	// rest only punctuation applies.
	crawl bool
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
	charMistakes map[rune]int      // this run's per-character mistake counts
	pairMistakes map[string]int    // this run's mistakes by key pair (see typingModel.pairMistakes)
	missedWords  []string          // the words this run got wrong, each once, for a drill
	floor        game.CrawlFloor   // the crawl floor this run was; Number 0 outside a crawl
	shieldLeft   bool              // a golden shield still unused at the end, which a crawl carries to the next floor
	hitCount     int               // how many mistakes actually dealt (or would deal) HP damage
	punctuation  bool              // whether punctuation was enabled this run
	zenMode      bool              // whether zen mode was enabled this run
	quotes       bool              // whether this run used quotes mode instead of random words
	custom       textsource.Source // the player's own file the run used, if any
	wordsTyped   int               // how many words were completed this run

	// ghostWPMSeries is the personal best's WPM curve this run was raced
	// against (loaded at start, before this run could have changed it),
	// for the results screen to overlay on its own graph. nil means there
	// was no ghost to race.
	ghostWPMSeries []float64

	specialLanded specialWords // special words landed clean this run
}

// source is what the run typed from: a crawl floor, or as runSource says.
// Saving a run and summarizing it both go through it, so they agree.
func (msg finishTypingMsg) source() textsource.Source {
	if msg.floor.Number > 0 {
		return textsource.Source{Kind: textsource.Crawl}
	}
	return runSource(msg.quotes, msg.custom)
}

// specialWords counts golden, green and blue words.
type specialWords struct{ golden, green, blue int }

type restartMsg struct{}

// startDrillMsg asks for a drill of words, the ones the run on the results
// screen missed.
type startDrillMsg struct{ words []string }

// nextFloorMsg asks for the next floor of the crawl in progress.
type nextFloorMsg struct{}

type viewProfileMsg struct{}
type switchProfileMsg struct{}
type viewHistoryMsg struct{}
type viewKeyboardMsg struct{}
