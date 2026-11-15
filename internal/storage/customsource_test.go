package storage

import (
	"database/sql"
	"reflect"
	"slices"
	"testing"
	"time"
)

const frenchList = "wordlist:/home/me/français.txt"

// A custom-source run that would satisfy nearly every achievement and
// challenge query on its own counts toward none of them, but still shows
// in the history and keeps its own personal best.
func TestCustomRunsCountTowardNoAchievementOrChallenge(t *testing.T) {
	db, p := newTestDB(t)
	// Monday 00:30: inside the night-owl hours and the week below.
	at := time.Date(2026, 11, 9, 0, 30, 0, 0, time.Local)

	queries := map[string]func() (any, error){
		"perfect run":    func() (any, error) { return HasPerfectAccuracyRun(db, p.ID) },
		"perfect runs":   func() (any, error) { return PerfectRunCount(db, p.ID) },
		"perfect streak": func() (any, error) { return MaxPerfectAccuracyStreak(db, p.ID) },
		"sprint":         func() (any, error) { return HasCompletedTestConfig(db, p.ID, "time", 15) },
		"long haul":      func() (any, error) { return HasCompletedTestConfig(db, p.ID, "words", 200) },
		"500 words":      func() (any, error) { return HasCompletedWordsAtLeast(db, p.ID, 500) },
		"night owl":      func() (any, error) { return HasTestInHourRange(db, p.ID, 0, 4) },
		"all weekdays":   func() (any, error) { return HasPracticedAllWeekdays(db, p.ID) },
		"both modes":     func() (any, error) { return HasPersonalBestInBothModes(db, p.ID) },
		"best overall":   func() (any, error) { return BestOverallWPM(db, p.ID) },
		"comebacks":      func() (any, error) { return ComebackCount(db, p.ID) },
		"perfect return": func() (any, error) { return HasPerfectComebackAfterKO(db, p.ID) },
		"zen runs":       func() (any, error) { return ZenRunCount(db, p.ID) },
		"punct runs":     func() (any, error) { return PunctuationRunCount(db, p.ID) },
		"quote runs":     func() (any, error) { return QuoteRunCount(db, p.ID) },
		"words typed":    func() (any, error) { return TotalWordsTyped(db, p.ID) },
		"play time":      func() (any, error) { return TotalPlayTime(db, p.ID) },
		"week totals": func() (any, error) {
			return WeekTotalsBetween(db, p.ID, at.Add(-time.Hour), at.AddDate(0, 0, 7), 10)
		},
	}
	before := map[string]any{}
	for name, q := range queries {
		v, err := q()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		before[name] = v
	}

	// A knockout then perfect runs (a comeback), one per weekday, in both
	// modes, at the configurations the achievements look for.
	if err := RecordTestResult(db, p.ID, 20, 50, "time", 15, true, false, false, frenchList, 5, 10*time.Second, at); err != nil {
		t.Fatal(err)
	}
	for day := range 7 {
		when := at.AddDate(0, 0, day).Add(time.Minute)
		for _, run := range []struct {
			mode   string
			target int
		}{{"time", 15}, {"words", 200}, {"words", 500}} {
			if err := RecordTestResult(db, p.ID, 150, 100, run.mode, run.target, false, true, true, frenchList, 500, 5*time.Minute, when); err != nil {
				t.Fatal(err)
			}
			if _, err := RecordPersonalBest(db, p.ID, run.mode, run.target, frenchList, 150, 100, nil, nil, when); err != nil {
				t.Fatal(err)
			}
		}
	}

	for name, q := range queries {
		got, err := q()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(got, before[name]) {
			t.Errorf("%s changed after custom-source runs: %v -> %v", name, before[name], got)
		}
	}

	history, _ := ListTestHistory(db, p.ID, 100)
	if len(history) != 22 || history[0].Source != frenchList {
		t.Fatalf("expected the custom runs in the history with their source, got %d runs", len(history))
	}
	pb, err := GetPersonalBest(db, p.ID, "time", 15, frenchList)
	if err != nil || pb == nil || pb.WPM != 150 {
		t.Fatalf("expected the word list's own personal best, got %+v, %v", pb, err)
	}
	if pb, _ := GetPersonalBest(db, p.ID, "time", 15, "random"); pb != nil {
		t.Fatalf("expected the word list's best kept apart from the random words', got %+v", pb)
	}
}

func TestSavedCustomSources(t *testing.T) {
	db, p := newTestDB(t)
	other, err := CreateProfile(db, "Other")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	book := "text:/home/me/book.txt"
	for i, s := range []string{frenchList, book, frenchList} {
		if err := SaveCustomSource(db, p.ID, s, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := SaveCustomSource(db, other.ID, book, now); err != nil {
		t.Fatal(err)
	}

	got, _ := ListCustomSources(db, p.ID)
	if want := []string{frenchList, book}; !slices.Equal(got, want) {
		t.Fatalf("expected each source once, oldest first: got %q, want %q", got, want)
	}

	if err := ForgetCustomSource(db, p.ID, frenchList); err != nil {
		t.Fatal(err)
	}
	if got, _ := ListCustomSources(db, p.ID); !slices.Equal(got, []string{book}) {
		t.Fatalf("expected only the book left, got %q", got)
	}

	// Saved sources are preferences, like settings: a reset keeps them,
	// deleting the profile removes them.
	if _, err := ResetProfile(db, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := ListCustomSources(db, p.ID); !slices.Equal(got, []string{book}) {
		t.Fatalf("expected a reset to keep the saved sources, got %q", got)
	}
	if err := DeleteProfile(db, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := ListCustomSources(db, p.ID); len(got) != 0 {
		t.Fatalf("expected deleting the profile to remove its sources, got %q", got)
	}
	if got, _ := ListCustomSources(db, other.ID); !slices.Equal(got, []string{book}) {
		t.Fatalf("expected another profile's sources untouched, got %q", got)
	}
}

func TestSelectedCustomSourceIsSaved(t *testing.T) {
	db, p := newTestDB(t)
	s := defaultSettings()
	s.CustomSource = frenchList
	if err := SaveSettings(db, p.ID, s); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadSettings(db, p.ID)
	if got != s {
		t.Fatalf("LoadSettings = %+v, want %+v", got, s)
	}
	// Changing the heatmap layout alone keeps it.
	if err := SaveKeyboardLayout(db, p.ID, "dvorak"); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadSettings(db, p.ID); got.CustomSource != frenchList {
		t.Fatalf("expected the selected source kept, got %+v", got)
	}
}

// A custom run's history and best come back from a backup with their
// source, and the saved list keeps its order.
func TestBackupKeepsCustomSources(t *testing.T) {
	src := openFileDB(t)
	p, err := CreateProfile(src, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 1, 12, 0, 0, 0, backupZone)
	if err := RecordTestResult(src, p.ID, 70, 100, "time", 15, false, false, false, frenchList, 20, 15*time.Second, when); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordPersonalBest(src, p.ID, "time", 15, frenchList, 70, 100, nil, nil, when); err != nil {
		t.Fatal(err)
	}
	// Saved out of alphabetical order, to show the order survives.
	for i, s := range []string{"wordlist:/z.txt", "text:/a.txt"} {
		if err := SaveCustomSource(src, p.ID, s, when.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	b, err := ExportProfile(src, p.ID, when)
	if err != nil {
		t.Fatal(err)
	}
	dst := openFileDB(t)
	imported, err := ImportProfile(dst, throughJSON(t, b), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := ListCustomSources(dst, imported.ID); !slices.Equal(got, []string{"wordlist:/z.txt", "text:/a.txt"}) {
		t.Fatalf("expected the saved sources in their order, got %q", got)
	}
	history, _ := ListTestHistory(dst, imported.ID, 10)
	if len(history) != 1 || history[0].Source != frenchList {
		t.Fatalf("expected the custom run with its source, got %+v", history)
	}
	if pb, _ := GetPersonalBest(dst, imported.ID, "time", 15, frenchList); pb == nil {
		t.Fatal("expected the custom personal best imported")
	}
}

// A backup without custom sources stays at version 2, which older builds
// read; one with any needs version 3, which they refuse by version.
func TestBackupVersionDependsOnCustomSources(t *testing.T) {
	db, p := newTestDB(t)
	if err := RecordTestResult(db, p.ID, 50, 99, "time", 30, false, false, false, "quotes", 20, 30*time.Second, time.Now()); err != nil {
		t.Fatal(err)
	}
	if b, _ := ExportProfile(db, p.ID, time.Now()); b.Version != 2 {
		t.Fatalf("expected version 2 without custom sources, got %d", b.Version)
	}

	for name, add := range map[string]func(*sql.DB, int64) error{
		"saved source": func(db *sql.DB, id int64) error { return SaveCustomSource(db, id, frenchList, time.Now()) },
		"selected source": func(db *sql.DB, id int64) error {
			s := defaultSettings()
			s.CustomSource = frenchList
			return SaveSettings(db, id, s)
		},
		"custom run": func(db *sql.DB, id int64) error {
			return RecordTestResult(db, id, 50, 99, "time", 30, false, false, false, frenchList, 20, 30*time.Second, time.Now())
		},
	} {
		db, p := newTestDB(t)
		if err := add(db, p.ID); err != nil {
			t.Fatal(err)
		}
		b, err := ExportProfile(db, p.ID, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if b.Version != 3 {
			t.Errorf("%s: expected version 3, got %d", name, b.Version)
		}
	}
}
