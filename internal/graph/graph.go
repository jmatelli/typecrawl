// Package graph renders a single-metric line chart (via ntcharts) with
// red markers overlaid at word indices where the user made a mistake.
package graph

import (
	"fmt"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/linechart"
	"github.com/charmbracelet/lipgloss"
)

var (
	axisStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	lineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	titleStyle = lipgloss.NewStyle().Bold(true)
	ghostStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// Render draws series (one value per word index) as a smooth braille line
// chart sized width x height, with a red dot at each index in errorX.
func Render(title string, series []float64, errorX []int, width, height int) string {
	if len(series) < 2 {
		return titleStyle.Render(title) + "\n" + axisStyle.Render("not enough data yet")
	}
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}

	minY, maxY := series[0], series[0]
	for _, v := range series {
		minY = min(minY, v)
		maxY = max(maxY, v)
	}
	if maxY == minY {
		maxY++
	}
	pad := (maxY - minY) * 0.1
	minY = max(minY-pad, 0)
	maxY += pad

	lc := linechart.New(width, height, 0, float64(len(series)-1), minY, maxY,
		linechart.WithStyles(axisStyle, labelStyle, lineStyle),
		linechart.WithXYSteps(8, 3),
		linechart.WithXLabelFormatter(func(_ int, v float64) string {
			return fmt.Sprintf("%d", int(v))
		}),
		linechart.WithYLabelFormatter(func(_ int, v float64) string {
			return fmt.Sprintf("%.0f", v)
		}),
	)

	if lc.GraphWidth() == 0 || lc.GraphHeight() == 0 {
		return titleStyle.Render(title) + "\n" + axisStyle.Render("terminal too small")
	}

	for i := 1; i < len(series); i++ {
		lc.DrawBrailleLine(
			canvas.Float64Point{X: float64(i - 1), Y: series[i-1]},
			canvas.Float64Point{X: float64(i), Y: series[i]},
		)
	}
	lc.DrawXYAxisAndLabel()

	for _, idx := range errorX {
		if idx < 0 || idx >= len(series) {
			continue
		}
		lc.DrawRuneWithStyle(canvas.Float64Point{X: float64(idx), Y: series[idx]}, '●', errorStyle)
	}

	return titleStyle.Render(title) + "\n" + lc.View()
}

// RenderCompare draws series alongside a ghost series (e.g. a personal
// best's pacing) on one shared chart, so the two curves can be compared
// directly. errorX still marks series's mistakes; the ghost has none
// marked. If ghost is too short to compare, it falls back to Render.
func RenderCompare(title string, series, ghost []float64, errorX []int, width, height int) string {
	if len(ghost) < 2 {
		return Render(title, series, errorX, width, height)
	}
	if len(series) < 2 {
		return titleStyle.Render(title) + "\n" + axisStyle.Render("not enough data yet")
	}
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}

	minY, maxY := series[0], series[0]
	for _, v := range series {
		minY = min(minY, v)
		maxY = max(maxY, v)
	}
	for _, v := range ghost {
		minY = min(minY, v)
		maxY = max(maxY, v)
	}
	if maxY == minY {
		maxY++
	}
	pad := (maxY - minY) * 0.1
	minY = max(minY-pad, 0)
	maxY += pad

	maxX := max(len(series)-1, len(ghost)-1)

	lc := linechart.New(width, height, 0, float64(maxX), minY, maxY,
		linechart.WithStyles(axisStyle, labelStyle, lineStyle),
		linechart.WithXYSteps(8, 3),
		linechart.WithXLabelFormatter(func(_ int, v float64) string {
			return fmt.Sprintf("%d", int(v))
		}),
		linechart.WithYLabelFormatter(func(_ int, v float64) string {
			return fmt.Sprintf("%.0f", v)
		}),
	)

	if lc.GraphWidth() == 0 || lc.GraphHeight() == 0 {
		return titleStyle.Render(title) + "\n" + axisStyle.Render("terminal too small")
	}

	for i := 1; i < len(ghost); i++ {
		lc.DrawBrailleLineWithStyle(
			canvas.Float64Point{X: float64(i - 1), Y: ghost[i-1]},
			canvas.Float64Point{X: float64(i), Y: ghost[i]},
			ghostStyle,
		)
	}
	for i := 1; i < len(series); i++ {
		lc.DrawBrailleLine(
			canvas.Float64Point{X: float64(i - 1), Y: series[i-1]},
			canvas.Float64Point{X: float64(i), Y: series[i]},
		)
	}
	lc.DrawXYAxisAndLabel()

	for _, idx := range errorX {
		if idx < 0 || idx >= len(series) {
			continue
		}
		lc.DrawRuneWithStyle(canvas.Float64Point{X: float64(idx), Y: series[idx]}, '●', errorStyle)
	}

	legend := lineStyle.Render("── you") + "   " + ghostStyle.Render("── ghost (PB)")
	return titleStyle.Render(title) + "\n" + lc.View() + "\n" + legend
}
