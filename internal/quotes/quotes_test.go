package quotes

import (
	"strings"
	"testing"
)

func TestWordsReturnsExactlyN(t *testing.T) {
	pool := []Quote{
		{ID: 1, Text: "A short quote."},
		{ID: 2, Text: "A somewhat longer quote with several more words in it."},
	}
	for _, n := range []int{0, 1, 3, 10, 50} {
		got := Words(pool, n)
		if len(got) != n {
			t.Errorf("Words(pool, %d): got %d words, want %d", n, len(got), n)
		}
	}
}

func TestWordsEmptyPoolOrNReturnsNil(t *testing.T) {
	if got := Words(nil, 5); got != nil {
		t.Errorf("expected nil for an empty pool, got %v", got)
	}
	pool := []Quote{{ID: 1, Text: "Hello world."}}
	if got := Words(pool, 0); got != nil {
		t.Errorf("expected nil for n=0, got %v", got)
	}
}

func TestWordsSkipsBlankQuotesWithoutHanging(t *testing.T) {
	pool := []Quote{
		{ID: 1, Text: "   "},
		{ID: 2, Text: ""},
		{ID: 3, Text: "The only real quote here."},
	}
	got := Words(pool, 20)
	if len(got) != 20 {
		t.Fatalf("expected 20 words from the one non-blank quote, got %d: %v", len(got), got)
	}
}

func TestWordsAllBlankReturnsNil(t *testing.T) {
	pool := []Quote{{ID: 1, Text: ""}, {ID: 2, Text: "   "}}
	if got := Words(pool, 10); got != nil {
		t.Errorf("expected nil when every quote in the pool is blank, got %v", got)
	}
}

func TestWordsLastWordEndsInSentencePunctuation(t *testing.T) {
	pool := []Quote{{ID: 1, Text: "One two three four five six seven eight nine ten."}}
	for _, n := range []int{1, 3, 5, 10} {
		got := Words(pool, n)
		last := got[len(got)-1]
		if !endsSentence(last) {
			t.Errorf("Words(pool, %d): last word %q does not end in sentence punctuation", n, last)
		}
	}
}

func TestWordsAddsMissingTrailingPeriod(t *testing.T) {
	pool := []Quote{{ID: 1, Text: "No terminal punctuation here"}}
	got := Words(pool, 5)
	last := got[len(got)-1]
	if !strings.HasSuffix(last, ".") {
		t.Errorf("expected the quote's forced trailing period to survive, got %q", last)
	}
}

func TestWordsDoesNotRepeatTheSameQuoteBackToBack(t *testing.T) {
	pool := []Quote{
		{ID: 1, Text: "Alpha."},
		{ID: 2, Text: "Beta."},
	}
	// Each quote here is a single word, so picking the same one twice in a
	// row would show up as two identical consecutive words.
	got := Words(pool, 40)
	for i := 1; i < len(got); i++ {
		if got[i] == got[i-1] {
			t.Fatalf("quote repeated back-to-back at index %d: %v", i, got[max(0, i-3):min(len(got), i+3)])
		}
	}
}

func TestWordsSingleQuotePoolStillTerminates(t *testing.T) {
	pool := []Quote{{ID: 1, Text: "Only one quote available."}}
	got := Words(pool, 30)
	if len(got) != 30 {
		t.Fatalf("expected 30 words even with a single-quote pool, got %d", len(got))
	}
}

func TestEndsSentence(t *testing.T) {
	cases := map[string]bool{
		"":          false,
		"hello":     false,
		"hello.":    true,
		"really?!":  true,
		"wait...":   true,
		"no comma,": false,
	}
	for s, want := range cases {
		if got := endsSentence(s); got != want {
			t.Errorf("endsSentence(%q) = %v, want %v", s, got, want)
		}
	}
}

// Quotes often use typographic punctuation that most keyboards can't
// type; it has to come out as the ASCII a player would type instead.
func TestWordsUseTypeablePunctuation(t *testing.T) {
	pool := []Quote{{ID: 1, Text: "“Don’t stop” — it’s ‘fine’… «really»."}}
	got := strings.Join(Words(pool, 6), " ")
	want := `"Don't stop" - it's 'fine'... "really".`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWordsKeepAccentedLettersAsOneCharacter(t *testing.T) {
	// Letters outside ASCII are kept: they can be typed, and changing
	// them would change the quote.
	if got := Words([]Quote{{ID: 2, Text: "Café déjà vu."}}, 3); strings.Join(got, " ") != "Café déjà vu." {
		t.Errorf("expected accented letters kept, got %q", got)
	}
	// A decomposed "é" (an e followed by a combining accent) becomes the
	// single character a keyboard sends.
	got := Words([]Quote{{ID: 3, Text: "cafe\u0301 noir."}}, 2)
	if got[0] != "caf\u00e9" {
		t.Errorf("expected the composed é, got %+q", got[0])
	}
}

func TestWordsDropInvisibleCharacters(t *testing.T) {
	got := Words([]Quote{{ID: 4, Text: "foo\u200Bbar soft\u00ADhyphen."}}, 2)
	if strings.Join(got, " ") != "foobar softhyphen." {
		t.Errorf("got %+q", got)
	}
}

// A quote ending inside its closing quote mark already ends a sentence;
// adding a period after the mark would leave an odd extra character.
func TestWordsDontAddAPeriodAfterAClosingQuote(t *testing.T) {
	for _, text := range []string{"He said “Go.”", "He said (go!)", "Wait…"} {
		got := Words([]Quote{{ID: 5, Text: text}}, 3)
		last := got[len(got)-1]
		if strings.HasSuffix(last, `".`) || strings.HasSuffix(last, ").") || strings.HasSuffix(last, "....") {
			t.Errorf("%q: got a stray period: %q", text, got)
		}
	}
}
