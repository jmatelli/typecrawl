package app

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jmatelli/typecrawl/internal/achievements"
	"github.com/jmatelli/typecrawl/internal/avatar"
	"github.com/jmatelli/typecrawl/internal/graph"
	"github.com/jmatelli/typecrawl/internal/heatmap"
	"github.com/jmatelli/typecrawl/internal/keyboard"
	"github.com/jmatelli/typecrawl/internal/stats"
)

var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// stripANSI removes styling so a view can be compared as plain text.
func stripANSI(s string) string { return sgrPattern.ReplaceAllString(s, "") }

// sgrParams returns every parameter of every styling sequence in s.
func sgrParams(s string) []int {
	var out []int
	for _, m := range sgrPattern.FindAllStringSubmatch(s, -1) {
		for _, p := range strings.Split(m[1], ";") {
			n, _ := strconv.Atoi(p) // an empty parameter means 0, a reset
			out = append(out, n)
		}
	}
	return out
}

// isColorParam reports whether an SGR parameter sets a foreground or
// background colour.
func isColorParam(n int) bool {
	return (n >= 30 && n <= 38) || (n >= 40 && n <= 48) || (n >= 90 && n <= 97) || (n >= 100 && n <= 107)
}

func hasParam(params []int, want int) bool {
	for _, n := range params {
		if n == want {
			return true
		}
	}
	return false
}

const viewDumpEnv = "TYPECRAWL_TEST_DUMP_VIEWS"

// TestMain lets the test binary double as a tiny program that prints
// every kind of styled output the app produces. Whether colour is on is
// decided once, when the program starts, so the only faithful way to test
// NO_COLOR is to start a process with it set.
func TestMain(m *testing.M) {
	if os.Getenv(viewDumpEnv) != "" {
		dumpViews()
		return
	}
	os.Exit(m.Run())
}

func dumpViews() {
	// Stand in for a colour-capable terminal: without this, a process
	// whose output is a pipe emits no styling at all and the test would
	// pass without proving anything.
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)

	typing := newTestTypingModel(modeWords, []string{"alpha", "beta", "gamma", "delta", "omega"}, 100)
	typing = typeString(typing, "alpha")
	typing = pressSpace(typing)
	typing = typeString(typing, "bx") // one right, one wrong
	typing.goldenWordIdx, typing.greenWordIdx, typing.blueWordIdx = 2, 3, 4
	typing.shieldActive = true
	typing.shieldUntil = time.Now().Add(time.Minute)
	typing.focusWeak = true
	typing.pasteAttempts = 1
	fmt.Println("=== typing")
	fmt.Println(typing.View())

	result := stats.Result{WPM: 60, Accuracy: 97, Duration: 30 * time.Second, WPMSeries: []float64{50, 55, 60, 58}, BurstSeries: []float64{50, 70, 65, 40}, ErrorWords: []int{2}}
	fmt.Println("=== results")
	fmt.Println(newResultsModel(result, 100, resultsExtra{xpGained: 50, isNewPB: true, personalBest: 60, oldLevel: 1, newLevel: 2, levelsGained: 1, hp: 80, maxHP: 100, ghostWPMSeries: []float64{40, 50, 55, 57}}).View())

	fmt.Println("=== packages")
	fmt.Println(graph.RenderCompare("wpm", []float64{50, 55, 60, 58}, []float64{40, 50, 55, 57}, []int{2}, 60, 8))
	fmt.Println(keyboard.Render(map[rune]int{'a': 1, 's': 3, 'd': 6, 'f': 10}))
	fmt.Println(heatmap.Render(map[string]int{time.Now().Format("2006-01-02"): 5, time.Now().AddDate(0, 0, -1).Format("2006-01-02"): 1}, 8))
	fmt.Println(avatar.Generate("alice"))
	for _, tier := range []achievements.Tier{achievements.Bronze, achievements.Silver, achievements.Gold, achievements.Diamond, achievements.Master, achievements.Grandmaster} {
		fmt.Println(tierStyle(tier).Render("tier"))
	}
	fmt.Println(titleStyle.Render("title"), levelUpStyle.Render("level up"), keycapLetterStyle.Render("k"))
}

// runViewDump runs the dump with the colour-related variables cleared and
// then set to extraEnv.
func runViewDump(t *testing.T, extraEnv ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	env := []string{viewDumpEnv + "=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "NO_COLOR=") && !strings.HasPrefix(kv, "CLICOLOR=") && !strings.HasPrefix(kv, "CLICOLOR_FORCE=") {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("view dump failed: %v\n%s", err, out)
	}
	return string(out)
}

func hasColor(params []int) bool {
	for _, n := range params {
		if isColorParam(n) {
			return true
		}
	}
	return false
}

func TestNoColorRemovesEveryColourButKeepsTextAttributes(t *testing.T) {
	// Controls: with colour on, the same output does contain colour. If
	// it didn't, the checks below would pass for the wrong reason.
	for _, env := range [][]string{nil, {"CLICOLOR=0", "CLICOLOR_FORCE=1"}} {
		if !hasColor(sgrParams(runViewDump(t, env...))) {
			t.Fatalf("expected colour sequences with %v; the dump no longer exercises styling", env)
		}
	}

	for _, env := range []string{"NO_COLOR=1", "CLICOLOR=0"} {
		t.Run(env, func(t *testing.T) {
			out := runViewDump(t, env)
			for _, n := range sgrParams(out) {
				if isColorParam(n) {
					t.Fatalf("the output still sets a colour (SGR %d):\n%q", n, out)
				}
			}

			// The typing view has to stay readable: typed text bold, a
			// mistake in reverse video, upcoming words faint.
			typing := out[strings.Index(out, "=== typing"):strings.Index(out, "=== results")]
			tp := sgrParams(typing)
			for name, code := range map[string]int{"bold": 1, "faint": 2, "underline": 4, "reverse": 7} {
				if !hasParam(tp, code) {
					t.Errorf("expected the typing view to still use %s", name)
				}
			}
			plain := stripANSI(typing)
			for _, want := range []string{"*gamma", "+delta", "~omega", "* shield active", "~ shield "} {
				if !strings.Contains(plain, want) {
					t.Errorf("expected %q in the colourless typing view, got:\n%s", want, plain)
				}
			}
		})
	}
}
