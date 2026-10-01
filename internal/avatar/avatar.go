// Package avatar generates small, deterministic identicon-style block art
// from a text seed (e.g. a profile name), purely for a bit of visual flavor
// on profile screens.
package avatar

import (
	"crypto/sha256"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	gridSize = 7 // odd, so there's a symmetric center column
	cell     = "██"
)

var palette = []string{"1", "2", "3", "4", "5", "6", "9", "10", "11", "12", "13", "14"}

// Generate deterministically builds a small colored block avatar for seed:
// the left half of a gridSize x gridSize grid is derived from a hash of
// seed and mirrored onto the right half (identicon-style), colored with a
// single hash-derived color.
func Generate(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(palette[int(sum[0])%len(palette)]))

	half := gridSize/2 + 1 // generated columns; the rest mirror these
	var b strings.Builder
	bit := 0
	for y := range gridSize {
		row := make([]bool, gridSize)
		for x := range half {
			byteIdx := (bit / 8) % len(sum)
			bitIdx := uint(bit % 8)
			on := sum[byteIdx]&(1<<bitIdx) != 0
			row[x] = on
			row[gridSize-1-x] = on
			bit++
		}
		for _, on := range row {
			if on {
				b.WriteString(style.Render(cell))
			} else {
				b.WriteString("  ")
			}
		}
		if y < gridSize-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
