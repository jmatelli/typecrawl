package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/graph"
	"github.com/jmatelli/typecrawl/internal/stats"
)

// resultsExtra carries the gamification outcome of a completed test:
// XP/level progress and how the HP pool held up.
type resultsExtra struct {
	xpGained              int
	xpBase                int
	xpBonus               int
	streakDays            int // consecutive days of practice, including today
	streakBonusXP         int // extra XP from the daily streak multiplier
	bestCombo             int // this run's longest mistake-free word streak
	allTimeBestWordStreak int // the profile's all-time record
	newWordStreakRecord   bool
	koCount               int // this profile's all-time total knockouts
	isNewPB               bool
	personalBest          float64 // best WPM for this mode/target after this run
	oldLevel              int
	newLevel              int
	levelsGained          int
	hp                    int
	maxHP                 int
	ko                    bool
	hitCount              int // how many mistakes dealt HP damage this run

	challengeCompleted   bool   // whether this run just claimed today's daily challenge bonus
	challengeBonusXP     int    // XP awarded for that (0 if not just completed)
	challengeDescription string // today's challenge description, shown regardless of completion

	weeklyBonusXP     int    // XP for completing this week's challenge with this run (0 if it didn't)
	weeklyDescription string // this week's challenge description

	// ghostWPMSeries is the personal best's WPM curve this run raced
	// against, overlaid on the WPM-over-time graph. nil if there was none.
	ghostWPMSeries []float64

	// missedWords are the words this run got wrong, which d drills. Empty
	// for a run that missed none, and for an unverified one.
	missedWords []string

	// floor is the crawl floor this run was (Number 0 outside a crawl),
	// floorBonusXP what clearing it paid, nextFloor the floor it leads on
	// to (0 once the crawl is over), and deepestFloor the profile's record.
	floor        game.CrawlFloor
	floorBonusXP int
	nextFloor    int
	deepestFloor int

	// unverified marks a run game.IsSuspiciousRun flagged as more likely
	// scripted/injected input than genuine typing -- its numbers are still
	// shown, but nothing else in extra is populated, since none of it was
	// actually persisted.
	unverified bool
}

type resultsModel struct {
	result stats.Result
	width  int
	extra  resultsExtra
	shared bool // the share key was pressed, so the view says where it went
}

func newResultsModel(result stats.Result, width int, extra resultsExtra) resultsModel {
	return resultsModel{result: result, width: width, extra: extra}
}

func (m resultsModel) Init() tea.Cmd { return nil }

func (m resultsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.extra.nextFloor > 0 {
		// Between floors: on down, or out of the crawl.
		switch keyMsg.String() {
		case "enter":
			return m, func() tea.Msg { return nextFloorMsg{} }
		case "esc":
			return m, func() tea.Msg { return restartMsg{} }
		}
	}
	switch keyMsg.String() {
	case "enter", "r":
		return m, func() tea.Msg { return restartMsg{} }
	case "s":
		m.shared = true
		return m, func() tea.Msg { return shareRunMsg{} }
	case "d":
		if missed := m.extra.missedWords; len(missed) > 0 {
			return m, func() tea.Msg { return startDrillMsg{words: missed} }
		}
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m resultsModel) View() string {
	if m.extra.unverified {
		return m.unverifiedView()
	}

	statsLine := fmt.Sprintf(
		"WPM: %s    Accuracy: %s    Time: %s",
		boldStyle.Render(fmt.Sprintf("%.1f", m.result.WPM)),
		boldStyle.Render(fmt.Sprintf("%.1f%%", m.result.Accuracy)),
		boldStyle.Render(m.result.Duration.Round(1e8).String()),
	)
	if !m.extra.ko {
		if m.extra.isNewPB {
			statsLine += "   " + levelUpStyle.Render("NEW PERSONAL BEST!")
		} else if m.extra.personalBest > 0 {
			statsLine += fmt.Sprintf("   (personal best: %s WPM)", boldStyle.Render(fmt.Sprintf("%.1f", m.extra.personalBest)))
		}
	}
	var xpLine string
	if m.extra.ko {
		xpLine = wrongStyle.Render("XP: +0 (knocked out -- no XP earned)")
	} else {
		xpLine = fmt.Sprintf("XP: %s", boldStyle.Render(fmt.Sprintf("+%d", m.extra.xpGained)))
		var parts []string
		if m.extra.xpBonus > 0 {
			parts = append(parts, fmt.Sprintf("%d base + %s combo", m.extra.xpBase, streakStyle.Render(fmt.Sprintf("+%d", m.extra.xpBonus))))
		}
		if m.extra.streakBonusXP > 0 {
			parts = append(parts, streakStyle.Render(fmt.Sprintf("+%d daily streak", m.extra.streakBonusXP)))
		}
		if m.extra.challengeBonusXP > 0 {
			parts = append(parts, streakStyle.Render(fmt.Sprintf("+%d daily challenge", m.extra.challengeBonusXP)))
		}
		if m.extra.weeklyBonusXP > 0 {
			parts = append(parts, streakStyle.Render(fmt.Sprintf("+%d weekly challenge", m.extra.weeklyBonusXP)))
		}
		if m.extra.floorBonusXP > 0 {
			parts = append(parts, streakStyle.Render(fmt.Sprintf("+%d floor %d", m.extra.floorBonusXP, m.extra.floor.Number)))
		}
		if len(parts) > 0 {
			xpLine += " (" + strings.Join(parts, " + ") + ")"
		}
	}
	if m.extra.levelsGained > 0 {
		xpLine += "   " + levelUpStyle.Render(fmt.Sprintf("LEVEL UP! %d -> %d", m.extra.oldLevel, m.extra.newLevel))
	} else {
		xpLine += fmt.Sprintf("   Level %d", m.extra.newLevel)
	}

	comboLine := fmt.Sprintf("This run's word streak: %s mistake-free words", boldStyle.Render(fmt.Sprintf("%d", m.extra.bestCombo)))
	if m.extra.newWordStreakRecord {
		comboLine += "   " + levelUpStyle.Render("NEW RECORD!")
	} else {
		comboLine += fmt.Sprintf("   (all-time best: %s)", boldStyle.Render(fmt.Sprintf("%d", m.extra.allTimeBestWordStreak)))
	}
	var challengeLine string
	if m.extra.challengeDescription != "" {
		if m.extra.challengeCompleted {
			challengeLine = levelUpStyle.Render("Daily challenge complete!") + " " + m.extra.challengeDescription
		} else {
			challengeLine = dimStyle.Render("Today's challenge: " + m.extra.challengeDescription)
		}
	}
	if m.extra.weeklyBonusXP > 0 {
		challengeLine = lipgloss.JoinVertical(lipgloss.Left, challengeLine,
			levelUpStyle.Render("Weekly challenge complete!")+" "+m.extra.weeklyDescription)
	}

	streakLine := fmt.Sprintf(
		"Daily streak: %s (%s XP multiplier)",
		boldStyle.Render(fmt.Sprintf("%d day(s)", m.extra.streakDays)),
		boldStyle.Render(fmt.Sprintf("x%.2f", game.DailyStreakMultiplier(m.extra.streakDays))),
	)

	hpLine := "HP: " + renderHPBar(m.extra.hp, m.extra.maxHP)
	hpLine += "  " + wrongStyle.Render(fmt.Sprintf("hits taken: %d", m.extra.hitCount))
	if m.extra.ko {
		hpLine += "  " + wrongStyle.Render(fmt.Sprintf("KO! you ran out of HP (KO #%d)", m.extra.koCount))
	}

	header := titleStyle.Render("Results")
	if line := m.floorLine(); line != "" {
		header = lipgloss.JoinVertical(lipgloss.Left, header, line)
	}

	graphWidth := m.width
	if graphWidth < 20 {
		graphWidth = 80
	}

	var wpmGraph string
	if len(m.extra.ghostWPMSeries) >= 2 {
		wpmGraph = graph.RenderCompare("WPM over time", m.result.WPMSeries, m.extra.ghostWPMSeries, m.result.ErrorWords, graphWidth, 10)
	} else {
		wpmGraph = graph.Render("WPM over time", m.result.WPMSeries, m.result.ErrorWords, graphWidth, 10)
	}
	burstGraph := graph.Render("Burst", m.result.BurstSeries, m.result.ErrorWords, graphWidth, 10)

	return lipgloss.JoinVertical(lipgloss.Left, header, "", statsLine, xpLine, comboLine, streakLine, challengeLine, hpLine, "", wpmGraph, "", burstGraph, "", m.footer())
}

// unverifiedView renders a flagged run's raw numbers without any of the
// gamification lines -- none of that state was actually touched, since
// game.IsSuspiciousRun caught it before anything was persisted.
func (m resultsModel) unverifiedView() string {
	statsLine := fmt.Sprintf(
		"WPM: %s    Accuracy: %s    Time: %s",
		boldStyle.Render(fmt.Sprintf("%.1f", m.result.WPM)),
		boldStyle.Render(fmt.Sprintf("%.1f%%", m.result.Accuracy)),
		boldStyle.Render(m.result.Duration.Round(1e8).String()),
	)
	warning := warnStyle.Render("Unverified run -- this looked more like scripted input than hand-typing, so it wasn't counted towards XP, personal bests, or achievements.")

	header := titleStyle.Render("Results")
	if line := m.floorLine(); line != "" {
		header = lipgloss.JoinVertical(lipgloss.Left, header, line)
	}
	graphWidth := m.width
	if graphWidth < 20 {
		graphWidth = 80
	}
	wpmGraph := graph.Render("WPM over time", m.result.WPMSeries, m.result.ErrorWords, graphWidth, 10)
	burstGraph := graph.Render("Burst", m.result.BurstSeries, m.result.ErrorWords, graphWidth, 10)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", statsLine, warning, "", wpmGraph, "", burstGraph, "", m.footer())
}

// footer is the key help, with a note once the run has been shared. The
// note doesn't claim the copy worked, since a terminal that ignores the
// clipboard sequence gives no sign of it.
// floorLine says how a crawl floor went, or "" outside a crawl.
func (m resultsModel) floorLine() string {
	f := m.extra.floor
	if f.Number == 0 {
		return ""
	}
	name := fmt.Sprintf("Floor %d", f.Number)
	if f.Boss {
		name = fmt.Sprintf("Floor %d boss", f.Number)
	}
	if m.extra.nextFloor > 0 {
		return levelUpStyle.Render(name+" cleared!") + fmt.Sprintf("   HP %d/%d carries down to floor %d.", m.extra.hp, m.extra.maxHP, m.extra.nextFloor)
	}
	return wrongStyle.Render(fmt.Sprintf("Knocked out on %s.", strings.ToLower(name[:1])+name[1:])) +
		fmt.Sprintf("   The crawl is over. Deepest floor: %d.", m.extra.deepestFloor)
}

func (m resultsModel) footer() string {
	if m.extra.nextFloor > 0 {
		next := game.FloorOf(m.extra.nextFloor, 1)
		what := fmt.Sprintf("descend to floor %d", m.extra.nextFloor)
		if next.Boss {
			what += " (boss)"
		}
		return m.withSharedNote(helpStyle.Render("enter: " + what + "   esc: leave the crawl   s: share   q: quit"))
	}
	keys := "enter: restart   s: share   q: quit"
	if n := min(len(m.extra.missedWords), drillMaxWords); n > 0 {
		plural := "s"
		if n == 1 {
			plural = ""
		}
		what := fmt.Sprintf("the %d word%s you missed", n, plural)
		if len(m.extra.missedWords) > n {
			what = fmt.Sprintf("the first %d of the %d words you missed", n, len(m.extra.missedWords))
		}
		keys = "enter: restart   d: drill " + what + "   s: share   q: quit"
	}
	return m.withSharedNote(helpStyle.Render(keys))
}

// withSharedNote puts the note about sharing above help, once the run
// was shared.
func (m resultsModel) withSharedNote(help string) string {
	if !m.shared {
		return help
	}
	return lipgloss.JoinVertical(lipgloss.Left, dimStyle.Render("summary sent to the clipboard; it's also printed when you quit"), help)
}
