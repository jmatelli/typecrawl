// Package heatmap renders a GitHub-style daily activity grid: one column
// per calendar week, one row per weekday, each cell shaded by how much
// activity happened that day.
package heatmap

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	cellWidth   = 2 // "■ " per day cell
	gutterWidth = 4 // weekday label column, e.g. "Mon "
	minWeeks    = 4
	maxWeeks    = 53
)

// levelColors run from "no activity" to "heaviest activity", as a green
// intensity ramp like GitHub's. Each is an AdaptiveColor so lipgloss picks
// the light- or dark-terminal variant automatically (via HasDarkBackground,
// which queries the terminal's actual background color) -- a fixed gray
// for "no activity" reads fine on a light background but nearly vanishes
// against a dark one, so that level in particular needs its own dark
// variant rather than one gray for both.
var levelColors = []lipgloss.AdaptiveColor{
	{Light: "#ebedf0", Dark: "#484f58"}, // no activity
	{Light: "#9be9a8", Dark: "#0e4429"},
	{Light: "#40c463", Dark: "#006d32"},
	{Light: "#30a14e", Dark: "#26a641"},
	{Light: "#216e39", Dark: "#39d353"}, // heaviest activity
}

var dimStyle = lipgloss.NewStyle().Faint(true)

func cellStyle(level int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(levelColors[level])
}

func levelFor(count int) int {
	switch {
	case count <= 0:
		return 0
	case count == 1:
		return 1
	case count <= 3:
		return 2
	case count <= 6:
		return 3
	default:
		return 4
	}
}

// Render draws the heatmap for counts (day string "2006-01-02" -> activity
// count) covering `weeks` calendar weeks up to and including today.
func Render(counts map[string]int, weeks int) string {
	weeks = min(max(weeks, minWeeks), maxWeeks)

	today := truncateToDay(time.Now())
	start := today.AddDate(0, 0, -(weeks*7 - 1))
	for start.Weekday() != time.Sunday {
		start = start.AddDate(0, 0, -1)
	}
	cols := int(today.Sub(start).Hours()/24)/7 + 1

	type cell struct {
		valid bool
		level int
	}
	grid := make([][]cell, 7)
	for r := range grid {
		grid[r] = make([]cell, cols)
	}
	monthAt := make([]string, cols)

	total := 0
	lastMonth := time.Month(0)
	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		col := int(d.Sub(start).Hours()/24) / 7
		row := int(d.Weekday())
		count := counts[d.Format("2006-01-02")]
		total += count
		grid[row][col] = cell{valid: true, level: levelFor(count)}
		if row == 0 && d.Month() != lastMonth {
			monthAt[col] = d.Month().String()[:3]
			lastMonth = d.Month()
		}
	}

	var b strings.Builder

	// Month labels, placed at each month-change column and skipped if
	// they'd overlap the previous label.
	labelRow := []rune(strings.Repeat(" ", cols*cellWidth))
	lastLabelEnd := -1
	for col, label := range monthAt {
		if label == "" {
			continue
		}
		pos := col * cellWidth
		if pos <= lastLabelEnd {
			continue
		}
		for i, r := range label {
			if pos+i >= len(labelRow) {
				break
			}
			labelRow[pos+i] = r
		}
		lastLabelEnd = pos + len(label)
	}
	b.WriteString(strings.Repeat(" ", gutterWidth))
	b.WriteString(dimStyle.Render(string(labelRow)))
	b.WriteString("\n")

	dayLabels := [7]string{"", "Mon", "", "Wed", "", "Fri", ""}
	for row := range 7 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("%-*s", gutterWidth, dayLabels[row])))
		for col := range cols {
			c := grid[row][col]
			if !c.valid {
				b.WriteString(strings.Repeat(" ", cellWidth))
				continue
			}
			b.WriteString(cellStyle(c.level).Render("■ "))
		}
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat(" ", gutterWidth))
	fmt.Fprintf(&b, "%d exercises in the last %d weeks   ", total, weeks)
	b.WriteString(dimStyle.Render("Less "))
	for _, lvl := range []int{0, 1, 2, 3, 4} {
		b.WriteString(cellStyle(lvl).Render("■ "))
	}
	b.WriteString(dimStyle.Render("More"))

	return b.String()
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
