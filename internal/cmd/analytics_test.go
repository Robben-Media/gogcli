package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
)

func analyticsTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Admin API: list account summaries (must come before accounts to avoid prefix match)
		case strings.Contains(r.URL.Path, "/v1beta/accountSummaries") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accountSummaries": []map[string]any{
					{
						"name":        "accountSummaries/123",
						"account":     "accounts/123",
						"displayName": "Test Account",
						"propertySummaries": []map[string]any{
							{
								"property":    "properties/456",
								"displayName": "Test Property",
							},
						},
					},
				},
				"nextPageToken": "",
			})
			return

		// Admin API: list accounts
		case strings.Contains(r.URL.Path, "/v1beta/accounts") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": []map[string]any{
					{
						"name":        "accounts/123",
						"displayName": "Test Account",
						"createTime":  "2025-01-01T00:00:00Z",
						"updateTime":  "2025-06-01T00:00:00Z",
					},
				},
				"nextPageToken": "",
			})
			return

		// Data API: run report
		case strings.HasSuffix(r.URL.Path, ":runReport") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dimensionHeaders": []map[string]any{
					{"name": "date"},
				},
				"metricHeaders": []map[string]any{
					{"name": "sessions", "type": "TYPE_INTEGER"},
				},
				"rows": []map[string]any{
					{
						"dimensionValues": []map[string]any{{"value": "20260101"}},
						"metricValues":    []map[string]any{{"value": "42"}},
					},
					{
						"dimensionValues": []map[string]any{{"value": "20260102"}},
						"metricValues":    []map[string]any{{"value": "55"}},
					},
				},
				"rowCount": 2,
			})
			return

		// Data API: run realtime report
		case strings.HasSuffix(r.URL.Path, ":runRealtimeReport") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metricHeaders": []map[string]any{
					{"name": "activeUsers", "type": "TYPE_INTEGER"},
				},
				"rows": []map[string]any{
					{
						"metricValues": []map[string]any{{"value": "7"}},
					},
				},
				"rowCount": 1,
			})
			return

		// Data API: get metadata
		case strings.Contains(r.URL.Path, "/metadata") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dimensions": []map[string]any{
					{"apiName": "date", "uiName": "Date", "description": "The date of the session"},
					{"apiName": "country", "uiName": "Country", "description": "Country of the user"},
				},
				"metrics": []map[string]any{
					{"apiName": "sessions", "uiName": "Sessions", "description": "Number of sessions"},
					{"apiName": "activeUsers", "uiName": "Active users", "description": "Number of active users"},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
}

func setupAnalyticsServices(t *testing.T, srv *httptest.Server) {
	t.Helper()

	origData := newAnalyticsDataService
	origAdmin := newAnalyticsAdminService
	t.Cleanup(func() {
		newAnalyticsDataService = origData
		newAnalyticsAdminService = origAdmin
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

	adminSvc, err := analyticsadmin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (admin): %v", err)
	}
	newAnalyticsAdminService = func(context.Context, string) (*analyticsadmin.Service, error) { return adminSvc, nil }
}

func TestExecute_AnalyticsAccounts_JSON(t *testing.T) {
	srv := analyticsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "analytics", "accounts"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Accounts []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"accounts"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(parsed.Accounts))
	}
	if parsed.Accounts[0].Name != "accounts/123" {
		t.Fatalf("unexpected account name: %q", parsed.Accounts[0].Name)
	}
	if parsed.Accounts[0].DisplayName != "Test Account" {
		t.Fatalf("unexpected display name: %q", parsed.Accounts[0].DisplayName)
	}
}

func TestExecute_AnalyticsProperties_JSON(t *testing.T) {
	srv := analyticsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "analytics", "properties"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		AccountSummaries []struct {
			Account     string `json:"account"`
			DisplayName string `json:"displayName"`
		} `json:"accountSummaries"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.AccountSummaries) != 1 {
		t.Fatalf("expected 1 account summary, got %d", len(parsed.AccountSummaries))
	}
	if parsed.AccountSummaries[0].Account != "accounts/123" {
		t.Fatalf("unexpected account: %q", parsed.AccountSummaries[0].Account)
	}
}

func TestExecute_AnalyticsReport_JSON(t *testing.T) {
	srv := analyticsTestServer()
	defer srv.Close()
	setupAnalyticsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "report",
				"--property", "456",
				"--metrics", "sessions",
				"--dimensions", "date",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DimensionHeaders []struct {
			Name string `json:"name"`
		} `json:"dimensionHeaders"`
		MetricHeaders []struct {
			Name string `json:"name"`
		} `json:"metricHeaders"`
		Rows []struct {
			DimensionValues []struct {
				Value string `json:"value"`
			} `json:"dimensionValues"`
			MetricValues []struct {
				Value string `json:"value"`
			} `json:"metricValues"`
		} `json:"rows"`
		RowCount int `json:"rowCount"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.RowCount != 2 {
		t.Fatalf("expected rowCount 2, got %d", parsed.RowCount)
	}
	if len(parsed.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(parsed.Rows))
	}
	if parsed.Rows[0].DimensionValues[0].Value != "20260101" {
		t.Fatalf("unexpected first dimension value: %q", parsed.Rows[0].DimensionValues[0].Value)
	}
	if parsed.Rows[0].MetricValues[0].Value != "42" {
		t.Fatalf("unexpected first metric value: %q", parsed.Rows[0].MetricValues[0].Value)
	}
	if len(parsed.DimensionHeaders) != 1 || parsed.DimensionHeaders[0].Name != "date" {
		t.Fatalf("unexpected dimension headers: %#v", parsed.DimensionHeaders)
	}
	if len(parsed.MetricHeaders) != 1 || parsed.MetricHeaders[0].Name != "sessions" {
		t.Fatalf("unexpected metric headers: %#v", parsed.MetricHeaders)
	}
}

func TestExecute_AnalyticsReport_MissingProperty(t *testing.T) {
	err := Execute([]string{"--json", "--account", "a@b.com", "analytics", "report", "--metrics", "sessions"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}
