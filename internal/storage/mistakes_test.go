package storage

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

// A database from before mistakes faded keeps every count: each becomes
// the starting weight, as of the upgrade. Opening it again changes
// nothing, and a new mistake adds to both the weight and the count.
func TestOldMistakeCountsSurviveTheMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE)`,
		`INSERT INTO profiles (name) VALUES ('alice')`,
		`CREATE TABLE char_mistakes (profile_id INTEGER NOT NULL, char TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (profile_id, char))`,
		`INSERT INTO char_mistakes VALUES (1, 'e', 12), (1, 'é', 3), (1, ';', 1)`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	upgraded := time.Now()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[rune]int{'e': 12, 'é': 3, ';': 1}
	if got, _ := WeakCharWeightsAt(db, 1, upgraded); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected the old counts as weights, got %v", got)
	}
	db.Close()

	// Opened again a day later: the weights keep fading from the first
	// upgrade, not reset to the counts.
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got, _ := WeakCharWeightsAt(db, 1, upgraded.Add(MistakeHalfLife)); got['e'] != 6 {
		t.Fatalf("expected e at half its weight a half-life on, got %v", got)
	}

	if err := RecordMistakes(db, 1, map[rune]int{'e': 2}, nil, upgraded); err != nil {
		t.Fatal(err)
	}
	if got, _ := WeakCharWeightsAt(db, 1, upgraded); got['e'] != 14 {
		t.Fatalf("expected the new mistakes added to the weight, got %v", got)
	}
	var count int
	if err := db.QueryRow(`SELECT count FROM char_mistakes WHERE char = 'e'`).Scan(&count); err != nil || count != 14 {
		t.Fatalf("expected the lifetime count kept at 14, got %d, %v", count, err)
	}
}

func TestMistakesFade(t *testing.T) {
	db, p := newTestDB(t)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := RecordMistakes(db, p.ID, map[rune]int{'q': 8, 'z': 1}, map[string]int{"th": 4}, start); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		after   time.Duration
		q, z    int
		th      int
		weakest []rune
	}{
		{0, 8, 1, 4, []rune{'q', 'z'}},
		{MistakeHalfLife, 4, 1, 2, []rune{'q', 'z'}}, // z at 0.5 rounds up
		{2 * MistakeHalfLife, 2, 0, 1, []rune{'q'}},  // z faded out
		{6 * MistakeHalfLife, 0, 0, 0, []rune{}},     // all faded out
		{-time.Hour, 8, 1, 4, []rune{'q', 'z'}},      // a clock set back doesn't add weight
	}
	for _, c := range cases {
		now := start.Add(c.after)
		chars, _ := WeakCharWeightsAt(db, p.ID, now)
		pairs, _ := WeakPairWeightsAt(db, p.ID, now)
		if chars['q'] != c.q || chars['z'] != c.z || pairs["th"] != c.th {
			t.Errorf("after %v: got chars %v pairs %v, want q=%d z=%d th=%d", c.after, chars, pairs, c.q, c.z, c.th)
		}
		if _, listed := chars['z']; listed != (c.z > 0) {
			t.Errorf("after %v: expected a faded-out key left out entirely, got %v", c.after, chars)
		}
	}

	// A mistake recorded later is added to the faded weight, not the old one.
	later := start.Add(MistakeHalfLife)
	if err := RecordMistakes(db, p.ID, map[rune]int{'q': 1}, nil, later); err != nil {
		t.Fatal(err)
	}
	if got, _ := WeakCharWeightsAt(db, p.ID, later); got['q'] != 5 {
		t.Fatalf("expected 8 faded to 4 plus 1, got %v", got)
	}
}

// The profile screen's weakest keys follow the faded weights: a key that
// was weak long ago drops below one mistyped recently.
func TestTopWeakCharsFollowsTheFadedWeights(t *testing.T) {
	db, p := newTestDB(t)
	now := time.Now()
	if err := RecordMistakes(db, p.ID, map[rune]int{'a': 20}, nil, now.Add(-4*MistakeHalfLife)); err != nil {
		t.Fatal(err)
	}
	if err := RecordMistakes(db, p.ID, map[rune]int{'b': 3, 'c': 3, 'd': 5}, nil, now); err != nil {
		t.Fatal(err)
	}
	got, err := TopWeakChars(db, p.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := []rune{'d', 'b', 'c'}; !slices.Equal(got, want) {
		t.Fatalf("expected %q (a faded to about 1), got %q", string(want), string(got))
	}
}

// A backup from before mistakes faded imports each count as its weight,
// fading from when it was exported.
func TestOlderBackupSeedsWeightsFromCounts(t *testing.T) {
	exported := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	b := &Backup{
		Version: 2, ExportedAt: exported,
		Profile:       Profile{Name: "Old", Level: 3},
		DailyActivity: map[string]int{},
		CharMistakes:  map[string]int{"k": 10},
	}
	db := openFileDB(t)
	p, err := ImportProfile(db, b, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := WeakCharWeightsAt(db, p.ID, exported); got['k'] != 10 {
		t.Fatalf("expected the count as the weight at export, got %v", got)
	}
	if got, _ := WeakCharWeightsAt(db, p.ID, exported.Add(MistakeHalfLife)); got['k'] != 5 {
		t.Fatalf("expected it to fade from the export, got %v", got)
	}
	if pairs, _ := WeakPairWeights(db, p.ID); len(pairs) != 0 {
		t.Fatalf("expected no pairs from an older backup, got %v", pairs)
	}
}

func TestResetAndDeleteClearKeyPairs(t *testing.T) {
	db, p := newTestDB(t)
	if err := RecordMistakes(db, p.ID, nil, map[string]int{"th": 2}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := ResetProfile(db, p.ID); err != nil {
		t.Fatal(err)
	}
	if pairs, _ := WeakPairWeights(db, p.ID); len(pairs) != 0 {
		t.Fatalf("expected a reset to clear the pairs, got %v", pairs)
	}
	if err := RecordMistakes(db, p.ID, nil, map[string]int{"th": 2}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProfile(db, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM key_pair_mistakes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("expected deleting the profile to clear its pairs, got %d, %v", n, err)
	}
}
