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
	pairs   map[string]int
	layout  keyboard.Layout
}

func newKeyboardViewModel(profile *storage.Profile, db *sql.DB) keyboardViewModel {
	weights, _ := storage.WeakCharWeights(db, profile.ID) // best-effort; nil renders as "not enough data"
	pairs, _ := storage.WeakPairWeights(db, profile.ID)   // best-effort; nil shows no pairs line
	settings, _ := storage.LoadSettings(db, profile.ID)   // best-effort; "" draws QWERTY
	return keyboardViewModel{db: db, profile: profile, weights: weights, pairs: pairs, layout: keyboard.Lookup(settings.KeyboardLayout)}
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
	// Ties in character order, so the order stays put between visits.
	sort.Slice(top, func(i, j int) bool {
		if top[i].n != top[j].n {
			return top[i].n > top[j].n
		}
		return top[i].r < top[j].r
	})
	if len(top) > 8 {
		top = top[:8]
	}
	parts := make([]string, len(top))
	for i, c := range top {
		parts[i] = fmt.Sprintf("%s:%d", string(c.r), c.n)
	}
	legend := "Most mistakes: " + wrongStyle.Render(strings.Join(parts, "  "))
	lines := []string{title, subtitle, "", kb, "", legend}
	if pairs := topPairs(m.pairs, 8); len(pairs) > 0 {
		lines = append(lines, "Weakest key pairs: "+wrongStyle.Render(strings.Join(pairs, "  ")))
	}
	note := dimStyle.Render("Mistakes fade: each counts half as much after 30 days.")
	return lipgloss.JoinVertical(lipgloss.Left, append(lines, note, "", help)...)
}

// topPairs lists up to n key pairs with the most mistakes, as "th:4",
// most first; ties in pair order, so the list is stable.
func topPairs(pairs map[string]int, n int) []string {
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if pairs[keys[i]] != pairs[keys[j]] {
			return pairs[keys[i]] > pairs[keys[j]]
		}
		return keys[i] < keys[j]
	})
	keys = keys[:min(n, len(keys))]
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fmt.Sprintf("%s:%d", k, pairs[k])
	}
	return out
}
