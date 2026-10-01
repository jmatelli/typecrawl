package app

import (
	"database/sql"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/storage"
)

type profileSelectedMsg struct{ profile *storage.Profile }
type createNewProfileMsg struct{}

// profileSelectModel lists every profile and lets the player pick one to
// play as, create a new one, or delete/reset an existing one. Delete and
// reset are armed by a first keypress and only take effect on a matching
// second keypress, so a stray key never destroys progress.
type profileSelectModel struct {
	db       *sql.DB
	profiles []*storage.Profile
	cursor   int
	confirm  string // "", "d", or "r" -- the armed action awaiting confirmation
	errMsg   string
}

func newProfileSelectModel(db *sql.DB, profiles []*storage.Profile) profileSelectModel {
	return profileSelectModel{db: db, profiles: profiles}
}

func (m profileSelectModel) Init() tea.Cmd { return nil }

func (m profileSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	if m.confirm != "" {
		if key == m.confirm {
			m.applyConfirmedAction()
		}
		m.confirm = ""
		return m, nil
	}

	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.profiles)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.profiles) > 0 {
			p := m.profiles[m.cursor]
			return m, func() tea.Msg { return profileSelectedMsg{profile: p} }
		}
	case "n":
		return m, func() tea.Msg { return createNewProfileMsg{} }
	case "d":
		if len(m.profiles) > 0 {
			m.confirm = "d"
			m.errMsg = ""
		}
	case "r":
		if len(m.profiles) > 0 {
			m.confirm = "r"
			m.errMsg = ""
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *profileSelectModel) applyConfirmedAction() {
	if len(m.profiles) == 0 {
		return
	}
	id := m.profiles[m.cursor].ID
	switch m.confirm {
	case "d":
		if err := storage.DeleteProfile(m.db, id); err != nil {
			m.errMsg = "couldn't delete profile"
			return
		}
		m.profiles = append(m.profiles[:m.cursor], m.profiles[m.cursor+1:]...)
		if m.cursor >= len(m.profiles) && m.cursor > 0 {
			m.cursor--
		}
	case "r":
		p, err := storage.ResetProfile(m.db, id)
		if err != nil {
			m.errMsg = "couldn't reset profile"
			return
		}
		m.profiles[m.cursor] = p
	}
}

func (m profileSelectModel) View() string {
	title := titleStyle.Render("Typecrawl — Select Profile")

	var rows []string
	if len(m.profiles) == 0 {
		rows = append(rows, dimStyle.Render("no profiles yet -- press n to create one"))
	}
	for i, p := range m.profiles {
		line := fmt.Sprintf("%-16s Lv.%-3d %d/%d xp", p.Name, p.Level, p.XP, game.XPToNextLevel(p.Level))
		if i == m.cursor {
			line = boldStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	list := lipgloss.JoinVertical(lipgloss.Left, rows...)

	var confirmLine string
	if m.confirm != "" && len(m.profiles) > 0 {
		action := "delete"
		if m.confirm == "r" {
			action = "reset"
		}
		confirmLine = wrongStyle.Render(fmt.Sprintf(
			"press %s again to %s %q, any other key cancels",
			m.confirm, action, m.profiles[m.cursor].Name,
		))
	}

	var errLine string
	if m.errMsg != "" {
		errLine = wrongStyle.Render(m.errMsg)
	}

	help := helpStyle.Render("enter: play   n: new profile   d: delete   r: reset   q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", list, "", confirmLine, errLine, "", help)
}
