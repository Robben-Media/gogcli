package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	errTestWatchWrite   = errors.New("injected watch write failure")
	errTestWatchClose   = errors.New("injected watch close failure")
	errTestWatchReplace = errors.New("injected watch replace failure")
)

type failingWatchTempFile struct {
	watchTempFile
	writeErr error
	closeErr error
}

func (f *failingWatchTempFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.watchTempFile.Write(p)
}

func (f *failingWatchTempFile) Close() error {
	if err := f.watchTempFile.Close(); err != nil {
		return err
	}
	return f.closeErr
}

func restoreWatchFS(t *testing.T) {
	t.Helper()
	origCreate := watchCreateTemp
	origReplace := watchReplace
	origRemove := watchRemove
	t.Cleanup(func() {
		watchCreateTemp = origCreate
		watchReplace = origReplace
		watchRemove = origRemove
	})
}

func seedWatchStateFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"account":"old@example.com","historyId":"1"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed state: %v", err)
	}
}

func listWatchTemps(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var temps []string
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, "gmail-watch-") && strings.HasSuffix(name, ".tmp") {
			temps = append(temps, name)
		}
	}
	return temps
}

func TestGmailWatchStore_Save_AtomicOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acct.json")
	seedWatchStateFile(t, path)

	store := &gmailWatchStore{
		path: path,
		state: gmailWatchState{
			Account:   "new@example.com",
			HistoryID: "42",
		},
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %04o, want 0600", info.Mode().Perm())
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), `"historyId": "42"`) {
		t.Fatalf("unexpected content: %s", got)
	}
	if temps := listWatchTemps(t, dir); len(temps) != 0 {
		t.Fatalf("leftover temps: %v", temps)
	}
}

func TestGmailWatchStore_Save_WriteFailurePreservesPriorAndCleansTemp(t *testing.T) {
	restoreWatchFS(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "acct.json")
	prior := `{"account":"old@example.com","historyId":"1"}` + "\n"
	seedWatchStateFile(t, path)

	watchCreateTemp = func(d, pattern string) (watchTempFile, error) {
		f, err := os.CreateTemp(d, pattern)
		if err != nil {
			return nil, err
		}
		return &failingWatchTempFile{watchTempFile: f, writeErr: errTestWatchWrite}, nil
	}

	store := &gmailWatchStore{
		path:  path,
		state: gmailWatchState{Account: "new@example.com", HistoryID: "99"},
	}
	err := store.Save()
	if err == nil || !errors.Is(err, errTestWatchWrite) {
		t.Fatalf("Save error = %v, want %v", err, errTestWatchWrite)
	}
	if !strings.Contains(err.Error(), "write") {
		t.Fatalf("expected actionable write error, got %v", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read prior: %v", readErr)
	}
	if string(got) != prior {
		t.Fatalf("prior file changed: %q", got)
	}
	if temps := listWatchTemps(t, dir); len(temps) != 0 {
		t.Fatalf("leftover temps: %v", temps)
	}
}

func TestGmailWatchStore_Save_CloseFailurePreservesPriorAndCleansTemp(t *testing.T) {
	restoreWatchFS(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "acct.json")
	prior := `{"account":"old@example.com","historyId":"1"}` + "\n"
	seedWatchStateFile(t, path)

	watchCreateTemp = func(d, pattern string) (watchTempFile, error) {
		f, err := os.CreateTemp(d, pattern)
		if err != nil {
			return nil, err
		}
		return &failingWatchTempFile{watchTempFile: f, closeErr: errTestWatchClose}, nil
	}

	store := &gmailWatchStore{
		path:  path,
		state: gmailWatchState{Account: "new@example.com", HistoryID: "99"},
	}
	err := store.Save()
	if err == nil || !errors.Is(err, errTestWatchClose) {
		t.Fatalf("Save error = %v, want %v", err, errTestWatchClose)
	}
	if !strings.Contains(err.Error(), "close") {
		t.Fatalf("expected actionable close error, got %v", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read prior: %v", readErr)
	}
	if string(got) != prior {
		t.Fatalf("prior file changed: %q", got)
	}
	if temps := listWatchTemps(t, dir); len(temps) != 0 {
		t.Fatalf("leftover temps: %v", temps)
	}
}

func TestGmailWatchStore_Save_ReplaceFailurePreservesPriorAndCleansTemp(t *testing.T) {
	restoreWatchFS(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "acct.json")
	prior := `{"account":"old@example.com","historyId":"1"}` + "\n"
	seedWatchStateFile(t, path)

	watchReplace = func(_, _ string) error { return errTestWatchReplace }

	store := &gmailWatchStore{
		path:  path,
		state: gmailWatchState{Account: "new@example.com", HistoryID: "99"},
	}
	err := store.Save()
	if err == nil || !errors.Is(err, errTestWatchReplace) {
		t.Fatalf("Save error = %v, want %v", err, errTestWatchReplace)
	}
	if !strings.Contains(err.Error(), "replace") {
		t.Fatalf("expected actionable replace error, got %v", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read prior: %v", readErr)
	}
	if string(got) != prior {
		t.Fatalf("prior file changed: %q", got)
	}
	if temps := listWatchTemps(t, dir); len(temps) != 0 {
		t.Fatalf("leftover temps: %v", temps)
	}
}

func TestGmailWatchStore_Update_MemoryUnchangedOnPersistFailure(t *testing.T) {
	restoreWatchFS(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "acct.json")
	prior := `{"account":"old@example.com","historyId":"1"}` + "\n"
	seedWatchStateFile(t, path)

	watchReplace = func(_, _ string) error { return errTestWatchReplace }

	store := &gmailWatchStore{
		path: path,
		state: gmailWatchState{
			Account:   "old@example.com",
			HistoryID: "1",
		},
	}
	err := store.Update(func(s *gmailWatchState) error {
		s.Account = "new@example.com"
		s.HistoryID = "99"
		return nil
	})
	if err == nil || !errors.Is(err, errTestWatchReplace) {
		t.Fatalf("Update error = %v, want %v", err, errTestWatchReplace)
	}

	got := store.Get()
	if got.Account != "old@example.com" || got.HistoryID != "1" {
		t.Fatalf("in-memory state changed on failed persist: %+v", got)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read prior: %v", readErr)
	}
	if string(data) != prior {
		t.Fatalf("prior file changed: %q", data)
	}
}

func TestGmailWatchStore_Update_ReferenceFieldsUnchangedOnPersistFailure(t *testing.T) {
	restoreWatchFS(t)
	path := filepath.Join(t.TempDir(), "acct.json")
	seedWatchStateFile(t, path)
	watchReplace = func(_, _ string) error { return errTestWatchReplace }

	store := &gmailWatchStore{
		path: path,
		state: gmailWatchState{
			Labels: []string{"INBOX"},
			Hook:   &gmailWatchHook{URL: "https://old.example"},
		},
	}
	err := store.Update(func(s *gmailWatchState) error {
		s.Labels[0] = "TRASH"
		s.Hook.URL = "https://new.example"
		return nil
	})
	if err == nil || !errors.Is(err, errTestWatchReplace) {
		t.Fatalf("Update error = %v, want %v", err, errTestWatchReplace)
	}

	got := store.Get()
	if len(got.Labels) != 1 || got.Labels[0] != "INBOX" {
		t.Fatalf("labels changed on failed persist: %v", got.Labels)
	}
	if got.Hook == nil || got.Hook.URL != "https://old.example" {
		t.Fatalf("hook changed on failed persist: %+v", got.Hook)
	}
}

func TestGmailWatchStore_StartHistoryID_ReturnsSaveError(t *testing.T) {
	restoreWatchFS(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "acct.json")

	watchReplace = func(_, _ string) error { return errTestWatchReplace }

	store := &gmailWatchStore{path: path}
	id, err := store.StartHistoryID("123")
	if err == nil || !errors.Is(err, errTestWatchReplace) {
		t.Fatalf("StartHistoryID error = %v, want %v", err, errTestWatchReplace)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0 on save failure", id)
	}
	got := store.Get()
	if got.HistoryID != "" {
		t.Fatalf("in-memory history advanced despite save failure: %+v", got)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no state file after failed initial save, stat=%v", statErr)
	}
	if temps := listWatchTemps(t, dir); len(temps) != 0 {
		t.Fatalf("leftover temps: %v", temps)
	}
}
