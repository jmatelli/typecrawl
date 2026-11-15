package keyboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/theme"
)

func TestRenderWithNoDataStillRendersFullLayout(t *testing.T) {
	out := Render(nil)
	for _, row := range rows {
		for _, r := range row {
			if !strings.ContainsRune(out, r) {
				t.Fatalf("expected key %q to appear in the rendered keyboard", r)
			}
		}
	}
}

func TestRenderHasFourRows(t *testing.T) {
	out := Render(map[rune]int{'j': 5})
	lines := strings.Split(out, "\n")
	if len(lines) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(lines))
	}
}

func TestKeyStyleBuckets(t *testing.T) {
	if theme.NoColor() {
		t.Skip("NO_COLOR is set, so no key has a colour to check")
	}
	cases := []struct {
		ratio float64
		color lipgloss.Color
	}{
		{0, "240"}, {-1, "240"},
		{0.1, "2"},
		{0.3, "3"},
		{0.6, "208"},
		{0.9, "1"}, {1.0, "1"},
	}
	for _, c := range cases {
		got := keyStyle(c.ratio).GetForeground()
		if got != c.color {
			t.Errorf("keyStyle(%v).GetForeground() = %v, want %v", c.ratio, got, c.color)
		}
	}
}

// The buckets must differ by more than colour, so the scale is still
// there with colour turned off or for a player who can't tell the colours
// apart.
func TestKeyStyleBucketsDifferWithoutColour(t *testing.T) {
	type attrs struct{ faint, bold, underline, reverse bool }
	seen := map[attrs]float64{}
	for _, ratio := range []float64{0.1, 0.3, 0.6, 0.9} {
		s := keyStyle(ratio)
		a := attrs{s.GetFaint(), s.GetBold(), s.GetUnderline(), s.GetReverse()}
		if other, dup := seen[a]; dup {
			t.Errorf("keyStyle(%v) and keyStyle(%v) have the same text attributes %+v", ratio, other, a)
		}
		seen[a] = ratio
	}
	// "No data" is told apart by its gray, or by faint when colour is off.
	if theme.NoColor() && !keyStyle(0).GetFaint() {
		t.Error("expected a key with no data to be faint when colour is off")
	}
}

func TestRenderNeverPanicsWithEmptyMap(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Render panicked with an empty weights map: %v", r)
		}
	}()
	_ = Render(map[rune]int{})
}

func TestRenderIgnoresCharactersNotOnTheKeyboard(t *testing.T) {
	// Weights for characters that don't appear on the drawn layout (e.g.
	// punctuation from a different locale) must be harmless, not panic,
	// and not affect the max-count normalization in a way that breaks
	// rendering.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Render panicked with off-keyboard characters in weights: %v", r)
		}
	}()
	out := Render(map[rune]int{'é': 50, 'j': 5})
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}
