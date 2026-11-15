package keyboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jmatelli/typecrawl/internal/theme"
)

func TestRenderWithNoDataStillRendersFullLayout(t *testing.T) {
	for _, l := range Layouts {
		out := Render(nil, l)
		for _, row := range l.rows {
			for _, k := range row {
				if !strings.ContainsRune(out, k.label()) {
					t.Fatalf("%s: expected key %q to appear in the rendered keyboard", l.Name, k.label())
				}
			}
		}
	}
}

func TestRenderHasFourRows(t *testing.T) {
	for _, l := range Layouts {
		out := Render(map[rune]int{'j': 5}, l)
		lines := strings.Split(out, "\n")
		if len(lines) != len(l.rows) || len(l.rows) != len(rowIndents) {
			t.Fatalf("%s: expected %d rows, got %d", l.Name, len(rowIndents), len(lines))
		}
	}
}

// Every layout must have each letter exactly once, and no character on
// two keys, or a mistake would be drawn twice or not at all.
func TestLayoutsCoverEveryLetterOnce(t *testing.T) {
	for _, l := range Layouts {
		seen := map[rune]int{}
		for _, row := range l.rows {
			for _, k := range row {
				for _, r := range k {
					seen[r]++
				}
			}
		}
		for r, n := range seen {
			if n > 1 {
				t.Errorf("%s: %q is on %d keys", l.Name, r, n)
			}
		}
		for r := 'a'; r <= 'z'; r++ {
			if seen[r] != 1 {
				t.Errorf("%s: letter %q is on %d keys, want 1", l.Name, r, seen[r])
			}
		}
		for r := '0'; r <= '9'; r++ {
			if seen[r] != 1 {
				t.Errorf("%s: digit %q is on %d keys, want 1", l.Name, r, seen[r])
			}
		}
	}
}

func TestLayoutIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range Layouts {
		if l.ID == "" || seen[l.ID] {
			t.Errorf("layout %q has an empty or duplicate ID", l.Name)
		}
		seen[l.ID] = true
	}
}

func TestLookupFallsBackToQWERTY(t *testing.T) {
	for _, id := range []string{"", "bépo-from-the-future"} {
		if got := Lookup(id); got.ID != QWERTY.ID {
			t.Errorf("Lookup(%q) = %s, want QWERTY", id, got.Name)
		}
	}
	for _, l := range Layouts {
		if got := Lookup(l.ID); got.ID != l.ID {
			t.Errorf("Lookup(%q) = %s", l.ID, got.Name)
		}
	}
}

func TestNextCyclesThroughEveryLayout(t *testing.T) {
	l := QWERTY
	for i := 1; i <= len(Layouts); i++ {
		l = Next(l)
		if want := Layouts[i%len(Layouts)]; l.ID != want.ID {
			t.Fatalf("step %d: got %s, want %s", i, l.Name, want.Name)
		}
	}
}

// The QWERTY rows drawn before layouts existed must not change.
func TestQWERTYLabelsUnchanged(t *testing.T) {
	want := []string{"1234567890", "qwertyuiop", "asdfghjkl;", "zxcvbnm,./"}
	for i, row := range QWERTY.rows {
		var got strings.Builder
		for _, k := range row {
			got.WriteRune(k.label())
		}
		if got.String() != want[i] {
			t.Errorf("row %d = %q, want %q", i, got.String(), want[i])
		}
	}
}

// A shifted character counts toward the key that types it.
func TestShiftedCharactersCountTowardTheirKey(t *testing.T) {
	// On AZERTY, '1' is Shift + '&'. With the only mistakes on '1', '&'
	// must be drawn as the hottest key.
	hot := keyStyle(1).Render(" & ")
	if out := Render(map[rune]int{'1': 4}, AZERTY); !strings.Contains(out, hot) {
		t.Fatalf("expected '&' drawn as the hottest key, got:\n%s", out)
	}
	if out := Render(map[rune]int{'1': 4}, AZERTY); strings.Contains(out, otherLabel) {
		t.Fatalf("'1' is on AZERTY and must not go to the other row, got:\n%s", out)
	}
}

func TestOffLayoutCharactersGetTheirOwnRow(t *testing.T) {
	out := Render(map[rune]int{'é': 3, 'j': 5}, QWERTY)
	lines := strings.Split(out, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, otherLabel) || !strings.ContainsRune(last, 'é') {
		t.Fatalf("expected an other row with 'é', got:\n%s", out)
	}
	// On AZERTY 'é' has a key, so there is no other row.
	if out := Render(map[rune]int{'é': 3, 'j': 5}, AZERTY); strings.Contains(out, otherLabel) {
		t.Fatalf("expected no other row on AZERTY, got:\n%s", out)
	}
	if out := Render(map[rune]int{'j': 5}, QWERTY); strings.Contains(out, otherLabel) {
		t.Fatalf("expected no other row without off-layout mistakes, got:\n%s", out)
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
	_ = Render(map[rune]int{}, QWERTY)
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
	out := Render(map[rune]int{'é': 50, 'j': 5}, QWERTY)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}
