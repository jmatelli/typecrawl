// Package words supplies the word lists typing tests are built from. Words
// get progressively longer and less common as the player levels up -- see
// Tier.
package words

import (
	"math/rand"
	"slices"
	"strings"
)

// tiers holds the word lists in increasing order of difficulty. Every 10
// player levels unlocks the next one; the last tier is the ceiling once a
// player outlevels all of them.
var tiers = [][]string{
	tierEasy,
	tierMedium,
	tierHard,
	tierExpert,
}

var tierNames = []string{"Easy", "Medium", "Hard", "Expert"}

var tierEasy = []string{
	"the", "be", "to", "of", "and", "a", "in", "that", "have", "it",
	"for", "not", "on", "with", "he", "as", "you", "do", "at", "this",
	"but", "his", "by", "from", "they", "we", "say", "her", "she", "or",
	"an", "will", "my", "one", "all", "would", "there", "their", "what", "so",
	"up", "out", "if", "about", "who", "get", "which", "go", "me", "when",
	"make", "can", "like", "time", "no", "just", "him", "know", "take", "people",
	"into", "year", "your", "good", "some", "could", "them", "see", "other", "than",
	"then", "now", "look", "only", "come", "its", "over", "think", "also", "back",
	"after", "use", "two", "how", "our", "work", "first", "well", "way", "even",
	"new", "want", "because", "any", "these", "give", "day", "most", "us", "is",
	"water", "long", "find", "here", "thing", "great", "man", "world", "life", "still",
	"hand", "part", "child", "eye", "place", "week", "case", "point", "number", "group",
	"problem", "fact", "house", "system", "school", "program", "question", "during", "against", "between",
}

// tierMedium unlocks at level 10: everyday words in the 6-9 letter range.
var tierMedium = []string{
	"because", "system", "program", "question", "between", "company", "problem",
	"example", "function", "keyboard", "together", "special", "already", "possible",
	"different", "important", "following", "understand", "remember", "practice",
	"exercise", "progress", "accuracy", "standard", "developed", "language",
	"computer", "training", "analysis", "strategy", "consider", "continue",
	"describe", "generate", "identify", "indicate", "maintain", "position",
	"probably", "resource", "surface", "activity", "attitude", "capacity",
}

// tierHard unlocks at level 20: long, multi-syllable words.
var tierHard = []string{
	"development", "information", "environment", "organization", "relationship",
	"performance", "technology", "communication", "understanding", "international",
	"responsibility", "opportunity", "significant", "consequently", "professional",
	"fundamental", "engineering", "architecture", "distribution", "infrastructure",
	"documentation", "implementation", "configuration", "specification",
	"classification", "recommendation", "transformation", "representative",
}

// tierExpert unlocks at level 30 and beyond: the longest, least common words.
var tierExpert = []string{
	"extraordinarily", "incomprehensible", "disproportionate", "characteristic",
	"unquestionably", "interdisciplinary", "counterproductive", "overwhelmingly",
	"circumstantial", "idiosyncratic", "juxtaposition", "serendipitous",
	"quintessential", "perpendicular", "misappropriation", "unconventionally",
	"overcomplicated", "underestimating", "misunderstanding", "incontrovertible",
	"inconsequential", "unpredictability", "disillusionment", "procrastination",
	"disproportionately", "counterintuitive", "notwithstanding", "unprecedented",
}

// Tier returns the word-difficulty tier unlocked at level: every 10 levels
// unlocks the next tier, capped at the hardest one defined.
func Tier(level int) int {
	return min(max(level/10, 0), len(tiers)-1)
}

// TierName returns a human-readable label for a tier, as returned by Tier.
func TierName(tier int) string {
	return tierNames[min(max(tier, 0), len(tierNames)-1)]
}

// noRepeatWindow is how many of the most recently used words are excluded
// when picking the next one, so the same word can never appear back to back
// and doesn't resurface too soon after. It's capped by the tier's size, so
// even a tiny list still refuses at least an immediate repeat.
const noRepeatWindow = 5

// ForLevel returns n random words drawn from the tier unlocked at level.
// tail, if given, is the end of an already-generated word list (e.g. when
// topping up a long test's lookahead buffer); it seeds the no-repeat
// window so the new batch doesn't repeat too soon after what came before.
// tail entries may carry Punctuate's capitalization/trailing punctuation --
// they're normalized back to base form before being compared against.
func ForLevel(n, level int, tail ...string) []string {
	list := tiers[Tier(level)]
	out := make([]string, 0, n)

	// Copy rather than alias tail: appending to it below must never touch
	// the caller's backing array (tail is often a slice of m.wordList).
	start := max(0, len(tail)-noRepeatWindow)
	recent := make([]string, len(tail)-start, noRepeatWindow)
	for i, w := range tail[start:] {
		recent[i] = baseForm(w)
	}

	for range n {
		w := pickWord(list, recent)
		out = append(out, w)
		recent = append(recent, w)
		if len(recent) > noRepeatWindow {
			recent = recent[1:]
		}
	}
	return out
}

// pickWord picks a random word from list that isn't in recent's tail,
// leaving at least one candidate so it always terminates.
func pickWord(list, recent []string) string {
	window := min(len(recent), len(list)-1)
	excluded := recent[len(recent)-window:]
	for range 30 {
		w := list[rand.Intn(len(list))]
		if !slices.Contains(excluded, w) {
			return w
		}
	}
	return list[rand.Intn(len(list))] // guarantees termination; vanishingly unlikely to be reached
}

// ForLevelWeak is like ForLevel, but weights word selection toward words
// containing characters the player mistypes most (weakChars: character ->
// mistake count), so practice concentrates on real weaknesses instead of
// pure random chance. It falls back to ForLevel's plain uniform selection
// when weakChars is empty (nothing learned yet to focus on).
func ForLevelWeak(n, level int, weakChars map[rune]int, tail ...string) []string {
	if len(weakChars) == 0 {
		return ForLevel(n, level, tail...)
	}

	list := tiers[Tier(level)]
	out := make([]string, 0, n)

	start := max(0, len(tail)-noRepeatWindow)
	recent := make([]string, len(tail)-start, noRepeatWindow)
	for i, w := range tail[start:] {
		recent[i] = baseForm(w)
	}

	for range n {
		w := pickWordWeighted(list, recent, weakChars)
		out = append(out, w)
		recent = append(recent, w)
		if len(recent) > noRepeatWindow {
			recent = recent[1:]
		}
	}
	return out
}

// weakCharCap bounds how much a single character's mistake count can
// contribute to a word's selection weight, so months of accumulated data
// skew practice heavily without making selection degenerate.
const weakCharCap = 20

// wordWeakScore sums weakChars' (capped) counts for each character in w,
// so words containing the player's most-mistyped characters score higher.
func wordWeakScore(w string, weakChars map[rune]int) int {
	score := 0
	for _, r := range w {
		score += min(weakChars[r], weakCharCap)
	}
	return score
}

// pickWordWeighted is pickWord's weighted counterpart: candidates are drawn
// from list excluding recent's tail, with each word's chance proportional
// to 1 + its weak-character score (so every word stays reachable, just
// less likely).
func pickWordWeighted(list, recent []string, weakChars map[rune]int) string {
	window := min(len(recent), len(list)-1)
	excluded := recent[len(recent)-window:]

	type candidate struct {
		word   string
		weight int
	}
	candidates := make([]candidate, 0, len(list))
	total := 0
	for _, w := range list {
		if slices.Contains(excluded, w) {
			continue
		}
		weight := 1 + wordWeakScore(w, weakChars)
		candidates = append(candidates, candidate{w, weight})
		total += weight
	}
	if len(candidates) == 0 {
		return list[rand.Intn(len(list))] // degenerate case: everything excluded
	}

	r := rand.Intn(total)
	for _, c := range candidates {
		r -= c.weight
		if r < 0 {
			return c.word
		}
	}
	return candidates[len(candidates)-1].word // unreachable in practice
}

// sentenceEnders are the marks that close a sentence, weighted toward '.'.
var sentenceEnders = []string{".", ".", ".", "!", "?"}

// Punctuate returns a copy of words dressed up with sentence-style
// capitalization and punctuation: the first word and any word following a
// sentence-ending mark are capitalized, sentences run roughly 6-9 words
// before ending in '.', '!', or '?', and a comma occasionally breaks up a
// sentence. The punctuation is attached directly to the word it follows, so
// it flows through the existing space-delimited word model unchanged.
func Punctuate(list []string) []string {
	out := make([]string, len(list))
	capNext := true
	sinceSentenceEnd := 0

	for i, w := range list {
		word := w
		if capNext {
			word = capitalize(word)
			capNext = false
		}
		sinceSentenceEnd++

		switch {
		case sinceSentenceEnd >= 6 && rand.Intn(3) == 0:
			word += sentenceEnders[rand.Intn(len(sentenceEnders))]
			capNext = true
			sinceSentenceEnd = 0
		case i < len(list)-1 && rand.Intn(10) == 0:
			word += ","
		}
		out[i] = word
	}

	if n := len(out); n > 0 && !endsSentence(out[n-1]) {
		out[n-1] = strings.TrimSuffix(out[n-1], ",") + "."
	}
	return out
}

func capitalize(w string) string {
	if w == "" {
		return w
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

// baseForm strips Punctuate's trailing punctuation and capitalization,
// recovering the plain lowercase word the no-repeat tracking compares
// against, regardless of whether the caller's history was punctuated.
func baseForm(w string) string {
	w = strings.TrimRight(w, ".!?,")
	if w == "" {
		return w
	}
	return strings.ToLower(w[:1]) + w[1:]
}

func endsSentence(w string) bool {
	if w == "" {
		return false
	}
	switch w[len(w)-1] {
	case '.', '!', '?':
		return true
	default:
		return false
	}
}
