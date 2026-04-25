package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func analyticsAdminStreamsTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// List data streams
		case strings.HasSuffix(r.URL.Path, "/dataStreams") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dataStreams": []map[string]any{
					{
						"name":        "properties/123/dataStreams/456",
						"type":        "WEB_DATA_STREAM",
						"displayName": "Healthcare LP",
						"webStreamData": map[string]any{
							"defaultUri":    "https://healthcare.itallyllc.com",
							"measurementId": "G-ABC123",
						},
						"createTime": "2026-01-01T00:00:00Z",
						"updateTime": "2026-01-01T00:00:00Z",
					},
				},
				"nextPageToken": "",
			})
			return

		// Create data stream
		case strings.HasSuffix(r.URL.Path, "/dataStreams") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/789",
				"type":        "WEB_DATA_STREAM",
				"displayName": "Healthcare LP",
				"webStreamData": map[string]any{
					"defaultUri":    "https://healthcare.itallyllc.com",
					"measurementId": "G-NEW123",
				},
				"createTime": "2026-03-02T00:00:00Z",
				"updateTime": "2026-03-02T00:00:00Z",
			})
			return

		// Get data stream
		case strings.Contains(r.URL.Path, "/dataStreams/") &&
			!strings.Contains(r.URL.Path, "/measurementProtocolSecrets") &&
			r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456",
				"type":        "WEB_DATA_STREAM",
				"displayName": "Healthcare LP",
				"webStreamData": map[string]any{
					"defaultUri":    "https://healthcare.itallyllc.com",
					"measurementId": "G-ABC123",
				},
				"createTime": "2026-01-01T00:00:00Z",
				"updateTime": "2026-01-01T00:00:00Z",
			})
			return

		// Delete data stream
		case strings.Contains(r.URL.Path, "/dataStreams/") &&
			!strings.Contains(r.URL.Path, "/measurementProtocolSecrets") &&
			r.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return

		// Patch data stream
		case strings.Contains(r.URL.Path, "/dataStreams/") &&
			!strings.Contains(r.URL.Path, "/measurementProtocolSecrets") &&
			r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456",
				"type":        "WEB_DATA_STREAM",
				"displayName": "Updated Name",
				"createTime":  "2026-01-01T00:00:00Z",
				"updateTime":  "2026-03-02T00:00:00Z",
			})
			return

		// List MP secrets
		case strings.HasSuffix(r.URL.Path, "/measurementProtocolSecrets") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"measurementProtocolSecrets": []map[string]any{
					{
						"name":        "properties/123/dataStreams/456/measurementProtocolSecrets/789",
						"displayName": "API Secret",
						"secretValue": "secret_abc123",
					},
				},
				"nextPageToken": "",
			})
			return

		// Create MP secret
		case strings.HasSuffix(r.URL.Path, "/measurementProtocolSecrets") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456/measurementProtocolSecrets/999",
				"displayName": "New Secret",
				"secretValue": "secret_new123",
			})
			return

		// Get MP secret
		case strings.Contains(r.URL.Path, "/measurementProtocolSecrets/") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456/measurementProtocolSecrets/789",
				"displayName": "API Secret",
				"secretValue": "secret_abc123",
			})
			return

		// Delete MP secret
		case strings.Contains(r.URL.Path, "/measurementProtocolSecrets/") && r.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return

		// Patch MP secret
		case strings.Contains(r.URL.Path, "/measurementProtocolSecrets/") && r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456/measurementProtocolSecrets/789",
				"displayName": "Updated Secret",
				"secretValue": "secret_abc123",
			})
			return
		}

		http.NotFound(w, r)
	}))
}

// --- Data Streams tests ---

func TestExecute_AADataStreamsList_JSON(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "data-streams", "list",
				"--property", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DataStreams []struct {
			Name          string `json:"name"`
			Type          string `json:"type"`
			DisplayName   string `json:"displayName"`
			WebStreamData struct {
				MeasurementId string `json:"measurementId"`
			} `json:"webStreamData"`
		} `json:"dataStreams"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.DataStreams) != 1 {
		t.Fatalf("expected 1 data stream, got %d", len(parsed.DataStreams))
	}
	if parsed.DataStreams[0].Name != "properties/123/dataStreams/456" {
		t.Fatalf("unexpected name: %q", parsed.DataStreams[0].Name)
	}
	if parsed.DataStreams[0].WebStreamData.MeasurementId != "G-ABC123" {
		t.Fatalf("unexpected measurement ID: %q", parsed.DataStreams[0].WebStreamData.MeasurementId)
	}
}

func TestExecute_AADataStreamsCreate_JSON(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "data-streams", "create",
				"--property", "123",
				"--type", "WEB_DATA_STREAM",
				"--display-name", "Healthcare LP",
				"--web-default-uri", "https://healthcare.itallyllc.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DataStream struct {
			Name          string `json:"name"`
			DisplayName   string `json:"displayName"`
			WebStreamData struct {
				MeasurementId string `json:"measurementId"`
			} `json:"webStreamData"`
		} `json:"dataStream"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.DataStream.Name != "properties/123/dataStreams/789" {
		t.Fatalf("unexpected name: %q", parsed.DataStream.Name)
	}
	if parsed.DataStream.WebStreamData.MeasurementId != "G-NEW123" {
		t.Fatalf("unexpected measurement ID: %q", parsed.DataStream.WebStreamData.MeasurementId)
	}
}

func TestExecute_AADataStreamsGet_JSON(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "data-streams", "get",
				"--property", "123",
				"456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DataStream struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"dataStream"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.DataStream.Name != "properties/123/dataStreams/456" {
		t.Fatalf("unexpected name: %q", parsed.DataStream.Name)
	}
}

func TestExecute_AADataStreamsDelete_WithoutForce(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	err := Execute([]string{
		"--json", "--account", "a@b.com", "--no-input",
		"analytics", "admin", "data-streams", "delete",
		"--property", "123",
		"456",
	})
	if err == nil {
		t.Fatal("expected error when deleting without --force")
	}
	if !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected 'without --force' in error, got: %v", err)
	}
}

func TestExecute_AADataStreamsDelete_WithForce_JSON(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com", "--force",
				"analytics", "admin", "data-streams", "delete",
				"--property", "123",
				"456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Deleted bool   `json:"deleted"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Deleted {
		t.Fatal("expected deleted=true")
	}
}

func TestExecute_AADataStreamsPatch_UpdateMask(t *testing.T) {
	var gotUpdateMask string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/dataStreams/") {
			gotUpdateMask = r.URL.Query().Get("updateMask")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456",
				"displayName": "New Name",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "data-streams", "patch",
				"--property", "123",
				"--display-name", "New Name",
				"456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotUpdateMask, "displayName") {
		t.Fatalf("expected updateMask to contain 'displayName', got %q", gotUpdateMask)
	}

	var parsed struct {
		DataStream struct {
			Name string `json:"name"`
		} `json:"dataStream"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
}

// --- MP Secrets tests ---

func TestExecute_AAMpSecretsList_JSON(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "mp-secrets", "list",
				"--property", "123",
				"--stream", "456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Secrets []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			SecretValue string `json:"secretValue"`
		} `json:"measurementProtocolSecrets"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(parsed.Secrets))
	}
	if parsed.Secrets[0].SecretValue != "secret_abc123" {
		t.Fatalf("unexpected secret value: %q", parsed.Secrets[0].SecretValue)
	}
}

func TestExecute_AAMpSecretsCreate_JSON(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "mp-secrets", "create",
				"--property", "123",
				"--stream", "456",
				"--display-name", "New Secret",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Secret struct {
			Name        string `json:"name"`
			SecretValue string `json:"secretValue"`
		} `json:"measurementProtocolSecret"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Secret.SecretValue != "secret_new123" {
		t.Fatalf("unexpected secret value: %q", parsed.Secret.SecretValue)
	}
}

func TestExecute_AAMpSecretsDelete_WithoutForce(t *testing.T) {
	srv := analyticsAdminStreamsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	err := Execute([]string{
		"--json", "--account", "a@b.com", "--no-input",
		"analytics", "admin", "mp-secrets", "delete",
		"--property", "123",
		"--stream", "456",
		"789",
	})
	if err == nil {
		t.Fatal("expected error when deleting without --force")
	}
	if !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected 'without --force' in error, got: %v", err)
	}
}

func TestExecute_AAMpSecretsPatch_UpdateMask(t *testing.T) {
	var gotUpdateMask string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/measurementProtocolSecrets/") {
			gotUpdateMask = r.URL.Query().Get("updateMask")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "properties/123/dataStreams/456/measurementProtocolSecrets/789",
				"displayName": "Updated Secret",
				"secretValue": "secret_abc123",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "admin", "mp-secrets", "patch",
				"--property", "123",
				"--stream", "456",
				"--display-name", "Updated Secret",
				"789",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotUpdateMask, "displayName") {
		t.Fatalf("expected updateMask to contain 'displayName', got %q", gotUpdateMask)
	}

	var parsed struct {
		Secret struct {
			Name string `json:"name"`
		} `json:"measurementProtocolSecret"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
}
