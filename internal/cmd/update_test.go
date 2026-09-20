package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/selfupdate"
)

func TestExecute_UpdateCheckPlainTSV(t *testing.T) {
	originalClient := newSelfUpdateClient
	originalVersion := version
	t.Cleanup(func() {
		newSelfUpdateClient = originalClient
		version = originalVersion
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(selfupdate.Release{TagName: "v9.9.9"})
	}))
	t.Cleanup(server.Close)

	newSelfUpdateClient = func() *selfupdate.Client {
		return &selfupdate.Client{BaseURL: server.URL, HTTP: server.Client()}
	}
	version = "1.2.3"

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "update", "--check"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	want := "CURRENT\tLATEST\tUPDATE\tASSET\n1.2.3\t9.9.9\ttrue\t" + selfupdate.AssetNameFor("v9.9.9") + "\n"
	if out != want {
		t.Fatalf("plain update check output = %q, want %q", out, want)
	}
}

func TestExecute_SkillsUpdateWritesInstructionsToStderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dirtySkill := filepath.Join(home, ".agents", "skills", "google-calendar")
	if err := os.MkdirAll(dirtySkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirtySkill, "SKILL.md"), []byte("local edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := Execute([]string{"skills", "update"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.Contains(stdout, "run:") || strings.Contains(stdout, "Tell the user") {
		t.Fatalf("stdout contains instructional text: %q", stdout)
	}
	if !strings.Contains(stderr, "run: gog skills install") {
		t.Fatalf("stderr missing install instruction: %q", stderr)
	}
	if !strings.Contains(stderr, "Tell the user which skills were skipped") {
		t.Fatalf("stderr missing dirty-skill instruction: %q", stderr)
	}
}
