package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/app"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func findProfileByName(profiles []*storage.Profile, name string) *storage.Profile {
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// version is overridden at release build time via
// -ldflags "-X main.version={{.Version}}" (see .goreleaser.yaml). A plain
// `go build`/`go run`/`go install` without that keeps the "dev" default.
var version = "dev"

func main() {
	profileFlag := flag.String("profile", "", "quick-launch: profile name to select (skips profile selection)")
	timeFlag := flag.Int("time", 0, "quick-launch: start a time-based test immediately, in seconds (requires --profile)")
	wordsFlag := flag.Int("words", 0, "quick-launch: start a word-count test immediately (requires --profile)")
	punctuationFlag := flag.Bool("punctuation", false, "quick-launch: enable punctuation (requires --time or --words)")
	zenFlag := flag.Bool("zen", false, "quick-launch: enable zen mode (requires --time or --words)")
	focusWeakFlag := flag.Bool("focus-weak", false, "quick-launch: bias word choice toward your weakest keys (requires --time or --words)")
	quotesFlag := flag.Bool("quotes", false, "quick-launch: use quotes instead of random words (implies punctuation; requires --time or --words)")
	listProfilesFlag := flag.Bool("list-profiles", false, "list saved profiles and exit")
	exportFlag := flag.String("export", "", "export a profile to this JSON file path and exit (requires --profile)")
	importFlag := flag.String("import", "", "import a profile from a JSON file written by --export and exit (add --profile to import it under a different name)")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")
	flag.Parse()

	if showVersion {
		fmt.Println("typecrawl " + version)
		return
	}

	if *timeFlag != 0 && *wordsFlag != 0 {
		fmt.Fprintln(os.Stderr, "error: specify only one of --time or --words")
		os.Exit(1)
	}
	hasMode := *timeFlag != 0 || *wordsFlag != 0
	if hasMode && *profileFlag == "" {
		fmt.Fprintln(os.Stderr, "error: --time/--words requires --profile")
		os.Exit(1)
	}
	if (*punctuationFlag || *zenFlag || *focusWeakFlag || *quotesFlag) && !hasMode {
		fmt.Fprintln(os.Stderr, "error: --punctuation/--zen/--focus-weak/--quotes requires --time or --words")
		os.Exit(1)
	}
	if *quotesFlag && *focusWeakFlag {
		fmt.Fprintln(os.Stderr, "error: --focus-weak cannot be combined with --quotes (weak-key biasing has no meaning over fixed quote text)")
		os.Exit(1)
	}
	if *exportFlag != "" && *profileFlag == "" {
		fmt.Fprintln(os.Stderr, "error: --export requires --profile")
		os.Exit(1)
	}
	if *importFlag != "" && (*exportFlag != "" || hasMode || *listProfilesFlag) {
		fmt.Fprintln(os.Stderr, "error: --import cannot be combined with --export, --list-profiles, --time or --words")
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
		data, err := storage.ExportProfile(db, profile.ID, time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading profile:", err)
			os.Exit(1)
		}
		buf, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error encoding export:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*exportFlag, buf, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error writing export file:", err)
			os.Exit(1)
		}
		fmt.Printf("exported %d exercises and %d personal bests for %q to %s\n", len(data.History), len(data.PersonalBests), profile.Name, *exportFlag)
		return
	}

	if *importFlag != "" {
		buf, err := os.ReadFile(*importFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading import file:", err)
			os.Exit(1)
		}
		var data storage.Backup
		if err := json.Unmarshal(buf, &data); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s is not a valid export file: %v\n", *importFlag, err)
			os.Exit(1)
		}
		profile, err := storage.ImportProfile(db, &data, *profileFlag)
		if errors.Is(err, storage.ErrProfileExists) || errors.Is(err, storage.ErrInvalidName) {
			fmt.Fprintf(os.Stderr, "error: %v; nothing was imported (use --profile NAME to import it under a different name)\n", err)
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v; nothing was imported\n", err)
			os.Exit(1)
		}
		fmt.Printf("imported %q (level %d) with %d exercises and %d personal bests\n", profile.Name, profile.Level, len(data.History), len(data.PersonalBests))
		if data.IsPartial() {
			fmt.Println("note: this file was written by an older version, which did not export settings, weak-key counts, or daily challenge claims; those start empty. It also did not record which runs used quotes mode, so every run is imported as a random-words run.")
		}
		return
	}

	opts := app.LaunchOptions{ProfileName: *profileFlag, Punctuation: *punctuationFlag, ZenMode: *zenFlag, FocusWeak: *focusWeakFlag, Quotes: *quotesFlag}
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
