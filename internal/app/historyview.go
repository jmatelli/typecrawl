package app

import (
	"database/sql"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/storage"
)

const historyLimit = 100

type historyViewModel struct {
	profile *storage.Profile
	entries []storage.TestHistoryEntry
}

func newHistoryViewModel(profile *storage.Profile, db *sql.DB) historyViewModel {
	entries, _ := storage.ListTestHistory(db, profile.ID, historyLimit) // best-effort; nil renders as "no history yet"
	return historyViewModel{profile: profile, entries: entries}
}

func (m historyViewModel) Init() tea.Cmd { return nil }

func (m historyViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m historyViewModel) View() string {
	title := titleStyle.Render("Test History")
	subtitle := fmt.Sprintf("%s   last %s exercises", boldStyle.Render(m.profile.Name), boldStyle.Render(fmt.Sprintf("%d", len(m.entries))))

	if len(m.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", dimStyle.Render("no exercises completed yet"), "",
			helpStyle.Render("esc: back to profile   ctrl+c: quit"))
	}

	header := fmt.Sprintf("%-17s %-12s %8s %8s %6s  %s", "Date", "Mode", "WPM", "Accuracy", "Words", "Flags")
	var lines []string
	lines = append(lines, dimStyle.Render(header))
	for _, e := range m.entries {
		modeStr := fmt.Sprintf("%s %d", e.Mode, e.Target)
		var flags []string
		if e.KO {
			flags = append(flags, "KO")
		}
		if e.Punctuation {
			flags = append(flags, "punct")
		}
		if e.ZenMode {
			flags = append(flags, "zen")
		}
		flagStr := strings.Join(flags, ",")

		row := fmt.Sprintf("%-17s %-12s %8.1f %7.1f%% %6d  %s",
			e.CreatedAt.Local().Format("2006-01-02 15:04"), modeStr, e.WPM, e.Accuracy, e.WordsTyped, flagStr,
		)
		if e.KO {
			lines = append(lines, wrongStyle.Render(row))
		} else {
			lines = append(lines, row)
		}
	}

	help := helpStyle.Render("esc: back to profile   ctrl+c: quit")
	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", strings.Join(lines, "\n"), "", help)
}
