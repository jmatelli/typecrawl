package storage

import (
	"testing"
	"time"
)

// The deepest floor is a record: a copy loaded before a deeper crawl
// elsewhere can't lower it when it saves, and a reset keeps it.
func TestDeepestFloorOnlyGoesUp(t *testing.T) {
	db, p := newTestDB(t)
	stale := *p
	p.DeepestFloor = 9
	if err := Save(db, p); err != nil {
		t.Fatal(err)
	}
	stale.DeepestFloor = 3
	if err := Save(db, &stale); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadProfile(db, p.ID); got.DeepestFloor != 9 {
		t.Fatalf("expected the deepest floor kept at 9, got %d", got.DeepestFloor)
	}
	if got, _ := ResetProfile(db, p.ID); got.DeepestFloor != 9 {
		t.Fatalf("expected a reset to keep the record, got %d", got.DeepestFloor)
	}
}

// Crawl floors are runs on the built-in words, so the achievement queries
// count them, and a backup with one needs version 3. The deepest floor
// round-trips; a negative one is refused.
func TestCrawlRunsInStorageAndBackups(t *testing.T) {
	db, p := newTestDB(t)
	if err := RecordTestResult(db, p.ID, 50, 100, "words", 15, false, false, false, "crawl", 15, 20*time.Second, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n, _ := PerfectRunCount(db, p.ID); n != 1 {
		t.Fatalf("expected the crawl floor counted as a perfect run, got %d", n)
	}
	p.DeepestFloor = 4
	if err := Save(db, p); err != nil {
		t.Fatal(err)
	}
	b, err := ExportProfile(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != 3 || b.Profile.DeepestFloor != 4 {
		t.Fatalf("expected a version 3 backup with floor 4, got version %d floor %d", b.Version, b.Profile.DeepestFloor)
	}
	dst := openFileDB(t)
	imported, err := ImportProfile(dst, throughJSON(t, b), "")
	if err != nil {
		t.Fatal(err)
	}
	if imported.DeepestFloor != 4 {
		t.Fatalf("expected floor 4 imported, got %d", imported.DeepestFloor)
	}
	bad := throughJSON(t, b)
	bad.Profile.DeepestFloor = -1
	if _, err := ImportProfile(openFileDB(t), bad, ""); err == nil {
		t.Fatal("expected a negative deepest floor refused")
	}
	bad = throughJSON(t, b)
	bad.PersonalBests = append(bad.PersonalBests, PersonalBest{Mode: "words", Target: 15, Source: "crawl", WPM: 50, Accuracy: 100, AchievedAt: time.Now()})
	if _, err := ImportProfile(openFileDB(t), bad, ""); err == nil {
		t.Fatal("expected a crawl personal best refused")
	}
}

// A floor finished faster than any personal best counts for the speed
// achievements; a knockout floor and a custom run don't.
func TestAchievementWPMCountsCrawlFloors(t *testing.T) {
	db, p := newTestDB(t)
	if _, err := RecordPersonalBest(db, p.ID, "time", 30, "random", 60, 99, nil, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordPersonalBest(db, p.ID, "time", 30, "wordlist:/a.txt", 150, 99, nil, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		wpm float64
		ko  bool
	}{{85, false}, {120, true}} {
		if err := RecordTestResult(db, p.ID, r.wpm, 95, "words", 15, r.ko, true, false, "crawl", 15, 20*time.Second, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := AchievementWPM(db, p.ID); got != 85 {
		t.Fatalf("expected 85 from the cleared floor, got %v", got)
	}
	if n, _ := PunctuationRunCount(db, p.ID); n != 2 {
		t.Fatalf("expected punctuated floors counted, got %d", n)
	}
	empty, q := newTestDB(t)
	if got, _ := AchievementWPM(empty, q.ID); got != 0 {
		t.Fatalf("expected 0 with nothing recorded, got %v", got)
	}
}
