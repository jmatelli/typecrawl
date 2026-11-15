package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmatelli/typecrawl/internal/app"
	"github.com/jmatelli/typecrawl/internal/textsource"
)

func TestWriteExitOutputJSONLinesOnStdoutOnly(t *testing.T) {
	runs := []app.RunSummary{{Profile: "a", Mode: "time", Target: 30, Verified: true}, {Profile: "a", Mode: "words", Target: 50}}
	var stdout, screen bytes.Buffer
	if err := writeExitOutput(&stdout, &screen, runs, runs[:1], true); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected one JSON line per run, got %q", stdout.String())
	}
	for i, l := range lines {
		var got app.RunSummary
		if err := json.Unmarshal([]byte(l), &got); err != nil || got.Target != runs[i].Target {
			t.Fatalf("line %d is not run %d as JSON: %q (%v)", i, i, l, err)
		}
	}
	if strings.Contains(stdout.String(), "typecrawl ·") {
		t.Fatal("the shared text summary must not go to stdout, where only JSON belongs")
	}
	if screen.String() != runs[0].Text()+"\n" {
		t.Fatalf("expected the shared summary on screen, got %q", screen.String())
	}
}

func TestWriteExitOutputWithoutJSON(t *testing.T) {
	runs := []app.RunSummary{{Mode: "time", Target: 15, Verified: true}}
	var stdout, screen bytes.Buffer
	if err := writeExitOutput(&stdout, &screen, runs, nil, false); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || screen.Len() != 0 {
		t.Fatalf("expected no output without --json or shared runs, got %q / %q", stdout.String(), screen.String())
	}
}

func TestCustomSourceFlag(t *testing.T) {
	if src, err := customSource("", ""); err != nil || src.Custom() {
		t.Fatalf("expected no source without either flag, got %+v, %v", src, err)
	}

	dir := t.TempDir()
	list := filepath.Join(dir, "french.txt")
	if err := os.WriteFile(list, []byte("bonjour merci"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	src, err := customSource("french.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != textsource.WordList || !filepath.IsAbs(src.Path) || filepath.Base(src.Path) != "french.txt" {
		t.Fatalf("expected the word list by its absolute path, got %+v", src)
	}
	if src, err := customSource("", list); err != nil || src.Kind != textsource.Text {
		t.Fatalf("expected a text source, got %+v, %v", src, err)
	}

	for name, path := range map[string]string{
		"missing": filepath.Join(dir, "nope.txt"),
		"blank":   writeTemp(t, dir, "blank.txt", " \n "),
		"binary":  writeTemp(t, dir, "bin.dat", "\xff\xfe\x00"),
	} {
		if _, err := customSource(path, ""); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: expected an error naming the file, got %v", name, err)
		}
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
