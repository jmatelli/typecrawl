package stats

import (
	"math"
	"testing"
	"time"
)

func TestRecordCharTracksTotalsAndAccuracy(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now()
	tr.RecordChar(true, 0)
	tr.RecordChar(true, 0)
	tr.RecordChar(false, 0)
	tr.RecordChar(true, 0)

	if tr.totalChars != 4 {
		t.Fatalf("expected 4 total chars, got %d", tr.totalChars)
	}
	if tr.totalCorrect != 3 {
		t.Fatalf("expected 3 correct chars, got %d", tr.totalCorrect)
	}
}

func TestFinishComputesAccuracyFromRecordedChars(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now().Add(-time.Minute)
	for range 8 {
		tr.RecordChar(true, 0)
	}
	for range 2 {
		tr.RecordChar(false, 0)
	}
	result := tr.Finish()
	if result.Accuracy != 80 {
		t.Fatalf("expected 80%% accuracy (8/10), got %v", result.Accuracy)
	}
	if result.TotalChars != 10 || result.CorrectChars != 8 {
		t.Fatalf("expected TotalChars=10 CorrectChars=8, got %d/%d", result.TotalChars, result.CorrectChars)
	}
}

func TestFinishWithNoCharsDefaultsTo100PercentAccuracy(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now()
	result := tr.Finish()
	if result.Accuracy != 100 {
		t.Fatalf("expected 100%% accuracy with zero chars typed, got %v", result.Accuracy)
	}
}

func TestFinishComputesWPMFromCorrectCharsAndDuration(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now().Add(-time.Minute)
	// 50 correct chars in exactly 1 minute = 50/5 = 10 WPM.
	for range 50 {
		tr.RecordChar(true, 0)
	}
	result := tr.Finish()
	if math.Abs(result.WPM-10) > 1 {
		t.Fatalf("expected ~10 WPM, got %v", result.WPM)
	}
}

func TestRecordWordCompleteTracksErrorWordsAndOffsets(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now().Add(-3 * time.Second)
	tr.RecordWordComplete("cat", "cat")   // correct
	tr.RecordWordComplete("dgo", "dog")   // mistake
	tr.RecordWordComplete("fish", "fish") // correct

	result := tr.Finish()
	if len(result.ErrorWords) != 1 || result.ErrorWords[0] != 1 {
		t.Fatalf("expected ErrorWords=[1], got %v", result.ErrorWords)
	}
	if len(result.WPMSeries) != 3 || len(result.BurstSeries) != 3 || len(result.WordEndOffsetsMS) != 3 {
		t.Fatalf("expected 3 entries per per-word series, got WPMSeries=%d BurstSeries=%d WordEndOffsetsMS=%d",
			len(result.WPMSeries), len(result.BurstSeries), len(result.WordEndOffsetsMS))
	}
}

func TestWordEndOffsetsMSTracksCumulativeElapsedTime(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now()
	tr.words = append(tr.words,
		wordRecord{endTime: tr.Start.Add(500 * time.Millisecond), chars: 5},
		wordRecord{endTime: tr.Start.Add(1200 * time.Millisecond), chars: 6},
		wordRecord{endTime: tr.Start.Add(1800 * time.Millisecond), chars: 4},
	)
	tr.totalChars, tr.totalCorrect = 15, 15

	result := tr.Finish()
	want := []int{500, 1200, 1800}
	if len(result.WordEndOffsetsMS) != len(want) {
		t.Fatalf("expected %d offsets, got %d: %v", len(want), len(result.WordEndOffsetsMS), result.WordEndOffsetsMS)
	}
	for i, w := range want {
		if diff := result.WordEndOffsetsMS[i] - w; diff < -5 || diff > 5 {
			t.Fatalf("offset %d: expected ~%dms, got %dms", i, w, result.WordEndOffsetsMS[i])
		}
	}
}

func TestRemoveLastWordUndoesTheMostRecentCompletion(t *testing.T) {
	tr := NewTracker()
	tr.Start = time.Now().Add(-5 * time.Second)
	tr.RecordWordComplete("one", "one")
	tr.RecordWordComplete("two", "two")
	tr.RemoveLastWord()
	tr.RecordWordComplete("three", "three") // replaces the undone word

	result := tr.Finish()
	if len(result.WPMSeries) != 2 {
		t.Fatalf("expected 2 words after undo+replace, got %d", len(result.WPMSeries))
	}
}

func TestRemoveLastWordOnEmptyTrackerDoesNotPanic(t *testing.T) {
	tr := NewTracker()
	tr.RemoveLastWord() // must be a no-op, not a panic
	if len(tr.words) != 0 {
		t.Fatalf("expected no words, got %d", len(tr.words))
	}
}

func TestKeystrokeCVDistinguishesUniformFromHumanTiming(t *testing.T) {
	base := time.Now()

	var uniform []charEvent
	for i := range 40 {
		uniform = append(uniform, charEvent{correct: true, at: base.Add(time.Duration(i) * 50 * time.Millisecond)})
	}
	cv, samples := keystrokeCV(uniform)
	if samples != 39 {
		t.Fatalf("expected 39 samples, got %d", samples)
	}
	if cv > 0.01 {
		t.Fatalf("expected near-zero CV for perfectly uniform timing, got %v", cv)
	}

	deltasMs := []int{40, 90, 60, 200, 55, 45, 70, 300, 50, 65, 40, 110, 80, 55, 250, 45}
	var irregular []charEvent
	at := base
	for _, d := range deltasMs {
		at = at.Add(time.Duration(d) * time.Millisecond)
		irregular = append(irregular, charEvent{correct: true, at: at})
	}
	cv2, samples2 := keystrokeCV(irregular)
	if samples2 != len(deltasMs)-1 {
		t.Fatalf("expected %d samples, got %d", len(deltasMs)-1, samples2)
	}
	if cv2 < 0.15 {
		t.Fatalf("expected clearly human-like CV (>0.15) for irregular timing, got %v", cv2)
	}
}

func TestKeystrokeCVWithTooFewSamplesReturnsZero(t *testing.T) {
	cv, samples := keystrokeCV(nil)
	if cv != 0 || samples != 0 {
		t.Fatalf("expected (0, 0) for no events, got (%v, %d)", cv, samples)
	}
	cv, samples = keystrokeCV([]charEvent{{at: time.Now()}})
	if cv != 0 || samples != 0 {
		t.Fatalf("expected (0, 0) for a single event, got (%v, %d)", cv, samples)
	}
}

func TestFinishPopulatesKeystrokeCVFromChars(t *testing.T) {
	// Bypass RecordChar's real time.Now() (which can produce identical,
	// filtered-out-as-zero timestamps when called in a tight loop faster
	// than clock resolution) and construct evenly-spaced events directly,
	// to verify only that Finish() wires keystrokeCV's output into Result.
	tr := NewTracker()
	tr.Start = time.Now()
	base := tr.Start
	for i := range 35 {
		tr.chars = append(tr.chars, charEvent{correct: true, at: base.Add(time.Duration(i) * 50 * time.Millisecond)})
	}
	tr.totalChars, tr.totalCorrect = 35, 35

	result := tr.Finish()
	if result.KeystrokeSamples != 34 {
		t.Fatalf("expected 34 samples (35 chars -> 34 deltas), got %d", result.KeystrokeSamples)
	}
	if result.KeystrokeCV > 0.01 {
		t.Fatalf("expected near-zero CV for evenly-spaced events, got %v", result.KeystrokeCV)
	}
}
