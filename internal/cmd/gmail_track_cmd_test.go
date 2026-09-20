package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/tracking"
)

const testTrackingWorkerName = "gog-email-tracker-a-b-com"

func setupTrackingEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")
}

func configureTracking(t *testing.T) {
	t.Helper()
	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("setup Execute: %v", err)
			}
		})
	})
}

func TestGmailTrackSetupAndStatus(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "Next steps") {
			t.Fatalf("expected next steps in stderr: %q", errOut)
		}
	})
	if !strings.Contains(out, "configured\ttrue") {
		t.Fatalf("unexpected setup output: %q", out)
	}

	statusOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(statusOut, "configured\ttrue") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
}

func TestGmailTrackSetup_JSON(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("setup output is not valid JSON: %v\n%s", err, out)
	}
	configPath, err := tracking.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := map[string]any{
		"configured":      true,
		"account":         "a@b.com",
		"configPath":      configPath,
		"workerURL":       "https://example.com",
		"workerName":      testTrackingWorkerName,
		"databaseName":    testTrackingWorkerName,
		"databaseID":      "",
		"adminConfigured": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("setup JSON = %#v, want %#v", got, want)
	}
}

func TestGmailTrackSetup_Plain(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	configPath, err := tracking.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := "KEY\tVALUE\n" +
		"configured\ttrue\n" +
		"account\ta@b.com\n" +
		"configPath\t" + configPath + "\n" +
		"workerURL\thttps://example.com\n" +
		"workerName\t" + testTrackingWorkerName + "\n" +
		"databaseName\t" + testTrackingWorkerName + "\n" +
		"databaseID\t\n" +
		"adminConfigured\ttrue\n"
	if out != want {
		t.Fatalf("setup plain output = %q, want %q", out, want)
	}
}

func TestGmailTrackStatus_JSON(t *testing.T) {
	setupTrackingEnv(t)
	configureTracking(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("status Execute: %v", err)
			}
		})
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("status output is not valid JSON: %v\n%s", err, out)
	}
	configPath, err := tracking.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := map[string]any{
		"configured":      true,
		"account":         "a@b.com",
		"configPath":      configPath,
		"workerURL":       "https://example.com",
		"workerName":      testTrackingWorkerName,
		"databaseName":    testTrackingWorkerName,
		"databaseID":      "",
		"adminConfigured": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status JSON = %#v, want %#v", got, want)
	}
}

func TestGmailTrackStatus_Plain(t *testing.T) {
	setupTrackingEnv(t)
	configureTracking(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("status Execute: %v", err)
			}
		})
	})
	configPath, err := tracking.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := "KEY\tVALUE\n" +
		"configured\ttrue\n" +
		"account\ta@b.com\n" +
		"configPath\t" + configPath + "\n" +
		"workerURL\thttps://example.com\n" +
		"workerName\t" + testTrackingWorkerName + "\n" +
		"databaseName\t" + testTrackingWorkerName + "\n" +
		"databaseID\t\n" +
		"adminConfigured\ttrue\n"
	if out != want {
		t.Fatalf("status plain output = %q, want %q", out, want)
	}
}

func TestGmailTrackStatus_JSONNotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("status output is not valid JSON: %v\n%s", err, out)
	}
	configPath, err := tracking.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := map[string]any{
		"configured":      false,
		"account":         "a@b.com",
		"configPath":      configPath,
		"workerURL":       "",
		"workerName":      "",
		"databaseName":    "",
		"databaseID":      "",
		"adminConfigured": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unconfigured status JSON = %#v, want %#v", got, want)
	}
}

func TestGmailTrackStatus_PlainNotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	configPath, err := tracking.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := "KEY\tVALUE\n" +
		"configured\tfalse\n" +
		"account\ta@b.com\n" +
		"configPath\t" + configPath + "\n" +
		"workerURL\t\n" +
		"workerName\t\n" +
		"databaseName\t\n" +
		"databaseID\t\n" +
		"adminConfigured\tfalse\n"
	if out != want {
		t.Fatalf("unconfigured status plain output = %q, want %q", out, want)
	}
}

func TestGmailTrackStatus_NotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "configured\tfalse") {
		t.Fatalf("unexpected status output: %q", out)
	}
}

func TestGmailTrackOpens(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/q/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tracking_id": "tid",
				"recipient":   "user@example.com",
				"sent_at":     "2025-01-01T00:00:00Z",
				"total_opens": 2,
				"human_opens": 1,
				"first_human_open": map[string]any{
					"at": "2025-01-01T02:00:00Z",
					"location": map[string]any{
						"city":    "SF",
						"region":  "CA",
						"country": "US",
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/opens"):
			if r.Header.Get("Authorization") != "Bearer adminkey" {
				t.Fatalf("unexpected auth: %q", r.Header.Get("Authorization"))
			}
			if r.URL.Query().Get("recipient") != "user@example.com" {
				t.Fatalf("unexpected recipient: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{
					{
						"tracking_id":  "tid",
						"recipient":    "user@example.com",
						"subject_hash": "hash",
						"sent_at":      "2025-01-01T00:00:00Z",
						"opened_at":    "2025-01-01T01:00:00Z",
						"is_bot":       false,
						"location":     map[string]any{"city": "SF", "region": "CA", "country": "US"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "tid"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "tracking_id\ttid") {
		t.Fatalf("unexpected tracking id output: %q", out)
	}
	if !strings.Contains(out, "first_human_open\t2025-01-01T02:00:00Z") {
		t.Fatalf("unexpected first open output: %q", out)
	}
	if !strings.Contains(out, "first_human_open_location\tSF, CA") {
		t.Fatalf("unexpected first open location output: %q", out)
	}

	adminOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com", "--since", "2025-01-01"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(adminOut, "tid\tuser@example.com") {
		t.Fatalf("unexpected admin output: %q", adminOut)
	}

	if _, err := parseTrackingSince("not-a-date"); err == nil {
		t.Fatalf("expected parseTrackingSince error")
	}
}

func TestGmailTrackOpens_JSON(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/q/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tracking_id": "tid",
				"recipient":   "user@example.com",
				"sent_at":     "2025-01-01T00:00:00Z",
				"total_opens": 2,
				"human_opens": 1,
			})
			return
		case strings.Contains(r.URL.Path, "/opens"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{
					{
						"tracking_id":  "tid",
						"recipient":    "user@example.com",
						"subject_hash": "hash",
						"sent_at":      "2025-01-01T00:00:00Z",
						"opened_at":    "2025-01-01T01:00:00Z",
						"is_bot":       false,
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	trackOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "track", "opens", "tid"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(trackOut, "\"tracking_id\"") {
		t.Fatalf("unexpected track json output: %q", trackOut)
	}

	adminOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(adminOut, "\"opens\"") {
		t.Fatalf("unexpected admin json output: %q", adminOut)
	}

	if parsed, err := parseTrackingSince("24h"); err != nil || parsed == "" {
		t.Fatalf("unexpected parseTrackingSince duration result: %q err=%v", parsed, err)
	}
	if parsed, err := parseTrackingSince("2025-01-01"); err != nil || parsed == "" {
		t.Fatalf("unexpected parseTrackingSince date result: %q err=%v", parsed, err)
	}
}

func TestGmailTrackOpens_AdminEmpty(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/opens") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "opens\t0") {
		t.Fatalf("unexpected empty admin output: %q", out)
	}
}

func TestGmailTrackOpens_NotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	cfg := &tracking.Config{Enabled: false}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens"}); err == nil {
		t.Fatalf("expected error for unconfigured tracking")
	}
}
