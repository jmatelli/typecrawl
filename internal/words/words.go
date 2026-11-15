// Package words supplies the word lists typing tests are built from. Words
// get progressively longer and less common as the player levels up -- see
// Tier.
package words

import (
	"math/rand"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
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
	return FromList(tiers[Tier(level)], n, weakChars, tail...)
}

// FromList is ForLevelWeak over a list of the caller's own, such as a
// player's word list file: n words drawn from list, weighted toward
// weakChars when it isn't empty, with the same no-repeat window. list
// must not be empty.
func FromList(list []string, n int, weakChars map[rune]int, tail ...string) []string {
	// The weights are worked out once per call rather than once per word:
	// a player's list can hold hundreds of thousands of words.
	var cumulative []int
	if len(weakChars) > 0 {
		cumulative = make([]int, len(list))
		total := 0
		for i, w := range list {
			total += 1 + wordWeakScore(w, weakChars)
			cumulative[i] = total
		}
	}
	draw := func() string {
		if cumulative == nil {
			return list[rand.Intn(len(list))]
		}
		// The first word whose running total passes r: each word's chance
		// is its weight, 1 plus its weak-character score, over the total.
		i, _ := slices.BinarySearch(cumulative, rand.Intn(cumulative[len(cumulative)-1])+1)
		return list[i]
	}

	out := make([]string, 0, n)
	start := max(0, len(tail)-noRepeatWindow)
	recent := make([]string, len(tail)-start, noRepeatWindow)
	for i, w := range tail[start:] {
		recent[i] = baseForm(w)
	}

	for range n {
		w := pickNotRecent(list, recent, draw)
		out = append(out, w)
		recent = append(recent, w)
		if len(recent) > noRepeatWindow {
			recent = recent[1:]
		}
	}
	return out
}

// pickNotRecent draws from list until it gets a word outside recent's
// tail. If a heavily weighted word keeps coming up, it falls back to the
// first such word after a random point, so it always terminates and never
// repeats a word inside the window.
func pickNotRecent(list, recent []string, draw func() string) string {
	window := min(len(recent), len(list)-1)
	excluded := recent[len(recent)-window:]
	for range 30 {
		if w := draw(); !slices.Contains(excluded, w) {
			return w
		}
	}
	offset := rand.Intn(len(list))
	for i := range list {
		if w := list[(offset+i)%len(list)]; !slices.Contains(excluded, w) {
			return w
		}
	}
	return list[offset] // every word is in the window: only when list repeats words
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

// capitalize upper-cases w's first letter, which may take more than one
// byte (é, ж).
func capitalize(w string) string {
	r, size := utf8.DecodeRuneInString(w)
	if size == 0 {
		return w
	}
	return string(unicode.ToUpper(r)) + w[size:]
}

// baseForm strips Punctuate's trailing punctuation and capitalization,
// recovering the plain lowercase word the no-repeat tracking compares
// against, regardless of whether the caller's history was punctuated.
func baseForm(w string) string {
	w = strings.TrimRight(w, ".!?,")
	r, size := utf8.DecodeRuneInString(w)
	if size == 0 {
		return w
	}
	return string(unicode.ToLower(r)) + w[size:]
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
