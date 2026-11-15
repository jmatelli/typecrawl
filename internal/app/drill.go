package app

import "math/rand"

const (
	// drillRepeats is how many times each missed word comes up in a drill.
	drillRepeats = 3
	// drillMaxWords caps how many different words a drill holds, so a
	// rough run doesn't turn into a very long drill.
	drillMaxWords = 30
)

// buildDrill lays out a drill of missed: each word drillRepeats times
// (the first drillMaxWords of them), shuffled so that no word follows
// itself when there's more than one to choose from.
func buildDrill(missed []string) []string {
	missed = missed[:min(len(missed), drillMaxWords)]
	left := make(map[string]int, len(missed))
	for _, w := range missed {
		left[w] = drillRepeats
	}
	out := make([]string, 0, len(missed)*drillRepeats)
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
