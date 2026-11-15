package app

import "math/rand"

const (
	// drillRepeats is how many times each missed word comes up in a drill,
	// at least.
	drillRepeats = 3
	// drillMinLength is the fewest words a drill has: a drill of one or
	// two short words repeated three times is over in a burst, fast
	// enough to read as scripted input (see game.IsSuspiciousRun) and too
	// short to practise much. Few words are repeated more to reach it.
	drillMinLength = 12
	// drillMaxWords caps how many different words a drill holds, so a
	// rough run doesn't turn into a very long drill.
	drillMaxWords = 30
)

// buildDrill lays out a drill of missed: each different word (the first
// drillMaxWords of them) drillRepeats times, or more to make
// drillMinLength, shuffled so that no word follows itself when there's
// more than one to choose from.
func buildDrill(missed []string) []string {
	var distinct []string
	seen := map[string]bool{}
	for _, w := range missed {
		if !seen[w] && len(distinct) < drillMaxWords {
			seen[w] = true
			distinct = append(distinct, w)
		}
	}
	missed = distinct
	if len(missed) == 0 {
		return nil
	}
	repeats := max(drillRepeats, (drillMinLength+len(missed)-1)/len(missed))
	left := make(map[string]int, len(missed))
	for _, w := range missed {
		left[w] = repeats
	}
	out := make([]string, 0, len(missed)*repeats)
	for len(out) < cap(out) {
		prev := ""
		if len(out) > 0 {
			prev = out[len(out)-1]
		}
		// A word holding more than half of what's left must go now, or
		// it would be forced to follow itself later. Otherwise any word
		// but the last one is picked at random.
		var options []string
		total := 0
		for _, w := range missed {
			total += left[w]
			if left[w] > 0 && w != prev {
				options = append(options, w)
			}
		}
		var w string
		switch {
		case len(options) == 0:
			w = prev // only the last word is left: it has to repeat
		default:
			w = options[rand.Intn(len(options))]
			for _, o := range options {
				if 2*left[o] > total {
					w = o
				}
			}
		}
		left[w]--
		out = append(out, w)
	}
	return out
}
