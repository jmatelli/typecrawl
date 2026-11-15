// Package textsource names where an exercise's text comes from: the
// built-in random words, cached quotes, or a file the player supplied --
// a word list drawn from at random, or a text typed in order. It also
// loads those files.
package textsource

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmatelli/typecrawl/internal/quotes"
)

// Kind is the kind of source a run's text comes from.
type Kind string

const (
	Random   Kind = "random"   // the built-in word tiers
	Quotes   Kind = "quotes"   // cached quotations
	WordList Kind = "wordlist" // a player's file of words, drawn at random
	Text     Kind = "text"     // a player's text file, typed in order
)

// Source is one source of exercise text. The zero value is Random.
type Source struct {
	Kind Kind
	Path string // the file, for WordList and Text; absolute once saved
}

// Builtin is the source for a run without a custom file: quotes or the
// random words.
func Builtin(quotes bool) Source {
	if quotes {
		return Source{Kind: Quotes}
	}
	return Source{Kind: Random}
}

// Custom reports whether s is a player's own file.
func (s Source) Custom() bool {
	return s.Kind == WordList || s.Kind == Text
}

// Key is how s is stored: "random", "quotes", or the kind and the file's
// path, as in "wordlist:/home/me/french.txt". Personal bests and history
// are kept per key, so each file has its own records.
func (s Source) Key() string {
	switch s.Kind {
	case WordList, Text:
		return string(s.Kind) + ":" + s.Path
	case Quotes:
		return string(Quotes)
	default:
		return string(Random)
	}
}

// Parse reads a key written by Key. ok is false for anything else.
func Parse(key string) (s Source, ok bool) {
	switch key {
	case string(Random):
		return Source{Kind: Random}, true
	case string(Quotes):
		return Source{Kind: Quotes}, true
	}
	kind, path, found := strings.Cut(key, ":")
	if !found || path == "" || (kind != string(WordList) && kind != string(Text)) {
		return Source{}, false
	}
	return Source{Kind: Kind(kind), Path: path}, true
}

// FileName is a custom source's file name, without the directories
// leading to it, or "" for a built-in source. It's what's shown of a
// file anywhere it might be shared: the directories can say more than a
// player means to share.
func (s Source) FileName() string {
	if !s.Custom() {
		return ""
	}
	// Split on either separator, so a path saved on Windows still shows
	// only its file name elsewhere.
	return s.Path[strings.LastIndexAny(s.Path, `/\`)+1:]
}

// Name is how s is shown to the player, with a file shown by its
// FileName.
func (s Source) Name() string {
	switch s.Kind {
	case WordList:
		return "word list " + s.FileName()
	case Text:
		return "text " + s.FileName()
	case Quotes:
		return "Quotes"
	default:
		return "Random words"
	}
}

// Resolve makes a source from a command-line path absolute, so the same
// file gets the same key wherever the game is started from.
// A symlink is followed, so a file reached by two paths keeps one set of
// records. A path with control characters is refused: it would be shown
// in the menu and the shared summary.
func Resolve(kind Kind, path string) (Source, error) {
	if strings.ContainsFunc(path, unicode.IsControl) {
		return Source{}, errors.New("the path contains control characters")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Source{}, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return Source{Kind: kind, Path: abs}, nil
}

// MaxFileSize caps how large a file can be loaded, so a wrong path (a
// video, a disk image) fails fast instead of filling memory.
const MaxFileSize = 4 << 20

var errTooLarge = fmt.Errorf("larger than %d MB", MaxFileSize>>20)

// ErrNoWords is returned for a file with nothing to type in it.
var ErrNoWords = errors.New("no words to type")

// Load reads s's file and returns its words, ready to type: in composed
// form, with typographic punctuation swapped for what a keyboard types
// (see quotes.Typeable) and invisible characters removed. Words are
// separated by any whitespace, including line breaks. A word list keeps
// each distinct word once, so repeating a line doesn't skew the draw; a
// text keeps every word, in order.
//
// An error says what's wrong without naming the file, so each caller can
// name it the way it shows files.
func Load(s Source) ([]string, error) {
	if !s.Custom() {
		return nil, fmt.Errorf("%s is not a file source", s.Kind)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		return nil, unwrapPath(err)
	}
	if info.IsDir() {
		return nil, errors.New("it's a directory")
	}
	// Only a regular file: a pipe or a device such as /dev/zero has no
	// size to check, could never end, or would be empty on the next read.
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if info.Size() > MaxFileSize {
		return nil, errTooLarge
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, unwrapPath(err)
	}
	defer f.Close()
	// Capped again while reading, in case the file grew since the check.
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, unwrapPath(err)
	}
	if len(data) > MaxFileSize {
		return nil, errTooLarge
	}
	return parse(data, s.Kind == WordList)
}

// unwrapPath drops the path an os error carries, keeping the reason
// (which errors.Is still recognizes, such as fs.ErrNotExist).
func unwrapPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

func parse(data []byte, distinct bool) ([]string, error) {
	data = bytes.TrimPrefix(data, []byte("\uFEFF"))
	if !utf8.Valid(data) {
		return nil, errors.New("not UTF-8 text")
	}
	text := strings.Map(func(r rune) rune {
		// The zero-width joiner and non-joiner are kept: Persian and some
		// Indic keyboards type them, and they hold emoji sequences together.
		if unicode.IsSpace(r) || unicode.IsGraphic(r) || r == '\u200C' || r == '\u200D' {
			return r
		}
		return -1 // control and other invisible characters can't be typed
	}, quotes.Typeable(string(data)))

	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(text) {
		if distinct {
			if seen[w] {
				continue
			}
			seen[w] = true
		}
		out = append(out, w)
	}
	if len(out) == 0 {
		return nil, ErrNoWords
	}
	return out, nil
}

// InOrder returns n words of text starting at word offset, going back to
// the start when the text runs out. Asking again with offset set to how
// many words were already taken carries on where the last call stopped.
func InOrder(text []string, offset, n int) []string {
	if len(text) == 0 || n <= 0 {
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = text[(offset+i)%len(text)]
	}
	return out
}
