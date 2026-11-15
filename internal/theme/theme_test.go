package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A colour made without going through this package would still be drawn
// with NO_COLOR set. Nothing in the type system prevents that, so this
// reads the source instead.
func TestEveryColourGoesThroughTheme(t *testing.T) {
	root := filepath.Join("..", "..")
	var checked int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "theme") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		for _, direct := range []string{"lipgloss.Color(", "lipgloss.AdaptiveColor{", "lipgloss.CompleteColor{", "lipgloss.CompleteAdaptiveColor{", "lipgloss.ANSIColor(", "termenv."} {
			if strings.Contains(string(src), direct) {
				t.Errorf("%s uses %s directly; use theme.Color or theme.Adaptive so NO_COLOR can turn it off", path, strings.TrimRight(direct, "({"))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 10 {
		t.Fatalf("only %d source files checked; the walk is not reaching the code", checked)
	}
}
