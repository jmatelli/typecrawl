package graph

import (
	"strings"
	"testing"
)

func TestRenderWithInsufficientDataShowsPlaceholder(t *testing.T) {
	for _, series := range [][]float64{nil, {1}, {}} {
		out := Render("Title", series, nil, 60, 10)
		if !strings.Contains(out, "not enough data yet") {
			t.Fatalf("series=%v: expected placeholder text, got:\n%s", series, out)
		}
	}
}

func TestRenderIncludesTitle(t *testing.T) {
	out := Render("My Chart Title", []float64{10, 20, 30}, nil, 60, 10)
	if !strings.Contains(out, "My Chart Title") {
		t.Fatalf("expected title in output, got:\n%s", out)
	}
}

func TestRenderClampsUndersizedDimensions(t *testing.T) {
	// Width/height below the minimum must not panic or produce empty output.
	out := Render("Title", []float64{10, 20, 30, 15}, nil, 1, 1)
	if out == "" {
		t.Fatal("expected non-empty output even with tiny requested dimensions")
	}
}

func TestRenderHandlesFlatSeriesWithoutDivideByZero(t *testing.T) {
	// All values equal -- maxY == minY internally, must not panic or
	// produce NaN/garbage.
	out := Render("Flat", []float64{50, 50, 50, 50}, nil, 60, 10)
	if out == "" {
		t.Fatal("expected non-empty output for a flat series")
	}
}

func TestRenderOutOfRangeErrorIndicesAreIgnored(t *testing.T) {
	// Indices outside the series must not panic (out-of-bounds access).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Render panicked on out-of-range error indices: %v", r)
		}
	}()
	out := Render("Title", []float64{1, 2, 3}, []int{-5, 0, 2, 99}, 60, 10)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestRenderCompareFallsBackToPlainRenderWithoutEnoughGhostData(t *testing.T) {
	series := []float64{40, 45, 50}
	for _, ghost := range [][]float64{nil, {1}} {
		got := RenderCompare("Title", series, ghost, nil, 60, 10)
		want := Render("Title", series, nil, 60, 10)
		if got != want {
			t.Fatalf("ghost=%v: expected fallback to plain Render", ghost)
		}
	}
}

func TestRenderCompareIncludesBothCurvesInLegend(t *testing.T) {
	series := []float64{40, 45, 50, 55, 60}
	ghost := []float64{38, 44, 52, 58, 62}
	out := RenderCompare("WPM over time", series, ghost, nil, 60, 10)
	if !strings.Contains(out, "you") || !strings.Contains(out, "ghost (PB)") {
		t.Fatalf("expected a legend mentioning both curves, got:\n%s", out)
	}
}

func TestRenderCompareHandlesGhostLongerThanSeries(t *testing.T) {
	// A ghost that ran further than this attempt (e.g. you finished early
	// in time mode) must not panic from an out-of-range X extent.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RenderCompare panicked with a longer ghost series: %v", r)
		}
	}()
	series := []float64{10, 20, 30}
	ghost := []float64{10, 20, 30, 40, 50, 60, 70}
	out := RenderCompare("Title", series, ghost, nil, 60, 10)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}
