// Package stats tracks keystrokes and word completions during a typing
// test and derives WPM/accuracy/burst data from them.
package stats

import (
	"math"
	"time"
)

type charEvent struct {
	correct bool
	wordIdx int
	at      time.Time
}

type wordRecord struct {
	endTime  time.Time // absolute completion time, not a relative duration,
	hasError bool      // so RemoveLastWord needs no other state fixed up
	chars    int       // target word length + 1 (for the trailing space)
}

// Tracker accumulates raw typing events for a single test run.
type Tracker struct {
	Start        time.Time
	chars        []charEvent
	words        []wordRecord
	totalCorrect int
	totalChars   int
}

func NewTracker() *Tracker {
	return &Tracker{}
}

// RecordChar logs one keystroke against the word currently being typed.
func (t *Tracker) RecordChar(correct bool, wordIdx int) {
	t.chars = append(t.chars, charEvent{correct: correct, wordIdx: wordIdx, at: time.Now()})
	t.totalChars++
	if correct {
		t.totalCorrect++
	}
}

// RecordWordComplete logs that a word was finished (space pressed).
func (t *Tracker) RecordWordComplete(typed, target string) {
	t.words = append(t.words, wordRecord{
		endTime:  time.Now(),
		hasError: typed != target,
		chars:    len(target) + 1,
	})
}

// RemoveLastWord undoes the most recent RecordWordComplete -- e.g. when the
// player backspaces back into a just-completed word to fix it. Since each
// word only stores its own absolute end time, truncating is always correct
// with nothing else to restore.
func (t *Tracker) RemoveLastWord() {
	if len(t.words) > 0 {
		t.words = t.words[:len(t.words)-1]
	}
}

// Result is the derived summary shown on the results screen.
type Result struct {
	WPM          float64
	Accuracy     float64
	Duration     time.Duration
	WPMSeries    []float64 // cumulative WPM sampled per completed word
	BurstSeries  []float64 // instantaneous WPM for each individual word
	ErrorWords   []int     // indices into WPMSeries/BurstSeries with a mistake
	TotalChars   int
	CorrectChars int

	// KeystrokeCV is the coefficient of variation (stddev/mean) of the
	// intervals between consecutive keystrokes -- real human typing rhythm
	// has meaningfully more relative spread than scripted/injected input
	// tends to. KeystrokeSamples is how many intervals fed it; below a
	// minimum sample size, CV is unreliable and should be ignored (see
	// game.IsSuspiciousRun).
	KeystrokeCV      float64
	KeystrokeSamples int

	// WordEndOffsetsMS is the cumulative elapsed time, in milliseconds,
	// from Start to each completed word -- the run's pacing curve, used to
	// race a later attempt against it as a "ghost".
	WordEndOffsetsMS []int
}

// keystrokeCV returns the coefficient of variation of the intervals between
// consecutive keystrokes in chars, plus how many intervals fed it.
func keystrokeCV(chars []charEvent) (cv float64, samples int) {
	if len(chars) < 2 {
		return 0, 0
	}
	deltas := make([]float64, 0, len(chars)-1)
	for i := 1; i < len(chars); i++ {
		if d := chars[i].at.Sub(chars[i-1].at).Seconds(); d > 0 {
			deltas = append(deltas, d)
		}
	}
	if len(deltas) < 2 {
		return 0, 0
	}

	var sum float64
	for _, d := range deltas {
		sum += d
	}
	mean := sum / float64(len(deltas))

	var variance float64
	for _, d := range deltas {
		diff := d - mean
		variance += diff * diff
	}
	variance /= float64(len(deltas))

	return math.Sqrt(variance) / mean, len(deltas)
}

// Finish closes out the tracker and computes the final Result.
func (t *Tracker) Finish() Result {
	end := time.Now()
	totalDur := end.Sub(t.Start)
	if totalDur <= 0 {
		totalDur = time.Millisecond
	}

	accuracy := 100.0
	if t.totalChars > 0 {
		accuracy = float64(t.totalCorrect) / float64(t.totalChars) * 100
	}

	wpm := (float64(t.totalCorrect) / 5.0) / totalDur.Minutes()

	wpmSeries := make([]float64, 0, len(t.words))
	burstSeries := make([]float64, 0, len(t.words))
	var errorWords []int

	cumChars := 0
	prevEnd := t.Start
	wordEndOffsetsMS := make([]int, 0, len(t.words))
	for i, w := range t.words {
		cumChars += w.chars

		elapsedMin := w.endTime.Sub(t.Start).Minutes()
		if elapsedMin <= 0 {
			elapsedMin = 1.0 / 60000
		}
		wpmSeries = append(wpmSeries, (float64(cumChars)/5.0)/elapsedMin)

		wordMin := w.endTime.Sub(prevEnd).Minutes()
		if wordMin <= 0 {
			wordMin = 1.0 / 60000
		}
		burstSeries = append(burstSeries, (float64(w.chars)/5.0)/wordMin)
		prevEnd = w.endTime

		wordEndOffsetsMS = append(wordEndOffsetsMS, int(w.endTime.Sub(t.Start).Milliseconds()))

		if w.hasError {
			errorWords = append(errorWords, i)
		}
	}

	cv, cvSamples := keystrokeCV(t.chars)

	return Result{
		WPM:          wpm,
		Accuracy:     accuracy,
		Duration:     totalDur,
		WPMSeries:    wpmSeries,
		BurstSeries:  burstSeries,
		ErrorWords:   errorWords,
		TotalChars:   t.totalChars,
		CorrectChars: t.totalCorrect,

		KeystrokeCV:      cv,
		KeystrokeSamples: cvSamples,

		WordEndOffsetsMS: wordEndOffsetsMS,
	}
}
