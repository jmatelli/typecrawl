// Package keyboard renders a keyboard layout in the terminal, colored by
// how often each key has been the site of a typing mistake.
package keyboard

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/jmatelli/typecrawl/internal/theme"
)

// A key is written as the runes it types: the first is its label (the
// character it types without Shift), the rest are what it types with
// Shift. Mistakes on any of them count toward the key. Letters only list
// their lower case, since mistakes are recorded lower-cased.
type key string

func (k key) label() rune { return []rune(k)[0] }

// Layout is a keyboard layout the weak-key heatmap can be drawn in.
type Layout struct {
	ID   string // stored in the profile's settings
	Name string // shown to the player
	rows [][]key
}

func keys(s ...string) []key {
	out := make([]key, len(s))
	for i, k := range s {
		out[i] = key(k)
	}
	return out
}

// letters splits a run of single-rune keys, for rows without shifted
// symbols worth listing.
func letters(s string) []key {
	out := make([]key, 0, len(s))
	for _, r := range s {
		out = append(out, key(string(r)))
	}
	return out
}

var usNumberRow = keys("1!", "2@", "3#", "4$", "5%", "6^", "7&", "8*", "9(", "0)")

// The layouts below list the four rows of keys that typing practice uses:
// the number row and the three letter rows. Keys outside them (backtick,
// brackets, the minus key on most layouts) aren't drawn; mistakes on
// their characters show up in the "other" row instead.
var (
	QWERTY = Layout{ID: "qwerty", Name: "QWERTY", rows: [][]key{
		usNumberRow,
		letters("qwertyuiop"),
		append(letters("asdfghjkl"), ";:"),
		append(letters("zxcvbnm"), ",<", ".>", "/?"),
	}}
	AZERTY = Layout{ID: "azerty", Name: "AZERTY", rows: [][]key{
		keys("&1", "é2", "\"3", "'4", "(5", "-6", "è7", "_8", "ç9", "à0"),
		letters("azertyuiop"),
		append(letters("qsdfghjklm"), "ù%"),
		append(letters("wxcvbn"), ",?", ";.", ":/", "!§"),
	}}
	QWERTZ = Layout{ID: "qwertz", Name: "QWERTZ", rows: [][]key{
		keys("1!", "2\"", "3§", "4$", "5%", "6&", "7/", "8(", "9)", "0=", "ß?"),
		letters("qwertzuiopü"),
		letters("asdfghjklöä"),
		append(letters("yxcvbnm"), ",;", ".:", "-_"),
	}}
	Dvorak = Layout{ID: "dvorak", Name: "Dvorak", rows: [][]key{
		usNumberRow,
		append(keys("'\"", ",<", ".>"), letters("pyfgcrl")...),
		append(letters("aoeuidhtns"), "-_"),
		append(keys(";:"), letters("qjkxbmwvz")...),
	}}
	Colemak = Layout{ID: "colemak", Name: "Colemak", rows: [][]key{
		usNumberRow,
		append(letters("qwfpgjluy"), ";:"),
		letters("arstdhneio"),
		append(letters("zxcvbkm"), ",<", ".>", "/?"),
	}}
)

// Layouts lists every layout, in the order the player cycles through them.
var Layouts = []Layout{QWERTY, AZERTY, QWERTZ, Dvorak, Colemak}

// Lookup returns the layout with the given ID. An empty or unknown ID
// (a profile from before layouts existed, or a backup from a newer
// version with more of them) gets QWERTY.
func Lookup(id string) Layout {
	for _, l := range Layouts {
		if l.ID == id {
			return l
		}
	}
	return QWERTY
}

// Next returns the layout after l in Layouts, wrapping around.
func Next(l Layout) Layout {
	for i, other := range Layouts {
		if other.ID == l.ID {
			return Layouts[(i+1)%len(Layouts)]
		}
	}
	return QWERTY
}

// rowIndents mimics a real keyboard's stagger, each row starting a bit
// further right than the one above it.
var rowIndents = []string{"", " ", "  ", "   "}

// keyStyle colors a key by its mistake count relative to the keyboard's
// hottest key (ratio 0 = never missed, ratio 1 = the most missed key).
//
// Each bucket also differs from its neighbours by a text attribute, so
// the scale survives for players who can't tell the colours apart and
// with colour turned off (see package theme): faint, plain, bold, bold
// and underlined, then reverse video for the worst keys.
func keyStyle(ratio float64) lipgloss.Style {
	switch {
	case ratio <= 0:
		// no data: dim gray, which already reads as faint
		return theme.Pick(lipgloss.NewStyle().Foreground(theme.Color("240")), lipgloss.NewStyle().Faint(true))
	case ratio < 0.25:
		return lipgloss.NewStyle().Foreground(theme.Color("2")) // green
	case ratio < 0.5:
		return lipgloss.NewStyle().Foreground(theme.Color("3")).Bold(true) // yellow
	case ratio < 0.75:
		return lipgloss.NewStyle().Foreground(theme.Color("208")).Bold(true).Underline(true) // orange
	default:
		return lipgloss.NewStyle().Foreground(theme.Color("1")).Bold(true).Reverse(true) // red
	}
}

// otherLabel starts the row of mistyped characters the layout has no key
// for, such as accented letters on QWERTY.
const otherLabel = "other "

// Render draws layout, coloring each key by its mistake count in weights
// relative to the most-missed key. A key's count is the sum over every
// character it types. Characters with mistakes that no key on the layout
// types are drawn in an extra row below, so no recorded mistake is hidden.
func Render(weights map[rune]int, layout Layout) string {
	onLayout := make(map[rune]bool)
	for _, row := range layout.rows {
		for _, k := range row {
			for _, r := range k {
				onLayout[r] = true
			}
		}
	}
	var others []rune
	for r, n := range weights {
		if n > 0 && !onLayout[r] && unicode.IsPrint(r) && !unicode.IsSpace(r) {
			others = append(others, r)
		}
	}
	sort.Slice(others, func(i, j int) bool { return others[i] < others[j] })

	keyCount := func(k key) int {
		n := 0
		for _, r := range k {
			n += weights[r]
		}
		return n
	}
	maxCount := 0
	for _, row := range layout.rows {
		for _, k := range row {
			maxCount = max(maxCount, keyCount(k))
		}
	}
	for _, r := range others {
		maxCount = max(maxCount, weights[r])
	}
	drawKey := func(b *strings.Builder, label rune, n int) {
		ratio := 0.0
		if maxCount > 0 && n > 0 {
			ratio = float64(n) / float64(maxCount)
		}
		b.WriteString(keyStyle(ratio).Render(fmt.Sprintf(" %s ", string(label))))
	}

	var b strings.Builder
	for ri, row := range layout.rows {
		b.WriteString(rowIndents[ri])
		for _, k := range row {
			drawKey(&b, k.label(), keyCount(k))
		}
		if ri < len(layout.rows)-1 {
			b.WriteString("\n")
		}
	}
	if len(others) > 0 {
		b.WriteString("\n\n")
		b.WriteString(keyStyle(0).Render(otherLabel))
		for _, r := range others {
			drawKey(&b, r, weights[r])
		}
	}
	return b.String()
}
