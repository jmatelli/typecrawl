package storage

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// numericZones behave like the real zones whose abbreviation is a numeric
// offset (Asia/Kathmandu, Asia/Tehran, America/St_Johns-style half hours):
// time.Time.String() prints them as "+0545 +0545". They're built by hand
// so the tests don't depend on the machine having a time zone database.
var numericZones = map[string]*time.Location{
	"+0545":   time.FixedZone("+0545", 5*3600+45*60),
	"+0330":   time.FixedZone("+0330", 3*3600+30*60),
	"-0930":   time.FixedZone("-0930", -(9*3600 + 30*60)),
	"unnamed": time.FixedZone("", 5*3600+30*60),
	"+04":     time.FixedZone("+04", 4*3600),
	"CEST":    time.FixedZone("CEST", 2*3600),
	"UTC":     time.UTC,
}

// Everything the app stamps with a time must read back, at the same
// instant and the same hour of day, whatever zone it was recorded in.
func TestTimestampsReadBackInEveryZone(t *testing.T) {
	for name, loc := range numericZones {
		t.Run(name, func(t *testing.T) {
			db, p := newTestDB(t)
			when := time.Date(2026, 10, 1, 23, 15, 7, 123456789, loc)

			if err := RecordTestResult(db, p.ID, 50, 100, "time", 30, false, false, false, "random", 20, 30*time.Second, when); err != nil {
				t.Fatal(err)
			}
			if _, err := RecordPersonalBest(db, p.ID, "time", 30, "random", 50, 100, []int{1000}, []float64{50}, when); err != nil {
				t.Fatal(err)
			}
			if err := ClaimDailyChallenge(db, p.ID, Day(when), when); err != nil {
				t.Fatal(err)
			}

			history, err := ListTestHistory(db, p.ID, 5)
			if err != nil {
				t.Fatalf("ListTestHistory: %v", err)
			}
			if len(history) != 1 || !history[0].CreatedAt.Equal(when) || history[0].CreatedAt.Hour() != 23 {
				t.Errorf("history: want %v, got %+v", when, history)
			}

			pb, err := GetPersonalBest(db, p.ID, "time", 30, "random")
			if err != nil || pb == nil {
				t.Fatalf("GetPersonalBest: %v (err %v)", pb, err)
			}
			if !pb.AchievedAt.Equal(when) || pb.AchievedAt.Hour() != 23 {
				t.Errorf("personal best: want %v, got %v", when, pb.AchievedAt)
			}

			if late, err := HasTestInHourRange(db, p.ID, 23, 24); err != nil || !late {
				t.Errorf("expected a run in the 23:00 hour (err %v)", err)
			}
			if _, err := HasPracticedAllWeekdays(db, p.ID); err != nil {
				t.Errorf("HasPracticedAllWeekdays: %v", err)
			}
			if _, err := RecentWPMHistory(db, p.ID, 5); err != nil {
				t.Errorf("RecentWPMHistory: %v", err)
			}

			day := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
			summary, err := Summary(db, p.ID, day, day.AddDate(0, 0, 1))
			if err != nil || summary.Exercises != 1 {
				t.Errorf("expected the run in its own day's summary, got %+v (err %v)", summary, err)
			}

			backup, err := ExportProfile(db, p.ID, when)
			if err != nil {
				t.Fatalf("ExportProfile: %v", err)
			}
			if len(backup.ChallengeClaims) != 1 || !backup.ChallengeClaims[0].ClaimedAt.Equal(when) {
				t.Errorf("challenge claim: want %v, got %+v", when, backup.ChallengeClaims)
			}
		})
	}
}

// Day mirrors the challenge package's day key without importing it.
func Day(t time.Time) string { return t.Format("2006-01-02") }

func rawTimestamps(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for table, column := range map[string]string{"test_history": "created_at", "personal_bests": "achieved_at", "daily_challenge_claims": "claimed_at"} {
		rows, err := db.Query("SELECT CAST(" + column + " AS TEXT) FROM " + table + " ORDER BY rowid")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out[table] = append(out[table], s)
		}
		rows.Close()
	}
	return out
}

// A database written by an earlier version in one of the affected zones
// holds rows the driver can't read. Opening it must repair them in place,
// without touching rows that were fine.
func TestOpenRepairsUnreadableTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typecrawl.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := CreateProfile(db, "tester")
	if err != nil {
		t.Fatal(err)
	}

	// Exactly what earlier versions stored: time.Time.String(), with the
	// monotonic-clock reading time.Now() carries.
	const (
		broken        = "2026-10-01 23:15:07.123456789 +0545 +0545 m=+12.345678901"
		brokenNoFrac  = "2026-10-02 06:00:00 -0930 -0930"
		readable      = "2026-10-03 09:30:00.5 +0200 CEST m=+3.000000001"
		readableShort = "2026-10-04 09:30:00 +0400 +04"
		// Matches the repair pattern but isn't a time: must be left
		// exactly as it is, not guessed at. It sorts last, so it stays
		// out of the way of the history read below.
		garbage = "not a time +0545 +0545"
	)
	for _, created := range []string{broken, brokenNoFrac, readable, readableShort} {
		if _, err := db.Exec(`INSERT INTO test_history (profile_id, wpm, accuracy, mode, target, created_at) VALUES (?, 50, 100, 'time', 30, ?)`, p.ID, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO personal_bests (profile_id, mode, target, source, best_wpm, accuracy, achieved_at) VALUES (?, 'time', 30, 'random', 50, 100, ?)`, p.ID, broken); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO daily_challenge_claims (profile_id, day, claimed_at) VALUES (?, '2026-10-01', ?)`, p.ID, broken); err != nil {
		t.Fatal(err)
	}

	other, err := CreateProfile(db, "other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO test_history (profile_id, wpm, accuracy, mode, target, created_at) VALUES (?, 50, 100, 'time', 30, ?)`, other.ID, garbage); err != nil {
		t.Fatal(err)
	}

	// This is the bug: the rows are there but can't be read.
	if _, err := ListTestHistory(db, p.ID, 10); err == nil {
		t.Fatal("expected the unrepaired rows to be unreadable; the test no longer reproduces the bug")
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	const repaired = "2026-10-01 23:15:07.123456789+05:45"
	want := map[string][]string{
		"test_history":           {repaired, "2026-10-02 06:00:00-09:30", readable, readableShort, garbage},
		"personal_bests":         {repaired},
		"daily_challenge_claims": {repaired},
	}
	if got := rawTimestamps(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("stored timestamps after repair:\nwant %v\ngot  %v", want, got)
	}

	history, err := ListTestHistory(db, p.ID, 10)
	if err != nil {
		t.Fatalf("ListTestHistory after repair: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected all 4 runs, got %d", len(history))
	}
	oldest := history[3].CreatedAt
	wantInstant := time.Date(2026, 10, 1, 23, 15, 7, 123456789, numericZones["+0545"])
	if !oldest.Equal(wantInstant) || oldest.Hour() != 23 {
		t.Errorf("repaired run: want %v at hour 23, got %v", wantInstant, oldest)
	}
	if pb, err := GetPersonalBest(db, p.ID, "time", 30, "random"); err != nil || pb == nil || !pb.AchievedAt.Equal(wantInstant) {
		t.Errorf("repaired personal best: got %+v (err %v)", pb, err)
	}
	db.Close()

	// Opening again changes nothing further.
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := rawTimestamps(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("a second open changed the timestamps again:\nwant %v\ngot  %v", want, got)
	}
}

// Existing players have rows in the old text form and will get new ones
// in the current form. Queries that sort and filter on that text must
// keep treating a later run as later.
func TestNewRunsOrderCorrectlyAfterOldFormatRows(t *testing.T) {
	db, p := newTestDB(t)
	for i, created := range []string{
		"2026-10-01 10:00:00.25 +0200 CEST m=+1.000000001",
		"2026-10-02 10:00:00 +0200 CEST",
	} {
		if _, err := db.Exec(`INSERT INTO test_history (profile_id, wpm, accuracy, mode, target, created_at) VALUES (?, ?, 100, 'time', 30, ?)`, p.ID, 10*(i+1), created); err != nil {
			t.Fatal(err)
		}
	}
	zone := numericZones["CEST"]
	// Later within the same second as the newest old row, then later that
	// day.
	for i, when := range []time.Time{
		time.Date(2026, 10, 2, 10, 0, 0, 400_000_000, zone),
		time.Date(2026, 10, 2, 22, 0, 0, 0, zone),
	} {
		if err := RecordTestResult(db, p.ID, float64(30+10*i), 100, "time", 30, false, false, false, "random", 20, 30*time.Second, when); err != nil {
			t.Fatal(err)
		}
	}

	trend, err := RecentWPMHistory(db, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []float64{10, 20, 30, 40}; !reflect.DeepEqual(trend, want) {
		t.Errorf("WPM trend, oldest first: want %v, got %v", want, trend)
	}

	day := time.Date(2026, 10, 2, 0, 0, 0, 0, zone)
	summary, err := Summary(db, p.ID, day, day.AddDate(0, 0, 1))
	if err != nil || summary.Exercises != 3 {
		t.Errorf("expected 3 runs on 2026-10-02, got %+v (err %v)", summary, err)
	}
	// From the exact second of an old-format row: it is included.
	fromOld, err := Summary(db, p.ID, time.Date(2026, 10, 2, 10, 0, 0, 0, zone), day.AddDate(0, 0, 1))
	if err != nil || fromOld.Exercises != 3 {
		t.Errorf("expected the 10:00:00 run to be included in a range starting at 10:00:00, got %+v (err %v)", fromOld, err)
	}
	// Up to the exact second of a new-format row: it is excluded.
	toNew, err := Summary(db, p.ID, day, time.Date(2026, 10, 2, 22, 0, 0, 0, zone))
	if err != nil || toNew.Exercises != 2 {
		t.Errorf("expected the 22:00:00 run to be excluded from a range ending at 22:00:00, got %+v (err %v)", toNew, err)
	}
}

// Summary's bounds are compared as text against rows in both stored
// forms. A row exactly on the start must be counted and a row exactly on
// the end must not, whichever form the row is in, including at a
// negative offset (where the new form's "-" has to sort against the old
// form's space and a "." fraction).
func TestSummaryBoundsAgainstBothStoredForms(t *testing.T) {
	for name, tc := range map[string]struct {
		loc     *time.Location
		oldForm string
	}{
		"positive offset": {numericZones["CEST"], "+0200 CEST"},
		"negative offset": {time.FixedZone("EST", -5*3600), "-0500 EST"},
	} {
		t.Run(name, func(t *testing.T) {
			db, p := newTestDB(t)
			from := time.Date(2026, 10, 2, 10, 0, 0, 0, tc.loc)
			to := time.Date(2026, 10, 2, 11, 0, 0, 0, tc.loc)

			insertOld := func(wpm int, created string) {
				t.Helper()
				if _, err := db.Exec(`INSERT INTO test_history (profile_id, wpm, accuracy, mode, target, created_at) VALUES (?, ?, 100, 'time', 30, ?)`, p.ID, wpm, created+" "+tc.oldForm); err != nil {
					t.Fatal(err)
				}
			}
			insertNew := func(wpm float64, when time.Time) {
				t.Helper()
				if err := RecordTestResult(db, p.ID, wpm, 100, "time", 30, false, false, false, "random", 20, 30*time.Second, when); err != nil {
					t.Fatal(err)
				}
			}

			// In order of the instant they happened at. Inside the
			// range: 20 through 50.
			insertOld(10, "2026-10-02 09:59:59.9")
			insertOld(20, "2026-10-02 10:00:00") // exactly on from
			insertNew(30, from)                  // exactly on from
			insertOld(40, "2026-10-02 10:30:00.75")
			insertNew(50, time.Date(2026, 10, 2, 10, 30, 0, 800_000_000, tc.loc))
			insertOld(60, "2026-10-02 11:00:00") // exactly on to
			insertNew(70, to)                    // exactly on to

			summary, err := Summary(db, p.ID, from, to)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Exercises != 4 || summary.AvgWPM != 35 {
				t.Errorf("expected the 4 runs from 10:00:00 up to but not including 11:00:00 (average 35), got %+v", summary)
			}

			trend, err := RecentWPMHistory(db, p.ID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if want := []float64{10, 20, 30, 40, 50, 60, 70}; !reflect.DeepEqual(trend, want) {
				t.Errorf("WPM trend, oldest first: want %v, got %v", want, trend)
			}
		})
	}
}
