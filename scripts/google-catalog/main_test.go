package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/googlecatalog"
)

func TestGenerateFromPinnedSourcesIsDeterministic(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(repoRoot)

	gomodcache := os.Getenv("GOMODCACHE")
	if gomodcache == "" {
		gomodcache = filepath.Join(build.Default.GOPATH, "pkg", "mod")
	}

	moduleRoot := filepath.Join(gomodcache, "google.golang.org", "api@"+moduleVersion)
	if _, err := os.Stat(filepath.Join(moduleRoot, "api-list.json")); err != nil {
		t.Skipf("pinned module source is unavailable: %v", err)
	}

	output := filepath.Join(t.TempDir(), "manifest.json")
	if err := run([]string{"-module-root", moduleRoot, "-output", output}); err != nil {
		t.Fatal(err)
	}

	generated, readErr := os.ReadFile(output)
	if readErr != nil {
		t.Fatal(readErr)
	}

	checkedIn, readErr := os.ReadFile(filepath.Join(repoRoot, "internal", "googlecatalog", "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}

	if !json.Valid(generated) {
		t.Fatal("generated manifest is not valid JSON")
	}

	if sha256Hex(generated) != sha256Hex(checkedIn) {
		t.Fatal("checked-in manifest differs from deterministic generated manifest")
	}
}

func TestGeneratedCatalogUsesRawActions(t *testing.T) {
	manifest := googlecatalog.MustLoad()

	actions := make(map[string]bool, len(manifest.Methods))
	for _, method := range manifest.Methods {
		if !strings.HasPrefix(method.Action, method.Service+":") {
			t.Fatalf("action for %s = %q, want service-prefixed raw action", method.ID, method.Action)
		}

		if actions[method.Action] {
			t.Fatalf("duplicate action %q", method.Action)
		}
		actions[method.Action] = true
	}

	if !actions["gmail:gmail.users.messages.get"] {
		t.Fatal("raw Gmail get action is absent")
	}
}

func TestSafeReadPOSTMatchesPinnedCatalog(t *testing.T) {
	methods, err := googlecatalog.Methods()
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range methods {
		if method.HTTPMethod != "POST" {
			if safeReadPOST[method.ID] {
				t.Errorf("safe-read allowlist contains non-POST method %q", method.ID)
			}

			continue
		}

		if method.ReadOnly != safeReadPOST[method.ID] {
			t.Errorf("POST %s: read_only = %t, safe-read allowlist = %t", method.ID, method.ReadOnly, safeReadPOST[method.ID])
		}
	}
}

func TestKnownAmbiguousPOSTsRemainWrites(t *testing.T) {
	methods, err := googlecatalog.Methods()
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]bool{
		"bigquery.jobs.query": true,
		"tagmanager.accounts.containers.workspaces.quick_preview": true,
		"mybusinessverifications.verificationTokens.generate":     true,
	}

	seen := make(map[string]bool, len(expected))
	for methodID := range safeReadPOST {
		if expected[methodID] {
			t.Errorf("ambiguous method %q is unexpectedly in the safe-read allowlist", methodID)
		}
	}

	for _, method := range methods {
		if expected[method.ID] {
			seen[method.ID] = true
			if method.ReadOnly {
				t.Errorf("ambiguous POST %s is unexpectedly read-only", method.ID)
			}
		}
	}

	for methodID := range expected {
		if !seen[methodID] {
			t.Errorf("boundary fixture %q is absent from the pinned catalog", methodID)
		}
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
