// Package theme is the one place colours come from, so that they can be
// turned off everywhere at once.
//
// Setting the NO_COLOR environment variable (https://no-color.org) to any
// non-empty value removes colour but keeps bold, faint, underline and
// reverse video, which the views rely on to stay readable. Every colour
// in the program must be made with Color; a lipgloss.Color used directly
// would still be drawn with NO_COLOR set.
package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// noColor follows the same rule as the styling library (termenv's
// EnvNoColor), so the two never disagree: NO_COLOR set to anything, or
// CLICOLOR=0 unless CLICOLOR_FORCE overrides it.
var noColor = os.Getenv("NO_COLOR") != "" ||
	(os.Getenv("CLICOLOR") == "0" && !cliColorForced())

func cliColorForced() bool {
	forced := os.Getenv("CLICOLOR_FORCE")
	return forced != "" && forced != "0"
}

// NoColor reports whether colour is turned off.
func NoColor() bool { return noColor }

// Color returns the ANSI colour with the given code, or no colour at all
// when colour is turned off.
func Color(code string) lipgloss.TerminalColor {
	if noColor {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(code)
}

// Adaptive is Color for a colour that differs between light and dark
// terminal backgrounds.
func Adaptive(light, dark string) lipgloss.TerminalColor {
	if noColor {
		return lipgloss.NoColor{}
	}
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

// Pick returns colored normally and plain when colour is turned off, for
// the places where dropping the colour alone would lose the meaning and
// another attribute has to stand in for it.
func Pick(colored, plain lipgloss.Style) lipgloss.Style {
	if noColor {
		return plain
	}
	return colored
}

// Apply prepares the default renderer for out, the terminal the program
// draws to. It only has work to do when colour is turned off.
//
// lipgloss reacts to NO_COLOR by switching to a profile that emits no
// escape sequences whatsoever, which drops bold, faint, underline and
// reverse along with the colours and leaves typed, mistyped and upcoming
// text looking identical. Since Color already withholds every colour,
// the renderer can safely be put back on a profile that keeps text
// attributes. That is only done when out is a terminal that would have
// been styled without NO_COLOR; a pipe or a dumb terminal stays plain.
func Apply(out *os.File) {
	if !noColor {
		return
	}
	if termenv.NewOutput(out).ColorProfile() != termenv.Ascii {
		lipgloss.SetColorProfile(termenv.ANSI)
	}
}

// UseOutput points the default renderer's colour detection at out, for
// when the program draws somewhere other than standard output (--json
// draws to standard error and keeps standard output for its JSON). Left
// alone, the renderer would look at standard output, find a pipe, and
// draw everything plain. Call it before Apply.
func UseOutput(out *os.File) {
	o := termenv.NewOutput(out)
	lipgloss.SetColorProfile(o.EnvColorProfile())
	lipgloss.SetHasDarkBackground(o.HasDarkBackground())
}
