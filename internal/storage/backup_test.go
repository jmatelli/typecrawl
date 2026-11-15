package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openFileDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "typecrawl.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// backupZone uses an offset no real time zone has, so it can't match the
// zone of whatever machine runs the tests. That guarantees two things: a
// round trip that silently shifted timestamps shows up as a changed hour
// of day, and the times come back from JSON with an unnamed zone, which
// is the case a naive import writes in a form the driver can't read back.
var backupZone = time.FixedZone("TST", 5*3600+17*60)

// seedFullProfile creates a profile with data in every table a profile
// owns, going through the same storage calls the app uses.
func seedFullProfile(t *testing.T, db *sql.DB) *Profile {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	p, err := CreateProfile(db, "Alice")
	must(err)
	p.Level, p.XP, p.BestWordStreak, p.KOCount, p.SuccessCount = 12, 3456, 41, 1, 3
	must(Save(db, p))
	must(SetEquippedTitle(db, p.ID, "wpm_60"))
	p.EquippedTitle = "wpm_60"

	must(SaveSettings(db, p.ID, Settings{Mode: "words", Duration: 60, WordCount: 50, Punctuation: true, Quotes: true, HideBattleStrip: true}))

	day1 := time.Date(2026, 10, 1, 23, 15, 0, 0, backupZone)
	day2 := time.Date(2026, 10, 2, 5, 40, 0, 0, backupZone)
	must(RecordTestResult(db, p.ID, 55.5, 100, "time", 30, false, false, false, "random", 28, 30*time.Second, day1))
	must(RecordTestResult(db, p.ID, 12, 71.2, "time", 30, true, false, false, "random", 6, 9*time.Second, day1.Add(time.Minute)))
	must(RecordTestResult(db, p.ID, 61.25, 100, "words", 50, false, true, false, "quotes", 50, 49*time.Second, day2))
	must(RecordTestResult(db, p.ID, 48, 96.5, "words", 100, false, true, true, "random", 100, 125*time.Second, day2.Add(time.Hour)))

	_, err = RecordPersonalBest(db, p.ID, "time", 30, "random", 55.5, 100, []int{900, 2100, 3050}, []float64{66.6, 57.1, 55.5}, day1)
	must(err)
	_, err = RecordPersonalBest(db, p.ID, "words", 50, "quotes", 61.25, 100, nil, nil, day2)
	must(err)

	must(RecordActivity(db, p.ID, day1))
	must(RecordActivity(db, p.ID, day1))
	must(RecordActivity(db, p.ID, day2))
	must(RecordActivity(db, p.ID, day2))

	must(RecordCharMistakes(db, p.ID, map[rune]int{'e': 7, ';': 2, 'é': 1}))
	must(ClaimDailyChallenge(db, p.ID, "2026-10-02", day2))
	return p
}

// throughJSON encodes b and decodes it again, the way a backup actually
// travels between two databases.
func throughJSON(t *testing.T, b *Backup) *Backup {
	t.Helper()
	buf, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var out Backup
	if err := json.Unmarshal(buf, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

// comparable renders a backup as JSON with the fields that legitimately
// differ between two databases (row id, export time) blanked out.
func comparable(t *testing.T, b *Backup) string {
	t.Helper()
	c := *b
	c.ExportedAt = time.Time{}
	c.Profile.ID = 0
	buf, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(buf)
}

func rowCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"profiles", "settings", "test_history", "personal_bests", "daily_activity", "char_mistakes", "daily_challenge_claims"} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[table] = n
	}
	return out
}

func TestExportImportRoundTripPreservesEverything(t *testing.T) {
	src := openFileDB(t)
	original := seedFullProfile(t, src)
	exported, err := ExportProfile(src, original.ID, time.Now())
	if err != nil {
		t.Fatalf("ExportProfile: %v", err)
	}
	if exported.Version != BackupVersion || exported.IsPartial() {
		t.Fatalf("expected a complete version %d backup, got version %d", BackupVersion, exported.Version)
	}

	dst := openFileDB(t)
	// An unrelated profile already in the destination must be untouched
	// and must not leak into the imported one.
	other, err := CreateProfile(dst, "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordActivity(dst, other.ID, time.Now()); err != nil {
		t.Fatal(err)
	}

	imported, err := ImportProfile(dst, throughJSON(t, exported), "")
	if err != nil {
		t.Fatalf("ImportProfile: %v", err)
	}
	if imported.ID == other.ID {
		t.Fatal("imported profile reused an existing profile's id")
	}

	reexported, err := ExportProfile(dst, imported.ID, time.Now())
	if err != nil {
		t.Fatalf("ExportProfile after import: %v", err)
	}
	if want, got := comparable(t, exported), comparable(t, reexported); want != got {
		t.Fatalf("backup changed across an import:\n--- original ---\n%s\n--- after import ---\n%s", want, got)
	}

	// The comparison above proves the stored rows match. These check the
	// things the app derives from them, on both sides.
	asOf := time.Date(2026, 10, 2, 12, 0, 0, 0, backupZone)
	for name, check := range map[string]func(*sql.DB, int64) (any, error){
		"streak":         func(db *sql.DB, id int64) (any, error) { return CurrentStreak(db, id, asOf) },
		"settings":       func(db *sql.DB, id int64) (any, error) { return LoadSettings(db, id) },
		"weak chars":     func(db *sql.DB, id int64) (any, error) { return WeakCharWeights(db, id) },
		"quote runs":     func(db *sql.DB, id int64) (any, error) { return QuoteRunCount(db, id) },
		"punct runs":     func(db *sql.DB, id int64) (any, error) { return PunctuationRunCount(db, id) },
		"zen runs":       func(db *sql.DB, id int64) (any, error) { return ZenRunCount(db, id) },
		"perfect streak": func(db *sql.DB, id int64) (any, error) { return MaxPerfectAccuracyStreak(db, id) },
		"comebacks":      func(db *sql.DB, id int64) (any, error) { return ComebackCount(db, id) },
		"play time":      func(db *sql.DB, id int64) (any, error) { return TotalPlayTime(db, id) },
		"words typed":    func(db *sql.DB, id int64) (any, error) { return TotalWordsTyped(db, id) },
		"night owl hour": func(db *sql.DB, id int64) (any, error) { return HasTestInHourRange(db, id, 23, 24) },
		"early hour":     func(db *sql.DB, id int64) (any, error) { return HasTestInHourRange(db, id, 5, 6) },
		"midday hour":    func(db *sql.DB, id int64) (any, error) { return HasTestInHourRange(db, id, 10, 20) },
		"claimed":        func(db *sql.DB, id int64) (any, error) { return HasClaimedDailyChallenge(db, id, "2026-10-02") },
		"wpm trend":      func(db *sql.DB, id int64) (any, error) { return RecentWPMHistory(db, id, 10) },
		"history":        func(db *sql.DB, id int64) (any, error) { return ListTestHistory(db, id, 10) },
		"accuracy trend": func(db *sql.DB, id int64) (any, error) { return RecentAccuracyHistory(db, id, 10) },
		"all weekdays":   func(db *sql.DB, id int64) (any, error) { return HasPracticedAllWeekdays(db, id) },
		"busy day":       func(db *sql.DB, id int64) (any, error) { return HasDayWithAtLeast(db, id, 2) },
		"perfect return": func(db *sql.DB, id int64) (any, error) { return HasPerfectComebackAfterKO(db, id) },
		"best overall":   func(db *sql.DB, id int64) (any, error) { return BestOverallWPM(db, id) },
		"both modes":     func(db *sql.DB, id int64) (any, error) { return HasPersonalBestInBothModes(db, id) },
		"perfect runs":   func(db *sql.DB, id int64) (any, error) { return PerfectRunCount(db, id) },
		"summary": func(db *sql.DB, id int64) (any, error) {
			return Summary(db, id, time.Date(2026, 10, 2, 0, 0, 0, 0, backupZone), time.Date(2026, 10, 3, 0, 0, 0, 0, backupZone))
		},
	} {
		want, err := check(src, original.ID)
		if err != nil {
			t.Fatalf("%s (source): %v", name, err)
		}
		got, err := check(dst, imported.ID)
		if err != nil {
			t.Fatalf("%s (imported): %v", name, err)
		}
		// Compared as JSON so two equal instants with the same offset
		// match even though their *time.Location pointers differ.
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("%s: source has %s, imported has %s", name, wantJSON, gotJSON)
		}
	}

	// Guard against the checks above passing only because both sides are
	// empty.
	if streak, _ := CurrentStreak(dst, imported.ID, asOf); streak != 2 {
		t.Errorf("expected a 2-day streak on the imported profile, got %d", streak)
	}
	if n, _ := QuoteRunCount(dst, imported.ID); n != 1 {
		t.Errorf("expected 1 quotes run on the imported profile, got %d", n)
	}
	if late, _ := HasTestInHourRange(dst, imported.ID, 23, 24); !late {
		t.Error("expected the 23:15 run to keep its hour of day")
	}

	if n, err := ActivityRange(dst, other.ID, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1)); err != nil || len(n) != 1 {
		t.Errorf("the pre-existing profile's activity changed: %v (err %v)", n, err)
	}
}

func TestImportProfileRefusesExistingNameAndChangesNothing(t *testing.T) {
	db := openFileDB(t)
	p := seedFullProfile(t, db)
	b, err := ExportProfile(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	before := rowCounts(t, db)

	for _, name := range []string{"", "Alice", "aLiCe", "  alice "} {
		if _, err := ImportProfile(db, throughJSON(t, b), name); !errors.Is(err, ErrProfileExists) {
			t.Errorf("name %q: expected ErrProfileExists, got %v", name, err)
		}
	}
	if after := rowCounts(t, db); !reflect.DeepEqual(before, after) {
		t.Fatalf("a refused import changed the database: before %v, after %v", before, after)
	}
}

func TestImportProfileUnderAnotherNameKeepsBothProfiles(t *testing.T) {
	db := openFileDB(t)
	p := seedFullProfile(t, db)
	b, err := ExportProfile(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	copied, err := ImportProfile(db, throughJSON(t, b), "Alice (restored)")
	if err != nil {
		t.Fatalf("ImportProfile: %v", err)
	}
	if copied.Name != "Alice (restored)" || copied.ID == p.ID || copied.Level != p.Level {
		t.Fatalf("unexpected imported profile: %+v", copied)
	}

	again, err := ExportProfile(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if comparable(t, b) != comparable(t, again) {
		t.Fatal("importing a copy modified the original profile")
	}
}

func TestImportProfileRejectsInvalidBackupsAndChangesNothing(t *testing.T) {
	db := openFileDB(t)
	p := seedFullProfile(t, db)
	valid, err := ExportProfile(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	before := rowCounts(t, db)

	cases := map[string]func(*Backup){
		"future version":         func(b *Backup) { b.Version = BackupVersion + 1 },
		"negative version":       func(b *Backup) { b.Version = -1 },
		"empty name":             func(b *Backup) { b.Profile.Name = "   " },
		"control char in name":   func(b *Backup) { b.Profile.Name = "evil\x1b[2J" },
		"overlong name":          func(b *Backup) { b.Profile.Name = strings.Repeat("é", maxBackupNameLength+1) },
		"level zero":             func(b *Backup) { b.Profile.Level = 0 },
		"negative xp":            func(b *Backup) { b.Profile.XP = -1 },
		"control char in title":  func(b *Backup) { b.Profile.EquippedTitle = "\x07" },
		"unknown settings mode":  func(b *Backup) { b.Settings.Mode = "sprint" },
		"unknown history mode":   func(b *Backup) { b.History[0].Mode = "" },
		"unknown history source": func(b *Backup) { b.History[1].Source = "books" },
		"accuracy over 100":      func(b *Backup) { b.History[0].Accuracy = 140 },
		"history without a date": func(b *Backup) { b.History[2].CreatedAt = time.Time{} },
		"negative best wpm":      func(b *Backup) { b.PersonalBests[0].WPM = -3 },
		"duplicate best":         func(b *Backup) { b.PersonalBests = append(b.PersonalBests, b.PersonalBests[0]) },
		"malformed activity day": func(b *Backup) { b.DailyActivity["2026-13-40"] = 1 },
		"negative activity":      func(b *Backup) { b.DailyActivity["2026-10-01"] = -1 },
		"multi-char mistake key": func(b *Backup) { b.CharMistakes["ab"] = 1 },
		"empty mistake key":      func(b *Backup) { b.CharMistakes[""] = 1 },
		"duplicate claim":        func(b *Backup) { b.ChallengeClaims = append(b.ChallengeClaims, b.ChallengeClaims[0]) },
		"malformed claim day":    func(b *Backup) { b.ChallengeClaims[0].Day = "yesterday" },
	}
	for name, corrupt := range cases {
		b := throughJSON(t, valid)
		corrupt(b)
		// A free name, so only the corruption can be what fails.
		target := "Fresh"
		if strings.Contains(name, "name") {
			target = ""
		}
		want := ErrInvalidBackup
		if strings.Contains(name, "name") {
			want = ErrInvalidName
		}
		if _, err := ImportProfile(db, b, target); !errors.Is(err, want) {
			t.Errorf("%s: expected %q, got %v", name, want, err)
		}
	}
	if after := rowCounts(t, db); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected imports changed the database: before %v, after %v", before, after)
	}
}

// A file written by --export before backups carried a version: no
// version, no source on history entries, and none of the newer sections.
const legacyExport = `{
  "exported_at": "2026-10-05T10:00:00+02:00",
  "profile": {"ID": 7, "Name": "Old Timer", "Level": 4, "XP": 310, "BestWordStreak": 18, "KOCount": 2, "SuccessCount": 9, "CreatedAt": "2026-10-01T10:00:00Z", "EquippedTitle": ""},
  "history": [
    {"WPM": 52.5, "Accuracy": 97, "Mode": "time", "Target": 30, "KO": false, "Punctuation": false, "ZenMode": false, "WordsTyped": 26, "Duration": 30000000000, "CreatedAt": "2026-10-04T21:00:00+02:00"},
    {"WPM": 40, "Accuracy": 91.5, "Mode": "words", "Target": 25, "KO": false, "Punctuation": true, "ZenMode": false, "WordsTyped": 25, "Duration": 37000000000, "CreatedAt": "2026-10-03T08:30:00+02:00"}
  ],
  "personal_bests": [
    {"Mode": "time", "Target": 30, "Source": "random", "WPM": 52.5, "Accuracy": 97, "AchievedAt": "2026-10-04T21:00:00+02:00", "GhostPace": [1000, 2200], "GhostWPMSeries": [60, 52.5]}
  ]
}`

func TestImportProfileAcceptsLegacyExport(t *testing.T) {
	var b Backup
	if err := json.Unmarshal([]byte(legacyExport), &b); err != nil {
		t.Fatal(err)
	}
	if !b.IsPartial() {
		t.Fatal("expected a versionless export to be reported as partial")
	}

	db := openFileDB(t)
	p, err := ImportProfile(db, &b, "")
	if err != nil {
		t.Fatalf("ImportProfile: %v", err)
	}
	if p.Name != "Old Timer" || p.Level != 4 || p.XP != 310 || p.BestWordStreak != 18 || p.KOCount != 2 || p.SuccessCount != 9 {
		t.Fatalf("unexpected profile: %+v", p)
	}

	history, err := ListTestHistory(db, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].WPM != 52.5 || history[1].WPM != 40 {
		t.Fatalf("expected both runs, newest first, got %+v", history)
	}
	for _, e := range history {
		if e.Source != "random" {
			t.Errorf("expected a legacy run to default to the random source, got %q", e.Source)
		}
	}

	pb, err := GetPersonalBest(db, p.ID, "time", 30, "random")
	if err != nil || pb == nil {
		t.Fatalf("expected the personal best to be restored, got %v (err %v)", pb, err)
	}
	if !reflect.DeepEqual(pb.GhostPace, []int{1000, 2200}) {
		t.Errorf("ghost pace not restored: %v", pb.GhostPace)
	}

	// The run happened at 21:00 in a +02:00 zone, and must still read so.
	if evening, err := HasTestInHourRange(db, p.ID, 21, 22); err != nil || !evening {
		t.Errorf("expected the 21:00 run to keep its hour of day (err %v)", err)
	}

	// The old format had no daily activity, but it's rebuilt from the
	// history: one run on the 3rd and one on the 4th is a 2-day streak.
	streak, err := CurrentStreak(db, p.ID, time.Date(2026, 10, 4, 22, 0, 0, 0, time.UTC))
	if err != nil || streak != 2 {
		t.Errorf("expected a 2-day streak rebuilt from history, got %d (err %v)", streak, err)
	}

	// Sections the old format never had come back as a fresh profile's.
	s, err := LoadSettings(db, p.ID)
	if err != nil || s != defaultSettings() {
		t.Errorf("expected default settings, got %+v (err %v)", s, err)
	}
}

func TestExportProfileOfFreshProfileRoundTrips(t *testing.T) {
	src := openFileDB(t)
	p, err := CreateProfile(src, "Newbie")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExportProfile(src, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	buf, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	// Empty lists are written as [] rather than null, which is easier on
	// external tooling reading the file.
	if !strings.Contains(string(buf), `"history":[]`) || !strings.Contains(string(buf), `"personal_bests":[]`) {
		t.Errorf("expected empty history and personal_bests arrays, got %s", buf)
	}

	dst := openFileDB(t)
	imported, err := ImportProfile(dst, throughJSON(t, b), "")
	if err != nil {
		t.Fatalf("ImportProfile: %v", err)
	}
	if imported.Name != "Newbie" || imported.Level != 1 || imported.XP != 0 {
		t.Fatalf("unexpected profile: %+v", imported)
	}
}

// A name that's invalid in the file is fixable by supplying another one,
// which matters for profiles created before names were restricted.
func TestImportProfileInvalidStoredNameCanBeOverridden(t *testing.T) {
	db := openFileDB(t)
	p := seedFullProfile(t, db)
	b, err := ExportProfile(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.Profile.Name = strings.Repeat("x", maxBackupNameLength+10)

	if _, err := ImportProfile(db, throughJSON(t, b), ""); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
	renamed, err := ImportProfile(db, throughJSON(t, b), "Shorter")
	if err != nil || renamed.Name != "Shorter" {
		t.Fatalf("expected the import to succeed under a valid name, got %+v (err %v)", renamed, err)
	}
}

// Validation failures never reach the database, so they can't show that a
// write failing part-way through is undone. This forces one: the backup is
// valid, but the last table written to refuses the insert.
func TestImportProfileRollsBackWhenAWriteFailsPartWay(t *testing.T) {
	src := openFileDB(t)
	p := seedFullProfile(t, src)
	b, err := ExportProfile(src, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	dst := openFileDB(t)
	if _, err := CreateProfile(dst, "Bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`CREATE TRIGGER refuse_claims BEFORE INSERT ON daily_challenge_claims BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatal(err)
	}
	before := rowCounts(t, dst)

	if _, err := ImportProfile(dst, throughJSON(t, b), ""); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("expected the import to fail on the refused insert, got %v", err)
	}
	if after := rowCounts(t, dst); !reflect.DeepEqual(before, after) {
		t.Fatalf("a failed import left rows behind: before %v, after %v", before, after)
	}
	profiles, err := ListProfiles(dst)
	if err != nil || len(profiles) != 1 || profiles[0].Name != "Bob" {
		t.Fatalf("expected only the pre-existing profile, got %+v (err %v)", profiles, err)
	}
}

// Imported rows store their timestamps in a different text form from the
// rows the app writes afterwards, and several queries sort and filter on
// that text. A run played after an import must still land in the right
// place relative to the imported ones.
func TestImportedProfileKeepsWorkingWithNewRuns(t *testing.T) {
	src := openFileDB(t)
	p := seedFullProfile(t, src)
	b, err := ExportProfile(src, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dst := openFileDB(t)
	imported, err := ImportProfile(dst, throughJSON(t, b), "")
	if err != nil {
		t.Fatal(err)
	}

	// The newest imported run is 2026-10-02 06:40:00 in backupZone. Play
	// one a moment later in that same second and one later the same day,
	// the way the app records them (a time.Time, stored in the driver's
	// own form).
	sameSecond := time.Date(2026, 10, 2, 6, 40, 0, 250_000_000, backupZone)
	later := time.Date(2026, 10, 2, 18, 0, 0, 500, backupZone)
	if err := RecordTestResult(dst, imported.ID, 70, 99, "time", 30, false, false, false, "random", 35, 30*time.Second, sameSecond); err != nil {
		t.Fatal(err)
	}
	if err := RecordTestResult(dst, imported.ID, 80, 98, "time", 30, false, false, false, "random", 40, 30*time.Second, later); err != nil {
		t.Fatal(err)
	}

	history, err := ListTestHistory(dst, imported.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var wpms []float64
	for _, e := range history {
		wpms = append(wpms, e.WPM)
	}
	// Newest first: the two new runs, then the imported ones in their
	// original order.
	if want := []float64{80, 70, 48, 61.25, 12, 55.5}; !reflect.DeepEqual(wpms, want) {
		t.Errorf("history order: want %v, got %v", want, wpms)
	}

	trend, err := RecentWPMHistory(dst, imported.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Oldest first, knockouts left out.
	if want := []float64{55.5, 61.25, 48, 70, 80}; !reflect.DeepEqual(trend, want) {
		t.Errorf("WPM trend: want %v, got %v", want, trend)
	}

	// A range covering exactly 2026-10-02 holds two imported runs and
	// both new ones.
	day := time.Date(2026, 10, 2, 0, 0, 0, 0, backupZone)
	summary, err := Summary(dst, imported.ID, day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Exercises != 4 {
		t.Errorf("expected 4 runs on 2026-10-02, got %+v", summary)
	}
	// A range that starts on the exact second of an imported run and ends
	// on the exact second of another: the start is included, the end not.
	first := time.Date(2026, 10, 1, 23, 15, 0, 0, backupZone)
	end := time.Date(2026, 10, 2, 5, 40, 0, 0, backupZone)
	exact, err := Summary(dst, imported.ID, first, end)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Exercises != 2 {
		t.Errorf("expected the 2 runs from 23:15:00 up to but not including 05:40:00, got %+v", exact)
	}
}

// Backups written before the battle strip existed have no HideBattleStrip
// field. Importing one must leave the strip on, its default.
func TestImportProfileFromBeforeTheBattleStripShowsIt(t *testing.T) {
	src := openFileDB(t)
	p, err := CreateProfile(src, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(src, p.ID, Settings{Mode: "words", Duration: 60, WordCount: 50, ZenMode: true}); err != nil {
		t.Fatal(err)
	}
	exported, err := ExportProfile(src, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	buf, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(buf), `,"HideBattleStrip":false`, "", 1)
	if old == string(buf) {
		t.Fatalf("expected the export to carry HideBattleStrip, got %s", buf)
	}

	var b Backup
	if err := json.Unmarshal([]byte(old), &b); err != nil {
		t.Fatal(err)
	}
	dst := openFileDB(t)
	imported, err := ImportProfile(dst, &b, "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(dst, imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.HideBattleStrip || !s.ZenMode {
		t.Errorf("expected the other settings kept and the battle strip shown, got %+v", s)
	}
}
