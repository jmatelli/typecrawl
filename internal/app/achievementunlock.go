package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/achievements"
)

// continueToResultsMsg dismisses the achievement-unlock screen and shows
// the results screen that was already built for this run.
type continueToResultsMsg struct{}

// achievementUnlockModel is a dedicated, hard-to-miss screen shown between
// finishing an exercise and the results screen, but only when that run
// actually unlocked something new -- it's skipped entirely otherwise.
type achievementUnlockModel struct {
	unlocked []achievements.Achievement
}

func newAchievementUnlockModel(unlocked []achievements.Achievement) achievementUnlockModel {
	return achievementUnlockModel{unlocked: unlocked}
}

func (m achievementUnlockModel) Init() tea.Cmd { return nil }

func (m achievementUnlockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return m, func() tea.Msg { return continueToResultsMsg{} }
	}
	return m, nil
}

func (m achievementUnlockModel) View() string {
	title := levelUpStyle.Render("Achievement Unlocked!")

	var lines []string
	for _, a := range m.unlocked {
		badge := tierStyle(a.Tier).Render(fmt.Sprintf("[%s]", a.Tier))
		lines = append(lines, fmt.Sprintf("%s %s  %s", badge, boldStyle.Render(a.Name), a.Description))
	}

	help := helpStyle.Render("press any key to continue")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", strings.Join(lines, "\n"), "", help)
}
