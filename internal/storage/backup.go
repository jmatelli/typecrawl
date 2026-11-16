package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jmatelli/typecrawl/internal/textsource"
)

// BackupVersion is the format version written by ExportProfile.
//
// Version 1 is the original --export shape, which had no version field at
// all (so it decodes as 0) and carried only the profile row, test history
// (without each run's source), and personal bests. Version 2 adds
// everything else a profile owns, so that an export can be imported back
// without losing anything. Version 3 is version 2 with runs on a player's
// own word lists or texts, drills, or crawl floors, which builds before
// version 3 can't read.
//
// ExportProfile writes version 3 only for a profile that has such runs or
// saved files, and version 2 otherwise, so a backup an older build can
// read stays readable by it. An older build then refuses a version 3
// backup with a clear "newer format" error, rather than calling one of
// its runs invalid.
const BackupVersion = 3

// customSourcesVersion is the version a backup with custom sources needs.
const customSourcesVersion = 3

// maxBackupNameLength mirrors the profile-creation screen's cap on name
// length, so an imported profile can't carry a name the app itself would
// never have let anyone type.
const maxBackupNameLength = 40

// ErrProfileExists is returned by ImportProfile when a profile with the
// target name is already in the database. Import never overwrites or
// merges into an existing profile.
var ErrProfileExists = errors.New("a profile with that name already exists")

// ErrInvalidName is returned by ImportProfile when the name the profile
// would get is not one the app allows. It's separate from
// ErrInvalidBackup because the caller can fix it by supplying another
// name: a profile created before names were restricted can carry one that
// is too long or contains control characters.
var ErrInvalidName = errors.New("invalid profile name")

// ErrInvalidBackup is returned by ImportProfile when the backup's contents
// could not have come from a real profile.
var ErrInvalidBackup = errors.New("invalid backup")

// WeeklyChallengeClaim records one week's claimed weekly-challenge bonus.
type WeeklyChallengeClaim struct {
	Week      string    `json:"week"` // challenge.WeekKey format, e.g. "2026-W46"
	ClaimedAt time.Time `json:"claimed_at"`
}

// ChallengeClaim records one day's claimed daily-challenge bonus.
type ChallengeClaim struct {
	Day       string    `json:"day"`
	ClaimedAt time.Time `json:"claimed_at"`
}

// Backup is a complete snapshot of one profile: everything needed to
// recreate it in another database. Achievements aren't listed because
// they aren't stored anywhere -- they're derived from the data below.
type Backup struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Profile    Profile   `json:"profile"`
	// History is ordered newest first.
	History       []TestHistoryEntry `json:"history"`
	PersonalBests []PersonalBest     `json:"personal_bests"`
	// Settings is nil in a version 1 backup.
	Settings *Settings `json:"settings,omitempty"`
	// DailyActivity maps a "YYYY-MM-DD" day to its exercise count.
	DailyActivity map[string]int `json:"daily_activity"`
	// CharMistakes maps a single character to its mistake count.
	CharMistakes    map[string]int   `json:"char_mistakes"`
	ChallengeClaims []ChallengeClaim `json:"challenge_claims"`
	// WeeklyChallengeClaims is absent from backups made before weekly
	// challenges existed, which then import with no weekly claims.
	WeeklyChallengeClaims []WeeklyChallengeClaim `json:"weekly_challenge_claims,omitempty"`
	// CustomSources are the profile's saved word lists and texts, as
	// textsource keys, oldest first. Only the paths travel, not the files.
	// Absent from backups made before custom sources existed.
	CustomSources []string `json:"custom_sources,omitempty"`
	// WeakKeys is each mistyped character's fading weight (see
	// MistakeHalfLife), one entry per CharMistakes key, and WeakPairs the
	// same for key pairs, with their lifetime counts. Both are absent from
	// backups made before mistakes faded; those import with each count as
	// its starting weight, fading from the export.
	WeakKeys  []MistakeWeight `json:"weak_keys,omitempty"`
	WeakPairs []MistakeWeight `json:"weak_pairs,omitempty"`
}

// MistakeWeight is one character's or key pair's fading mistake weight,
// as of WeightAt. Count, the lifetime count, is only set for pairs:
// a character's is in CharMistakes.
type MistakeWeight struct {
	Key      string    `json:"key"`
	Count    int       `json:"count,omitempty"`
	Weight   float64   `json:"weight"`
	WeightAt time.Time `json:"weight_at"`
}

// IsPartial reports whether b predates version 2 and so lacks settings,
// weak-key counts, challenge claims, and each run's source. (It lacks
// daily activity too, but import rebuilds that; see dailyActivity.)
func (b *Backup) IsPartial() bool {
	return b.Version < 2
}

// dailyActivity returns the per-day exercise counts to import. A version 1
// backup doesn't carry them, but they can be rebuilt exactly: the app
// records one activity tick per run, keyed by the same local date as that
// run's history timestamp, and a profile reset clears both together.
func (b *Backup) dailyActivity() map[string]int {
	if !b.IsPartial() || len(b.DailyActivity) > 0 {
		return b.DailyActivity
	}
	out := make(map[string]int)
	for _, e := range b.History {
		out[e.CreatedAt.Format("2006-01-02")]++
	}
	return out
}

// ExportProfile snapshots the profile with the given id and everything it
// owns.
func ExportProfile(db *sql.DB, id int64, now time.Time) (*Backup, error) {
	// A single transaction, so the snapshot can't straddle a write.
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Read inside the transaction, not passed in, so the level and XP
	// can't be older than the history they're exported with.
	profile, err := scanProfile(tx.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}

	// Empty sections are written as [] and {} rather than null or left
	// out, so the file has the same shape whatever the profile contains.
	b := &Backup{
		Version:         BackupVersion,
		ExportedAt:      now,
		Profile:         *profile,
		History:         []TestHistoryEntry{},
		PersonalBests:   []PersonalBest{},
		DailyActivity:   map[string]int{},
		CharMistakes:    map[string]int{},
		ChallengeClaims: []ChallengeClaim{},
	}

	var s Settings
	var punctuation, zenMode, focusWeak, quotes, hideBattleStrip int
	err = tx.QueryRow(`SELECT mode, duration, word_count, punctuation, zen_mode, focus_weak, quotes, hide_battle_strip, keyboard_layout, custom_source FROM settings WHERE profile_id = ?`, id).
		Scan(&s.Mode, &s.Duration, &s.WordCount, &punctuation, &zenMode, &focusWeak, &quotes, &hideBattleStrip, &s.KeyboardLayout, &s.CustomSource)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s = defaultSettings()
	case err != nil:
		return nil, err
	default:
		s.Punctuation, s.ZenMode, s.FocusWeak, s.Quotes = punctuation != 0, zenMode != 0, focusWeak != 0, quotes != 0
		s.HideBattleStrip = hideBattleStrip != 0
	}
	b.Settings = &s

	// Ordered by id rather than created_at: id is the order runs were
	// actually recorded in, which is what import has to reproduce.
	err = queryEach(tx, `
		SELECT wpm, accuracy, mode, target, ko, punctuation, zen_mode, source, words_typed, duration_seconds, created_at
		FROM test_history WHERE profile_id = ? ORDER BY id DESC
	`, id, func(rows *sql.Rows) error {
		var e TestHistoryEntry
		var ko, punct, zen, seconds int
		if err := rows.Scan(&e.WPM, &e.Accuracy, &e.Mode, &e.Target, &ko, &punct, &zen, &e.Source, &e.WordsTyped, &seconds, &e.CreatedAt); err != nil {
			return err
		}
		e.KO, e.Punctuation, e.ZenMode = ko != 0, punct != 0, zen != 0
		e.Duration = time.Duration(seconds) * time.Second
		b.History = append(b.History, e)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `
		SELECT mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series
		FROM personal_bests WHERE profile_id = ? ORDER BY mode, source, target
	`, id, func(rows *sql.Rows) error {
		var pb PersonalBest
		var pace, series string
		if err := rows.Scan(&pb.Mode, &pb.Target, &pb.Source, &pb.WPM, &pb.Accuracy, &pb.AchievedAt, &pace, &series); err != nil {
			return err
		}
		pb.GhostPace = parseGhostPace(pace)
		pb.GhostWPMSeries = parseFloats(series)
		b.PersonalBests = append(b.PersonalBests, pb)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `SELECT day, count FROM daily_activity WHERE profile_id = ?`, id, func(rows *sql.Rows) error {
		var day string
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			return err
		}
		b.DailyActivity[day] = count
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `SELECT char, count, weight, weight_at FROM char_mistakes WHERE profile_id = ? ORDER BY char`, id, func(rows *sql.Rows) error {
		var ch string
		var count int
		var w MistakeWeight
		var since int64
		if err := rows.Scan(&ch, &count, &w.Weight, &since); err != nil {
			return err
		}
		b.CharMistakes[ch] = count
		if since == 0 {
			// Written by an older copy of the game running alongside,
			// after the migration: start it as the migration would.
			w.Weight, since = float64(count), now.Unix()
		}
		w.Key, w.WeightAt = ch, time.Unix(since, 0).UTC()
		b.WeakKeys = append(b.WeakKeys, w)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `SELECT pair, count, weight, weight_at FROM key_pair_mistakes WHERE profile_id = ? ORDER BY pair`, id, func(rows *sql.Rows) error {
		var w MistakeWeight
		var since int64
		if err := rows.Scan(&w.Key, &w.Count, &w.Weight, &since); err != nil {
			return err
		}
		w.WeightAt = time.Unix(since, 0).UTC()
		b.WeakPairs = append(b.WeakPairs, w)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `SELECT day, claimed_at FROM daily_challenge_claims WHERE profile_id = ? ORDER BY day`, id, func(rows *sql.Rows) error {
		var c ChallengeClaim
		if err := rows.Scan(&c.Day, &c.ClaimedAt); err != nil {
			return err
		}
		b.ChallengeClaims = append(b.ChallengeClaims, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `SELECT week, claimed_at FROM weekly_challenge_claims WHERE profile_id = ? ORDER BY week`, id, func(rows *sql.Rows) error {
		var c WeeklyChallengeClaim
		if err := rows.Scan(&c.Week, &c.ClaimedAt); err != nil {
			return err
		}
		b.WeeklyChallengeClaims = append(b.WeeklyChallengeClaims, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = queryEach(tx, `SELECT source FROM custom_sources WHERE profile_id = ? ORDER BY added_at, source`, id, func(rows *sql.Rows) error {
		var source string
		if err := rows.Scan(&source); err != nil {
			return err
		}
		b.CustomSources = append(b.CustomSources, source)
		return nil
	})
	if err != nil {
		return nil, err
	}

	b.Version = customSourcesVersion - 1
	if b.usesCustomSources() {
		b.Version = customSourcesVersion
	}
	return b, nil
}

// usesCustomSources reports whether b holds anything only version 3 can
// carry: a saved or selected custom source, or a run or best on one or on
// a drill.
func (b *Backup) usesCustomSources() bool {
	custom := func(key string) bool {
		src, ok := textsource.Parse(key)
		return ok && src.Kind != textsource.Random && src.Kind != textsource.Quotes
	}
	if len(b.CustomSources) > 0 || (b.Settings != nil && b.Settings.CustomSource != "") {
		return true
	}
	for _, e := range b.History {
		if custom(e.Source) {
			return true
		}
	}
	for _, pb := range b.PersonalBests {
		if custom(pb.Source) {
			return true
		}
	}
	return false
}

func queryEach(tx *sql.Tx, query string, profileID int64, scan func(*sql.Rows) error) error {
	rows, err := tx.Query(query, profileID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ImportProfile creates a new profile from b and returns it. name
// overrides the name stored in the backup when non-empty.
//
// It is all-or-nothing: an invalid backup, or a name that is already
// taken (compared case-insensitively, as --profile lookups are), changes
// nothing. An existing profile is never overwritten or merged into.
func ImportProfile(db *sql.DB, b *Backup, name string) (*Profile, error) {
	if name == "" {
		name = b.Profile.Name
	}
	name = strings.TrimSpace(name)
	if err := validateProfileName(name); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidName, err)
	}
	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBackup, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	taken, err := nameTaken(tx, name)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, fmt.Errorf("%w: %q", ErrProfileExists, name)
	}

	p := b.Profile
	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	res, err := tx.Exec(`
		INSERT INTO profiles (name, level, xp, best_word_streak, ko_count, success_count, created_at, equipped_title, prestige, deepest_floor)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, name, p.Level, p.XP, p.BestWordStreak, p.KOCount, p.SuccessCount, dbTime(createdAt), p.EquippedTitle, p.Prestige, p.DeepestFloor)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if s := b.Settings; s != nil {
		_, err := tx.Exec(`
			INSERT INTO settings (profile_id, mode, duration, word_count, punctuation, zen_mode, focus_weak, quotes, hide_battle_strip, keyboard_layout, custom_source)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, s.Mode, s.Duration, s.WordCount, boolInt(s.Punctuation), boolInt(s.ZenMode), boolInt(s.FocusWeak), boolInt(s.Quotes), boolInt(s.HideBattleStrip), s.KeyboardLayout, s.CustomSource)
		if err != nil {
			return nil, err
		}
	}

	// History is stored newest first; insert oldest first so row ids
	// follow the order the runs originally happened in.
	for i := len(b.History) - 1; i >= 0; i-- {
		e := b.History[i]
		_, err := tx.Exec(`
			INSERT INTO test_history (profile_id, wpm, accuracy, mode, target, ko, punctuation, zen_mode, source, words_typed, duration_seconds, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, e.WPM, e.Accuracy, e.Mode, e.Target, boolInt(e.KO), boolInt(e.Punctuation), boolInt(e.ZenMode), sourceOrDefault(e.Source), e.WordsTyped, int(e.Duration.Seconds()), dbTime(e.CreatedAt))
		if err != nil {
			return nil, err
		}
	}

	for _, pb := range b.PersonalBests {
		_, err := tx.Exec(`
			INSERT INTO personal_bests (profile_id, mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, pb.Mode, pb.Target, sourceOrDefault(pb.Source), pb.WPM, pb.Accuracy, dbTime(pb.AchievedAt), serializeGhostPace(pb.GhostPace), serializeFloats(pb.GhostWPMSeries))
		if err != nil {
			return nil, err
		}
	}

	for day, count := range b.dailyActivity() {
		if _, err := tx.Exec(`INSERT INTO daily_activity (profile_id, day, count) VALUES (?, ?, ?)`, id, day, count); err != nil {
			return nil, err
		}
	}

	// A backup from before mistakes faded starts each count as its
	// weight, fading from when it was exported.
	seededAt := b.ExportedAt
	if now := time.Now(); seededAt.IsZero() || seededAt.After(now) {
		seededAt = now
	}
	weights := make(map[string]MistakeWeight, len(b.WeakKeys))
	for _, w := range b.WeakKeys {
		weights[w.Key] = w
	}
	for ch, count := range b.CharMistakes {
		w, ok := weights[ch]
		if !ok {
			w = MistakeWeight{Weight: float64(count), WeightAt: seededAt}
		}
		if _, err := tx.Exec(`INSERT INTO char_mistakes (profile_id, char, count, weight, weight_at) VALUES (?, ?, ?, ?, ?)`,
			id, ch, count, w.Weight, w.WeightAt.Unix()); err != nil {
			return nil, err
		}
	}
	for _, w := range b.WeakPairs {
		if _, err := tx.Exec(`INSERT INTO key_pair_mistakes (profile_id, pair, count, weight, weight_at) VALUES (?, ?, ?, ?, ?)`,
			id, w.Key, w.Count, w.Weight, w.WeightAt.Unix()); err != nil {
			return nil, err
		}
	}

	for _, c := range b.ChallengeClaims {
		if _, err := tx.Exec(`INSERT INTO daily_challenge_claims (profile_id, day, claimed_at) VALUES (?, ?, ?)`, id, c.Day, dbTime(c.ClaimedAt)); err != nil {
			return nil, err
		}
	}
	for _, c := range b.WeeklyChallengeClaims {
		if _, err := tx.Exec(`INSERT INTO weekly_challenge_claims (profile_id, week, claimed_at) VALUES (?, ?, ?)`, id, c.Week, dbTime(c.ClaimedAt)); err != nil {
			return nil, err
		}
	}
	// One second apart, so they keep their order when listed.
	for i, source := range b.CustomSources {
		if err := saveCustomSource(tx, id, source, b.ExportedAt.Add(time.Duration(i)*time.Second)); err != nil {
			return nil, err
		}
	}

	imported, err := scanProfile(tx.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return imported, nil
}

func nameTaken(tx *sql.Tx, name string) (bool, error) {
	rows, err := tx.Query(`SELECT name FROM profiles`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var existing string
		if err := rows.Scan(&existing); err != nil {
			return false, err
		}
		if strings.EqualFold(existing, name) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// sourceOrDefault fills in the source for version 1 backups, which were
// written before runs recorded one.
func sourceOrDefault(s string) string {
	if s == "" {
		return "random"
	}
	return s
}

func validateProfileName(name string) error {
	switch {
	case name == "":
		return errors.New("it is empty")
	case utf8.RuneCountInString(name) > maxBackupNameLength:
		return fmt.Errorf("it is longer than %d characters", maxBackupNameLength)
	case strings.ContainsFunc(name, unicode.IsControl):
		// Same rule as the profile-creation screen: a name must not be
		// able to smuggle terminal escape sequences into the UI.
		return errors.New("it contains control characters")
	}
	return nil
}

// validate rejects a backup that could not have been produced by a real
// profile, so a corrupt or hand-edited file fails up front instead of
// leaving odd rows behind.
func (b *Backup) validate() error {
	if b.Version < 0 || b.Version > BackupVersion {
		return fmt.Errorf("unsupported format version %d (this build reads up to version %d)", b.Version, BackupVersion)
	}

	p := b.Profile
	if p.Level < 1 || p.XP < 0 || p.BestWordStreak < 0 || p.KOCount < 0 || p.SuccessCount < 0 || p.Prestige < 0 || p.DeepestFloor < 0 {
		return errors.New("profile has a level below 1 or a negative counter")
	}
	if strings.ContainsFunc(p.EquippedTitle, unicode.IsControl) {
		return errors.New("equipped title contains control characters")
	}

	if s := b.Settings; s != nil {
		if !validMode(s.Mode) || s.Duration <= 0 || s.WordCount <= 0 {
			return errors.New("settings have an unknown mode or a non-positive duration or word count")
		}
		// An unknown layout is let through rather than rejected: a backup
		// from a newer version may name one this version lacks, and it
		// only changes how the heatmap is drawn (it falls back to QWERTY).
		if len(s.KeyboardLayout) > 32 || strings.ContainsFunc(s.KeyboardLayout, unicode.IsControl) {
			return errors.New("settings have a malformed keyboard layout")
		}
		if s.CustomSource != "" && !validCustomSource(s.CustomSource) {
			return errors.New("settings select a malformed custom source")
		}
	}

	for i, e := range b.History {
		if !validMode(e.Mode) || !validSource(e.Source) || e.WPM < 0 || e.Accuracy < 0 || e.Accuracy > 100 || e.WordsTyped < 0 || e.Duration < 0 || e.CreatedAt.IsZero() {
			return fmt.Errorf("history entry %d is not a valid run", i)
		}
	}

	type bestKey struct {
		mode, source string
		target       int
	}
	seen := make(map[bestKey]bool, len(b.PersonalBests))
	for i, pb := range b.PersonalBests {
		if !validMode(pb.Mode) || !validSource(pb.Source) || (pb.Source == string(textsource.Drill) || pb.Source == string(textsource.Crawl)) || pb.WPM < 0 || pb.Accuracy < 0 || pb.Accuracy > 100 || pb.AchievedAt.IsZero() {
			return fmt.Errorf("personal best %d is not valid", i)
		}
		k := bestKey{pb.Mode, sourceOrDefault(pb.Source), pb.Target}
		if seen[k] {
			return fmt.Errorf("personal best %d duplicates an earlier one", i)
		}
		seen[k] = true
	}

	seenSources := make(map[string]bool, len(b.CustomSources))
	for i, source := range b.CustomSources {
		if !validCustomSource(source) || seenSources[source] {
			return fmt.Errorf("custom source %d is malformed or a duplicate", i)
		}
		seenSources[source] = true
	}

	for day, count := range b.DailyActivity {
		if !validDay(day) || count < 0 {
			return fmt.Errorf("daily activity entry %q is not valid", day)
		}
	}

	for ch, count := range b.CharMistakes {
		if utf8.RuneCountInString(ch) != 1 || !utf8.ValidString(ch) || count < 0 {
			return fmt.Errorf("character mistake entry %q is not valid", ch)
		}
	}
	// A weight starts at its count and only fades, so it's never above
	// it; nor is its time after the export.
	latest := b.ExportedAt.Add(24 * time.Hour)
	if err := validMistakeWeights(b.WeakKeys, 1, latest, func(w MistakeWeight) bool {
		count, ok := b.CharMistakes[w.Key]
		return ok && w.Count == 0 && w.Weight <= float64(count)
	}); err != nil {
		return fmt.Errorf("weak key %w", err)
	}
	if err := validMistakeWeights(b.WeakPairs, 2, latest, func(w MistakeWeight) bool {
		return w.Count >= 0 && w.Weight <= float64(w.Count)
	}); err != nil {
		return fmt.Errorf("weak pair %w", err)
	}

	claimed := make(map[string]bool, len(b.ChallengeClaims))
	for _, c := range b.ChallengeClaims {
		if !validDay(c.Day) || claimed[c.Day] {
			return fmt.Errorf("challenge claim %q is invalid or duplicated", c.Day)
		}
		claimed[c.Day] = true
	}

	claimedWeeks := make(map[string]bool, len(b.WeeklyChallengeClaims))
	for _, c := range b.WeeklyChallengeClaims {
		if !validWeek(c.Week) || claimedWeeks[c.Week] || c.ClaimedAt.IsZero() {
			return fmt.Errorf("weekly challenge claim %q is invalid or duplicated", c.Week)
		}
		claimedWeeks[c.Week] = true
	}

	return nil
}

// validWeek accepts an ISO week key such as "2026-W46".
func validWeek(w string) bool {
	var year, week int
	if len(w) != 8 {
		return false
	}
	if n, err := fmt.Sscanf(w, "%4d-W%2d", &year, &week); err != nil || n != 2 {
		return false
	}
	return fmt.Sprintf("%04d-W%02d", year, week) == w && week >= 1 && week <= 53
}

// validMistakeWeights checks entries keyed by strings of runes visible
// characters (they're printed on the Weak Keys screen, so no escape
// sequences), with a usable weight, a time no later than latest, no key
// twice, and whatever ok asks.
func validMistakeWeights(entries []MistakeWeight, runes int, latest time.Time, ok func(MistakeWeight) bool) error {
	seen := make(map[string]bool, len(entries))
	for i, w := range entries {
		valid := utf8.ValidString(w.Key) && utf8.RuneCountInString(w.Key) == runes && !seen[w.Key] &&
			!strings.ContainsFunc(w.Key, func(r rune) bool { return !unicode.IsGraphic(r) }) &&
			w.Weight >= 0 && !math.IsInf(w.Weight, 0) && !math.IsNaN(w.Weight) &&
			w.WeightAt.Unix() > 0 && !w.WeightAt.After(latest) && ok(w)
		if !valid {
			return fmt.Errorf("%d is not valid", i)
		}
		seen[w.Key] = true
	}
	return nil
}

func validMode(m string) bool {
	return m == "time" || m == "words"
}

// validSource also accepts "", which is what a version 1 backup carries.
func validSource(s string) bool {
	if s == "" {
		return true
	}
	_, ok := textsource.Parse(s)
	return ok && validKeyText(s)
}

// validCustomSource accepts a word list or text key.
func validCustomSource(s string) bool {
	src, ok := textsource.Parse(s)
	return ok && src.Custom() && validKeyText(s)
}

// validKeyText bounds a source key: a path, but not an arbitrarily long
// one or one with control characters in it.
func validKeyText(s string) bool {
	return len(s) <= 4096 && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl)
}

func validDay(day string) bool {
	t, err := time.Parse("2006-01-02", day)
	return err == nil && t.Format("2006-01-02") == day
}
