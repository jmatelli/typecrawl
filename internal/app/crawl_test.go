package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmatelli/typecrawl/internal/game"
	"github.com/jmatelli/typecrawl/internal/stats"
	"github.com/jmatelli/typecrawl/internal/storage"
	"github.com/jmatelli/typecrawl/internal/words"
)

// past leaves a run's results screen, past any achievement popup.
func past(m Model) Model {
	for m.state == stateAchievementUnlock {
		m = runCmd(m, func() tea.Msg { return continueToResultsMsg{} })
	}
	return m
}

// typeFloor types the floor in progress through key presses: every word
// right, except that wrong words get one wrong key each first.
func typeFloor(t *testing.T, m Model, wrong int) Model {
	t.Helper()
	var cmd tea.Cmd
	for i, w := range m.typing.wordList[:m.typing.target] {
		if i < wrong {
			m, _ = pressKey(t, m, "#")
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
			m = next.(Model)
		}
		m, _ = pressKey(t, m, w)
		m.typing.tracker.Start = time.Now().Add(-time.Minute) // not scripted-fast
		next, c := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m, cmd = next.(Model), c
	}
	return past(runCmd(m, cmd))
}

// A crawl from the menu: HP lost on a floor carries to the next, each
// floor is a normal run on the built-in words that counts as an exercise,
// clearing one pays a depth bonus, and the deepest floor is kept.
func TestCrawlCarriesHPFromFloorToFloor(t *testing.T) {
	m, profile := newTestApp(t)
	m.menu = newMenuModel(profile, m.db)
	m.menu = press(t, m.menu, 'm')
	m.menu = press(t, m.menu, 'm')
	if m.menu.mode != modeCrawl || !containsSubstring(m.menu.View(), "Mode: Crawl") || !containsSubstring(m.menu.View(), "Deepest floor: no crawl yet") {
		t.Fatalf("expected crawl mode on the third mode, got:\n%s", m.menu.View())
	}
	_, cmd := m.menu.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = runCmd(m, cmd)
	if m.state != stateTyping || m.typing.floor.Number != 1 || m.typing.target != game.CrawlFirstFloorWords {
		t.Fatalf("expected floor 1 of %d words, got state %v floor %+v target %d", game.CrawlFirstFloorWords, m.state, m.typing.floor, m.typing.target)
	}
	if !containsSubstring(m.typing.View(), "Floor 1") {
		t.Fatal("expected the floor in the header")
	}
	maxHP := m.typing.maxHP

	m = typeFloor(t, m, 3) // three mistakes cost HP
	hp := m.results.extra.hp
	if m.state != stateResults || hp >= maxHP || m.results.extra.nextFloor != 2 {
		t.Fatalf("expected floor 1 cleared with HP lost, got state %v hp %d/%d next %d", m.state, hp, maxHP, m.results.extra.nextFloor)
	}
	extra := m.results.extra
	if extra.floorBonusXP != game.CrawlDepthBonus(game.FloorOf(1, 1)) || extra.xpGained != extra.xpBase+extra.xpBonus+extra.streakBonusXP+extra.challengeBonusXP+extra.weeklyBonusXP+extra.floorBonusXP {
		t.Fatalf("expected the depth bonus in the XP, got %+v", extra)
	}
	view := m.results.View()
	if !containsSubstring(view, "Floor 1 cleared!") || !containsSubstring(view, "enter: descend to floor 2") || containsSubstring(view, "d: drill") {
		t.Fatalf("expected the floor cleared and no drill mid-crawl, got:\n%s", view)
	}
	history, _ := storage.ListTestHistory(m.db, profile.ID, 1)
	if len(history) != 1 || history[0].Source != "crawl" || history[0].Mode != "words" || history[0].Target != game.CrawlFirstFloorWords {
		t.Fatalf("expected the floor saved as a crawl run, got %+v", history)
	}
	if pb, _ := storage.GetPersonalBest(m.db, profile.ID, "words", game.CrawlFirstFloorWords, "crawl"); pb != nil {
		t.Fatal("expected no personal best from a floor")
	}
	if profile.SuccessCount != 1 || profile.DeepestFloor != 1 {
		t.Fatalf("expected the floor counted and floor 1 as the deepest, got %+v", profile)
	}

	next, c := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = runCmd(next.(Model), c)
	if m.state != stateTyping || m.typing.floor.Number != 2 || m.typing.hp != hp || m.typing.target != game.FloorOf(2, 1).Words {
		t.Fatalf("expected floor 2 starting at %d HP, got floor %+v hp %d", hp, m.typing.floor, m.typing.hp)
	}
}

// A knockout ends the crawl and records the floor it happened on; back at
// the menu, it's still on crawl mode.
func TestKnockoutEndsTheCrawl(t *testing.T) {
	m, profile := newTestApp(t)
	m, _ = m.startCrawl(false)
	m.crawl.floor = 7
	m, _ = m.startFloor()
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 60, Duration: 30 * time.Second},
		mode:   modeWords, target: m.typing.target, hp: 0, maxHP: m.typing.maxHP, ko: true,
		floor: m.typing.floor, missedWords: []string{"alpha"},
	})
	m = past(next.(Model))
	if m.crawl != nil || m.results.extra.nextFloor != 0 {
		t.Fatal("expected the knockout to end the crawl")
	}
	if profile.DeepestFloor != 7 || profile.KOCount != 1 {
		t.Fatalf("expected floor 7 as the deepest and a knockout counted, got %+v", profile)
	}
	if stored, _ := storage.LoadProfile(m.db, profile.ID); stored.DeepestFloor != 7 {
		t.Fatalf("expected the deepest floor saved, got %d", stored.DeepestFloor)
	}
	view := m.results.View()
	if !containsSubstring(view, "Knocked out on floor 7.") || !containsSubstring(view, "Deepest floor: 7") || !containsSubstring(view, "d: drill") {
		t.Fatalf("expected the knockout, the record and a drill, got:\n%s", view)
	}
	if m.results.extra.xpGained != 0 || m.results.extra.floorBonusXP != 0 {
		t.Fatal("expected no XP for the floor knocked out on")
	}
	_, cmd := m.results.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = runCmd(m, cmd)
	if m.state != stateMenu || m.menu.mode != modeCrawl || !containsSubstring(m.menu.View(), "Deepest floor: 7") {
		t.Fatalf("expected the menu on crawl mode with the record, got state %v:\n%s", m.state, m.menu.View())
	}
	if !containsSubstring(newProfileViewModel(profile, m.db, 100).View(), "Deepest floor: 7") {
		t.Fatal("expected the deepest floor on the profile page")
	}
}

// Leaving between floors keeps what was earned and ends the crawl.
func TestLeavingBetweenFloors(t *testing.T) {
	m, profile := newTestApp(t)
	m, _ = m.startCrawl(false)
	m = typeFloor(t, m, 0)
	if m.results.extra.nextFloor != 2 {
		t.Fatal("expected floor 2 next")
	}
	_, cmd := m.results.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = runCmd(m, cmd)
	if m.state != stateMenu || m.crawl != nil || profile.DeepestFloor != 1 {
		t.Fatalf("expected the menu with the crawl over, got state %v crawl %+v", m.state, m.crawl)
	}
	// A stray next-floor message once the crawl is over does nothing.
	if next, _ := m.update(nextFloorMsg{}); next.(Model).state != stateMenu {
		t.Fatal("expected no floor without a crawl")
	}
}

// A boss floor is longer and a tier harder, and its words and damage
// come from that tier. An unused golden shield carries down with the HP.
func TestBossFloorAndCarriedShield(t *testing.T) {
	m, profile := newTestApp(t)
	m, _ = m.startCrawl(true)
	m.crawl.floor, m.crawl.hp, m.crawl.shield = 5, 42, true
	m, _ = m.startFloor()
	boss := game.FloorOf(5, game.DifficultyLevel(profile.Level, profile.Prestige))
	if m.typing.target != boss.Words || m.typing.level != boss.Level || !m.typing.punctuation {
		t.Fatalf("expected the boss's %d words at level %d with punctuation, got %d at %d", boss.Words, boss.Level, m.typing.target, m.typing.level)
	}
	if words.Tier(m.typing.level) <= words.Tier(game.FloorOf(4, 1).Level) {
		t.Fatal("expected the boss a tier above floor 4")
	}
	if m.typing.hp != 42 || !m.typing.shieldActive || m.typing.zenMode {
		t.Fatalf("expected 42 HP and the shield carried, got %d %v", m.typing.hp, m.typing.shieldActive)
	}
	if !containsSubstring(m.typing.View(), "BOSS") {
		t.Fatal("expected the boss marked in the header")
	}
}

// The anti-cheat check applies to a floor as to any run, and ends the
// crawl.
func TestScriptedFloorEndsTheCrawl(t *testing.T) {
	m, profile := newTestApp(t)
	m, _ = m.startCrawl(false)
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: game.MaxPlausibleWPM + 1, Accuracy: 100, Duration: 5 * time.Second},
		mode:   modeWords, target: m.typing.target, hp: 100, maxHP: 100, floor: m.typing.floor,
	})
	m = next.(Model)
	if !m.results.extra.unverified || m.crawl != nil || profile.DeepestFloor != 0 {
		t.Fatalf("expected an unverified run that ends the crawl and records nothing, got crawl %+v deepest %d", m.crawl, profile.DeepestFloor)
	}
	if history, _ := storage.ListTestHistory(m.db, profile.ID, 1); len(history) != 0 {
		t.Fatal("expected nothing saved")
	}
}

// Floors count toward achievements like any run on the built-in words.
func TestCrawlFloorsCountTowardAchievements(t *testing.T) {
	m, _ := newTestApp(t)
	m, _ = m.startCrawl(false)
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 100, Duration: 20 * time.Second},
		mode:   modeWords, target: m.typing.target, hp: 100, maxHP: 100, floor: m.typing.floor, wordsTyped: 15,
	})
	m = next.(Model)
	if m.state != stateAchievementUnlock {
		t.Fatal("expected a first floor to unlock the first-exercise achievement")
	}
	m = past(m)
	if m.results.extra.nextFloor != 2 {
		t.Fatal("expected the crawl to carry on after the popup")
	}
}

func TestMenuInCrawlMode(t *testing.T) {
	m := newTestMenuModel(t)
	m = press(t, m, 'm')
	m = press(t, m, 'm')
	for _, k := range []rune{'z', 'f', 's'} {
		m = press(t, m, k)
	}
	if m.zenMode || m.focusWeak || m.quotes {
		t.Fatal("expected z, f and s to do nothing in crawl mode")
	}
	view := m.View()
	if !containsSubstring(view, "Zen mode: N/A") || !containsSubstring(view, "Source: Random words   ") || !containsSubstring(view, "a boss every 5") {
		t.Fatalf("unexpected crawl menu:\n%s", view)
	}
	if settings, _ := storage.LoadSettings(m.db, m.profile.ID); settings.Mode != "words" {
		t.Fatalf("expected crawl mode saved as words for older versions, got %q", settings.Mode)
	}
	m = press(t, m, 'm')
	if m.mode != modeTime {
		t.Fatal("expected tab to come back round to time")
	}
}

func TestQuickLaunchCrawl(t *testing.T) {
	m, profile := newTestApp(t)
	launched, err := NewWithOptions(m.db, []*storage.Profile{profile}, LaunchOptions{ProfileName: profile.Name, Mode: "crawl", Punctuation: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := launched.pendingAutoStart; s == nil || !s.crawl || !s.punctuation || launched.menu.mode != modeCrawl {
		t.Fatalf("expected a crawl queued, got %+v", s)
	}
	// The queued start runs once the terminal size is known.
	next, _ := launched.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	launched = next.(Model)
	if launched.state != stateTyping || launched.crawl == nil || launched.typing.floor.Number != 1 || !launched.typing.punctuation {
		t.Fatalf("expected floor 1 of a crawl to start, got state %v floor %+v", launched.state, launched.typing.floor)
	}
}

func TestSharedCrawlFloor(t *testing.T) {
	m, _ := newTestApp(t)
	m, _ = m.startCrawl(false)
	m.crawl.floor = 3
	m, _ = m.startFloor()
	next, _ := m.update(finishTypingMsg{
		result: stats.Result{WPM: 40, Accuracy: 95, Duration: 20 * time.Second},
		mode:   modeWords, target: m.typing.target, hp: 80, maxHP: 100, floor: m.typing.floor,
	})
	run := past(next.(Model)).Runs()[0]
	if run.Source != "crawl" || run.CrawlFloor != 3 || !containsSubstring(run.Text(), "words 19 · crawl floor 3") {
		t.Fatalf("expected the crawl floor in the summary, got %+v:\n%s", run, run.Text())
	}
}
