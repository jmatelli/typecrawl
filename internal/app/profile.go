package app

import (
	"database/sql"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/avatar"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// maxNameLength caps how long a profile name can be -- generous for any
// real name, but enough to stop a huge pasted block of text from becoming
// one.
const maxNameLength = 40

// revealMinDuration is how long the avatar reveal ignores input for. The
// screen is dismissed by "any key", and Enter both submits the name and is
// the key people habitually press again right after -- without this guard
// that reflexive second Enter (or OS key-repeat) would create the profile
// and immediately dismiss the reveal before it's ever seen.
const revealMinDuration = 700 * time.Millisecond

type profileCreatedMsg struct {
	profile *storage.Profile
}

type profileModel struct {
	db       *sql.DB
	name     string
	errMsg   string
	created  *storage.Profile // set once creation succeeds, to show the avatar reveal
	revealAt time.Time
}

func newProfileModel(db *sql.DB) profileModel {
	return profileModel{db: db}
}

func (m profileModel) Init() tea.Cmd { return nil }

func (m profileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// Once the profile exists, any key (other than quit) advances past the
	// avatar reveal into the game -- but only once it's had a moment to
	// actually be seen.
	if m.created != nil {
		if keyMsg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if time.Since(m.revealAt) < revealMinDuration {
			return m, nil
		}
		profile := m.created
		return m, func() tea.Msg { return profileCreatedMsg{profile: profile} }
	}

	switch keyMsg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		name := strings.TrimSpace(m.name)
		if name == "" {
			m.errMsg = "name can't be empty"
			return m, nil
		}
		profile, err := storage.CreateProfile(m.db, name)
		if err != nil {
			m.errMsg = "that name is taken, try another"
			return m, nil
		}
		m.created = profile
		m.revealAt = time.Now()
		return m, nil
	case tea.KeyBackspace:
		// Trim by rune, not by byte -- a byte-level trim on a multi-byte
		// rune (accents, CJK, emoji) would leave a dangling continuation
		// byte in m.name, corrupting it as invalid UTF-8.
		if runes := []rune(m.name); len(runes) > 0 {
			m.name = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes:
		m.errMsg = ""
		// Reject control characters (this is how a pasted or otherwise
		// smuggled-in raw ANSI/OSC escape sequence would arrive) so a
		// profile name can never inject terminal escape codes into
		// anything that later renders it verbatim.
		for _, r := range keyMsg.Runes {
			if len([]rune(m.name)) >= maxNameLength {
				break
			}
			if !unicode.IsControl(r) {
				m.name += string(r)
			}
		}
	}
	return m, nil
}

func (m profileModel) View() string {
	if m.created != nil {
		title := titleStyle.Render("Welcome, " + m.created.Name + "!")
		art := avatar.Generate(m.created.Name)
		help := helpStyle.Render("press any key to continue")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", art, "", help)
	}

	title := titleStyle.Render("Welcome to Typecrawl")
	prompt := "Enter your name: " + m.name + "▎"

	var errLine string
	if m.errMsg != "" {
		errLine = wrongStyle.Render(m.errMsg)
	}

	help := helpStyle.Render("enter: confirm   ctrl+c: quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", prompt, errLine, "", help)
}
