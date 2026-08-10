package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestAuthServiceAccountSet_AndList_Text(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Service account configured") {
		t.Fatalf("unexpected output: %q", out)
	}

	storedPath, err := config.ServiceAccountPath("user@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("expected stored key at %q: %v", storedPath, err)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})
	if !strings.Contains(listOut, "user@example.com") || !strings.Contains(listOut, "service_account") {
		t.Fatalf("unexpected list output: %q", listOut)
	}
}

func TestAuthServiceAccountSet_PlainKeepsAdviceOffStdout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	const email = "user@example.com"
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com","client_id":"12345"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "auth", "service-account", "set", email, "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	storedPath, err := config.ServiceAccountPath(email)
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	wantStdout := "KEY\tVALUE\n" +
		"email\t" + email + "\n" +
		"path\t" + storedPath + "\n" +
		"client_email\tsvc@example.com\n" +
		"client_id\t12345\n"
	if stdout != wantStdout {
		t.Fatalf("plain stdout mismatch:\n got: %q\nwant: %q", stdout, wantStdout)
	}
	wantAdvice := "Service account configured. Use: gog <cmd> --account " + email
	if !strings.Contains(stderr, wantAdvice) {
		t.Fatalf("plain stderr missing usage advice %q: %q", wantAdvice, stderr)
	}
}

func TestAuthServiceAccountSet_JSONExcludesNarrativeAdvice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	const email = "user@example.com"
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com","client_id":"12345"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "service-account", "set", email, "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode JSON output %q: %v", stdout, err)
	}
	if got["stored"] != true || got["email"] != email || got["client_email"] != "svc@example.com" || got["client_id"] != "12345" {
		t.Fatalf("unexpected JSON result: %#v", got)
	}
	if strings.Contains(stdout, "Service account configured") || strings.Contains(stderr, "Service account configured") {
		t.Fatalf("JSON output included narrative advice: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestAuthStatus_ShowsServiceAccountPreferred(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "user@example.com", "auth", "status"}); err != nil {
				t.Fatalf("status: %v", err)
			}
		})
	})
	if !strings.Contains(out, "auth_preferred\tservice_account") {
		t.Fatalf("unexpected status output: %q", out)
	}
}
