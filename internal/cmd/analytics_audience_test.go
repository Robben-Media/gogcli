package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
)

// Test server for analytics audience exports commands
func analyticsAudienceTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Create audience export
		case strings.HasSuffix(r.URL.Path, "/audienceExports") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/create-audience-export-123",
				"done": false,
				"metadata": map[string]any{
					"@type": "type.googleapis.com/google.analytics.data.v1beta.AudienceListMetadata",
				},
			})
			return

		// Get audience export
		case strings.Contains(r.URL.Path, "/audienceExports/") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":query"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":                "properties/123456/audienceExports/789",
				"audience":            "properties/123456/audiences/456",
				"audienceDisplayName": "Purchasers",
				"state":               "ACTIVE",
				"rowCount":            1000,
				"beginCreatingTime":   "2025-01-01T00:00:00Z",
				"dimensions": []map[string]any{
					{"dimensionName": "deviceId"},
				},
			})
			return

		// List audience exports
		case strings.HasSuffix(r.URL.Path, "/audienceExports") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"audienceExports": []map[string]any{
					{
						"name":                "properties/123456/audienceExports/789",
						"audience":            "properties/123456/audiences/456",
						"audienceDisplayName": "Purchasers",
						"state":               "ACTIVE",
						"rowCount":            1000,
					},
					{
						"name":                "properties/123456/audienceExports/790",
						"audience":            "properties/123456/audiences/457",
						"audienceDisplayName": "Active Users",
						"state":               "CREATING",
						"rowCount":            0,
					},
				},
				"nextPageToken": "",
			})
			return

		// Query audience export
		case strings.Contains(r.URL.Path, ":query") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"audienceExport": map[string]any{
					"name":                "properties/123456/audienceExports/789",
					"audience":            "properties/123456/audiences/456",
					"audienceDisplayName": "Purchasers",
					"state":               "ACTIVE",
					"rowCount":            1000,
					"dimensions": []map[string]any{
						{"dimensionName": "deviceId"},
					},
				},
				"audienceRows": []map[string]any{
					{
						"dimensionValues": []map[string]any{
							{"value": "device123"},
						},
					},
					{
						"dimensionValues": []map[string]any{
							{"value": "device456"},
						},
					},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
}

func setupAnalyticsAudienceServices(t *testing.T, srv *httptest.Server) {
	t.Helper()

	origData := newAnalyticsDataService
	t.Cleanup(func() {
		newAnalyticsDataService = origData
	})

	dataSvc, err := analyticsdata.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (data): %v", err)
	}
	newAnalyticsDataService = func(context.Context, string) (*analyticsdata.Service, error) { return dataSvc, nil }
}

func TestExecute_AnalyticsAudienceExportsCreate_JSON(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "audience-exports", "create",
				"--property", "123456",
				"--audience", "properties/123456/audiences/456",
				"--dimensions", "deviceId",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Operation struct {
			Name string `json:"name"`
			Done bool   `json:"done"`
		} `json:"operation"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Operation.Name != "operations/create-audience-export-123" {
		t.Fatalf("unexpected operation name: %q", parsed.Operation.Name)
	}
	if parsed.Operation.Done {
		t.Fatalf("expected done=false for new operation")
	}
}

func TestExecute_AnalyticsAudienceExportsCreate_Text(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "audience-exports", "create",
				"--property", "123456",
				"--audience", "properties/123456/audiences/456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "OPERATION_NAME") {
		t.Fatalf("expected OPERATION_NAME in output: %q", out)
	}
}

func TestExecute_AnalyticsAudienceExportsCreate_MissingAudience(t *testing.T) {
	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"analytics", "audience-exports", "create",
		"--property", "123456",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestExecute_AnalyticsAudienceExportsGet_JSON(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "audience-exports", "get",
				"--name", "properties/123456/audienceExports/789",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Name                string `json:"name"`
		Audience            string `json:"audience"`
		AudienceDisplayName string `json:"audienceDisplayName"`
		State               string `json:"state"`
		RowCount            int64  `json:"rowCount"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Name != "properties/123456/audienceExports/789" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
	if parsed.State != "ACTIVE" {
		t.Fatalf("expected state=ACTIVE, got %q", parsed.State)
	}
	if parsed.RowCount != 1000 {
		t.Fatalf("expected rowCount=1000, got %d", parsed.RowCount)
	}
}

func TestExecute_AnalyticsAudienceExportsGet_Text(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "audience-exports", "get",
				"--name", "properties/123456/audienceExports/789",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "NAME") {
		t.Fatalf("expected NAME in output: %q", out)
	}
	if !strings.Contains(out, "ACTIVE") {
		t.Fatalf("expected ACTIVE state in output: %q", out)
	}
}

func TestExecute_AnalyticsAudienceExportsList_JSON(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "audience-exports", "list",
				"--property", "123456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		AudienceExports []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"audienceExports"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.AudienceExports) != 2 {
		t.Fatalf("expected 2 audience exports, got %d", len(parsed.AudienceExports))
	}
	if parsed.AudienceExports[0].State != "ACTIVE" {
		t.Fatalf("expected first export state=ACTIVE, got %q", parsed.AudienceExports[0].State)
	}
}

func TestExecute_AnalyticsAudienceExportsList_Text(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "audience-exports", "list",
				"--property", "123456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "NAME") {
		t.Fatalf("expected NAME header in output: %q", out)
	}
	if !strings.Contains(out, "AUDIENCE") {
		t.Fatalf("expected AUDIENCE header in output: %q", out)
	}
}

func TestExecute_AnalyticsAudienceExportsQuery_JSON(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "audience-exports", "query",
				"--name", "properties/123456/audienceExports/789",
				"--limit", "100",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		AudienceExport struct {
			Name string `json:"name"`
		} `json:"audienceExport"`
		AudienceRows []struct {
			DimensionValues []struct {
				Value string `json:"value"`
			} `json:"dimensionValues"`
		} `json:"audienceRows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.AudienceRows) != 2 {
		t.Fatalf("expected 2 audience rows, got %d", len(parsed.AudienceRows))
	}
	if parsed.AudienceRows[0].DimensionValues[0].Value != "device123" {
		t.Fatalf("unexpected first dimension value: %q", parsed.AudienceRows[0].DimensionValues[0].Value)
	}
}

func TestExecute_AnalyticsAudienceExportsQuery_Text(t *testing.T) {
	srv := analyticsAudienceTestServer(t)
	defer srv.Close()
	setupAnalyticsAudienceServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "audience-exports", "query",
				"--name", "properties/123456/audienceExports/789",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "deviceId") {
		t.Fatalf("expected deviceId header in output: %q", out)
	}
	if !strings.Contains(out, "device123") {
		t.Fatalf("expected device123 value in output: %q", out)
	}
}

func TestExecute_AnalyticsAudienceExportsQuery_MissingName(t *testing.T) {
	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"analytics", "audience-exports", "query",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}
