//go:build unix

package textsource

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A pipe or a device is refused at once rather than read: /dev/zero never
// ends, and a pipe would block the game until something writes to it.
func TestLoadRefusesSpecialFiles(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fifo, "/dev/zero"} {
		done := make(chan error, 1)
		go func() {
			_, err := Load(Source{Kind: Text, Path: path})
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s: expected it refused", path)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: Load blocked instead of refusing it", path)
		}
	}
}

func TestResolveFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "words.txt")
	if err := os.WriteFile(real, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	a, _ := Resolve(WordList, real)
	b, _ := Resolve(WordList, link)
	if a.Key() != b.Key() {
		t.Fatalf("expected one key for a file and a link to it, got %q and %q", a.Key(), b.Key())
	}
}
