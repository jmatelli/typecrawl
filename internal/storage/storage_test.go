package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// newTestDB opens a fresh in-memory database and creates one profile,
// returning both for convenience across the tests below.
func newTestDB(t *testing.T) (*sql.DB, *Profile) {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	p, err := CreateProfile(db, "tester")
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	return db, p
}

// --- Profile CRUD ---

func TestCreateProfileDefaults(t *testing.T) {
	db, p := newTestDB(t)
	_ = db
	if p.Level != 1 || p.XP != 0 || p.EquippedTitle != "" {
		t.Fatalf("expected a fresh profile at level 1, 0 XP, no title, got %+v", p)
	}
	if p.ID == 0 {
		t.Fatal("expected a non-zero assigned ID")
	}
}

func TestCreateProfileRejectsDuplicateName(t *testing.T) {
	db, _ := newTestDB(t)
	if _, err := CreateProfile(db, "tester"); err == nil {
		t.Fatal("expected an error creating a second profile with the same name")
	}
}

func TestListProfilesOrderedByID(t *testing.T) {
	db, first := newTestDB(t)
	second, err := CreateProfile(db, "second")
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := ListProfiles(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].ID != first.ID || profiles[1].ID != second.ID {
		t.Fatalf("expected profiles in creation order, got %+v", profiles)
	}
}

func TestSavePersistsProfileFields(t *testing.T) {
	db, p := newTestDB(t)
	p.Level = 5
	p.XP = 123
	p.BestWordStreak = 42
	p.KOCount = 2
	p.SuccessCount = 10
	if err := Save(db, p); err != nil {
		t.Fatal(err)
	}
	profiles, err := ListProfiles(db)
	if err != nil {
		t.Fatal(err)
	}
	got := profiles[0]
	if got.Level != 5 || got.XP != 123 || got.BestWordStreak != 42 || got.KOCount != 2 || got.SuccessCount != 10 {
		t.Fatalf("expected saved fields to persist, got %+v", got)
	}
}

func TestSetEquippedTitlePersists(t *testing.T) {
	db, p := newTestDB(t)
	if err := SetEquippedTitle(db, p.ID, "first_exercise"); err != nil {
		t.Fatal(err)
	}
	profiles, _ := ListProfiles(db)
	if profiles[0].EquippedTitle != "first_exercise" {
		t.Fatalf("expected equipped title to persist, got %q", profiles[0].EquippedTitle)
	}
}

func TestDeleteProfileRemovesItAndItsData(t *testing.T) {
	db, p := newTestDB(t)
	RecordActivity(db, p.ID, time.Now())
	if err := DeleteProfile(db, p.ID); err != nil {
		t.Fatal(err)
	}
	profiles, err := ListProfiles(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected no profiles after delete, got %d", len(profiles))
	}
}

func TestResetProfileKeepsNameAndAllTimeRecordsButClearsProgress(t *testing.T) {
	db, p := newTestDB(t)
	p.Level, p.XP, p.BestWordStreak, p.KOCount, p.SuccessCount = 10, 500, 99, 3, 20
	Save(db, p)
	SetEquippedTitle(db, p.ID, "level_10")
	RecordActivity(db, p.ID, time.Now())
	RecordPersonalBest(db, p.ID, "time", 30, 80, 95, nil, nil, time.Now())

	reset, err := ResetProfile(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Name != "tester" {
		t.Fatalf("expected name preserved, got %q", reset.Name)
	}
	if reset.Level != 1 || reset.XP != 0 {
		t.Fatalf("expected level/XP reset, got level=%d xp=%d", reset.Level, reset.XP)
	}
	if reset.BestWordStreak != 99 || reset.KOCount != 3 || reset.SuccessCount != 20 {
		t.Fatalf("expected all-time records preserved, got %+v", reset)
	}
	if reset.EquippedTitle != "" {
		t.Fatalf("expected equipped title cleared on reset, got %q", reset.EquippedTitle)
	}
	pb, _ := GetPersonalBest(db, p.ID, "time", 30)
	if pb != nil {
		t.Fatal("expected personal bests cleared on reset")
	}
}

// --- Settings ---

func TestSettingsRoundTrip(t *testing.T) {
	db, p := newTestDB(t)
	s := Settings{Mode: "words", Duration: 60, WordCount: 100, Punctuation: true, ZenMode: true, FocusWeak: true}
	if err := SaveSettings(db, p.ID, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != s {
		t.Fatalf("LoadSettings = %+v, want %+v", got, s)
	}
}

func TestLoadSettingsDefaultsWhenNoneSaved(t *testing.T) {
	db, p := newTestDB(t)
	got, err := LoadSettings(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultSettings()
	if got != want {
		t.Fatalf("LoadSettings with nothing saved = %+v, want defaults %+v", got, want)
	}
}

func TestSaveSettingsOverwritesPreviousValue(t *testing.T) {
	db, p := newTestDB(t)
	SaveSettings(db, p.ID, Settings{Mode: "time", Duration: 30, WordCount: 50})
	SaveSettings(db, p.ID, Settings{Mode: "words", Duration: 15, WordCount: 200, ZenMode: true})
	got, _ := LoadSettings(db, p.ID)
	if got.Mode != "words" || got.WordCount != 200 || !got.ZenMode {
		t.Fatalf("expected the second save to overwrite the first, got %+v", got)
	}
}

// --- Activity / streaks ---

func TestRecordActivityIncrementsSameDayCount(t *testing.T) {
	db, p := newTestDB(t)
	now := time.Now()
	RecordActivity(db, p.ID, now)
	RecordActivity(db, p.ID, now.Add(time.Hour))
	RecordActivity(db, p.ID, now.Add(2*time.Hour))

	counts, err := ActivityRange(db, p.ID, now.AddDate(0, 0, -1), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if counts[now.Format("2006-01-02")] != 3 {
		t.Fatalf("expected 3 activities recorded today, got %d", counts[now.Format("2006-01-02")])
	}
}

func TestCurrentStreakCountsConsecutiveDaysEndingToday(t *testing.T) {
	db, p := newTestDB(t)
	now := time.Now()
	RecordActivity(db, p.ID, now)
	RecordActivity(db, p.ID, now.AddDate(0, 0, -1))
	RecordActivity(db, p.ID, now.AddDate(0, 0, -2))
	// A gap at -4 breaks the streak before that.
	RecordActivity(db, p.ID, now.AddDate(0, 0, -4))

	streak, err := CurrentStreak(db, p.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if streak != 3 {
		t.Fatalf("expected a 3-day streak, got %d", streak)
	}
}

func TestCurrentStreakGrantsAGracePeriodForTodayNotYetDone(t *testing.T) {
	// By design, a streak doesn't drop to 0 the instant midnight passes
	// before today's exercise happens: if there's no activity yet today
	// but there was yesterday, the streak still counts through yesterday.
	db, p := newTestDB(t)
	now := time.Now()
	RecordActivity(db, p.ID, now.AddDate(0, 0, -1))
	RecordActivity(db, p.ID, now.AddDate(0, 0, -2))
	streak, err := CurrentStreak(db, p.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if streak != 2 {
		t.Fatalf("expected the 2-day streak through yesterday to still count, got %d", streak)
	}
}

func TestCurrentStreakBreaksAfterAFullMissedDay(t *testing.T) {
	// Two consecutive days with no activity (not just "today not done
	// yet") must actually break the streak.
	db, p := newTestDB(t)
	now := time.Now()
	RecordActivity(db, p.ID, now.AddDate(0, 0, -2)) // a 2-day-old gap, not yesterday
	streak, err := CurrentStreak(db, p.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if streak != 0 {
		t.Fatalf("expected a broken streak (missed yesterday entirely), got %d", streak)
	}
}

func TestCurrentStreakIsZeroWithNoActivityAtAll(t *testing.T) {
	db, p := newTestDB(t)
	streak, err := CurrentStreak(db, p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if streak != 0 {
		t.Fatalf("expected 0, got %d", streak)
	}
}

// --- Personal bests / ghost data ---

func TestRecordPersonalBestOnlyUpdatesOnImprovement(t *testing.T) {
	db, p := newTestDB(t)
	isNew, err := RecordPersonalBest(db, p.ID, "time", 30, 50, 90, []int{500, 1000}, []float64{40, 55}, time.Now())
	if err != nil || !isNew {
		t.Fatalf("expected first PB to be new, err=%v isNew=%v", err, isNew)
	}

	isNew, err = RecordPersonalBest(db, p.ID, "time", 30, 40, 99, []int{1}, []float64{1}, time.Now())
	if err != nil || isNew {
		t.Fatalf("expected a slower run to NOT be a new PB, err=%v isNew=%v", err, isNew)
	}
	pb, _ := GetPersonalBest(db, p.ID, "time", 30)
	if pb.WPM != 50 {
		t.Fatalf("expected PB to remain 50 WPM after a non-improving run, got %v", pb.WPM)
	}

	isNew, err = RecordPersonalBest(db, p.ID, "time", 30, 70, 95, []int{300, 700}, []float64{60, 70}, time.Now())
	if err != nil || !isNew {
		t.Fatalf("expected a faster run to be a new PB, err=%v isNew=%v", err, isNew)
	}
	pb, _ = GetPersonalBest(db, p.ID, "time", 30)
	if pb.WPM != 70 {
		t.Fatalf("expected updated PB of 70 WPM, got %v", pb.WPM)
	}
	if !reflect.DeepEqual(pb.GhostPace, []int{300, 700}) {
		t.Fatalf("expected ghost pace updated to [300 700], got %v", pb.GhostPace)
	}
	if !reflect.DeepEqual(pb.GhostWPMSeries, []float64{60, 70}) {
		t.Fatalf("expected ghost WPM series updated to [60 70], got %v", pb.GhostWPMSeries)
	}
}

func TestGetPersonalBestReturnsNilWhenNoneRecorded(t *testing.T) {
	db, p := newTestDB(t)
	pb, err := GetPersonalBest(db, p.ID, "time", 30)
	if err != nil {
		t.Fatal(err)
	}
	if pb != nil {
		t.Fatalf("expected nil, got %+v", pb)
	}
}

func TestBestOverallWPMPicksHighestAcrossConfigs(t *testing.T) {
	db, p := newTestDB(t)
	RecordPersonalBest(db, p.ID, "time", 30, 50, 90, nil, nil, time.Now())
	RecordPersonalBest(db, p.ID, "words", 100, 80, 95, nil, nil, time.Now())
	RecordPersonalBest(db, p.ID, "time", 60, 60, 92, nil, nil, time.Now())

	best, err := BestOverallWPM(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if best == nil || best.WPM != 80 || best.Mode != "words" {
		t.Fatalf("expected best overall to be the 80 WPM words run, got %+v", best)
	}
}

func TestListPersonalBestsReturnsAllConfigsOrdered(t *testing.T) {
	db, p := newTestDB(t)
	RecordPersonalBest(db, p.ID, "words", 100, 80, 95, nil, nil, time.Now())
	RecordPersonalBest(db, p.ID, "time", 30, 50, 90, nil, nil, time.Now())
	RecordPersonalBest(db, p.ID, "time", 60, 60, 92, nil, nil, time.Now())

	bests, err := ListPersonalBests(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bests) != 3 {
		t.Fatalf("expected 3 personal bests, got %d", len(bests))
	}
	// Ordered by mode then target: time/30, time/60, words/100.
	if bests[0].Mode != "time" || bests[0].Target != 30 {
		t.Fatalf("expected time/30 first, got %+v", bests[0])
	}
	if bests[2].Mode != "words" {
		t.Fatalf("expected words last (alphabetically after time), got %+v", bests[2])
	}
}

func TestHasPersonalBestInBothModes(t *testing.T) {
	db, p := newTestDB(t)
	got, _ := HasPersonalBestInBothModes(db, p.ID)
	if got {
		t.Fatal("expected false before any PB")
	}
	RecordPersonalBest(db, p.ID, "time", 30, 50, 90, nil, nil, time.Now())
	got, _ = HasPersonalBestInBothModes(db, p.ID)
	if got {
		t.Fatal("expected false with only one mode")
	}
	RecordPersonalBest(db, p.ID, "words", 100, 60, 92, nil, nil, time.Now())
	got, _ = HasPersonalBestInBothModes(db, p.ID)
	if !got {
		t.Fatal("expected true once both modes have a PB")
	}
}

func TestGhostPaceAndGhostWPMSeriesSerializationRoundTrip(t *testing.T) {
	cases := []struct {
		pace []int
		wpm  []float64
	}{
		{nil, nil},
		{[]int{0}, []float64{0}},
		{[]int{100, 200, 300}, []float64{40.5, 55.25, 60}},
	}
	db, p := newTestDB(t)
	for i, c := range cases {
		target := 30 + i
		RecordPersonalBest(db, p.ID, "time", target, float64(i+1), 90, c.pace, c.wpm, time.Now())
		pb, err := GetPersonalBest(db, p.ID, "time", target)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(pb.GhostPace, c.pace) {
			t.Errorf("case %d: GhostPace = %v, want %v", i, pb.GhostPace, c.pace)
		}
		if !reflect.DeepEqual(pb.GhostWPMSeries, c.wpm) {
			t.Errorf("case %d: GhostWPMSeries = %v, want %v", i, pb.GhostWPMSeries, c.wpm)
		}
	}
}

func TestParseGhostPaceSkipsMalformedEntries(t *testing.T) {
	if got := parseGhostPace(""); got != nil {
		t.Fatalf("expected nil for empty string, got %v", got)
	}
	if got := serializeGhostPace(nil); got != "" {
		t.Fatalf("expected empty string for nil slice, got %q", got)
	}
	got := parseGhostPace("100,abc,300")
	if !reflect.DeepEqual(got, []int{100, 300}) {
		t.Fatalf("expected malformed entries skipped, got %v", got)
	}
}

func TestParseFloatsSkipsMalformedEntries(t *testing.T) {
	if got := parseFloats(""); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	got := parseFloats("1.5,bad,2.25")
	if !reflect.DeepEqual(got, []float64{1.5, 2.25}) {
		t.Fatalf("expected malformed entries skipped, got %v", got)
	}
}

// --- Char mistakes / weak keys ---

func TestRecordCharMistakesAccumulatesAcrossCalls(t *testing.T) {
	db, p := newTestDB(t)
	RecordCharMistakes(db, p.ID, map[rune]int{'j': 3, 'q': 1})
	RecordCharMistakes(db, p.ID, map[rune]int{'j': 2})

	weights, err := WeakCharWeights(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if weights['j'] != 5 || weights['q'] != 1 {
		t.Fatalf("expected j=5 q=1, got %+v", weights)
	}
}

func TestTopWeakCharsRanksByCount(t *testing.T) {
	db, p := newTestDB(t)
	RecordCharMistakes(db, p.ID, map[rune]int{'j': 20, 'q': 15, 'z': 1})
	top, err := TopWeakChars(db, p.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0] != 'j' || top[1] != 'q' {
		t.Fatalf("expected [j q], got %v", top)
	}
}

func TestWeakCharWeightsEmptyWithNoMistakes(t *testing.T) {
	db, p := newTestDB(t)
	weights, err := WeakCharWeights(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(weights) != 0 {
		t.Fatalf("expected no weights, got %+v", weights)
	}
}

// --- Test history and derived achievement/challenge queries ---

func TestRecordTestResultAndListTestHistory(t *testing.T) {
	db, p := newTestDB(t)
	now := time.Now()
	RecordTestResult(db, p.ID, 65.5, 97.2, "time", 30, false, true, false, 40, 30*time.Second, now)
	RecordTestResult(db, p.ID, 40, 80, "words", 50, true, false, true, 20, 25*time.Second, now.Add(time.Minute))

	history, err := ListTestHistory(db, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(history))
	}
	// Newest first.
	if history[0].Mode != "words" || !history[0].KO || !history[0].ZenMode {
		t.Fatalf("expected the words/KO/zen run first, got %+v", history[0])
	}
	if history[1].Mode != "time" || !history[1].Punctuation {
		t.Fatalf("expected the time/punctuation run second, got %+v", history[1])
	}
}

func TestRecentWPMHistoryExcludesKOsAndOrdersOldestFirst(t *testing.T) {
	db, p := newTestDB(t)
	base := time.Now().Add(-time.Hour)
	RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, base)
	RecordTestResult(db, p.ID, 50, 60, "time", 30, true, false, false, 10, 10*time.Second, base.Add(time.Minute))
	RecordTestResult(db, p.ID, 55, 95, "time", 30, false, false, false, 42, 30*time.Second, base.Add(2*time.Minute))

	hist, err := RecentWPMHistory(db, p.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 || hist[0] != 50 || hist[1] != 55 {
		t.Fatalf("expected [50 55], got %v", hist)
	}
}

func TestRecentAccuracyHistoryExcludesKOsAndOrdersOldestFirst(t *testing.T) {
	db, p := newTestDB(t)
	base := time.Now().Add(-time.Hour)
	RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, base)
	RecordTestResult(db, p.ID, 50, 60, "time", 30, true, false, false, 10, 10*time.Second, base.Add(time.Minute))
	RecordTestResult(db, p.ID, 55, 95, "time", 30, false, false, false, 42, 30*time.Second, base.Add(2*time.Minute))

	hist, err := RecentAccuracyHistory(db, p.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 || hist[0] != 90 || hist[1] != 95 {
		t.Fatalf("expected [90 95], got %v", hist)
	}
}

func TestHasPerfectAccuracyRun(t *testing.T) {
	db, p := newTestDB(t)
	got, _ := HasPerfectAccuracyRun(db, p.ID)
	if got {
		t.Fatal("expected false before any run")
	}
	RecordTestResult(db, p.ID, 50, 100, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	got, _ = HasPerfectAccuracyRun(db, p.ID)
	if !got {
		t.Fatal("expected true after a 100% accuracy run")
	}
}

func TestHasPerfectAccuracyRunIgnoresKOs(t *testing.T) {
	db, p := newTestDB(t)
	RecordTestResult(db, p.ID, 50, 100, "time", 30, true, false, false, 40, 30*time.Second, time.Now())
	got, _ := HasPerfectAccuracyRun(db, p.ID)
	if got {
		t.Fatal("expected a KO'd 100%-accuracy run to not count")
	}
}

func TestHasCompletedTestConfigRequiresExactMatch(t *testing.T) {
	db, p := newTestDB(t)
	RecordTestResult(db, p.ID, 50, 90, "words", 200, false, false, false, 200, 60*time.Second, time.Now())
	got, _ := HasCompletedTestConfig(db, p.ID, "words", 200)
	if !got {
		t.Fatal("expected exact match to be found")
	}
	got, _ = HasCompletedTestConfig(db, p.ID, "words", 100)
	if got {
		t.Fatal("expected a different target to not match")
	}
}

func TestHasCompletedWordsAtLeastIsOpenEnded(t *testing.T) {
	db, p := newTestDB(t)
	RecordTestResult(db, p.ID, 50, 90, "words", 600, false, false, false, 600, 90*time.Second, time.Now())
	got, _ := HasCompletedWordsAtLeast(db, p.ID, 500)
	if !got {
		t.Fatal("expected a 600-word run to satisfy a >=500 threshold")
	}
	got, _ = HasCompletedWordsAtLeast(db, p.ID, 700)
	if got {
		t.Fatal("expected a 600-word run to NOT satisfy a >=700 threshold")
	}
}

func TestHasTestInHourRange(t *testing.T) {
	db, p := newTestDB(t)
	lateNight := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, lateNight)
	got, _ := HasTestInHourRange(db, p.ID, 0, 4)
	if !got {
		t.Fatal("expected a 2am run to match the midnight-4am range")
	}
	got, _ = HasTestInHourRange(db, p.ID, 4, 7)
	if got {
		t.Fatal("expected a 2am run to NOT match the 4am-7am range")
	}
}

func TestPerfectRunCountExcludesKOs(t *testing.T) {
	db, p := newTestDB(t)
	for range 9 {
		RecordTestResult(db, p.ID, 60, 100, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	}
	n, _ := PerfectRunCount(db, p.ID)
	if n != 9 {
		t.Fatalf("expected 9 perfect runs, got %d", n)
	}
	RecordTestResult(db, p.ID, 60, 100, "time", 30, true, false, false, 20, 15*time.Second, time.Now())
	n, _ = PerfectRunCount(db, p.ID)
	if n != 9 {
		t.Fatalf("expected a KO'd 100%% run to not count, got %d", n)
	}
}

func TestMaxPerfectAccuracyStreakTracksBreaksAndResets(t *testing.T) {
	db, p := newTestDB(t)
	n, _ := MaxPerfectAccuracyStreak(db, p.ID)
	if n != 0 {
		t.Fatalf("expected 0 with no history, got %d", n)
	}

	RecordTestResult(db, p.ID, 50, 100, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	RecordTestResult(db, p.ID, 55, 100, "time", 30, false, false, false, 42, 30*time.Second, time.Now())
	n, _ = MaxPerfectAccuracyStreak(db, p.ID)
	if n != 2 {
		t.Fatalf("expected a streak of 2, got %d", n)
	}

	RecordTestResult(db, p.ID, 50, 95, "time", 30, false, false, false, 38, 30*time.Second, time.Now())
	n, _ = MaxPerfectAccuracyStreak(db, p.ID)
	if n != 2 {
		t.Fatalf("expected max still 2 after a break, got %d", n)
	}

	for range 3 {
		RecordTestResult(db, p.ID, 50, 100, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	}
	n, _ = MaxPerfectAccuracyStreak(db, p.ID)
	if n != 3 {
		t.Fatalf("expected new best streak of 3, got %d", n)
	}

	// A KO at 100% accuracy must NOT extend the streak.
	RecordTestResult(db, p.ID, 30, 100, "time", 30, true, false, false, 20, 20*time.Second, time.Now())
	RecordTestResult(db, p.ID, 50, 100, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	n, _ = MaxPerfectAccuracyStreak(db, p.ID)
	if n != 3 {
		t.Fatalf("expected a KO to reset the streak (max stays 3), got %d", n)
	}
}

func TestComebackCountAndPerfectComeback(t *testing.T) {
	db, p := newTestDB(t)
	n, _ := ComebackCount(db, p.ID)
	if n != 0 {
		t.Fatalf("expected 0 with no history, got %d", n)
	}

	for range 4 {
		RecordTestResult(db, p.ID, 40, 80, "time", 30, true, false, false, 15, 15*time.Second, time.Now())
		RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	}
	n, _ = ComebackCount(db, p.ID)
	if n != 4 {
		t.Fatalf("expected 4 comebacks, got %d", n)
	}
	perfect, _ := HasPerfectComebackAfterKO(db, p.ID)
	if perfect {
		t.Fatal("expected no perfect comeback yet")
	}

	RecordTestResult(db, p.ID, 40, 80, "time", 30, true, false, false, 15, 15*time.Second, time.Now())
	RecordTestResult(db, p.ID, 55, 100, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	n, _ = ComebackCount(db, p.ID)
	perfect, _ = HasPerfectComebackAfterKO(db, p.ID)
	if n != 5 || !perfect {
		t.Fatalf("expected count=5 perfect=true, got count=%d perfect=%v", n, perfect)
	}
}

func TestPunctuationAndZenRunCounts(t *testing.T) {
	db, p := newTestDB(t)
	for range 3 {
		RecordTestResult(db, p.ID, 50, 90, "time", 30, false, true, false, 40, 30*time.Second, time.Now())
	}
	for range 2 {
		RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, true, 40, 30*time.Second, time.Now())
	}
	punct, _ := PunctuationRunCount(db, p.ID)
	zen, _ := ZenRunCount(db, p.ID)
	if punct != 3 || zen != 2 {
		t.Fatalf("expected punctuation=3 zen=2, got punctuation=%d zen=%d", punct, zen)
	}
}

func TestTotalWordsTypedIncludesKOs(t *testing.T) {
	db, p := newTestDB(t)
	RecordTestResult(db, p.ID, 50, 90, "words", 100, false, false, false, 100, 60*time.Second, time.Now())
	RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 42, 30*time.Second, time.Now())
	RecordTestResult(db, p.ID, 30, 70, "time", 30, true, false, false, 18, 15*time.Second, time.Now())
	n, _ := TotalWordsTyped(db, p.ID)
	if n != 160 {
		t.Fatalf("expected 160 total words, got %d", n)
	}
}

func TestTotalPlayTimeSumsDurationsIncludingKOs(t *testing.T) {
	db, p := newTestDB(t)
	d, _ := TotalPlayTime(db, p.ID)
	if d != 0 {
		t.Fatalf("expected 0 with no history, got %v", d)
	}
	RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, time.Now())
	RecordTestResult(db, p.ID, 40, 80, "words", 100, false, false, false, 60, 90*time.Second, time.Now())
	RecordTestResult(db, p.ID, 20, 60, "time", 60, true, false, false, 15, 20*time.Second, time.Now())
	d, _ = TotalPlayTime(db, p.ID)
	if d != 140*time.Second {
		t.Fatalf("expected 140s total play time, got %v", d)
	}
}

func TestHasPracticedAllWeekdays(t *testing.T) {
	db, p := newTestDB(t)
	base := time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC) // a Sunday
	for i := range 6 {
		RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, base.AddDate(0, 0, i))
	}
	got, _ := HasPracticedAllWeekdays(db, p.ID)
	if got {
		t.Fatal("expected false with only 6 distinct weekdays")
	}
	RecordTestResult(db, p.ID, 50, 90, "time", 30, false, false, false, 40, 30*time.Second, base.AddDate(0, 0, 6))
	got, _ = HasPracticedAllWeekdays(db, p.ID)
	if !got {
		t.Fatal("expected true with all 7 weekdays covered")
	}
}

func TestHasDayWithAtLeast(t *testing.T) {
	db, p := newTestDB(t)
	now := time.Now()
	for range 9 {
		RecordActivity(db, p.ID, now)
	}
	got, _ := HasDayWithAtLeast(db, p.ID, 10)
	if got {
		t.Fatal("expected false at 9 exercises in a day")
	}
	RecordActivity(db, p.ID, now)
	got, _ = HasDayWithAtLeast(db, p.ID, 10)
	if !got {
		t.Fatal("expected true at 10 exercises in a day")
	}
}

// --- Daily challenge claims ---

func TestDailyChallengeClaimRoundTrip(t *testing.T) {
	db, p := newTestDB(t)
	day := "2026-03-15"
	claimed, err := HasClaimedDailyChallenge(db, p.ID, day)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("expected unclaimed before any claim")
	}
	if err := ClaimDailyChallenge(db, p.ID, day, time.Now()); err != nil {
		t.Fatal(err)
	}
	claimed, _ = HasClaimedDailyChallenge(db, p.ID, day)
	if !claimed {
		t.Fatal("expected claimed after ClaimDailyChallenge")
	}
}

func TestClaimDailyChallengeIsIdempotent(t *testing.T) {
	db, p := newTestDB(t)
	day := "2026-03-15"
	if err := ClaimDailyChallenge(db, p.ID, day, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Claiming again must not error (INSERT OR IGNORE).
	if err := ClaimDailyChallenge(db, p.ID, day, time.Now()); err != nil {
		t.Fatalf("expected a second claim for the same day to be a harmless no-op, got %v", err)
	}
}

func TestDailyChallengeClaimsAreClearedOnDeleteAndReset(t *testing.T) {
	db, p := newTestDB(t)
	ClaimDailyChallenge(db, p.ID, "2026-03-15", time.Now())
	ResetProfile(db, p.ID)
	claimed, _ := HasClaimedDailyChallenge(db, p.ID, "2026-03-15")
	if claimed {
		t.Fatal("expected daily challenge claims cleared on profile reset")
	}
}

// --- Period summary ---

func TestSummaryAggregatesExercisesAvgWPMAndBestDay(t *testing.T) {
	db, p := newTestDB(t)
	now := time.Now()
	inRange := now.Add(-2 * 24 * time.Hour)
	outOfRange := now.Add(-40 * 24 * time.Hour)

	RecordActivity(db, p.ID, inRange)
	RecordActivity(db, p.ID, inRange)
	RecordActivity(db, p.ID, inRange.Add(24*time.Hour))
	RecordActivity(db, p.ID, outOfRange)

	RecordTestResult(db, p.ID, 60, 90, "time", 30, false, false, false, 40, 30*time.Second, inRange)
	RecordTestResult(db, p.ID, 80, 95, "time", 30, false, false, false, 40, 30*time.Second, inRange.Add(24*time.Hour))
	RecordTestResult(db, p.ID, 20, 60, "time", 30, true, false, false, 10, 15*time.Second, inRange) // KO, excluded from AvgWPM
	RecordTestResult(db, p.ID, 999, 99, "time", 30, false, false, false, 40, 30*time.Second, outOfRange)

	s, err := Summary(db, p.ID, now.AddDate(0, 0, -7), now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Exercises != 3 {
		t.Fatalf("expected 3 exercises in the 7-day window, got %d", s.Exercises)
	}
	if s.AvgWPM < 69 || s.AvgWPM > 71 {
		t.Fatalf("expected avg WPM ~70 (60 and 80 averaged, KO excluded), got %v", s.AvgWPM)
	}
	if s.BestDay != inRange.Format("2006-01-02") || s.BestDayCount != 2 {
		t.Fatalf("expected best day %s with count 2, got %s/%d", inRange.Format("2006-01-02"), s.BestDay, s.BestDayCount)
	}

	empty, err := Summary(db, p.ID, now.AddDate(0, 0, 10), now.AddDate(0, 0, 20))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Exercises != 0 || empty.BestDay != "" {
		t.Fatalf("expected an empty summary for a range with no activity, got %+v", empty)
	}
}

// --- Data directory resolution ---

func TestUserDataDirRespectsXDGOnLinux(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/xdg/data")
	got, err := userDataDirFor("linux")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/custom/xdg/data" {
		t.Fatalf("expected XDG_DATA_HOME to be respected, got %q", got)
	}
}

func TestUserDataDirFallsBackOnLinuxWithoutXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := userDataDirFor("linux")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share")
	if got != want {
		t.Fatalf("expected fallback %q, got %q", want, got)
	}
}

func TestUserDataDirUsesOSUserConfigDirOnDarwin(t *testing.T) {
	want, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := userDataDirFor("darwin")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("expected %q (os.UserConfigDir), got %q", want, got)
	}
}

func TestDefaultPathEndsUpUnderTypecrawl(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "typecrawl.db" {
		t.Fatalf("expected filename typecrawl.db, got %q", filepath.Base(path))
	}
	if filepath.Base(filepath.Dir(path)) != "typecrawl" {
		t.Fatalf("expected parent dir named typecrawl, got %q", filepath.Dir(path))
	}
}

// --- Migration backward compatibility ---

func TestMigrateAddsNewColumnsToAPreExistingSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate a database from before several columns existed.
	_, err = db.Exec(`
		CREATE TABLE profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE,
			level INTEGER NOT NULL DEFAULT 1, xp INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE test_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL,
			wpm REAL NOT NULL, accuracy REAL NOT NULL, mode TEXT NOT NULL,
			target INTEGER NOT NULL, ko INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE personal_bests (
			profile_id INTEGER NOT NULL, mode TEXT NOT NULL, target INTEGER NOT NULL,
			best_wpm REAL NOT NULL, accuracy REAL NOT NULL, achieved_at DATETIME NOT NULL,
			PRIMARY KEY (profile_id, mode, target)
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate on old schema failed: %v", err)
	}

	for _, check := range []struct{ table, column string }{
		{"test_history", "duration_seconds"},
		{"test_history", "punctuation"},
		{"test_history", "zen_mode"},
		{"test_history", "words_typed"},
		{"personal_bests", "ghost_pace"},
		{"personal_bests", "ghost_wpm_series"},
		{"profiles", "equipped_title"},
		{"profiles", "best_word_streak"},
	} {
		var count int
		q := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`
		if err := db.QueryRow(q, check.table, check.column).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("expected migrate to add %s.%s to a pre-existing table", check.table, check.column)
		}
	}
}

func TestAddColumnIfMissingIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := addColumnIfMissing(db, "t", "extra", "TEXT NOT NULL DEFAULT ''"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := addColumnIfMissing(db, "t", "extra", "TEXT NOT NULL DEFAULT ''"); err != nil {
		t.Fatalf("second call (duplicate column) should be treated as success, got %v", err)
	}
}
