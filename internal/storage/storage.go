// Package storage persists player profiles (level, XP) in a local SQLite
// database so progress survives between sessions.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Profile struct {
	ID             int64
	Name           string
	Level          int
	XP             int
	BestWordStreak int // all-time longest mistake-free word streak
	KOCount        int // how many exercises have ended in a knockout
	SuccessCount   int // how many exercises finished without a knockout
	CreatedAt      time.Time
	// EquippedTitle is an achievement ID the player chose to show off next
	// to their name, or "" for none. It's just a display choice -- the
	// achievement it names must still be independently unlocked to be
	// worth anything (see the app package's title-cycling logic).
	EquippedTitle string
}

// userDataDir returns the OS's conventional base directory for per-user
// application data. On Linux this follows the XDG Base Directory spec's
// data directory ($XDG_DATA_HOME, or ~/.local/share) rather than
// os.UserConfigDir's ~/.config -- this is a database of real user data
// (profiles, stats, history), not settings, so the data directory is the
// semantically correct one. macOS and Windows don't draw that distinction
// the same way, so os.UserConfigDir's platform default (~/Library/Application
// Support, %AppData%) already serves both roles there.
func userDataDir() (string, error) {
	return userDataDirFor(runtime.GOOS)
}

// userDataDirFor is userDataDir's logic parameterized on GOOS, so it's
// exercisable in tests regardless of which platform they run on.
func userDataDirFor(goos string) (string, error) {
	if goos == "linux" {
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return dir, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share"), nil
	}
	return os.UserConfigDir()
}

// DefaultPath returns the on-disk location of the Typecrawl database,
// creating its parent directory if necessary.
func DefaultPath() (string, error) {
	dir, err := userDataDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "typecrawl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "typecrawl.db"), nil
}

// Open opens (and migrates) the SQLite database at path.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS profiles (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			name             TEXT NOT NULL UNIQUE,
			level            INTEGER NOT NULL DEFAULT 1,
			xp               INTEGER NOT NULL DEFAULT 0,
			best_word_streak INTEGER NOT NULL DEFAULT 0,
			ko_count         INTEGER NOT NULL DEFAULT 0,
			success_count    INTEGER NOT NULL DEFAULT 0,
			created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			equipped_title   TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS daily_activity (
			profile_id INTEGER NOT NULL REFERENCES profiles(id),
			day        TEXT NOT NULL,
			count      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (profile_id, day)
		);
		CREATE TABLE IF NOT EXISTS settings (
			profile_id  INTEGER PRIMARY KEY REFERENCES profiles(id),
			mode        TEXT NOT NULL DEFAULT 'time',
			duration    INTEGER NOT NULL DEFAULT 30,
			word_count  INTEGER NOT NULL DEFAULT 100,
			punctuation INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS personal_bests (
			profile_id  INTEGER NOT NULL REFERENCES profiles(id),
			mode        TEXT NOT NULL,
			target      INTEGER NOT NULL,
			source      TEXT NOT NULL DEFAULT 'random',
			best_wpm    REAL NOT NULL,
			accuracy    REAL NOT NULL,
			achieved_at DATETIME NOT NULL,
			ghost_pace  TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (profile_id, mode, target, source)
		);
		CREATE TABLE IF NOT EXISTS char_mistakes (
			profile_id INTEGER NOT NULL REFERENCES profiles(id),
			char       TEXT NOT NULL,
			count      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (profile_id, char)
		);
		CREATE TABLE IF NOT EXISTS test_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id  INTEGER NOT NULL REFERENCES profiles(id),
			wpm         REAL NOT NULL,
			accuracy    REAL NOT NULL,
			mode        TEXT NOT NULL,
			target      INTEGER NOT NULL,
			ko          INTEGER NOT NULL DEFAULT 0,
			punctuation INTEGER NOT NULL DEFAULT 0,
			zen_mode    INTEGER NOT NULL DEFAULT 0,
			words_typed INTEGER NOT NULL DEFAULT 0,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS daily_challenge_claims (
			profile_id INTEGER NOT NULL REFERENCES profiles(id),
			day        TEXT NOT NULL,
			claimed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (profile_id, day)
		);
		CREATE TABLE IF NOT EXISTS quotes_cache (
			id     INTEGER PRIMARY KEY,
			quote  TEXT NOT NULL,
			author TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "profiles", "best_word_streak", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "profiles", "ko_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "profiles", "success_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "settings", "zen_mode", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "settings", "focus_weak", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "settings", "quotes", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "test_history", "punctuation", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "test_history", "zen_mode", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "test_history", "words_typed", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "test_history", "duration_seconds", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "personal_bests", "ghost_pace", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "personal_bests", "ghost_wpm_series", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "test_history", "source", "TEXT NOT NULL DEFAULT 'random'"); err != nil {
		return err
	}
	if err := migratePersonalBestsSource(db); err != nil {
		return err
	}
	return addColumnIfMissing(db, "profiles", "equipped_title", "TEXT NOT NULL DEFAULT ''")
}

// migratePersonalBestsSource adds the source column to a personal_bests
// table created before quotes mode existed, where every row is implicitly a
// random-words PB. The column alone isn't enough here, unlike every other
// addColumnIfMissing use in this file: source is also part of the primary
// key (a quotes run and a random-words run at the same mode/target must be
// two separate PB rows, not one overwriting the other), and SQLite can't
// ALTER a PRIMARY KEY in place -- so an already-existing table has to be
// rebuilt under the new key instead. Gated on the column's absence, so this
// runs at most once per database.
func migratePersonalBestsSource(db *sql.DB) error {
	hasSource, err := columnExists(db, "personal_bests", "source")
	if err != nil || hasSource {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE personal_bests_new (
			profile_id  INTEGER NOT NULL REFERENCES profiles(id),
			mode        TEXT NOT NULL,
			target      INTEGER NOT NULL,
			source      TEXT NOT NULL DEFAULT 'random',
			best_wpm    REAL NOT NULL,
			accuracy    REAL NOT NULL,
			achieved_at DATETIME NOT NULL,
			ghost_pace  TEXT NOT NULL DEFAULT '',
			ghost_wpm_series TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (profile_id, mode, target, source)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO personal_bests_new (profile_id, mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series)
		SELECT profile_id, mode, target, 'random', best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series FROM personal_bests
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE personal_bests`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE personal_bests_new RENAME TO personal_bests`); err != nil {
		return err
	}
	return tx.Commit()
}

// columnExists reports whether table has the given column, used where a
// plain addColumnIfMissing isn't enough (see migratePersonalBestsSource).
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// addColumnIfMissing runs an ALTER TABLE ADD COLUMN for databases created
// before that column existed. SQLite has no "ADD COLUMN IF NOT EXISTS", so
// a "duplicate column" failure (already migrated) is treated as success.
func addColumnIfMissing(db *sql.DB, table, column, def string) error {
	_, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def))
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}

// ListProfiles returns every profile, ordered by creation order.
func ListProfiles(db *sql.DB) ([]*Profile, error) {
	rows, err := db.Query(`SELECT id, name, level, xp, best_word_streak, ko_count, success_count, created_at, equipped_title FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Level, &p.XP, &p.BestWordStreak, &p.KOCount, &p.SuccessCount, &p.CreatedAt, &p.EquippedTitle); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// DeleteProfile permanently removes a profile and everything tied to it.
func DeleteProfile(db *sql.DB, id int64) error {
	for _, table := range []string{"daily_activity", "settings", "personal_bests", "char_mistakes", "test_history", "daily_challenge_claims"} {
		if _, err := db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE profile_id = ?`, table), id); err != nil {
			return err
		}
	}
	_, err := db.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}

// ResetProfile resets a profile's level, XP, activity history, personal
// bests, weak-key stats, and test history back to the starting values,
// keeping its name and its all-time records (best word streak, KO count).
func ResetProfile(db *sql.DB, id int64) (*Profile, error) {
	if _, err := db.Exec(`UPDATE profiles SET level = 1, xp = 0, equipped_title = '' WHERE id = ?`, id); err != nil {
		return nil, err
	}
	for _, table := range []string{"daily_activity", "personal_bests", "char_mistakes", "test_history", "daily_challenge_claims"} {
		if _, err := db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE profile_id = ?`, table), id); err != nil {
			return nil, err
		}
	}
	row := db.QueryRow(`SELECT id, name, level, xp, best_word_streak, ko_count, success_count, created_at, equipped_title FROM profiles WHERE id = ?`, id)
	return scanProfile(row)
}

// RecordActivity logs one completed exercise for profileID on the given
// day, incrementing that day's count.
func RecordActivity(db *sql.DB, profileID int64, when time.Time) error {
	day := when.Format("2006-01-02")
	_, err := db.Exec(`
		INSERT INTO daily_activity (profile_id, day, count) VALUES (?, ?, 1)
		ON CONFLICT(profile_id, day) DO UPDATE SET count = count + 1
	`, profileID, day)
	return err
}

// ActivityRange returns activity counts keyed by "YYYY-MM-DD" for profileID
// between from and to (inclusive). Days with no recorded activity are
// simply absent from the map (a missing key reads as 0).
func ActivityRange(db *sql.DB, profileID int64, from, to time.Time) (map[string]int, error) {
	rows, err := db.Query(`
		SELECT day, count FROM daily_activity
		WHERE profile_id = ? AND day BETWEEN ? AND ?
	`, profileID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var day string
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			return nil, err
		}
		out[day] = count
	}
	return out, rows.Err()
}

// CurrentStreak returns the number of consecutive days up to and including
// asOf that profileID has completed at least one exercise. If asOf's own
// day has no activity yet, the streak isn't considered broken until that
// day ends -- it's computed as of the most recent day that does have
// activity, counting back from there.
func CurrentStreak(db *sql.DB, profileID int64, asOf time.Time) (int, error) {
	end := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, asOf.Location())
	start := end.AddDate(0, 0, -400)

	activity, err := ActivityRange(db, profileID, start, end)
	if err != nil {
		return 0, err
	}

	day := end
	if activity[day.Format("2006-01-02")] == 0 {
		day = day.AddDate(0, 0, -1)
	}

	streak := 0
	for activity[day.Format("2006-01-02")] > 0 {
		streak++
		day = day.AddDate(0, 0, -1)
	}
	return streak, nil
}

// Settings holds a profile's remembered menu choices, so they survive
// between exercises and sessions.
type Settings struct {
	Mode        string // "time" or "words"
	Duration    int    // seconds, used when Mode == "time"
	WordCount   int    // words, used when Mode == "words"
	Punctuation bool
	ZenMode     bool // disables HP loss/KO for a pure-practice experience
	FocusWeak   bool // biases word choice toward the player's weakest characters
	Quotes      bool // draws exercise text from cached quotes instead of random words
}

func defaultSettings() Settings {
	return Settings{Mode: "time", Duration: 30, WordCount: 100}
}

// LoadSettings returns profileID's saved settings, or sensible defaults if
// none have been saved yet.
func LoadSettings(db *sql.DB, profileID int64) (Settings, error) {
	row := db.QueryRow(`SELECT mode, duration, word_count, punctuation, zen_mode, focus_weak, quotes FROM settings WHERE profile_id = ?`, profileID)

	var s Settings
	var punctuation, zenMode, focusWeak, quotes int
	err := row.Scan(&s.Mode, &s.Duration, &s.WordCount, &punctuation, &zenMode, &focusWeak, &quotes)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	s.Punctuation = punctuation != 0
	s.ZenMode = zenMode != 0
	s.FocusWeak = focusWeak != 0
	s.Quotes = quotes != 0
	return s, nil
}

// SaveSettings persists profileID's menu choices, replacing any previous
// values.
func SaveSettings(db *sql.DB, profileID int64, s Settings) error {
	toInt := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	_, err := db.Exec(`
		INSERT INTO settings (profile_id, mode, duration, word_count, punctuation, zen_mode, focus_weak, quotes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id) DO UPDATE SET
			mode        = excluded.mode,
			duration    = excluded.duration,
			word_count  = excluded.word_count,
			punctuation = excluded.punctuation,
			zen_mode    = excluded.zen_mode,
			focus_weak  = excluded.focus_weak,
			quotes      = excluded.quotes
	`, profileID, s.Mode, s.Duration, s.WordCount, toInt(s.Punctuation), toInt(s.ZenMode), toInt(s.FocusWeak), toInt(s.Quotes))
	return err
}

// PersonalBest is a profile's best recorded WPM for one test configuration
// (mode + target).
type PersonalBest struct {
	Mode   string
	Target int
	// Source is "random" or "quotes" -- a quotes run and a random-words run
	// at the same mode/target are tracked as separate personal bests, since
	// typing fixed quote text is a meaningfully different challenge from
	// random words.
	Source     string
	WPM        float64
	Accuracy   float64
	AchievedAt time.Time
	// GhostPace is this best run's cumulative elapsed milliseconds at each
	// completed word, used to race a later attempt against it. Empty for
	// personal bests recorded before this was tracked.
	GhostPace []int
	// GhostWPMSeries is this best run's cumulative-WPM-per-word curve
	// (stats.Result.WPMSeries), used to overlay it on a later run's
	// results graph. Empty for personal bests recorded before this was
	// tracked.
	GhostWPMSeries []float64
}

// serializeGhostPace encodes a per-word elapsed-milliseconds sequence as
// comma-separated integers, the simplest form that survives a TEXT column
// round-trip without a JSON dependency.
func serializeGhostPace(ms []int) string {
	if len(ms) == 0 {
		return ""
	}
	parts := make([]string, len(ms))
	for i, v := range ms {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}

// parseGhostPace decodes serializeGhostPace's format, skipping any
// malformed entry rather than failing the whole read.
func parseGhostPace(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if v, err := strconv.Atoi(p); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// serializeFloats encodes a float64 sequence as comma-separated values,
// mirroring serializeGhostPace but for WPM series rather than millisecond
// offsets.
func serializeFloats(vs []float64) string {
	if len(vs) == 0 {
		return ""
	}
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strings.Join(parts, ",")
}

// parseFloats decodes serializeFloats's format, skipping any malformed
// entry rather than failing the whole read.
func parseFloats(s string) []float64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		if v, err := strconv.ParseFloat(p, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// GetPersonalBest returns profileID's best WPM for the given mode/target/
// source, or nil if none has been recorded yet.
func GetPersonalBest(db *sql.DB, profileID int64, mode string, target int, source string) (*PersonalBest, error) {
	row := db.QueryRow(`
		SELECT mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series
		FROM personal_bests WHERE profile_id = ? AND mode = ? AND target = ? AND source = ?
	`, profileID, mode, target, source)

	var pb PersonalBest
	var ghostPace, ghostWPMSeries string
	if err := row.Scan(&pb.Mode, &pb.Target, &pb.Source, &pb.WPM, &pb.Accuracy, &pb.AchievedAt, &ghostPace, &ghostWPMSeries); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	pb.GhostPace = parseGhostPace(ghostPace)
	pb.GhostWPMSeries = parseFloats(ghostWPMSeries)
	return &pb, nil
}

// RecordPersonalBest updates profileID's best WPM for mode/target/source if
// wpm beats the existing record (or none exists yet), returning whether it
// did. ghostPaceMS and ghostWPMSeries are the new run's per-word pacing and
// WPM curve, stored as the new ghost to race and compare against next time.
func RecordPersonalBest(db *sql.DB, profileID int64, mode string, target int, source string, wpm, accuracy float64, ghostPaceMS []int, ghostWPMSeries []float64, when time.Time) (bool, error) {
	existing, err := GetPersonalBest(db, profileID, mode, target, source)
	if err != nil {
		return false, err
	}
	if existing != nil && existing.WPM >= wpm {
		return false, nil
	}
	_, err = db.Exec(`
		INSERT INTO personal_bests (profile_id, mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, mode, target, source) DO UPDATE SET
			best_wpm         = excluded.best_wpm,
			accuracy         = excluded.accuracy,
			achieved_at      = excluded.achieved_at,
			ghost_pace       = excluded.ghost_pace,
			ghost_wpm_series = excluded.ghost_wpm_series
	`, profileID, mode, target, source, wpm, accuracy, when, serializeGhostPace(ghostPaceMS), serializeFloats(ghostWPMSeries))
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListPersonalBests returns every personal best profileID has recorded,
// ordered by mode, then source, then target.
func ListPersonalBests(db *sql.DB, profileID int64) ([]PersonalBest, error) {
	rows, err := db.Query(`
		SELECT mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series
		FROM personal_bests WHERE profile_id = ? ORDER BY mode, source, target
	`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PersonalBest
	for rows.Next() {
		var pb PersonalBest
		var ghostPace, ghostWPMSeries string
		if err := rows.Scan(&pb.Mode, &pb.Target, &pb.Source, &pb.WPM, &pb.Accuracy, &pb.AchievedAt, &ghostPace, &ghostWPMSeries); err != nil {
			return nil, err
		}
		pb.GhostPace = parseGhostPace(ghostPace)
		pb.GhostWPMSeries = parseFloats(ghostWPMSeries)
		out = append(out, pb)
	}
	return out, rows.Err()
}

// BestOverallWPM returns profileID's single highest personal best across
// every test configuration and source, or nil if none has been recorded
// yet.
func BestOverallWPM(db *sql.DB, profileID int64) (*PersonalBest, error) {
	row := db.QueryRow(`
		SELECT mode, target, source, best_wpm, accuracy, achieved_at, ghost_pace, ghost_wpm_series
		FROM personal_bests WHERE profile_id = ? ORDER BY best_wpm DESC LIMIT 1
	`, profileID)

	var pb PersonalBest
	var ghostPace, ghostWPMSeries string
	if err := row.Scan(&pb.Mode, &pb.Target, &pb.Source, &pb.WPM, &pb.Accuracy, &pb.AchievedAt, &ghostPace, &ghostWPMSeries); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	pb.GhostPace = parseGhostPace(ghostPace)
	pb.GhostWPMSeries = parseFloats(ghostWPMSeries)
	return &pb, nil
}

// RecordCharMistakes increments profileID's per-character mistake counters,
// used to bias later practice toward the player's weakest keys.
func RecordCharMistakes(db *sql.DB, profileID int64, counts map[rune]int) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO char_mistakes (profile_id, char, count) VALUES (?, ?, ?)
		ON CONFLICT(profile_id, char) DO UPDATE SET count = count + excluded.count
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for r, n := range counts {
		if _, err := stmt.Exec(profileID, string(r), n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// WeakCharWeights returns profileID's per-character mistake counts, for
// weighting word selection toward characters the player struggles with.
func WeakCharWeights(db *sql.DB, profileID int64) (map[rune]int, error) {
	rows, err := db.Query(`SELECT char, count FROM char_mistakes WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[rune]int)
	for rows.Next() {
		var ch string
		var count int
		if err := rows.Scan(&ch, &count); err != nil {
			return nil, err
		}
		if len(ch) > 0 {
			out[[]rune(ch)[0]] = count
		}
	}
	return out, rows.Err()
}

// TopWeakChars returns up to limit characters with the highest mistake
// counts, most-mistaken first.
func TopWeakChars(db *sql.DB, profileID int64, limit int) ([]rune, error) {
	rows, err := db.Query(`
		SELECT char FROM char_mistakes WHERE profile_id = ? ORDER BY count DESC LIMIT ?
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []rune
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return nil, err
		}
		if len(ch) > 0 {
			out = append(out, []rune(ch)[0])
		}
	}
	return out, rows.Err()
}

// RecordTestResult appends one completed exercise to profileID's history,
// used to chart WPM trend over time and to evaluate achievements.
func RecordTestResult(db *sql.DB, profileID int64, wpm, accuracy float64, mode string, target int, ko, punctuation, zenMode bool, source string, wordsTyped int, duration time.Duration, when time.Time) error {
	toInt := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	_, err := db.Exec(`
		INSERT INTO test_history (profile_id, wpm, accuracy, mode, target, ko, punctuation, zen_mode, source, words_typed, duration_seconds, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, profileID, wpm, accuracy, mode, target, toInt(ko), toInt(punctuation), toInt(zenMode), source, wordsTyped, int(duration.Seconds()), when)
	return err
}

// RecentWPMHistory returns profileID's last limit non-KO WPM values,
// ordered oldest to newest (suitable for a trend chart).
func RecentWPMHistory(db *sql.DB, profileID int64, limit int) ([]float64, error) {
	rows, err := db.Query(`
		SELECT wpm FROM (
			SELECT wpm, created_at FROM test_history
			WHERE profile_id = ? AND ko = 0
			ORDER BY created_at DESC LIMIT ?
		) ORDER BY created_at ASC
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []float64
	for rows.Next() {
		var wpm float64
		if err := rows.Scan(&wpm); err != nil {
			return nil, err
		}
		out = append(out, wpm)
	}
	return out, rows.Err()
}

// HasClaimedDailyChallenge reports whether profileID already claimed the
// daily challenge bonus for the given calendar day (challenge.Day format).
func HasClaimedDailyChallenge(db *sql.DB, profileID int64, day string) (bool, error) {
	return exists(db, `SELECT 1 FROM daily_challenge_claims WHERE profile_id = ? AND day = ?`, profileID, day)
}

// ClaimDailyChallenge records that profileID has claimed the daily
// challenge bonus for day, so it isn't awarded twice.
func ClaimDailyChallenge(db *sql.DB, profileID int64, day string, when time.Time) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO daily_challenge_claims (profile_id, day, claimed_at) VALUES (?, ?, ?)`, profileID, day, when)
	return err
}

// CachedQuote is one quote cached locally from the quotes API -- shared
// across all profiles rather than scoped to one, since the quote text
// itself has nothing to do with any particular player.
type CachedQuote struct {
	ID     int
	Text   string
	Author string
}

// CachedQuotes returns every quote currently cached.
func CachedQuotes(db *sql.DB) ([]CachedQuote, error) {
	rows, err := db.Query(`SELECT id, quote, author FROM quotes_cache`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CachedQuote
	for rows.Next() {
		var q CachedQuote
		if err := rows.Scan(&q.ID, &q.Text, &q.Author); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// StoreQuotes adds qs to the cache, skipping any whose id is already
// present (a later fetch can legitimately return quotes we've already
// cached from an earlier one).
func StoreQuotes(db *sql.DB, qs []CachedQuote) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO quotes_cache (id, quote, author) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, q := range qs {
		if _, err := stmt.Exec(q.ID, q.Text, q.Author); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PeriodSummary aggregates a profile's activity over a date range.
type PeriodSummary struct {
	Exercises    int
	AvgWPM       float64 // 0 if no non-KO exercises fall in the range
	BestDay      string  // "2006-01-02", "" if no activity in the range
	BestDayCount int
}

// Summary aggregates profileID's activity in [from, to).
func Summary(db *sql.DB, profileID int64, from, to time.Time) (PeriodSummary, error) {
	var s PeriodSummary
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM test_history WHERE profile_id = ? AND created_at >= ? AND created_at < ?`,
		profileID, from, to,
	).Scan(&s.Exercises); err != nil {
		return s, err
	}

	var avgWPM sql.NullFloat64
	if err := db.QueryRow(
		`SELECT AVG(wpm) FROM test_history WHERE profile_id = ? AND ko = 0 AND created_at >= ? AND created_at < ?`,
		profileID, from, to,
	).Scan(&avgWPM); err != nil {
		return s, err
	}
	if avgWPM.Valid {
		s.AvgWPM = avgWPM.Float64
	}

	row := db.QueryRow(
		`SELECT day, count FROM daily_activity WHERE profile_id = ? AND day >= ? AND day < ? ORDER BY count DESC LIMIT 1`,
		profileID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err := row.Scan(&s.BestDay, &s.BestDayCount); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	return s, nil
}

// TestHistoryEntry is one row from a profile's exercise log.
type TestHistoryEntry struct {
	WPM         float64
	Accuracy    float64
	Mode        string
	Target      int
	KO          bool
	Punctuation bool
	ZenMode     bool
	WordsTyped  int
	Duration    time.Duration
	CreatedAt   time.Time
}

// ListTestHistory returns profileID's most recent exercises, newest first.
func ListTestHistory(db *sql.DB, profileID int64, limit int) ([]TestHistoryEntry, error) {
	rows, err := db.Query(`
		SELECT wpm, accuracy, mode, target, ko, punctuation, zen_mode, words_typed, duration_seconds, created_at
		FROM test_history WHERE profile_id = ? ORDER BY created_at DESC, id DESC LIMIT ?
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestHistoryEntry
	for rows.Next() {
		var e TestHistoryEntry
		var koInt, punctInt, zenInt, durationSeconds int
		if err := rows.Scan(&e.WPM, &e.Accuracy, &e.Mode, &e.Target, &koInt, &punctInt, &zenInt, &e.WordsTyped, &durationSeconds, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.KO = koInt != 0
		e.Punctuation = punctInt != 0
		e.ZenMode = zenInt != 0
		e.Duration = time.Duration(durationSeconds) * time.Second
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecentAccuracyHistory returns profileID's last limit non-KO accuracy
// values, ordered oldest to newest (suitable for a trend chart).
func RecentAccuracyHistory(db *sql.DB, profileID int64, limit int) ([]float64, error) {
	rows, err := db.Query(`
		SELECT accuracy FROM (
			SELECT accuracy, created_at FROM test_history
			WHERE profile_id = ? AND ko = 0
			ORDER BY created_at DESC LIMIT ?
		) ORDER BY created_at ASC
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []float64
	for rows.Next() {
		var accuracy float64
		if err := rows.Scan(&accuracy); err != nil {
			return nil, err
		}
		out = append(out, accuracy)
	}
	return out, rows.Err()
}

// HasPerfectAccuracyRun reports whether profileID has ever completed
// (non-KO) an exercise with 100% accuracy.
func HasPerfectAccuracyRun(db *sql.DB, profileID int64) (bool, error) {
	return exists(db, `SELECT 1 FROM test_history WHERE profile_id = ? AND ko = 0 AND accuracy >= 100 LIMIT 1`, profileID)
}

// HasCompletedTestConfig reports whether profileID has ever completed
// (non-KO) an exercise with exactly this mode and target.
func HasCompletedTestConfig(db *sql.DB, profileID int64, mode string, target int) (bool, error) {
	return exists(db, `SELECT 1 FROM test_history WHERE profile_id = ? AND ko = 0 AND mode = ? AND target = ? LIMIT 1`,
		profileID, mode, target)
}

// HasTestInHourRange reports whether profileID has completed any exercise
// (KO or not -- this is about when you practice, not how well it went)
// whose local start hour falls in [startHour, endHour).
func HasTestInHourRange(db *sql.DB, profileID int64, startHour, endHour int) (bool, error) {
	rows, err := db.Query(`SELECT created_at FROM test_history WHERE profile_id = ?`, profileID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return false, err
		}
		if h := t.Hour(); h >= startHour && h < endHour {
			return true, nil
		}
	}
	return false, rows.Err()
}

// HasCompletedWordsAtLeast reports whether profileID has ever completed
// (non-KO) a words-mode exercise with at least this many words -- unlike
// HasCompletedTestConfig's exact match, this suits an open-ended milestone
// (e.g. "500 or more"), which the CLI's arbitrary --words target can satisfy
// with any value at or above the threshold.
func HasCompletedWordsAtLeast(db *sql.DB, profileID int64, minWords int) (bool, error) {
	return exists(db, `SELECT 1 FROM test_history WHERE profile_id = ? AND ko = 0 AND mode = 'words' AND target >= ? LIMIT 1`,
		profileID, minWords)
}

// HasPersonalBestInBothModes reports whether profileID has recorded a
// personal best in both time mode and words mode.
func HasPersonalBestInBothModes(db *sql.DB, profileID int64) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(DISTINCT mode) FROM personal_bests WHERE profile_id = ?`, profileID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count >= 2, nil
}

// HasPracticedAllWeekdays reports whether profileID has completed an
// exercise on all 7 days of the week (Sunday through Saturday), not
// necessarily in the same week or consecutively.
func HasPracticedAllWeekdays(db *sql.DB, profileID int64) (bool, error) {
	rows, err := db.Query(`SELECT created_at FROM test_history WHERE profile_id = ?`, profileID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	seen := make(map[time.Weekday]bool)
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return false, err
		}
		seen[t.Weekday()] = true
	}
	return len(seen) >= 7, rows.Err()
}

// HasDayWithAtLeast reports whether profileID completed at least n
// exercises on any single day.
func HasDayWithAtLeast(db *sql.DB, profileID int64, n int) (bool, error) {
	return exists(db, `SELECT 1 FROM daily_activity WHERE profile_id = ? AND count >= ? LIMIT 1`, profileID, n)
}

// PerfectRunCount returns how many non-KO exercises profileID has finished
// with 100% accuracy.
func PerfectRunCount(db *sql.DB, profileID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM test_history WHERE profile_id = ? AND ko = 0 AND accuracy >= 100`, profileID).Scan(&n)
	return n, err
}

// MaxPerfectAccuracyStreak returns the longest run of consecutive,
// back-to-back exercises profileID has completed with 100% accuracy and no
// knockout -- any other result (a mistake, or a KO) resets the streak.
func MaxPerfectAccuracyStreak(db *sql.DB, profileID int64) (int, error) {
	rows, err := db.Query(`SELECT ko, accuracy FROM test_history WHERE profile_id = ? ORDER BY id ASC`, profileID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var best, current int
	for rows.Next() {
		var koInt int
		var accuracy float64
		if err := rows.Scan(&koInt, &accuracy); err != nil {
			return 0, err
		}
		if koInt == 0 && accuracy >= 100 {
			current++
			if current > best {
				best = current
			}
		} else {
			current = 0
		}
	}
	return best, rows.Err()
}

// comebackStats scans profileID's history in the order it was recorded and
// reports how many times a knockout was immediately followed by another
// exercise that finished without a knockout, plus whether any of those
// follow-up exercises scored 100% accuracy.
func comebackStats(db *sql.DB, profileID int64) (count int, anyPerfect bool, err error) {
	rows, err := db.Query(`SELECT ko, accuracy FROM test_history WHERE profile_id = ? ORDER BY id ASC`, profileID)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()

	prevKO := false
	for rows.Next() {
		var koInt int
		var accuracy float64
		if err := rows.Scan(&koInt, &accuracy); err != nil {
			return 0, false, err
		}
		ko := koInt != 0
		if prevKO && !ko {
			count++
			if accuracy >= 100 {
				anyPerfect = true
			}
		}
		prevKO = ko
	}
	return count, anyPerfect, rows.Err()
}

// ComebackCount returns how many times profileID has completed an exercise
// right after being knocked out on the previous one.
func ComebackCount(db *sql.DB, profileID int64) (int, error) {
	count, _, err := comebackStats(db, profileID)
	return count, err
}

// HasPerfectComebackAfterKO reports whether profileID ever followed a
// knockout with a 100%-accuracy exercise on the very next attempt.
func HasPerfectComebackAfterKO(db *sql.DB, profileID int64) (bool, error) {
	_, perfect, err := comebackStats(db, profileID)
	return perfect, err
}

// PunctuationRunCount returns how many random-words exercises profileID has
// completed with punctuation enabled. Scoped to source = 'random' -- a
// quotes-mode run also has punctuation = 1 (it's forced on, see
// newTypingModel), but it already has its own dedicated achievement line
// (QuoteRunCount/"Bookworm"), so counting it here too would double-count
// the same run under two different achievements.
func PunctuationRunCount(db *sql.DB, profileID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM test_history WHERE profile_id = ? AND punctuation = 1 AND source = 'random'`, profileID).Scan(&n)
	return n, err
}

// ZenRunCount returns how many exercises profileID has completed in zen
// mode.
func ZenRunCount(db *sql.DB, profileID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM test_history WHERE profile_id = ? AND zen_mode = 1`, profileID).Scan(&n)
	return n, err
}

// QuoteRunCount returns how many exercises profileID has completed in
// quotes mode.
func QuoteRunCount(db *sql.DB, profileID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM test_history WHERE profile_id = ? AND source = 'quotes'`, profileID).Scan(&n)
	return n, err
}

// TotalPlayTime returns the lifetime sum of exercise durations completed by
// profileID, across every exercise including knockouts.
func TotalPlayTime(db *sql.DB, profileID int64) (time.Duration, error) {
	var totalSeconds int64
	err := db.QueryRow(`SELECT COALESCE(SUM(duration_seconds), 0) FROM test_history WHERE profile_id = ?`, profileID).Scan(&totalSeconds)
	if err != nil {
		return 0, err
	}
	return time.Duration(totalSeconds) * time.Second, nil
}

// TotalWordsTyped returns the lifetime sum of words completed across all of
// profileID's exercises, including knockouts (partial progress still
// counts as words typed).
func TotalWordsTyped(db *sql.DB, profileID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COALESCE(SUM(words_typed), 0) FROM test_history WHERE profile_id = ?`, profileID).Scan(&n)
	return n, err
}

// exists runs query and reports whether it returned any row.
func exists(db *sql.DB, query string, args ...any) (bool, error) {
	var one int
	err := db.QueryRow(query, args...).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}

// CreateProfile inserts a brand-new profile at level 1 with 0 XP.
func CreateProfile(db *sql.DB, name string) (*Profile, error) {
	res, err := db.Exec(`INSERT INTO profiles (name, level, xp) VALUES (?, 1, 0)`, name)
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := db.QueryRow(`SELECT id, name, level, xp, best_word_streak, ko_count, success_count, created_at, equipped_title FROM profiles WHERE id = ?`, id)
	return scanProfile(row)
}

// Save persists the profile's level, XP, best word streak, KO count, and
// success count.
// SetEquippedTitle sets the achievement ID profileID wants shown next to
// its name (empty clears it). Storage doesn't validate that the ID is a
// real, unlocked achievement -- that's the app layer's job, since checking
// it would need the achievements package, which storage deliberately
// doesn't depend on.
func SetEquippedTitle(db *sql.DB, profileID int64, title string) error {
	_, err := db.Exec(`UPDATE profiles SET equipped_title = ? WHERE id = ?`, title, profileID)
	return err
}

func Save(db *sql.DB, p *Profile) error {
	_, err := db.Exec(
		`UPDATE profiles SET level = ?, xp = ?, best_word_streak = ?, ko_count = ?, success_count = ? WHERE id = ?`,
		p.Level, p.XP, p.BestWordStreak, p.KOCount, p.SuccessCount, p.ID,
	)
	return err
}

func scanProfile(row *sql.Row) (*Profile, error) {
	var p Profile
	if err := row.Scan(&p.ID, &p.Name, &p.Level, &p.XP, &p.BestWordStreak, &p.KOCount, &p.SuccessCount, &p.CreatedAt, &p.EquippedTitle); err != nil {
		return nil, err
	}
	return &p, nil
}
