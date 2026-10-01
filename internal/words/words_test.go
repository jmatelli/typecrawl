package words

import (
	"slices"
	"strings"
	"testing"
)

func TestTierBoundaries(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{0, 0}, {1, 0}, {9, 0},
		{10, 1}, {19, 1},
		{20, 2}, {29, 2},
		{30, 3}, {40, 3}, {1000, 3}, // caps at the last defined tier
		{-5, 0}, // never negative
	}
	for _, c := range cases {
		if got := Tier(c.level); got != c.want {
			t.Errorf("Tier(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestTierNameMatchesTier(t *testing.T) {
	want := []string{"Easy", "Medium", "Hard", "Expert"}
	for tier, name := range want {
		if got := TierName(tier); got != name {
			t.Errorf("TierName(%d) = %q, want %q", tier, got, name)
		}
	}
	// Out-of-range tiers clamp instead of panicking.
	if got := TierName(99); got != "Expert" {
		t.Errorf("TierName(99) = %q, want %q (clamped)", got, "Expert")
	}
	if got := TierName(-1); got != "Easy" {
		t.Errorf("TierName(-1) = %q, want %q (clamped)", got, "Easy")
	}
}

func TestForLevelReturnsRequestedCountFromTheRightTier(t *testing.T) {
	cases := []struct{ level, tier int }{
		{0, 0}, {5, 0}, {9, 0},
		{10, 1}, {15, 1}, {19, 1},
		{20, 2}, {25, 2}, {29, 2},
		{30, 3}, {40, 3},
	}
	for _, c := range cases {
		list := ForLevel(50, c.level)
		if len(list) != 50 {
			t.Fatalf("level %d: expected 50 words, got %d", c.level, len(list))
		}
		tierWords := tiers[c.tier]
		for _, w := range list {
			if !slices.Contains(tierWords, w) {
				t.Fatalf("level %d: word %q is not from tier %d's list", c.level, w, c.tier)
			}
		}
	}
}

func TestForLevelNeverImmediatelyRepeatsAWord(t *testing.T) {
	// tierEasy is large enough that immediate repeats should never happen
	// across many trials if the no-repeat window works.
	for trial := range 20 {
		list := ForLevel(200, 0)
		for i := 1; i < len(list); i++ {
			if list[i] == list[i-1] {
				t.Fatalf("trial %d: word %q repeated immediately at index %d", trial, list[i], i)
			}
		}
	}
}

func TestForLevelRespectsTailForNoRepeatWindow(t *testing.T) {
	// Seed a tail of the same word repeated noRepeatWindow times; the very
	// next generated word must not be that word (it's excluded by the
	// window), assuming the tier has more than one word (true for all of
	// them).
	word := tierEasy[0]
	tail := make([]string, noRepeatWindow)
	for i := range tail {
		tail[i] = word
	}
	for trial := range 20 {
		list := ForLevel(1, 0, tail...)
		if list[0] == word {
			t.Fatalf("trial %d: expected the tail's word to be excluded by the no-repeat window, got %q again", trial, word)
		}
	}
}

func TestForLevelWeakFallsBackToPlainSelectionWithNoWeakChars(t *testing.T) {
	list := ForLevel(0, 0) // just to reference the tier, not used directly
	_ = list
	got := ForLevelWeak(30, 0, nil)
	if len(got) != 30 {
		t.Fatalf("expected 30 words, got %d", len(got))
	}
}

func TestForLevelWeakBiasesTowardWeakCharacters(t *testing.T) {
	// Pick the rarest letter that actually appears in tierEasy (fewest
	// words contain it), so biasing toward it produces the strongest,
	// least noisy signal versus uniform selection.
	counts := make(map[rune]int)
	for _, w := range tierEasy {
		seen := make(map[rune]bool)
		for _, r := range w {
			if !seen[r] {
				counts[r] = counts[r] + 1
				seen[r] = true
			}
		}
	}
	var rareChar rune
	rareCount := len(tierEasy) + 1
	for r, n := range counts {
		if n > 0 && n < rareCount {
			rareChar, rareCount = r, n
		}
	}
	if rareCount == 0 {
		t.Fatal("expected at least one character to appear in tierEasy")
	}

	containsChar := func(w string) bool {
		for _, r := range w {
			if r == rareChar {
				return true
			}
		}
		return false
	}

	weak := map[rune]int{rareChar: weakCharCap}
	const n = 800
	weightedCount := 0
	for _, w := range ForLevelWeak(n, 0, weak) {
		if containsChar(w) {
			weightedCount++
		}
	}
	uniformCount := 0
	for _, w := range ForLevel(n, 0) {
		if containsChar(w) {
			uniformCount++
		}
	}

	if weightedCount <= uniformCount {
		t.Fatalf("expected weak-char weighting toward %q to select matching words more often than uniform selection; weighted=%d uniform=%d (out of %d)",
			rareChar, weightedCount, uniformCount, n)
	}
}

func TestPunctuateCapitalizesFirstWordAndEndsWithTerminalPunctuation(t *testing.T) {
	list := []string{"the", "quick", "brown", "fox", "jumps", "over", "the", "lazy", "dog"}
	out := Punctuate(list)

	if len(out) != len(list) {
		t.Fatalf("expected same length, got %d want %d", len(out), len(list))
	}
	if out[0] == "" || !strings.Contains("ABCDEFGHIJKLMNOPQRSTUVWXYZ", string(out[0][0])) {
		t.Fatalf("expected first word capitalized, got %q", out[0])
	}
	last := out[len(out)-1]
	if !endsSentence(last) {
		t.Fatalf("expected the list to end with terminal punctuation, got %q", last)
	}
}

func TestPunctuateCapitalizesWordsAfterSentenceEnds(t *testing.T) {
	// Run several times since sentence breaks are randomized; just verify
	// that whenever a word ends a sentence, the next word is capitalized.
	list := ForLevel(60, 0)
	for range 10 {
		out := Punctuate(list)
		for i := range len(out) - 1 {
			if endsSentence(out[i]) {
				next := out[i+1]
				if next == "" || next[0] < 'A' || next[0] > 'Z' {
					t.Fatalf("word %q ends a sentence but next word %q isn't capitalized", out[i], next)
				}
			}
		}
	}
}

func TestBaseFormStripsPunctuationAndCapitalization(t *testing.T) {
	cases := map[string]string{
		"Hello,":   "hello",
		"World.":   "world",
		"Really?!": "really",
		"plain":    "plain",
		"":         "",
	}
	for in, want := range cases {
		if got := baseForm(in); got != want {
			t.Errorf("baseForm(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestForLevelWithPunctuatedTailStillAvoidsImmediateRepeat(t *testing.T) {
	// Tail carrying Punctuate's decorations must still be recognized via
	// baseForm so the no-repeat window isn't silently defeated.
	word := tierEasy[1]
	tail := []string{strings.ToUpper(word[:1]) + word[1:] + "."}
	for trial := range 20 {
		list := ForLevel(1, 0, tail...)
		if list[0] == word {
			t.Fatalf("trial %d: expected punctuated tail to still exclude %q via baseForm", trial, word)
		}
	}
}
