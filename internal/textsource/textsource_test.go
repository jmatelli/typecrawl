package textsource

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeyRoundTrips(t *testing.T) {
	for _, s := range []Source{
		{Kind: Random},
		{Kind: Quotes},
		{Kind: WordList, Path: "/home/me/french.txt"},
		{Kind: Text, Path: `C:\Users\me\code: notes.txt`},
	} {
		got, ok := Parse(s.Key())
		if !ok || got != s {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", s.Key(), got, ok, s)
		}
	}
	if (Source{}).Key() != "random" {
		t.Error("expected the zero source to be the random words")
	}
	for _, bad := range []string{"", "wordlist:", "text", "file:/a", "Random"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("expected Parse(%q) to fail", bad)
		}
	}
}

// A file's name is shown, never the directories leading to it.
func TestNameHidesTheDirectory(t *testing.T) {
	cases := map[Source]string{
		{Kind: WordList, Path: "/home/me/private/french.txt"}: "word list french.txt",
		{Kind: Text, Path: `C:\Users\me\book.txt`}:            "text book.txt",
		{Kind: Quotes}: "Quotes",
		{}:             "Random words",
	}
	for s, want := range cases {
		if got := s.Name(); got != want {
			t.Errorf("%+v.Name() = %q, want %q", s, got, want)
		}
	}
}

func TestResolveMakesThePathAbsolute(t *testing.T) {
	s, err := Resolve(WordList, "words.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(s.Path) || filepath.Base(s.Path) != "words.txt" {
		t.Fatalf("expected an absolute path to words.txt, got %q", s.Path)
	}
}

func TestLoadWordList(t *testing.T) {
	path := writeFile(t, "\uFEFFcafé\r\nnaïve  école\n\n  привет\n日本語\ncafé\n\u201cquoted\u201d\x07bell\n")
	got, err := Load(Source{Kind: WordList, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"café", "naïve", "école", "привет", "日本語", `"quoted"bell`}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// An accent typed as a letter plus a combining mark is stored as the one
// character a keyboard sends.
func TestLoadComposesAccents(t *testing.T) {
	path := writeFile(t, "cafe\u0301")
	got, err := Load(Source{Kind: WordList, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "café" {
		t.Fatalf("expected the composed form, got %q", got[0])
	}
}

func TestLoadTextKeepsEveryWordInOrder(t *testing.T) {
	path := writeFile(t, "the cat saw the\ndog")
	got, err := Load(Source{Kind: Text, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"the", "cat", "saw", "the", "dog"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoadRejectsUnusableFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(Source{Kind: WordList, Path: dir}); err == nil {
		t.Error("expected a directory to be refused")
	}
	if _, err := Load(Source{Kind: WordList, Path: filepath.Join(dir, "missing.txt")}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected a missing file to be reported as missing, got %v", err)
	}
	if _, err := Load(Source{Kind: Text, Path: writeFile(t, " \n\t\n")}); !errors.Is(err, ErrNoWords) {
		t.Errorf("expected a blank file to have no words, got %v", err)
	}
	if _, err := Load(Source{Kind: Text, Path: writeFile(t, "ok \xff\xfe")}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("expected invalid UTF-8 to be refused, got %v", err)
	}
	big := writeFile(t, strings.Repeat("a ", MaxFileSize/2+1))
	if _, err := Load(Source{Kind: WordList, Path: big}); err == nil {
		t.Error("expected a file over the size cap to be refused")
	}
	if _, err := Load(Source{Kind: Quotes}); err == nil {
		t.Error("expected a built-in source to have no file to load")
	}
}

func TestInOrderWrapsAndContinues(t *testing.T) {
	text := []string{"a", "b", "c"}
	if got := InOrder(text, 0, 2); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("got %q", got)
	}
	if got := InOrder(text, 2, 5); !slices.Equal(got, []string{"c", "a", "b", "c", "a"}) {
		t.Fatalf("expected the text to wrap around, got %q", got)
	}
	if InOrder(nil, 0, 3) != nil || InOrder(text, 0, 0) != nil {
		t.Fatal("expected nothing from an empty text or a zero count")
	}
}
