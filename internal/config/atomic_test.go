package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestWriteFileAtomicConcurrentWritersLeaveOneCompleteDocument(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	const writers = 16

	docs := make([][]byte, writers)
	for i := range docs {
		docs[i] = bytes.Repeat([]byte(fmt.Sprintf("writer-%02d\n", i)), 4096)
	}

	var wg sync.WaitGroup

	errs := make(chan error, writers)

	for i := range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			errs <- WriteFileAtomic(path, docs[i])
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write failed: %v", err)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	matched := false

	for _, doc := range docs {
		if bytes.Equal(got, doc) {
			matched = true
		}
	}

	if !matched {
		t.Fatalf("destination is not one complete submitted document (%d bytes)", len(got))
	}

	assertOnlyEntries(t, dir, "config.json")
}

func TestWriteFileAtomicAcceptsLongDestinationNames(t *testing.T) {
	t.Parallel()

	// 251 bytes fits NAME_MAX (255) with the old fixed ".tmp" suffix.
	name := strings.Repeat("c", 246) + ".json"
	dir := t.TempDir()

	if err := WriteFileAtomic(filepath.Join(dir, name), []byte("{}\n")); err != nil {
		t.Fatalf("long destination name rejected: %v", err)
	}

	assertOnlyEntries(t, dir, name)
}

func TestWriteFileAtomicIsOwnerOnly(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}

	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := WriteFileAtomic(path, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}
}

func TestWriteFileAtomicRenameFailureKeepsDestinationAndRemovesStaging(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A non-empty directory at the destination makes the final rename fail.
	path := filepath.Join(dir, "config.json")
	if err := os.MkdirAll(filepath.Join(path, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("new\n")); err == nil {
		t.Fatal("expected rename failure")
	}

	if info, err := os.Stat(filepath.Join(path, "keep")); err != nil || !info.IsDir() {
		t.Fatalf("previous destination changed: %v", err)
	}

	assertOnlyEntries(t, dir, "config.json")
}

func TestWriteFileAtomicCreateFailureLeavesDestination(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing-dir", "config.json")
	if err := WriteFileAtomic(path, []byte("new\n")); err == nil {
		t.Fatal("expected staging creation failure")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("destination created despite failure: %v", err)
	}
}

func assertOnlyEntries(t *testing.T, dir string, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != len(want) {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}

		t.Fatalf("directory entries = %v, want %v", names, want)
	}

	for i, e := range entries {
		if e.Name() != want[i] {
			t.Fatalf("directory entry %q, want %q", e.Name(), want[i])
		}
	}
}
