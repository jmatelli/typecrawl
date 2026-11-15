package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jmatelli/typecrawl/internal/avatar"
	"github.com/jmatelli/typecrawl/internal/storage"
	"github.com/jmatelli/typecrawl/internal/theme"
)

// newBattleModel is a typing model with the battle strip on, in a
// terminal big enough to show it.
func newBattleModel(wordList ...string) typingModel {
	m := newTestTypingModel(modeWords, wordList, 100)
	m.width, m.height = 80, 24
	m.battle = true
	m.avatar = avatar.New("Alice")
	return m
}

func stripOf(t *testing.T, view string) []string {
	t.Helper()
	lines := strings.Split(stripANSI(view), "\n")
	// Found by the monster's head, which is in the same row in every
	// pose, since the words above can take fewer than visibleLines lines
	// near the end of a run.
	for i, line := range lines {
		if strings.Contains(line, " .-") && i >= monsterRow && i-monsterRow+stripHeight <= len(lines) {
			return lines[i-monsterRow : i-monsterRow+stripHeight]
		}
	}
	t.Fatalf("expected the battle strip in the view, got:\n%s", strings.Join(lines, "\n"))
	return nil
}

func monsterOf(t *testing.T, m typingModel) string {
	t.Helper()
	row := stripOf(t, m.View())[monsterRow+1]
	_, monster, _ := strings.Cut(row, "▶") // right of the sword tip
	return strings.Join(strings.Fields(monster), " ")
}

func TestSwordWidthMatchesTheSword(t *testing.T) {
	if got := len([]rune(sword)); got != swordWidth {
		t.Fatalf("swordWidth = %d, but the sword is %d columns", swordWidth, got)
	}
}

func TestBattleStripIsShownOnlyWhenOnAndItFits(t *testing.T) {
	cases := []struct {
		name          string
		battle        bool
		width, height int
		want          bool
	}{
		{"on, big terminal", true, 80, 24, true},
		{"turned off", false, 80, 24, false},
		{"exactly fits", true, stripWidth, typingLines + 1 + stripHeight, true},
		{"one row short", true, 80, typingLines + stripHeight, false},
		{"one column short", true, stripWidth - 1, 24, false},
		{"size not known yet", true, 0, 0, false},
	}
	// Enough words to fill every line, so the view is as tall as it gets.
	wordList := strings.Fields(strings.Repeat("some words that wrap over several lines of text ", 10))
	for _, c := range cases {
		m := newBattleModel(wordList...)
		m.battle, m.width, m.height = c.battle, c.width, c.height
		view := m.View()
		if got := strings.Contains(view, sword); got != c.want {
			t.Errorf("%s: strip shown = %v, want %v", c.name, got, c.want)
		}
		if lines := strings.Count(view, "\n") + 1; c.want && lines > c.height {
			t.Errorf("%s: the view is %d lines in a %d-line terminal", c.name, lines, c.height)
		}
	}
	m := newBattleModel(wordList...)
	m.width, m.height = stripWidth, typingLines+1+stripHeight
	if lines := strings.Count(m.View(), "\n") + 1; lines != m.height {
		t.Errorf("expected the view to fill the %d-line terminal exactly, got %d lines; typingLines is off", m.height, lines)
	}
}

func TestBattleStripDrawsTheProfileAvatarUnchanged(t *testing.T) {
	if theme.NoColor() {
		t.Skip("NO_COLOR is set, so there is no colour to compare")
	}
	// With colour on, so the colour is compared too.
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := newBattleModel("alpha", "beta")
	strip := strings.Split(renderBattleStrip(m.battleFrame(time.Now())), "\n")
	profile := strings.Split(avatar.Generate("Alice"), "\n")
	if !strings.Contains(profile[0], "\x1b[") {
		t.Fatal("expected the avatar in colour; the test isn't checking the colour")
	}
	for y, row := range profile {
		if !strings.HasPrefix(strip[y], strings.Repeat(" ", stripMargin)+row) {
			t.Errorf("row %d: expected the profile avatar %q, got %q", y, row, strip[y])
		}
	}
}

func TestMonsterLosesALetterForEachCorrectOne(t *testing.T) {
	m := newBattleModel("brown", "fox")
	if got := monsterOf(t, m); got != "| b r o w n |" {
		t.Fatalf("expected the whole word as the monster, got %q", got)
	}
	m = typeString(m, "br")
	if got := monsterOf(t, m); got != "| o w n |" {
		t.Errorf("after two correct letters: got %q", got)
	}
	m = typeString(m, "x") // a typo stops the cutting
	if got := monsterOf(t, m); got != "| o w n |" {
		t.Errorf("after a typo: got %q", got)
	}
	m = pressBackspace(m)
	m = pressBackspace(m) // backspacing a correct letter puts it back
	if got := monsterOf(t, m); got != "| r o w n |" {
		t.Errorf("after backspacing a correct letter: got %q", got)
	}
	m = typeString(m, "rown")
	if got := monsterOf(t, m); got != "| x_x |" {
		t.Errorf("with every letter cut: got %q", got)
	}
	m = pressSpace(m)
	if got := monsterOf(t, m); got != "| f o x |" {
		t.Errorf("after the word: expected the next word as the monster, got %q", got)
	}
}

func TestLongWordsAreTrimmedOnTheMonster(t *testing.T) {
	m := newBattleModel("extraordinarily", "fox")
	if got := monsterOf(t, m); got != "| e x t r a o r d i n a … |" {
		t.Errorf("got %q", got)
	}
}

// The widest the strip gets is a long word walking in. No frame may be
// wider than stripWidth, which decides when the strip fits.
func TestNoFrameIsWiderThanStripWidth(t *testing.T) {
	widest := 0
	for _, p := range []battlePose{poseIdle, poseLunge, poseHit, poseBlock, poseKO} {
		for _, strength := range []int{1, 2} {
			f := newBattleModel("extraordinarily").battleFrame(time.Now())
			f.pose, f.hitStrength, f.approach = p, strength, stripApproach
			f.shieldMarker, f.shieldStyle = goldenWordMarker, goldenWordStyle
			f.drop = blueWordMarker + " 5s shield!"
			for _, line := range strings.Split(renderBattleStrip(f), "\n") {
				w := lipgloss.Width(line)
				widest = max(widest, w)
				if w > stripWidth {
					t.Errorf("pose %v: line is %d columns, more than stripWidth %d: %q", p, w, stripWidth, stripANSI(line))
				}
			}
		}
	}
	if widest != stripWidth {
		t.Errorf("the widest frame is %d columns but stripWidth is %d; the strip hides itself in terminals it fits", widest, stripWidth)
	}
}

func TestSpecialWordsAreDistinctMonsters(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(*typingModel)
		want string
	}{
		{"golden", func(m *typingModel) { m.goldenWordIdx = 0 }, goldenWordMarker},
		{"green", func(m *typingModel) { m.greenWordIdx = 0 }, greenWordMarker},
		{"blue", func(m *typingModel) { m.blueWordIdx = 0 }, blueWordMarker},
	} {
		m := newBattleModel("fox", "dog")
		c.set(&m)
		if got, want := monsterOf(t, m), c.want+" f o x "+c.want; got != want {
			t.Errorf("%s: got %q, want %q", c.name, got, want)
		}
	}
}

func TestEachEventHasItsOwnPose(t *testing.T) {
	m := newBattleModel("brown", "fox")
	now := time.Now()
	if got := m.battleFrame(now).pose; got != poseIdle {
		t.Fatalf("expected idle before typing, got %v", got)
	}

	m = typeString(m, "b")
	if got := m.battleFrame(time.Now()).pose; got != poseLunge {
		t.Errorf("expected a lunge after a correct letter, got %v", got)
	}
	if got := m.battleFrame(time.Now().Add(lungeFor)).pose; got != poseIdle {
		t.Errorf("expected the lunge to end, got %v", got)
	}

	m = typeString(m, "x")
	f := m.battleFrame(time.Now())
	if f.pose != poseHit || f.hitStrength != 1 {
		t.Errorf("expected a single hit after a mistake, got pose %v strength %d", f.pose, f.hitStrength)
	}
	m = typeString(m, "x") // a second mistake straight away is a combo
	if f := m.battleFrame(time.Now()); f.pose != poseHit || f.hitStrength != 2 {
		t.Errorf("expected a heavier hit in a combo, got pose %v strength %d", f.pose, f.hitStrength)
	}

	m = newBattleModel("brown", "fox")
	m.shieldActive = true
	m = typeString(m, "x")
	if f := m.battleFrame(time.Now()); f.pose != poseBlock || f.shieldMarker != goldenWordMarker {
		t.Errorf("expected the golden shield to block, got pose %v shield %q", f.pose, f.shieldMarker)
	}

	for _, shield := range []bool{false, true} {
		m = newBattleModel("brown", "fox")
		m.zenMode, m.shieldActive = true, shield
		m = typeString(m, "x")
		if got := m.battleFrame(time.Now()).pose; got != poseIdle {
			t.Errorf("expected nothing to hit back in zen mode (shield %v), got %v", shield, got)
		}
	}

	// A correct letter and a mistake in the same instant: the mistake
	// came second, so it wins.
	m = newBattleModel("brown", "fox")
	at := time.Now()
	m.lastCutAt, m.lastHitAt, m.lastHitStrength = at, at, 1
	if got := m.battleFrame(at).pose; got != poseHit {
		t.Errorf("expected a hit to win over a cut at the same moment, got %v", got)
	}

	m = newBattleModel("brown", "fox")
	m.hp = 1
	m = typeString(m, "x")
	if !m.ko {
		t.Fatal("expected a knockout")
	}
	if got := m.battleFrame(time.Now()).pose; got != poseKO {
		t.Errorf("expected the knocked-out pose, got %v", got)
	}

	// Every pose draws differently.
	seen := map[string]battlePose{}
	for _, p := range []battlePose{poseIdle, poseLunge, poseHit, poseBlock, poseKO} {
		f := newBattleModel("brown").battleFrame(now)
		f.pose, f.hitStrength = p, 1
		if p == poseBlock {
			f.shieldMarker, f.shieldStyle = goldenWordMarker, goldenWordStyle
		}
		out := stripANSI(renderBattleStrip(f))
		if other, dup := seen[out]; dup {
			t.Errorf("poses %v and %v look the same:\n%s", other, p, out)
		}
		seen[out] = p
	}
}

func TestShieldsAreDrawnInFrontOfTheAvatar(t *testing.T) {
	m := newBattleModel("brown", "fox")
	if strings.Contains(strings.Join(stripOf(t, m.View()), "\n"), ")") {
		t.Fatal("expected no shield without one active")
	}
	m.shieldActive = true
	if got := stripOf(t, m.View())[1]; !strings.Contains(got, goldenWordMarker) {
		t.Errorf("expected the golden shield's marker, got %q", got)
	}
	m.shieldUntil = time.Now().Add(time.Minute)
	if got := stripOf(t, m.View())[1]; !strings.Contains(got, blueWordMarker) {
		t.Errorf("expected the timed shield to show while it runs, got %q", got)
	}
}

func TestDefeatedMonstersDropTheirReward(t *testing.T) {
	m := newBattleModel("fox", "dog", "cat", "owl")
	m.goldenWordIdx = 0
	m = typeString(m, "fox")
	m = pressSpace(m)
	f := m.battleFrame(time.Now())
	if f.drop != goldenWordMarker+" shield!" {
		t.Errorf("expected the golden word to drop a shield, got %q", f.drop)
	}
	if f.approach == 0 {
		t.Error("expected the next monster to be walking in")
	}
	if f := m.battleFrame(time.Now().Add(dropFor)); f.drop != "" || f.approach != 0 {
		t.Errorf("expected the drop gone and the monster at rest, got drop %q approach %d", f.drop, f.approach)
	}

	// Backspacing into the defeated word brings the monster back, so its
	// drop goes.
	if f := pressBackspace(m).battleFrame(time.Now()); f.drop != "" {
		t.Errorf("expected no drop over a reopened word, got %q", f.drop)
	}

	m.shieldActive = false // or it would forgive the typo below
	m.greenWordIdx = 1
	m = typeString(m, "dxg") // not clean: no heal, so nothing to drop
	m = pressSpace(m)
	if f := m.battleFrame(time.Now()); f.drop != "poof!" {
		t.Errorf("expected a missed green word to drop nothing, got %q", f.drop)
	}
}

// A knockout stays on screen before the results, but the pause must not
// count as typing time.
func TestKnockoutPausesWithoutCountingThePause(t *testing.T) {
	m := newBattleModel("brown", "fox")
	m.hp = 1
	m = typeString(m, "b")
	start := time.Now() // the pause starts in the Update below
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(typingModel)
	if !m.ko || !m.finished || cmd == nil {
		t.Fatalf("expected a knockout to finish the run, ko=%v finished=%v", m.ko, m.finished)
	}

	msg, ok := cmd().(finishTypingMsg)
	waited := time.Since(start)
	if !ok || !msg.ko {
		t.Fatalf("expected a knocked-out result, got %#v", msg)
	}
	if waited < koPause-50*time.Millisecond {
		t.Errorf("expected the results to wait %v, waited %v", koPause, waited)
	}
	if msg.result.Duration >= koPause {
		t.Errorf("the pause was counted as typing time: run lasted %v", msg.result.Duration)
	}

	// The result is delivered once, however it gets there.
	if again := m.pendingFinish(); again != nil {
		t.Errorf("expected the result to be delivered only once, got %#v a second time", again)
	}

	// Without the strip there's nothing to look at, so no pause.
	m = newBattleModel("brown", "fox")
	m.battle = false
	m.hp = 1
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	start = time.Now()
	if _, ok := cmd().(finishTypingMsg); !ok || time.Since(start) > 50*time.Millisecond {
		t.Error("expected the results straight away with the strip off")
	}
}

func TestBattleStripToggleIsSavedPerProfile(t *testing.T) {
	menu := newTestMenuModel(t)
	if !menu.battle {
		t.Fatal("expected the battle strip on by default")
	}
	next, _ := menu.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	menu = next.(menuModel)
	if menu.battle || !strings.Contains(menu.View(), "Battle strip: Off (b)") {
		t.Fatalf("expected the toggle to turn it off, got:\n%s", menu.View())
	}
	if s, _ := storage.LoadSettings(menu.db, menu.profile.ID); !s.HideBattleStrip {
		t.Fatal("expected the choice saved")
	}
	if newMenuModel(menu.profile, menu.db).battle {
		t.Fatal("expected the saved choice to be loaded back")
	}

	app := Model{db: menu.db, profile: menu.profile, state: stateMenu, width: 100, height: 40}
	app, _ = app.startTyping(startTypingMsg{mode: modeWords, target: 10})
	if app.typing.battleVisible() {
		t.Error("expected a run to follow the saved setting and hide the strip")
	}
	_ = storage.SaveSettings(menu.db, menu.profile.ID, storage.Settings{Mode: "words", Duration: 30, WordCount: 50})
	app, _ = app.startTyping(startTypingMsg{mode: modeWords, target: 10})
	if !app.typing.battleVisible() {
		t.Error("expected the strip in a run with it turned on")
	}
	if got, want := app.typing.avatar, avatar.New(menu.profile.Name); got != want {
		t.Error("expected the run to use the profile's own avatar")
	}
}

// Quitting during the pause after a knockout must still record it, or a
// knocked-out player could quit to keep it off their record.
func TestQuittingDuringTheKnockoutPauseStillRecordsIt(t *testing.T) {
	m := newBattleModel("brown", "fox")
	m.hp = 1
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(typingModel)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected ctrl+c to do something during the pause")
	}
	// tea.Sequence returns its commands as an unexported slice type; run
	// them in order, the way Bubble Tea does.
	seq := reflect.ValueOf(cmd())
	if seq.Kind() != reflect.Slice {
		t.Fatalf("expected a sequence of commands, got %#v", seq.Interface())
	}
	var msgs []tea.Msg
	for i := range seq.Len() {
		msgs = append(msgs, seq.Index(i).Interface().(tea.Cmd)())
	}
	if len(msgs) != 2 {
		t.Fatalf("expected the result and then quit, got %#v", msgs)
	}
	if r, ok := msgs[0].(finishTypingMsg); !ok || !r.ko {
		t.Errorf("expected the knocked-out result first, got %#v", msgs[0])
	}
	if _, ok := msgs[1].(tea.QuitMsg); !ok {
		t.Errorf("expected quit second, got %#v", msgs[1])
	}
}
