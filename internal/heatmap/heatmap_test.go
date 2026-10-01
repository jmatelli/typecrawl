package heatmap

import (
	"strings"
	"testing"
	"time"
)

func TestLevelForBoundaries(t *testing.T) {
	cases := []struct {
		count int
		want  int
	}{
		{-1, 0}, {0, 0}, {1, 1}, {2, 2}, {3, 2}, {4, 3}, {6, 3}, {7, 4}, {100, 4},
	}
	for _, c := range cases {
		if got := levelFor(c.count); got != c.want {
			t.Errorf("levelFor(%d) = %d, want %d", c.count, got, c.want)
		}
	}
}

func TestRenderWithNoActivityStillRendersAGrid(t *testing.T) {
	out := Render(nil, 8)
	if out == "" {
		t.Fatal("expected non-empty output for nil counts")
	}
	if !strings.Contains(out, "0 exercises") {
		t.Fatalf("expected a 0-exercise summary line, got:\n%s", out)
	}
}

func TestRenderSumsTotalExercisesAcrossTheRange(t *testing.T) {
	today := time.Now()
	counts := map[string]int{
		today.Format("2006-01-02"):                    3,
		today.AddDate(0, 0, -1).Format("2006-01-02"):  2,
		today.AddDate(0, 0, -40).Format("2006-01-02"): 100, // outside an 8-week window, shouldn't count
	}
	out := Render(counts, 8)
	if !strings.Contains(out, "5 exercises") {
		t.Fatalf("expected the 2 in-range days (3+2=5) to be summed, 40-day-old activity excluded; got:\n%s", out)
	}
}

func TestRenderClampsWeeksToValidRange(t *testing.T) {
	// Should not panic and should still render something reasonable for
	// out-of-range requests.
	for _, weeks := range []int{0, -5, 1000} {
		out := Render(nil, weeks)
		if out == "" {
			t.Fatalf("weeks=%d: expected non-empty output", weeks)
		}
	}
}

func TestRenderIncludesLegend(t *testing.T) {
	out := Render(nil, 8)
	if !strings.Contains(out, "Less") || !strings.Contains(out, "More") {
		t.Fatalf("expected a Less/More legend, got:\n%s", out)
	}
}

func TestTruncateToDayZeroesTimeOfDay(t *testing.T) {
	now := time.Date(2026, 3, 15, 13, 45, 30, 0, time.UTC)
	got := truncateToDay(now)
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("truncateToDay(%v) = %v, want %v", now, got, want)
	}
}

func TestRenderNeverPanicsAcrossAWideRangeOfInputs(t *testing.T) {
	for _, weeks := range []int{4, 10, 26, 53} {
		counts := map[string]int{}
		for i := 0; i < weeks*7; i++ {
			day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
			counts[day] = i % 10
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Render panicked for weeks=%d: %v", weeks, r)
				}
			}()
			_ = Render(counts, weeks)
		}()
	}
}
