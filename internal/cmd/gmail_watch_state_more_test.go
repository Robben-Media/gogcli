package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGmailWatchStatePath_CollisionFreeForNormalizedAccounts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	plusPath, err := gmailWatchStatePath(" User+Sales@Example.com ")
	if err != nil {
		t.Fatalf("plus account path: %v", err)
	}
	underscorePath, err := gmailWatchStatePath("user_sales@example.com")
	if err != nil {
		t.Fatalf("underscore account path: %v", err)
	}
	if plusPath == underscorePath {
		t.Fatalf("distinct normalized accounts share path %q", plusPath)
	}

	normalizedPath, err := gmailWatchStatePath("user+sales@example.com")
	if err != nil {
		t.Fatalf("normalized account path: %v", err)
	}
	if plusPath != normalizedPath {
		t.Fatalf("equivalent normalized accounts resolved to %q and %q", plusPath, normalizedPath)
	}
	if filepath.Ext(plusPath) != ".json" {
		t.Fatalf("watch state path must remain a JSON file: %q", plusPath)
	}
}

func TestLoadGmailWatchStore_MigratesMatchingLegacyState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	newPath, err := gmailWatchStatePath("user+sales@example.com")
	if err != nil {
		t.Fatalf("new state path: %v", err)
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

func TestLoadGmailWatchStore_RejectsCollidingLegacyStateForAnotherAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	requestedPath, err := gmailWatchStatePath("user+sales@example.com")
	if err != nil {
		t.Fatalf("requested state path: %v", err)
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
