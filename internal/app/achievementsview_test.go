package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/achievements"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func TestCycleEquippedTitleWrapsThroughNoneAndUnlocked(t *testing.T) {
	ids := []string{"first_exercise", "level_5"}
	seq := []string{""}
	cur := ""
	for range 4 {
		cur = cycleEquippedTitle(cur, ids)
		seq = append(seq, cur)
	}
	want := []string{"", "first_exercise", "level_5", "", "first_exercise"}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("step %d: got %q want %q (full seq %v)", i, seq[i], want[i], seq)
		}
	}
}

func TestCycleEquippedTitleRestartsFromStaleCurrent(t *testing.T) {
	got := cycleEquippedTitle("some_reset_achievement_no_longer_valid", []string{"first_exercise"})
	if got != "" {
		t.Fatalf("expected a stale current title to restart the cycle at None, got %q", got)
	}
}

func TestCycleEquippedTitleWithNoUnlockedAchievementsIsANoOp(t *testing.T) {
	if got := cycleEquippedTitle("", nil); got != "" {
		t.Fatalf("expected cycling with nothing unlocked to stay at None, got %q", got)
	}
}

func TestTitleNameAndSuffix(t *testing.T) {
	if got := titleName(""); got != "" {
		t.Fatalf("expected empty for no ID, got %q", got)
	}
	if got := titleName("not_a_real_id"); got != "" {
		t.Fatalf("expected empty for an unrecognized ID, got %q", got)
	}
	if got := titleName("first_exercise"); got != "First Steps" {
		t.Fatalf("expected 'First Steps', got %q", got)
	}
	if got := titleSuffix(""); got != "" {
		t.Fatalf("expected empty suffix for no title, got %q", got)
	}
	if got := titleSuffix("first_exercise"); got == "" {
		t.Fatal("expected a non-empty suffix for a real title")
	}
}

func TestUnlockedAchievementIDsMatchesManualFilter(t *testing.T) {
	p := achievements.Progress{TotalExercises: 100, Level: 20}
	got := unlockedAchievementIDs(p)
	manual := make([]string, 0)
	for _, a := range achievements.All {
		if a.Unlocked(p) {
			manual = append(manual, a.ID)
		}
	}
	if len(got) != len(manual) {
		t.Fatalf("expected %d unlocked IDs, got %d", len(manual), len(got))
	}
	for i := range manual {
		if got[i] != manual[i] {
			t.Fatalf("index %d: got %q want %q", i, got[i], manual[i])
		}
	}
}

func TestNewlyUnlockedReturnsOnlyTheDelta(t *testing.T) {
	before := achievements.Progress{TotalExercises: 1}
	after := achievements.Progress{TotalExercises: 10}
	got := newlyUnlocked(before, after)
	foundExercises10 := false
	for _, a := range got {
		if a.ID == "first_exercise" {
			t.Fatal("expected first_exercise (already unlocked before) to NOT be in the delta")
		}
		if a.ID == "exercises_10" {
			foundExercises10 = true
		}
	}
	if !foundExercises10 {
		t.Fatal("expected exercises_10 to be in the newly-unlocked delta")
	}
}

func TestNewlyUnlockedIsEmptyWhenNothingChanges(t *testing.T) {
	p := achievements.Progress{TotalExercises: 5}
	if got := newlyUnlocked(p, p); len(got) != 0 {
		t.Fatalf("expected no newly-unlocked achievements for identical progress, got %v", got)
	}
}

func TestBuildAchievementProgressReflectsProfileFields(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, err := storage.CreateProfile(db, "tester")
	if err != nil {
		t.Fatal(err)
	}
	profile.Level = 15
	profile.SuccessCount = 8
	profile.KOCount = 2
	profile.BestWordStreak = 30
	storage.Save(db, profile)

	p := buildAchievementProgress(db, profile, 5, 72.5)
	if p.Level != 15 {
		t.Errorf("expected Level=15, got %d", p.Level)
	}
	if p.TotalExercises != 8 { // SuccessCount only -- KOs don't count as "completed"
		t.Errorf("expected TotalExercises=8, got %d", p.TotalExercises)
	}
	if p.BestWordStreak != 30 {
		t.Errorf("expected BestWordStreak=30, got %d", p.BestWordStreak)
	}
	if p.DailyStreak != 5 {
		t.Errorf("expected DailyStreak=5 (passed through), got %d", p.DailyStreak)
	}
	if p.BestWPM != 72.5 {
		t.Errorf("expected BestWPM=72.5 (passed through), got %v", p.BestWPM)
	}
}

func TestAchievementsViewModelNavigation(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, err := storage.CreateProfile(db, "tester")
	if err != nil {
		t.Fatal(err)
	}

	m := newAchievementsViewModel(profile, db)
	view := m.View()
	if !containsSubstring(view, "Achievements") || !containsSubstring(view, "0/") {
		t.Fatalf("expected the achievements list view, got:\n%s", view)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = next
	if cmd == nil {
		t.Fatal("expected esc to return a command")
	}
	msg := cmd()
	if _, ok := msg.(viewProfileMsg); !ok {
		t.Fatalf("expected viewProfileMsg, got %T", msg)
	}
}

func TestNewAchievementsViewModelUsesCurrentStreakAndBestWPM(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profile, err := storage.CreateProfile(db, "tester")
	if err != nil {
		t.Fatal(err)
	}
	storage.RecordActivity(db, profile.ID, time.Now())
	storage.RecordPersonalBest(db, profile.ID, "time", 30, "random", 65, 95, nil, nil, time.Now())

	m := newAchievementsViewModel(profile, db)
	if m.progress.DailyStreak < 1 {
		t.Fatalf("expected a daily streak of at least 1, got %d", m.progress.DailyStreak)
	}
	if m.progress.BestWPM != 65 {
		t.Fatalf("expected BestWPM=65, got %v", m.progress.BestWPM)
	}
}
