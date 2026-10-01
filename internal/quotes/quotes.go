// Package quotes supplies real quotations as an alternative exercise text
// source to internal/words' random word lists, fetched from a free public
// quotes API and cached locally (see the storage package) so a session
// after the first one never needs the network at all.
package quotes

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Quote is one quotation, identified by the API's own id so repeated
// fetches can be deduplicated against what's already cached.
type Quote struct {
	ID     int
	Text   string
	Author string
}

const (
	fetchURL     = "https://dummyjson.com/quotes?limit=200"
	fetchTimeout = 5 * time.Second
)

// Fetch retrieves a batch of quotes from the public quotes API. A network
// or decoding failure is the caller's to handle -- see Fallback for what to
// fall back to when there's no cache to fall back on either.
func Fetch() ([]Quote, error) {
	client := http.Client{Timeout: fetchTimeout}
	resp, err := client.Get(fetchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quotes API returned %s", resp.Status)
	}

	var body struct {
		Quotes []struct {
			ID     int    `json:"id"`
			Quote  string `json:"quote"`
			Author string `json:"author"`
		} `json:"quotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	out := make([]Quote, len(body.Quotes))
	for i, q := range body.Quotes {
		out[i] = Quote{ID: q.ID, Text: q.Quote, Author: q.Author}
	}
	return out, nil
}

// Fallback is a small built-in quote set used only when there's no cache
// yet and the network fetch also fails (e.g. the very first launch with no
// internet connection) -- it keeps quotes mode usable fully offline instead
// of failing outright. IDs are negative so they can never collide with a
// real one from the API.
var Fallback = []Quote{
	{ID: -1, Text: "The only way to do great work is to love what you do.", Author: "Steve Jobs"},
	{ID: -2, Text: "Life is what happens when you're busy making other plans.", Author: "John Lennon"},
	{ID: -3, Text: "In the middle of difficulty lies opportunity.", Author: "Albert Einstein"},
	{ID: -4, Text: "It is during our darkest moments that we must focus to see the light.", Author: "Aristotle"},
	{ID: -5, Text: "Whoever is happy will make others happy too.", Author: "Anne Frank"},
	{ID: -6, Text: "Do not go where the path may lead, go instead where there is no path and leave a trail.", Author: "Ralph Waldo Emerson"},
	{ID: -7, Text: "The future belongs to those who believe in the beauty of their dreams.", Author: "Eleanor Roosevelt"},
	{ID: -8, Text: "It does not matter how slowly you go as long as you do not stop.", Author: "Confucius"},
	{ID: -9, Text: "Everything you've ever wanted is on the other side of fear.", Author: "George Addair"},
	{ID: -10, Text: "Success is not final, failure is not fatal: it is the courage to continue that counts.", Author: "Winston Churchill"},
	{ID: -11, Text: "Believe you can and you're halfway there.", Author: "Theodore Roosevelt"},
	{ID: -12, Text: "The only impossible journey is the one you never begin.", Author: "Tony Robbins"},
	{ID: -13, Text: "In three words I can sum up everything I've learned about life: it goes on.", Author: "Robert Frost"},
	{ID: -14, Text: "If you want to lift yourself up, lift up someone else.", Author: "Booker T. Washington"},
	{ID: -15, Text: "Spread love everywhere you go. Let no one ever come to you without leaving happier.", Author: "Mother Teresa"},
}

// Words returns n words built by concatenating whole quotes end to end
// (never splitting a quote mid-sentence) from pool, picked at random
// without repeating the immediately previous pick, until at least n words
// are collected -- then trimmed to exactly n, forcing the cut-off word to
// end in sentence punctuation if the trim landed mid-quote.
func Words(pool []Quote, n int) []string {
	pool = nonEmpty(pool)
	if len(pool) == 0 || n <= 0 {
		return nil
	}

	out := make([]string, 0, n)
	last := -1
	for len(out) < n {
		idx := rand.Intn(len(pool))
		for len(pool) > 1 && idx == last {
			idx = rand.Intn(len(pool))
		}
		last = idx
		out = append(out, wordsOf(pool[idx])...)
	}

	out = out[:n]
	if ln := len(out); ln > 0 && !endsSentence(out[ln-1]) {
		out[ln-1] = strings.TrimRight(out[ln-1], ",;:") + "."
	}
	return out
}

// nonEmpty filters out any quote with blank text, guaranteeing every
// remaining entry contributes at least one word -- otherwise a pool of
// entirely-blank quotes (malformed API data) would spin Words forever.
func nonEmpty(pool []Quote) []Quote {
	out := make([]Quote, 0, len(pool))
	for _, q := range pool {
		if strings.TrimSpace(q.Text) != "" {
			out = append(out, q)
		}
	}
	return out
}

// wordsOf splits a quote's text into its space-delimited words, adding a
// trailing period first if the quote doesn't already end in one -- most do,
// but not all source material does.
func wordsOf(q Quote) []string {
	text := strings.TrimSpace(q.Text)
	if !endsSentence(text) {
		text += "."
	}
	return strings.Fields(text)
}

func endsSentence(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '!', '?':
		return true
	default:
		return false
	}
}
