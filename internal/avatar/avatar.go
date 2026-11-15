// Package avatar generates small, deterministic identicon-style block art
// from a text seed (e.g. a profile name), purely for a bit of visual flavor
// on profile screens.
package avatar

import (
	"crypto/sha256"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jmatelli/typecrawl/internal/theme"
)

const (
	gridSize = 7 // odd, so there's a symmetric center column
	cell     = "██"
)

var palette = []string{"1", "2", "3", "4", "5", "6", "9", "10", "11", "12", "13", "14"}

// Size is the avatar's height in rows. It is twice as many columns wide,
// since each cell is drawn two characters wide to look square.
const Size = gridSize

// Avatar is the pattern and colour generated from a seed.
type Avatar struct {
	cells [gridSize][gridSize]bool
	color string
}

// New deterministically builds the avatar for seed: the left half of a
// gridSize x gridSize grid is derived from a hash of seed and mirrored
// onto the right half (identicon-style), with a single hash-derived color.
func New(seed string) Avatar {
	sum := sha256.Sum256([]byte(seed))
	a := Avatar{color: palette[int(sum[0])%len(palette)]}

	half := gridSize/2 + 1 // generated columns; the rest mirror these
	bit := 0
	for y := range gridSize {
		for x := range half {
			byteIdx := (bit / 8) % len(sum)
			bitIdx := uint(bit % 8)
			on := sum[byteIdx]&(1<<bitIdx) != 0
			a.cells[y][x] = on
			a.cells[y][gridSize-1-x] = on
			bit++
		}
	}
	return a
}

// Style is the avatar's own colour.
func (a Avatar) Style() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Color(a.color))
}

// Rows draws the avatar one row per string, filled cells in style. Every
// row is Size*2 columns wide.
func (a Avatar) Rows(style lipgloss.Style) []string {
	rows := make([]string, gridSize)
	for y, row := range a.cells {
		var b strings.Builder
		for _, on := range row {
			if on {
				b.WriteString(style.Render(cell))
			} else {
				b.WriteString("  ")
			}
		}
		rows[y] = b.String()
	}
	return rows
}

// Generate draws the avatar for seed in its own colour.
func Generate(seed string) string {
	a := New(seed)
	return strings.Join(a.Rows(a.Style()), "\n")
}
