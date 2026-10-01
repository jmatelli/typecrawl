package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/app"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// exportData is the shape written by --export: a snapshot of one profile's
// stats, readable as-is or fed to external tooling.
type exportData struct {
	ExportedAt    time.Time                  `json:"exported_at"`
	Profile       storage.Profile            `json:"profile"`
	History       []storage.TestHistoryEntry `json:"history"`
	PersonalBests []storage.PersonalBest     `json:"personal_bests"`
}

func findProfileByName(profiles []*storage.Profile, name string) *storage.Profile {
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

func main() {
	profileFlag := flag.String("profile", "", "quick-launch: profile name to select (skips profile selection)")
	timeFlag := flag.Int("time", 0, "quick-launch: start a time-based test immediately, in seconds (requires --profile)")
	wordsFlag := flag.Int("words", 0, "quick-launch: start a word-count test immediately (requires --profile)")
	punctuationFlag := flag.Bool("punctuation", false, "quick-launch: enable punctuation (requires --time or --words)")
	zenFlag := flag.Bool("zen", false, "quick-launch: enable zen mode (requires --time or --words)")
	focusWeakFlag := flag.Bool("focus-weak", false, "quick-launch: bias word choice toward your weakest keys (requires --time or --words)")
	listProfilesFlag := flag.Bool("list-profiles", false, "list saved profiles and exit")
	exportFlag := flag.String("export", "", "export a profile's stats to this JSON file path and exit (requires --profile)")
	flag.Parse()

	if *timeFlag != 0 && *wordsFlag != 0 {
		fmt.Fprintln(os.Stderr, "error: specify only one of --time or --words")
		os.Exit(1)
	}
	hasMode := *timeFlag != 0 || *wordsFlag != 0
	if hasMode && *profileFlag == "" {
		fmt.Fprintln(os.Stderr, "error: --time/--words requires --profile")
		os.Exit(1)
	}
	if (*punctuationFlag || *zenFlag || *focusWeakFlag) && !hasMode {
		fmt.Fprintln(os.Stderr, "error: --punctuation/--zen/--focus-weak requires --time or --words")
		os.Exit(1)
	}
	if *exportFlag != "" && *profileFlag == "" {
		fmt.Fprintln(os.Stderr, "error: --export requires --profile")
		os.Exit(1)
	}

	dbPath, err := storage.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error resolving data directory:", err)
		os.Exit(1)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening database:", err)
		os.Exit(1)
	}
	defer db.Close()

	profiles, err := storage.ListProfiles(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading profiles:", err)
		os.Exit(1)
	}

	if *listProfilesFlag {
		if len(profiles) == 0 {
			fmt.Println("no profiles yet")
		}
		for _, p := range profiles {
			fmt.Printf("%s (level %d)\n", p.Name, p.Level)
		}
		return
	}

	if *exportFlag != "" {
		profile := findProfileByName(profiles, *profileFlag)
		if profile == nil {
			names := make([]string, len(profiles))
			for i, p := range profiles {
				names[i] = p.Name
			}
			fmt.Fprintf(os.Stderr, "error: no profile named %q (available: %s)\n", *profileFlag, strings.Join(names, ", "))
			os.Exit(1)
		}
		history, err := storage.ListTestHistory(db, profile.ID, 1_000_000)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error loading history:", err)
			os.Exit(1)
		}
		bests, err := storage.ListPersonalBests(db, profile.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error loading personal bests:", err)
			os.Exit(1)
		}
		data := exportData{ExportedAt: time.Now(), Profile: *profile, History: history, PersonalBests: bests}
		buf, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error encoding export:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*exportFlag, buf, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error writing export file:", err)
			os.Exit(1)
		}
		fmt.Printf("exported %d exercises and %d personal bests for %q to %s\n", len(history), len(bests), profile.Name, *exportFlag)
		return
	}

	opts := app.LaunchOptions{ProfileName: *profileFlag, Punctuation: *punctuationFlag, ZenMode: *zenFlag, FocusWeak: *focusWeakFlag}
	if *timeFlag != 0 {
		opts.Mode = "time"
		opts.Target = *timeFlag
	} else if *wordsFlag != 0 {
		opts.Mode = "words"
		opts.Target = *wordsFlag
	}

	m, err := app.NewWithOptions(db, profiles, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running program:", err)
		os.Exit(1)
	}
}
