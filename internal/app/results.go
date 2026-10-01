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

	// ghostWPMSeries is the personal best's WPM curve this run raced
	// against, overlaid on the WPM-over-time graph. nil if there was none.
	ghostWPMSeries []float64

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
	switch keyMsg.String() {
	case "enter", "r":
		return m, func() tea.Msg { return restartMsg{} }
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

	help := helpStyle.Render("enter: restart   q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, "", statsLine, xpLine, comboLine, streakLine, challengeLine, hpLine, "", wpmGraph, "", burstGraph, "", help)
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
	graphWidth := m.width
	if graphWidth < 20 {
		graphWidth = 80
	}
	wpmGraph := graph.Render("WPM over time", m.result.WPMSeries, m.result.ErrorWords, graphWidth, 10)
	burstGraph := graph.Render("Burst", m.result.BurstSeries, m.result.ErrorWords, graphWidth, 10)
	help := helpStyle.Render("enter: restart   q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, "", statsLine, warning, "", wpmGraph, "", burstGraph, "", help)
}
