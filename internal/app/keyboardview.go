package app

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/keyboard"
	"github.com/jmatelli/typecrawl/internal/storage"
)

type keyboardViewModel struct {
	db      *sql.DB
	profile *storage.Profile
	weights map[rune]int
	layout  keyboard.Layout
}

func newKeyboardViewModel(profile *storage.Profile, db *sql.DB) keyboardViewModel {
	weights, _ := storage.WeakCharWeights(db, profile.ID) // best-effort; nil renders as "not enough data"
	settings, _ := storage.LoadSettings(db, profile.ID)   // best-effort; "" draws QWERTY
	return keyboardViewModel{db: db, profile: profile, weights: weights, layout: keyboard.Lookup(settings.KeyboardLayout)}
}

func (m keyboardViewModel) Init() tea.Cmd { return nil }

func (m keyboardViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc", "b", "q":
		return m, func() tea.Msg { return viewProfileMsg{} }
	case "l":
		m.layout = keyboard.Next(m.layout)
		_ = storage.SaveKeyboardLayout(m.db, m.profile.ID, m.layout.ID) // best-effort; the screen still shows the choice
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m keyboardViewModel) View() string {
	title := titleStyle.Render("Weak Keys")
	subtitle := boldStyle.Render(m.profile.Name) + dimStyle.Render("   layout: ") + m.layout.Name
	help := helpStyle.Render("l: change layout   esc: back to profile   ctrl+c: quit")

	if len(m.weights) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "",
			dimStyle.Render("not enough mistakes recorded yet"), "", help)
	}

	kb := keyboard.Render(m.weights, m.layout)

	type charCount struct {
		r rune
		n int
	}
	top := make([]charCount, 0, len(m.weights))
	for r, n := range m.weights {
		top = append(top, charCount{r, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	if len(top) > 8 {
		top = top[:8]
	}
	parts := make([]string, len(top))
	for i, c := range top {
		parts[i] = fmt.Sprintf("%s:%d", string(c.r), c.n)
	}
	legend := "Most mistakes: " + wrongStyle.Render(strings.Join(parts, "  "))

	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", kb, "", legend, "", help)
}
