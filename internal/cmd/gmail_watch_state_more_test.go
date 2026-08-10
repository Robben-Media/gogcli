package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setGmailWatchTestConfigDir(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
}

func TestGmailWatchStatePath_CollisionFreeForNormalizedAccounts(t *testing.T) {
	setGmailWatchTestConfigDir(t)

	plusPath, err := gmailWatchStatePath(" User+Sales@Example.com ")
	if err != nil {
		t.Fatalf("plus account path: %v", err)
	}
	for _, other := range []string{
		"user_sales@example.com",
		"user!sales@example.com",
	} {
		otherPath, pathErr := gmailWatchStatePath(other)
		if pathErr != nil {
			t.Fatalf("account %q path: %v", other, pathErr)
		}
		if plusPath == otherPath {
			t.Fatalf("distinct normalized accounts %q and %q share path %q", "user+sales@example.com", other, plusPath)
		}
	}

	normalizedPath, err := gmailWatchStatePath("user+sales@example.com")
	if err != nil {
		t.Fatalf("normalized account path: %v", err)
	}
	if plusPath != normalizedPath {
		t.Fatalf("equivalent normalized accounts resolved to %q and %q", plusPath, normalizedPath)
	}
	nonASCIIPath, err := gmailWatchStatePath("üser@example.com")
	if err != nil {
		t.Fatalf("non-ASCII account path: %v", err)
	}
	legacyCollisionPath, err := gmailWatchStatePath("_ser@example.com")
	if err != nil {
		t.Fatalf("legacy collision account path: %v", err)
	}
	if nonASCIIPath == legacyCollisionPath {
		t.Fatalf("non-ASCII collision pair shares path %q", nonASCIIPath)
	}
	if filepath.Ext(plusPath) != ".json" {
		t.Fatalf("watch state path must remain a JSON file: %q", plusPath)
	}
}

func TestLoadGmailWatchStore_MigratesMatchingLegacyState(t *testing.T) {
	setGmailWatchTestConfigDir(t)

	newPath, pathErr := gmailWatchStatePath("user+sales@example.com")
	if pathErr != nil {
		t.Fatalf("new state path: %v", pathErr)
	}
	legacyPath := filepath.Join(filepath.Dir(newPath), "user_sales_example_com.json")
	payload := []byte("{\"account\":\" User+Sales@Example.COM \",\"topic\":\"projects/p/topics/t\",\"historyId\":\"123\"}\n")
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	store, err := loadGmailWatchStore("user+sales@example.com")
	if err != nil {
		t.Fatalf("load legacy state: %v", err)
	}
	if store.path != newPath {
		t.Fatalf("loaded store path = %q, want %q", store.path, newPath)
	}
	if store.Get().HistoryID != "123" {
		t.Fatalf("loaded history ID = %q, want 123", store.Get().HistoryID)
	}
	migrated, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read migrated state: %v", err)
	}
	if string(migrated) != string(payload) {
		t.Fatalf("migration changed state contents:\n got: %s\nwant: %s", migrated, payload)
	}
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state still exists after migration: %v", err)
	}
}

func TestLoadGmailWatchStore_RejectsUnparsableLegacyStateWithoutMutation(t *testing.T) {
	setGmailWatchTestConfigDir(t)

	newPath, pathErr := gmailWatchStatePath("user+sales@example.com")
	if pathErr != nil {
		t.Fatalf("new state path: %v", pathErr)
	}
	legacyPath := filepath.Join(filepath.Dir(newPath), legacySanitizeAccountForPath("user+sales@example.com")+".json")
	payload := []byte("not json\n")
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	if _, err := loadGmailWatchStore("user+sales@example.com"); err == nil {
		t.Fatal("unparsable legacy state was accepted")
	}
	if got, err := os.ReadFile(legacyPath); err != nil || string(got) != string(payload) {
		t.Fatalf("unparsable legacy state changed: data=%q err=%v", got, err)
	}
	if _, err := os.Stat(newPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unparsable legacy state was migrated: %v", err)
	}
}

func TestLoadGmailWatchStore_LegacyCleanupFailureKeepsPublishedStateUsable(t *testing.T) {
	setGmailWatchTestConfigDir(t)

	newPath, pathErr := gmailWatchStatePath("user+sales@example.com")
	if pathErr != nil {
		t.Fatalf("new state path: %v", pathErr)
	}
	legacyPath := filepath.Join(filepath.Dir(newPath), legacySanitizeAccountForPath("user+sales@example.com")+".json")
	payload := []byte("{\"account\":\"user+sales@example.com\",\"historyId\":\"123\"}\n")
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	origRemove := watchRemove
	t.Cleanup(func() { watchRemove = origRemove })
	cleanupErr := errors.New("remove legacy state")
	watchRemove = func(path string) error {
		if path == legacyPath {
			return cleanupErr
		}
		return os.Remove(path)
	}

	store, err := loadGmailWatchStore("user+sales@example.com")
	if err != nil {
		t.Fatalf("load matching legacy state: %v", err)
	}
	if store.Get().HistoryID != "123" {
		t.Fatalf("loaded history ID = %q, want 123", store.Get().HistoryID)
	}
	if got, err := os.ReadFile(newPath); err != nil || string(got) != string(payload) {
		t.Fatalf("published state unusable: data=%q err=%v", got, err)
	}
}

func TestLoadGmailWatchStore_PublishFailureLeavesLegacyStateIntact(t *testing.T) {
	setGmailWatchTestConfigDir(t)

	newPath, pathErr := gmailWatchStatePath("user+sales@example.com")
	if pathErr != nil {
		t.Fatalf("new state path: %v", pathErr)
	}
	legacyPath := filepath.Join(filepath.Dir(newPath), legacySanitizeAccountForPath("user+sales@example.com")+".json")
	payload := []byte("{\"account\":\"user+sales@example.com\",\"historyId\":\"123\"}\n")
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	origLink := watchLink
	t.Cleanup(func() { watchLink = origLink })
	publishErr := errors.New("publish migrated state")
	watchLink = func(_, _ string) error { return publishErr }

	if _, err := loadGmailWatchStore("user+sales@example.com"); !errors.Is(err, publishErr) {
		t.Fatalf("load error = %v, want %v", err, publishErr)
	}
	if got, err := os.ReadFile(legacyPath); err != nil || string(got) != string(payload) {
		t.Fatalf("legacy state changed: data=%q err=%v", got, err)
	}
	if _, err := os.Stat(newPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new state published after failure: %v", err)
	}
}

func TestLoadGmailWatchStore_RejectsCollidingLegacyStateForAnotherAccount(t *testing.T) {
	setGmailWatchTestConfigDir(t)

	requestedPath, pathErr := gmailWatchStatePath("user+sales@example.com")
	if pathErr != nil {
		t.Fatalf("requested state path: %v", pathErr)
	}
	legacyPath := filepath.Join(filepath.Dir(requestedPath), "user_sales_example_com.json")
	payload := []byte("{\"account\":\"user_sales@example.com\",\"historyId\":\"456\"}\n")
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatalf("write colliding legacy state: %v", err)
	}

	if _, err := loadGmailWatchStore("user+sales@example.com"); err == nil {
		t.Fatal("colliding legacy state was accepted for another account")
	}
	if _, err := os.Stat(requestedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("colliding legacy state was migrated to requested path: %v", err)
	}
	if got, err := os.ReadFile(legacyPath); err != nil || string(got) != string(payload) {
		t.Fatalf("colliding legacy state changed: data=%q err=%v", got, err)
	}
}

func TestIsStaleHistoryID(t *testing.T) {
	stale, err := isStaleHistoryID("5", "4")
	if err != nil {
		t.Fatalf("isStaleHistoryID: %v", err)
	}
	if !stale {
		t.Fatalf("expected stale for older history id")
	}

	stale, err = isStaleHistoryID("5", "6")
	if err != nil {
		t.Fatalf("isStaleHistoryID: %v", err)
	}
	if stale {
		t.Fatalf("expected non-stale for newer history id")
	}

	stale, err = isStaleHistoryID("", "")
	if err != nil {
		t.Fatalf("isStaleHistoryID empty: %v", err)
	}
	if stale {
		t.Fatalf("expected non-stale for empty ids")
	}

	if _, err := isStaleHistoryID("bad", "5"); err == nil {
		t.Fatalf("expected error for invalid history id")
	}
}
