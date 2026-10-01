package app

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/achievements"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// viewAchievementsMsg switches from the profile page to the full
// achievements list.
type viewAchievementsMsg struct{}

// unlockedAchievementIDs returns the IDs of every achievement p has
// unlocked, in achievements.All's order.
func unlockedAchievementIDs(p achievements.Progress) []string {
	var ids []string
	for _, a := range achievements.All {
		if a.Unlocked(p) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// cycleEquippedTitle returns the next title to equip after current: ""
// (no title), then every ID in unlockedIDs, wrapping around. If current
// isn't found (e.g. stale after a profile reset), it just restarts the
// cycle from the top.
func cycleEquippedTitle(current string, unlockedIDs []string) string {
	options := append([]string{""}, unlockedIDs...)
	for i, id := range options {
		if id == current {
			return options[(i+1)%len(options)]
		}
	}
	return ""
}

// titleName returns the display name for an equipped-title achievement ID,
// or "" if id is empty or unrecognized (e.g. stale after a profile reset).
func titleName(id string) string {
	if id == "" {
		return ""
	}
	for _, a := range achievements.All {
		if a.ID == id {
			return a.Name
		}
	}
	return ""
}

// titleSuffix renders an equipped title for display next to a profile's
// name, or "" if none is equipped (or it no longer resolves to a real
// achievement).
func titleSuffix(equippedID string) string {
	name := titleName(equippedID)
	if name == "" {
		return ""
	}
	return "  " + streakStyle.Render("["+name+"]")
}

// tierStyle colors an achievement's tier badge -- bronze/silver/gold/
// diamond, in ascending order of how impressive it is.
func tierStyle(t achievements.Tier) lipgloss.Style {
	switch t {
	case achievements.Bronze:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("130")).Bold(true) // copper/bronze
	case achievements.Silver:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("251")).Bold(true) // light gray
	case achievements.Gold:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true) // yellow-gold
	case achievements.Diamond:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // bright cyan
	case achievements.Master:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Bold(true) // purple
	case achievements.Grandmaster:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true) // bright magenta
	default:
		return boldStyle
	}
}

// buildAchievementProgress bundles a profile's stats into the shape the
// achievements package evaluates against. Shared by the profile page, the
// achievements list, and the finish-of-exercise unlock check, so all three
// agree on exactly what "current progress" means. The extra queries are
// best-effort: a failure just means that one flag reads false, same as not
// having earned it yet, rather than blocking the rest of the snapshot.
func buildAchievementProgress(db *sql.DB, profile *storage.Profile, dailyStreak int, bestWPM float64) achievements.Progress {
	perfectRunCount, _ := storage.PerfectRunCount(db, profile.ID)
	maxPerfectStreak, _ := storage.MaxPerfectAccuracyStreak(db, profile.ID)
	completedLongHaul, _ := storage.HasCompletedTestConfig(db, profile.ID, "words", 200)
	completedSprint, _ := storage.HasCompletedTestConfig(db, profile.ID, "time", 15)
	lateNight, _ := storage.HasTestInHourRange(db, profile.ID, 0, 4)
	earlyMorning, _ := storage.HasTestInHourRange(db, profile.ID, 4, 7)

	hasBestInBothModes, _ := storage.HasPersonalBestInBothModes(db, profile.ID)
	practicedAllWeekdays, _ := storage.HasPracticedAllWeekdays(db, profile.ID)
	hadBigDay, _ := storage.HasDayWithAtLeast(db, profile.ID, 10)
	completedUltraLongHaul, _ := storage.HasCompletedWordsAtLeast(db, profile.ID, 500)
	comebackCount, _ := storage.ComebackCount(db, profile.ID)
	hadPerfectComeback, _ := storage.HasPerfectComebackAfterKO(db, profile.ID)
	punctuationRunCount, _ := storage.PunctuationRunCount(db, profile.ID)
	zenRunCount, _ := storage.ZenRunCount(db, profile.ID)
	totalWordsTyped, _ := storage.TotalWordsTyped(db, profile.ID)
	totalPlayTime, _ := storage.TotalPlayTime(db, profile.ID)

	return achievements.Progress{
		Level:          profile.Level,
		TotalExercises: profile.SuccessCount + profile.KOCount,
		BestWordStreak: profile.BestWordStreak,
		KOCount:        profile.KOCount,
		DailyStreak:    dailyStreak,
		BestWPM:        bestWPM,

		PerfectRunCount:       perfectRunCount,
		MaxPerfectStreak:      maxPerfectStreak,
		CompletedLongHaul:     completedLongHaul,
		CompletedSprint:       completedSprint,
		PracticedLateNight:    lateNight,
		PracticedEarlyMorning: earlyMorning,

		HasBestInBothModes:     hasBestInBothModes,
		PracticedAllWeekdays:   practicedAllWeekdays,
		HadBigDay:              hadBigDay,
		CompletedUltraLongHaul: completedUltraLongHaul,
		ComebackCount:          comebackCount,
		HadPerfectComeback:     hadPerfectComeback,
		PunctuationRunCount:    punctuationRunCount,
		ZenRunCount:            zenRunCount,
		TotalWordsTyped:        totalWordsTyped,
		TotalPlaySeconds:       int(totalPlayTime.Seconds()),
	}
}

// newlyUnlocked returns the achievements present in after but not before,
// in achievements.All's order.
func newlyUnlocked(before, after achievements.Progress) []achievements.Achievement {
	var out []achievements.Achievement
	for _, a := range achievements.All {
		if !a.Unlocked(before) && a.Unlocked(after) {
			out = append(out, a)
		}
	}
	return out
}

type achievementsViewModel struct {
	profile  *storage.Profile
	progress achievements.Progress
}

func newAchievementsViewModel(profile *storage.Profile, db *sql.DB) achievementsViewModel {
	streakDays, _ := storage.CurrentStreak(db, profile.ID, time.Now())
	bestOverall, _ := storage.BestOverallWPM(db, profile.ID)

	var bestWPM float64
	if bestOverall != nil {
		bestWPM = bestOverall.WPM
	}

	return achievementsViewModel{
		profile:  profile,
		progress: buildAchievementProgress(db, profile, streakDays, bestWPM),
	}
}

func (m achievementsViewModel) Init() tea.Cmd { return nil }

func (m achievementsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc", "b", "q":
		return m, func() tea.Msg { return viewProfileMsg{} }
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m achievementsViewModel) View() string {
	title := titleStyle.Render("Achievements")
	subtitle := fmt.Sprintf(
		"%s   %s unlocked",
		boldStyle.Render(m.profile.Name),
		boldStyle.Render(fmt.Sprintf("%d/%d", achievements.UnlockedCount(m.progress), len(achievements.All))),
	)

	var lines []string
	for _, a := range achievements.All {
		tierLabel := fmt.Sprintf("%-11s", a.Tier.String()) // width of "Grandmaster", the longest tier name
		if a.Unlocked(m.progress) {
			badge := tierStyle(a.Tier).Render(tierLabel)
			lines = append(lines, fmt.Sprintf("[x] %s %s  %s", badge, boldStyle.Render(a.Name), a.Description))
		} else {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("[ ] %s %s  %s", tierLabel, a.Name, a.Description)))
		}
	}

	help := helpStyle.Render("esc: back to profile   ctrl+c: quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", strings.Join(lines, "\n"), "", help)
}
