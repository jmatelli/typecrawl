package app

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/achievements"
	"github.com/jmatelli/typecrawl/internal/avatar"
	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/graph"
	"github.com/jmatelli/typecrawl/internal/heatmap"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// profileDeletedMsg is emitted after deleting the profile being viewed --
// there's no longer an active profile, so the root model routes back to
// profile selection.
type profileDeletedMsg struct{}

type profileViewModel struct {
	db              *sql.DB
	profile         *storage.Profile
	activity        map[string]int
	weeks           int
	streakDays      int
	bestOverall     *storage.PersonalBest
	weakChars       []rune
	wpmHistory      []float64
	accuracyHistory []float64
	weekSummary     storage.PeriodSummary
	monthSummary    storage.PeriodSummary
	confirm         string // "", "d", or "r" -- the armed action awaiting confirmation
	errMsg          string
}

func newProfileViewModel(profile *storage.Profile, db *sql.DB, width int) profileViewModel {
	weeks := min(max(width/2-gutterAllowance, 8), 53)

	end := time.Now()
	start := end.AddDate(0, 0, -(weeks * 7))
	activity, _ := storage.ActivityRange(db, profile.ID, start, end) // best-effort; nil renders as all-zero
	streakDays, _ := storage.CurrentStreak(db, profile.ID, end)
	bestOverall, _ := storage.BestOverallWPM(db, profile.ID)
	weakChars, _ := storage.TopWeakChars(db, profile.ID, 5)
	wpmHistory, _ := storage.RecentWPMHistory(db, profile.ID, 30)
	accuracyHistory, _ := storage.RecentAccuracyHistory(db, profile.ID, 30)
	weekSummary, _ := storage.Summary(db, profile.ID, end.AddDate(0, 0, -7), end)
	monthSummary, _ := storage.Summary(db, profile.ID, end.AddDate(0, -1, 0), end)

	return profileViewModel{
		db: db, profile: profile, activity: activity, weeks: weeks, streakDays: streakDays,
		bestOverall: bestOverall, weakChars: weakChars, wpmHistory: wpmHistory, accuracyHistory: accuracyHistory,
		weekSummary: weekSummary, monthSummary: monthSummary,
	}
}

// gutterAllowance accounts for the heatmap's weekday-label gutter so the
// grid itself still fits within the requested width.
const gutterAllowance = 3

func (m profileViewModel) Init() tea.Cmd { return nil }

func (m profileViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	if m.confirm != "" {
		confirmed := key == m.confirm
		action := m.confirm
		m.confirm = ""
		if !confirmed {
			return m, nil
		}
		switch action {
		case "d":
			if err := storage.DeleteProfile(m.db, m.profile.ID); err != nil {
				m.errMsg = "couldn't delete profile"
				return m, nil
			}
			return m, func() tea.Msg { return profileDeletedMsg{} }
		case "r":
			reset, err := storage.ResetProfile(m.db, m.profile.ID)
			if err != nil {
				m.errMsg = "couldn't reset profile"
				return m, nil
			}
			// Mutate in place: profile is the same pointer the root model
			// and menu hold, so this reset is immediately visible there too.
			*m.profile = *reset
			m.activity = map[string]int{} // ResetProfile also cleared these
			m.streakDays = 0
			m.bestOverall = nil
			m.weakChars = nil
			m.wpmHistory = nil
			m.accuracyHistory = nil
			m.weekSummary = storage.PeriodSummary{}
			m.monthSummary = storage.PeriodSummary{}
		}
		return m, nil
	}

	switch key {
	case "esc", "b", "q":
		return m, func() tea.Msg { return restartMsg{} }
	case "a":
		return m, func() tea.Msg { return viewAchievementsMsg{} }
	case "h":
		return m, func() tea.Msg { return viewHistoryMsg{} }
	case "k":
		return m, func() tea.Msg { return viewKeyboardMsg{} }
	case "t":
		ids := unlockedAchievementIDs(m.achievementProgress())
		next := cycleEquippedTitle(m.profile.EquippedTitle, ids)
		if err := storage.SetEquippedTitle(m.db, m.profile.ID, next); err == nil {
			m.profile.EquippedTitle = next
		}
	case "d":
		m.confirm = "d"
		m.errMsg = ""
	case "r":
		m.confirm = "r"
		m.errMsg = ""
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// achievementProgress bundles this profile's current stats into the shape
// the achievements package evaluates against.
func (m profileViewModel) achievementProgress() achievements.Progress {
	var bestWPM float64
	if m.bestOverall != nil {
		bestWPM = m.bestOverall.WPM
	}
	return buildAchievementProgress(m.db, m.profile, m.streakDays, bestWPM)
}

func (m profileViewModel) View() string {
	title := titleStyle.Render("Profile")
	art := avatar.Generate(m.profile.Name)

	needed := game.XPToNextLevel(m.profile.Level)
	bestWPMLine := "Best WPM:     " + dimStyle.Render("no record yet")
	if m.bestOverall != nil {
		bestWPMLine = fmt.Sprintf("Best WPM:     %s (%s %ds)",
			boldStyle.Render(fmt.Sprintf("%.1f", m.bestOverall.WPM)),
			m.bestOverall.Mode, m.bestOverall.Target,
		)
	}
	stats := lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("Name:         %s%s", boldStyle.Render(m.profile.Name), titleSuffix(m.profile.EquippedTitle)),
		fmt.Sprintf("Level:        %s", boldStyle.Render(fmt.Sprintf("%d", m.profile.Level))),
		fmt.Sprintf("XP:           %s", boldStyle.Render(fmt.Sprintf("%d/%d", m.profile.XP, needed))),
		fmt.Sprintf("Max HP:       %s", boldStyle.Render(fmt.Sprintf("%d", game.MaxHP(m.profile.Level)))),
		bestWPMLine,
		fmt.Sprintf("Daily streak: %s", streakStyle.Render(fmt.Sprintf("%d day(s)", m.streakDays))),
		fmt.Sprintf("Best streak:  %s", streakStyle.Render(fmt.Sprintf("%d mistake-free words", m.profile.BestWordStreak))),
		fmt.Sprintf("Successes:    %s   KOs: %s%s",
			correctStyle.Render(fmt.Sprintf("%d", m.profile.SuccessCount)),
			wrongStyle.Render(fmt.Sprintf("%d", m.profile.KOCount)),
			successRateSuffix(m.profile.SuccessCount, m.profile.KOCount),
		),
		fmt.Sprintf("Joined:       %s", m.profile.CreatedAt.Format("2006-01-02")),
	)

	body := lipgloss.JoinHorizontal(lipgloss.Top, art, "    ", stats)
	activityGraph := heatmap.Render(m.activity, m.weeks)

	weakLine := "Weakest keys: " + dimStyle.Render("not enough data yet")
	if len(m.weakChars) > 0 {
		labels := make([]string, len(m.weakChars))
		for i, r := range m.weakChars {
			labels[i] = string(r)
		}
		weakLine = "Weakest keys: " + wrongStyle.Render(strings.Join(labels, "  "))
	}

	graphWidth := m.weeks*2 + gutterAllowance
	if graphWidth < 20 {
		graphWidth = 40
	}
	var trendGraph string
	if len(m.wpmHistory) >= 2 {
		trendGraph = graph.Render("WPM trend (last 30 tests)", m.wpmHistory, nil, graphWidth, 8)
	} else {
		trendGraph = dimStyle.Render("WPM trend: not enough completed tests yet")
	}
	var accuracyTrendGraph string
	if len(m.accuracyHistory) >= 2 {
		accuracyTrendGraph = graph.Render("Accuracy trend (last 30 tests)", m.accuracyHistory, nil, graphWidth, 8)
	} else {
		accuracyTrendGraph = dimStyle.Render("Accuracy trend: not enough completed tests yet")
	}

	weekLine := formatPeriodSummary("This week", m.weekSummary)
	monthLine := formatPeriodSummary("This month", m.monthSummary)

	var confirmLine string
	if m.confirm != "" {
		action := "delete"
		if m.confirm == "r" {
			action = "reset"
		}
		confirmLine = wrongStyle.Render(fmt.Sprintf(
			"press %s again to %s %q, any other key cancels", m.confirm, action, m.profile.Name,
		))
	}

	var errLine string
	if m.errMsg != "" {
		errLine = wrongStyle.Render(m.errMsg)
	}

	progress := m.achievementProgress()
	achievementsLine := fmt.Sprintf(
		"Achievements: %s   (a to view all)",
		boldStyle.Render(fmt.Sprintf("%d/%d unlocked", achievements.UnlockedCount(progress), len(achievements.All))),
	)

	var almostLine string
	if nearest := achievements.Nearest(progress, 3); len(nearest) > 0 {
		parts := make([]string, len(nearest))
		for i, lp := range nearest {
			parts[i] = fmt.Sprintf("%s (%s)", boldStyle.Render(lp.Achievement.Name), formatNudgeProgress(lp))
		}
		almostLine = "Almost there:  " + strings.Join(parts, "   ")
	}

	help := helpStyle.Render("esc: back to menu   a: achievements   h: history   k: weak keys   t: cycle title   r: reset   d: delete   ctrl+c: quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title, "", body, "",
		weakLine, "",
		trendGraph, "",
		accuracyTrendGraph, "",
		activityGraph, "",
		weekLine, monthLine, "",
		achievementsLine, almostLine, "",
		confirmLine, errLine, help,
	)
}

// formatPeriodSummary renders one "This week: 5 exercises, avg 68 WPM,
// best day Tuesday (3)" style line, or a dim placeholder if s has no
// activity at all in its range.
func formatPeriodSummary(label string, s storage.PeriodSummary) string {
	if s.Exercises == 0 {
		return fmt.Sprintf("%s: ", label) + dimStyle.Render("no exercises yet")
	}
	bestDay := "-"
	if s.BestDay != "" {
		if t, err := time.Parse("2006-01-02", s.BestDay); err == nil {
			bestDay = t.Weekday().String()
		} else {
			bestDay = s.BestDay
		}
	}
	return fmt.Sprintf("%s: %s exercises, avg %s WPM, best day %s",
		label,
		boldStyle.Render(fmt.Sprintf("%d", s.Exercises)),
		boldStyle.Render(fmt.Sprintf("%.0f", s.AvgWPM)),
		boldStyle.Render(fmt.Sprintf("%s (%d)", bestDay, s.BestDayCount)),
	)
}

// formatNudgeProgress renders a locked achievement's current/target as a
// short "7/10" style fragment. Time-played achievements are shown in
// hours rather than raw seconds, since that's how their thresholds read
// on the achievements page.
func formatNudgeProgress(lp achievements.LockedProgress) string {
	if strings.HasPrefix(lp.Achievement.ID, "time_") {
		return fmt.Sprintf("%.1fh/%.0fh", lp.Current/3600, lp.Target/3600)
	}
	return fmt.Sprintf("%.0f/%.0f", lp.Current, lp.Target)
}

// successRateSuffix formats a "(N% success rate)" suffix, or an empty
// string if no exercises have been completed yet to compute a rate from.
func successRateSuffix(successes, kos int) string {
	total := successes + kos
	if total == 0 {
		return ""
	}
	rate := float64(successes) / float64(total) * 100
	return dimStyle.Render(fmt.Sprintf("  (%.0f%% success rate)", rate))
}
