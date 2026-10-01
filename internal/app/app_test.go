package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/achievements"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func newTestApp(t *testing.T) (Model, *storage.Profile) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	profile, err := storage.CreateProfile(db, "tester")
	if err != nil {
		t.Fatal(err)
	}
	m := Model{db: db, profile: profile, state: stateMenu, width: 100, height: 40}
	return m, profile
}

// --- Construction / routing ---

func TestNewRoutesToProfileCreateWhenNoProfilesExist(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := New(db, nil)
	if m.state != stateProfileCreate {
		t.Fatalf("expected stateProfileCreate with no profiles, got %v", m.state)
	}
}

func TestNewRoutesToProfileSelectWhenProfilesExist(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, _ := storage.CreateProfile(db, "someone")
	m := New(db, []*storage.Profile{p})
	if m.state != stateProfileSelect {
		t.Fatalf("expected stateProfileSelect with existing profiles, got %v", m.state)
	}
}

func TestFindProfileByNameIsCaseInsensitive(t *testing.T) {
	profiles := []*storage.Profile{{Name: "Alice"}, {Name: "bob"}}
	if got := findProfileByName(profiles, "ALICE"); got == nil || got.Name != "Alice" {
		t.Fatalf("expected case-insensitive match for Alice, got %v", got)
	}
	if got := findProfileByName(profiles, "carol"); got != nil {
		t.Fatalf("expected nil for an unknown name, got %v", got)
	}
}

func TestNewWithOptionsErrorsOnUnknownProfile(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, _ := storage.CreateProfile(db, "alice")
	_, err = NewWithOptions(db, []*storage.Profile{p}, LaunchOptions{ProfileName: "ghost"})
	if err == nil {
		t.Fatal("expected an error for an unrecognized profile name")
	}
}

func TestNewWithOptionsSelectsProfileAndSkipsToMenu(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, _ := storage.CreateProfile(db, "alice")
	m, err := NewWithOptions(db, []*storage.Profile{p}, LaunchOptions{ProfileName: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if m.state != stateMenu || m.profile == nil || m.profile.Name != "alice" {
		t.Fatalf("expected stateMenu with alice selected, got state=%v profile=%v", m.state, m.profile)
	}
	if m.pendingAutoStart != nil {
		t.Fatal("expected no pending auto-start without a Mode")
	}
}

func TestNewWithOptionsQueuesAutoStartWhenModeGiven(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, _ := storage.CreateProfile(db, "alice")
	m, err := NewWithOptions(db, []*storage.Profile{p}, LaunchOptions{ProfileName: "alice", Mode: "words", Target: 50, ZenMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.pendingAutoStart == nil {
		t.Fatal("expected a pending auto-start to be queued")
	}
	if m.pendingAutoStart.mode != modeWords || m.pendingAutoStart.target != 50 || !m.pendingAutoStart.zenMode {
		t.Fatalf("expected mode=words target=50 zen=true, got %+v", m.pendingAutoStart)
	}
}

func TestPendingAutoStartFiresOnFirstWindowSizeMsg(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, _ := storage.CreateProfile(db, "alice")
	m, err := NewWithOptions(db, []*storage.Profile{p}, LaunchOptions{ProfileName: "alice", Mode: "time", Target: 30})
	if err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m2 := next.(Model)
	if m2.state != stateTyping {
		t.Fatalf("expected stateTyping once the auto-start fires, got %v", m2.state)
	}
	if m2.pendingAutoStart != nil {
		t.Fatal("expected pendingAutoStart cleared after firing")
	}
	if cmd == nil {
		t.Fatal("expected typing.Init()'s command")
	}
}

func TestIsScrollKeyRecognizesOnlyScrollBindings(t *testing.T) {
	scrollKeys := []tea.KeyType{tea.KeyPgUp, tea.KeyPgDown, tea.KeyCtrlU, tea.KeyCtrlD}
	for _, k := range scrollKeys {
		if !isScrollKey(tea.KeyMsg{Type: k}) {
			t.Errorf("expected %v to be recognized as a scroll key", k)
		}
	}
	nonScrollKeys := []tea.KeyType{tea.KeyEnter, tea.KeySpace, tea.KeyEsc, tea.KeyRunes}
	for _, k := range nonScrollKeys {
		if isScrollKey(tea.KeyMsg{Type: k}) {
			t.Errorf("expected %v to NOT be recognized as a scroll key", k)
		}
	}
}

// --- Screen-routing messages ---

func TestViewProfileMsgRoutesToProfileView(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.update(viewProfileMsg{})
	m2 := next.(Model)
	if m2.state != stateProfileView {
		t.Fatalf("expected stateProfileView, got %v", m2.state)
	}
}

func TestViewAchievementsMsgRoutesToAchievements(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.update(viewAchievementsMsg{})
	m2 := next.(Model)
	if m2.state != stateAchievements {
		t.Fatalf("expected stateAchievements, got %v", m2.state)
	}
}

func TestViewHistoryAndKeyboardMsgsRoute(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.update(viewHistoryMsg{})
	if next.(Model).state != stateHistory {
		t.Fatalf("expected stateHistory, got %v", next.(Model).state)
	}
	next, _ = m.update(viewKeyboardMsg{})
	if next.(Model).state != stateKeyboard {
		t.Fatalf("expected stateKeyboard, got %v", next.(Model).state)
	}
}

func TestSwitchProfileMsgRoutesToProfileSelectWithRefreshedList(t *testing.T) {
	m, p := newTestApp(t)
	storage.CreateProfile(m.db, "second")
	next, _ := m.update(switchProfileMsg{})
	m2 := next.(Model)
	if m2.state != stateProfileSelect {
		t.Fatalf("expected stateProfileSelect, got %v", m2.state)
	}
	if len(m2.profiles) != 2 {
		t.Fatalf("expected the profile list refreshed to include the new profile, got %d", len(m2.profiles))
	}
	_ = p
}

func TestProfileDeletedMsgClearsProfileAndRoutesToSelect(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.update(profileDeletedMsg{})
	m2 := next.(Model)
	if m2.state != stateProfileSelect || m2.profile != nil {
		t.Fatalf("expected profile cleared and routed to select, got state=%v profile=%v", m2.state, m2.profile)
	}
}

func TestRestartMsgReturnsToMenu(t *testing.T) {
	m, _ := newTestApp(t)
	m.state = stateResults
	next, _ := m.update(restartMsg{})
	m2 := next.(Model)
	if m2.state != stateMenu {
		t.Fatalf("expected stateMenu, got %v", m2.state)
	}
}

func TestContinueToResultsMsgRoutesToResults(t *testing.T) {
	m, _ := newTestApp(t)
	m.state = stateAchievementUnlock
	next, _ := m.update(continueToResultsMsg{})
	if next.(Model).state != stateResults {
		t.Fatalf("expected stateResults, got %v", next.(Model).state)
	}
}

// --- finishTypingMsg: the core gamification flow ---

func TestSuspiciousRunTouchesNothingPersisted(t *testing.T) {
	m, profile := newTestApp(t)

	suspiciousResult := stats.Result{WPM: 300, Accuracy: 100, KeystrokeCV: 0.5, KeystrokeSamples: 200}
	next, _ := m.update(finishTypingMsg{
		result: suspiciousResult, mode: modeTime, target: 30,
		hp: 100, maxHP: 100, bestCombo: 20, hitCount: 0,
	})
	m2 := next.(Model)

	if m2.state != stateResults {
		t.Fatalf("expected stateResults, got %v", m2.state)
	}
	if !m2.results.extra.unverified {
		t.Fatal("expected the results extra to be marked unverified")
	}
	if m2.profile.XP != 0 || m2.profile.Level != 1 || m2.profile.SuccessCount != 0 || m2.profile.KOCount != 0 {
		t.Fatalf("expected profile completely untouched, got %+v", m2.profile)
	}
	n, _ := storage.TotalWordsTyped(m.db, profile.ID)
	if n != 0 {
		t.Fatalf("expected zero words recorded to history, got %d", n)
	}
	pb, _ := storage.BestOverallWPM(m.db, profile.ID)
	if pb != nil {
		t.Fatal("expected no personal best recorded")
	}
}

func TestNormalRunAwardsXPAndRecordsHistory(t *testing.T) {
	m, profile := newTestApp(t)

	legitResult := stats.Result{WPM: 65, Accuracy: 97, KeystrokeCV: 0.45, KeystrokeSamples: 150}
	next, _ := m.update(finishTypingMsg{
		result: legitResult, mode: modeTime, target: 30,
		hp: 90, maxHP: 100, bestCombo: 12, hitCount: 1, wordsTyped: 40,
	})
	m2 := next.(Model)

	// A first-ever exercise legitimately unlocks achievements too, which
	// routes to stateAchievementUnlock first -- expected, not a bug.
	if m2.state != stateResults && m2.state != stateAchievementUnlock {
		t.Fatalf("expected stateResults or stateAchievementUnlock, got %v", m2.state)
	}
	if m2.results.extra.unverified {
		t.Fatal("expected a normal run to NOT be marked unverified")
	}
	if m2.profile.SuccessCount != 1 {
		t.Fatalf("expected SuccessCount to increment, got %d", m2.profile.SuccessCount)
	}
	if m2.profile.XP == 0 && m2.profile.Level == 1 {
		t.Fatal("expected some XP progress from a legitimate run")
	}
	n, _ := storage.TotalWordsTyped(m.db, profile.ID)
	if n != 40 {
		t.Fatalf("expected 40 words recorded to history, got %d", n)
	}
}

func TestKOedRunEarnsNoXPButIncrementsKOCount(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 60, KeystrokeCV: 0.4, KeystrokeSamples: 50},
		mode:   modeTime, target: 30, ko: true, hp: 0, maxHP: 100, bestCombo: 3,
	})
	m2 := next.(Model)
	if m2.profile.XP != 0 || m2.profile.Level != 1 {
		t.Fatalf("expected no XP/level change on a KO, got XP=%d level=%d", m2.profile.XP, m2.profile.Level)
	}
	if m2.profile.KOCount != 1 {
		t.Fatalf("expected KOCount=1, got %d", m2.profile.KOCount)
	}
	if m2.results.extra.ko != true {
		t.Fatal("expected resultsExtra.ko = true")
	}
}

func TestKOedRunDoesNotSetANewWordStreakRecord(t *testing.T) {
	m, _ := newTestApp(t)
	m.profile.BestWordStreak = 5
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 60, KeystrokeCV: 0.4, KeystrokeSamples: 50},
		mode:   modeTime, target: 30, ko: true, hp: 0, maxHP: 100, bestCombo: 50, // would be a new record if not KO'd
	})
	m2 := next.(Model)
	if m2.profile.BestWordStreak != 5 {
		t.Fatalf("expected BestWordStreak unchanged by a KO'd run, got %d", m2.profile.BestWordStreak)
	}
}

// TestKOedFirstExerciseDoesNotUnlockFirstSteps is a regression test for a
// reported bug: a brand-new profile's very first exercise ending in a KO
// must NOT unlock "First Steps" (or any "Complete X exercises" achievement)
// -- getting knocked out isn't completing the exercise, and KOs already
// have their own dedicated achievement line (ko_1/ko_5/ko_10).
func TestKOedFirstExerciseDoesNotUnlockFirstSteps(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 30, Accuracy: 50, KeystrokeCV: 0.4, KeystrokeSamples: 50},
		mode:   modeTime, target: 30, ko: true, hp: 0, maxHP: 100, bestCombo: 2,
	})
	m2 := next.(Model)

	// The achievement-unlock screen legitimately still shows -- this same
	// KO unlocks "Down But Not Out" (ko_1). What must NOT happen is
	// "First Steps" unlocking alongside it.
	progress := buildAchievementProgress(m.db, m2.profile, 0, 0)
	if progress.TotalExercises != 0 {
		t.Fatalf("expected TotalExercises=0 after a KO (SuccessCount still 0), got %d", progress.TotalExercises)
	}
	byID := make(map[string]bool)
	for _, a := range achievements.All {
		byID[a.ID] = a.Unlocked(progress)
	}
	if byID["first_exercise"] {
		t.Fatal("expected 'First Steps' to remain locked after a KO'd first exercise")
	}
	// It SHOULD still unlock the KO-specific track, though.
	if !byID["ko_1"] {
		t.Fatal("expected 'Down But Not Out' (ko_1) to unlock from this same KO")
	}
}

func TestNewWordStreakRecordIsSavedOnANormalRun(t *testing.T) {
	m, _ := newTestApp(t)
	m.profile.BestWordStreak = 5
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 60, Accuracy: 95, KeystrokeCV: 0.4, KeystrokeSamples: 100},
		mode:   modeTime, target: 30, hp: 90, maxHP: 100, bestCombo: 20,
	})
	m2 := next.(Model)
	if m2.profile.BestWordStreak != 20 {
		t.Fatalf("expected BestWordStreak updated to 20, got %d", m2.profile.BestWordStreak)
	}
	if !m2.results.extra.newWordStreakRecord {
		t.Fatal("expected newWordStreakRecord=true in resultsExtra")
	}
}

func TestPersonalBestIsRecordedOnImprovement(t *testing.T) {
	m, profile := newTestApp(t)
	result := stats.Result{
		WPM: 70, Accuracy: 96, KeystrokeCV: 0.4, KeystrokeSamples: 100,
		WordEndOffsetsMS: []int{400, 900, 1500}, WPMSeries: []float64{60, 65, 70},
	}
	next, _ := m.update(finishTypingMsg{result: result, mode: modeTime, target: 30, hp: 100, maxHP: 100, wordsTyped: 3})
	m2 := next.(Model)
	if !m2.results.extra.isNewPB {
		t.Fatal("expected this run to be a new PB")
	}
	pb, err := storage.GetPersonalBest(m.db, profile.ID, "time", 30)
	if err != nil || pb == nil {
		t.Fatalf("expected a stored PB, err=%v", err)
	}
	if len(pb.GhostPace) != 3 || pb.GhostPace[2] != 1500 {
		t.Fatalf("expected ghost pace [400 900 1500], got %v", pb.GhostPace)
	}
	if len(pb.GhostWPMSeries) != 3 || pb.GhostWPMSeries[2] != 70 {
		t.Fatalf("expected ghost WPM series [60 65 70], got %v", pb.GhostWPMSeries)
	}
}

func TestStartTypingLoadsGhostFromExistingPersonalBest(t *testing.T) {
	m, profile := newTestApp(t)
	storage.RecordPersonalBest(m.db, profile.ID, "time", 30, 70, 96, []int{400, 900, 1500}, []float64{60, 65, 70}, time.Now())

	next, _ := m.startTyping(startTypingMsg{mode: modeTime, target: 30})
	if len(next.typing.ghostPaceMS) != 3 || next.typing.ghostPaceMS[2] != 1500 {
		t.Fatalf("expected the new typingModel to load ghost pace [400 900 1500], got %v", next.typing.ghostPaceMS)
	}
	if len(next.typing.ghostWPMSeries) != 3 {
		t.Fatalf("expected the ghost WPM series loaded too, got %v", next.typing.ghostWPMSeries)
	}
}

func TestStartTypingWithNoExistingPersonalBestHasNoGhost(t *testing.T) {
	m, _ := newTestApp(t)
	next, _ := m.startTyping(startTypingMsg{mode: modeTime, target: 30})
	if next.typing.ghostPaceMS != nil {
		t.Fatalf("expected no ghost pace without an existing PB, got %v", next.typing.ghostPaceMS)
	}
}

func TestStartTypingAssignsHPFromProfileLevel(t *testing.T) {
	m, _ := newTestApp(t)
	m.profile.Level = 10
	next, _ := m.startTyping(startTypingMsg{mode: modeTime, target: 30})
	if next.typing.maxHP <= 100 {
		t.Fatalf("expected maxHP above the level-1 baseline for a level-10 profile, got %d", next.typing.maxHP)
	}
	if next.state != stateTyping {
		t.Fatalf("expected stateTyping, got %v", next.state)
	}
}
