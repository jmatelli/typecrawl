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
