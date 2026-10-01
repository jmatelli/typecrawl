// Package keyboard renders a QWERTY layout in the terminal, colored by how
// often each key has been the site of a typing mistake.
package keyboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var rows = [][]rune{
	[]rune("1234567890"),
	[]rune("qwertyuiop"),
	[]rune("asdfghjkl;"),
	[]rune("zxcvbnm,./"),
}

// rowIndents mimics a real keyboard's stagger, each row starting a bit
// further right than the one above it.
var rowIndents = []string{"", " ", "  ", "   "}

// keyStyle colors a key by its mistake count relative to the keyboard's
// hottest key (ratio 0 = never missed, ratio 1 = the most missed key).
func keyStyle(ratio float64) lipgloss.Style {
	switch {
	case ratio <= 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // no data: dim gray
	case ratio < 0.25:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case ratio < 0.5:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	case ratio < 0.75:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // orange
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	}
}

// Render draws a QWERTY keyboard, coloring each key by its mistake count in
// weights relative to the highest count present.
func Render(weights map[rune]int) string {
	maxCount := 0
	for _, n := range weights {
		if n > maxCount {
			maxCount = n
		}
	}

	var b strings.Builder
	for ri, row := range rows {
		b.WriteString(rowIndents[ri])
		for _, r := range row {
			ratio := 0.0
			if n := weights[r]; maxCount > 0 && n > 0 {
				ratio = float64(n) / float64(maxCount)
			}
			b.WriteString(keyStyle(ratio).Render(fmt.Sprintf(" %s ", string(r))))
		}
		if ri < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
