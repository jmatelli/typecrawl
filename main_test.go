package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmatelli/typecrawl/internal/app"
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
