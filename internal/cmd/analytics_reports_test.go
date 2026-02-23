package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/option"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
)

// Test server for analytics reports commands
func analyticsReportsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Pivot report
		case strings.HasSuffix(r.URL.Path, ":runPivotReport") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pivotHeaders": []map[string]any{
					{
						"pivotDimensionHeaders": []map[string]any{
							{
								"dimensionValues": []map[string]any{
									{"value": "Chrome"},
									{"value": "Firefox"},
								},
							},
						},
					},
				},
				"dimensionHeaders": []map[string]any{
					{"name": "country"},
				},
				"metricHeaders": []map[string]any{
					{"name": "sessions", "type": "TYPE_INTEGER"},
				},
				"rows": []map[string]any{
					{
						"dimensionValues": []map[string]any{{"value": "US"}},
						"metricValues":    []map[string]any{{"value": "100"}},
					},
				},
				"rowCount": 1,
			})
			return

		// Batch reports
		case strings.HasSuffix(r.URL.Path, ":batchRunReports") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"reports": []map[string]any{
					{
						"dimensionHeaders": []map[string]any{{"name": "date"}},
						"metricHeaders":    []map[string]any{{"name": "sessions", "type": "TYPE_INTEGER"}},
						"rows": []map[string]any{
							{
								"dimensionValues": []map[string]any{{"value": "20260101"}},
								"metricValues":    []map[string]any{{"value": "42"}},
							},
						},
						"rowCount": 1,
					},
					{
						"dimensionHeaders": []map[string]any{{"name": "country"}},
						"metricHeaders":    []map[string]any{{"name": "users", "type": "TYPE_INTEGER"}},
						"rows": []map[string]any{
							{
								"dimensionValues": []map[string]any{{"value": "US"}},
								"metricValues":    []map[string]any{{"value": "100"}},
							},
						},
						"rowCount": 1,
					},
				},
			})
			return

		// Batch pivot reports
		case strings.HasSuffix(r.URL.Path, ":batchRunPivotReports") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pivotReports": []map[string]any{
					{
						"dimensionHeaders": []map[string]any{{"name": "country"}},
						"metricHeaders":    []map[string]any{{"name": "sessions", "type": "TYPE_INTEGER"}},
						"rows": []map[string]any{
							{
								"dimensionValues": []map[string]any{{"value": "US"}},
								"metricValues":    []map[string]any{{"value": "100"}},
							},
						},
					},
				},
			})
			return

		// Check compatibility
		case strings.HasSuffix(r.URL.Path, ":checkCompatibility") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dimensionCompatibilities": []map[string]any{
					{
						"dimensionMetadata": map[string]any{"apiName": "date"},
						"compatibility":     "COMPATIBLE",
					},
					{
						"dimensionMetadata": map[string]any{"apiName": "country"},
						"compatibility":     "COMPATIBLE",
					},
				},
				"metricCompatibilities": []map[string]any{
					{
						"metricMetadata": map[string]any{"apiName": "sessions"},
						"compatibility":  "COMPATIBLE",
					},
					{
						"metricMetadata": map[string]any{"apiName": "activeUsers"},
						"compatibility":  "INCOMPATIBLE",
					},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
}

func setupAnalyticsReportsServices(t *testing.T, srv *httptest.Server) {
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

func TestExecute_AnalyticsPivotReport_JSON(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	// Note: limit must be a string because the API uses ,string struct tag
	pivotsJSON := `[{"fieldNames": ["browser"], "limit": "10"}]`

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "pivot-report",
				"--property", "456",
				"--dimensions", "country",
				"--metrics", "sessions",
				"--pivots-json", pivotsJSON,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		PivotHeaders []any `json:"pivotHeaders"`
		Rows         []any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(parsed.Rows))
	}
}

func TestExecute_AnalyticsPivotReport_Text(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	// Note: limit must be a string because the API uses ,string struct tag
	pivotsJSON := `[{"fieldNames": ["browser"], "limit": "10"}]`

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "pivot-report",
				"--property", "456",
				"--dimensions", "country",
				"--metrics", "sessions",
				"--pivots-json", pivotsJSON,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "country") {
		t.Fatalf("expected country header in output: %q", out)
	}
}

func TestExecute_AnalyticsPivotReport_MissingPivotsJSON(t *testing.T) {
	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"analytics", "pivot-report",
		"--property", "456",
		"--dimensions", "country",
		"--metrics", "sessions",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestExecute_AnalyticsBatchReports_JSON(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	requestsJSON := `[{"metrics": [{"name": "sessions"}], "dimensions": [{"name": "date"}], "dateRanges": [{"startDate": "28daysAgo", "endDate": "today"}]}]`

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "batch-reports",
				"--property", "456",
				"--requests-json", requestsJSON,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Reports []struct {
			RowCount int `json:"rowCount"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(parsed.Reports))
	}
}

func TestExecute_AnalyticsBatchReports_Text(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	requestsJSON := `[{"metrics": [{"name": "sessions"}]}]`

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "batch-reports",
				"--property", "456",
				"--requests-json", requestsJSON,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Report 1:") {
		t.Fatalf("expected 'Report 1:' in output: %q", out)
	}
}

func TestExecute_AnalyticsBatchReports_TooManyRequests(t *testing.T) {
	// Create 6 requests (limit is 5)
	requestsJSON := `[{"metrics": [{"name": "sessions"}]}, {"metrics": [{"name": "sessions"}]}, {"metrics": [{"name": "sessions"}]}, {"metrics": [{"name": "sessions"}]}, {"metrics": [{"name": "sessions"}]}, {"metrics": [{"name": "sessions"}]}]`

	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"analytics", "batch-reports",
		"--property", "456",
		"--requests-json", requestsJSON,
	})
	if err == nil {
		t.Fatalf("expected error for too many requests")
	}
}

func TestExecute_AnalyticsBatchPivotReports_JSON(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	// Note: limit must be a string because the API uses ,string struct tag
	requestsJSON := `[{"dimensions": [{"name": "country"}], "metrics": [{"name": "sessions"}], "pivots": [{"fieldNames": ["browser"], "limit": "10"}], "dateRanges": [{"startDate": "28daysAgo", "endDate": "today"}]}]`

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "batch-pivot-reports",
				"--property", "456",
				"--requests-json", requestsJSON,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		PivotReports []struct {
			DimensionHeaders []any `json:"dimensionHeaders"`
		} `json:"pivotReports"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.PivotReports) != 1 {
		t.Fatalf("expected 1 pivot report, got %d", len(parsed.PivotReports))
	}
}

func TestExecute_AnalyticsCheckCompatibility_JSON(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"analytics", "check-compatibility",
				"--property", "456",
				"--dimensions", "date,country",
				"--metrics", "sessions,activeUsers",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DimensionCompatibilities []struct {
			DimensionMetadata struct {
				ApiName string `json:"apiName"`
			} `json:"dimensionMetadata"`
			Compatibility string `json:"compatibility"`
		} `json:"dimensionCompatibilities"`
		MetricCompatibilities []struct {
			MetricMetadata struct {
				ApiName string `json:"apiName"`
			} `json:"metricMetadata"`
			Compatibility string `json:"compatibility"`
		} `json:"metricCompatibilities"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.DimensionCompatibilities) != 2 {
		t.Fatalf("expected 2 dimension compatibilities, got %d", len(parsed.DimensionCompatibilities))
	}
	if len(parsed.MetricCompatibilities) != 2 {
		t.Fatalf("expected 2 metric compatibilities, got %d", len(parsed.MetricCompatibilities))
	}
}

func TestExecute_AnalyticsCheckCompatibility_Text(t *testing.T) {
	srv := analyticsReportsTestServer(t)
	defer srv.Close()
	setupAnalyticsReportsServices(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"analytics", "check-compatibility",
				"--property", "456",
				"--dimensions", "date",
				"--metrics", "sessions",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "DIMENSION") {
		t.Fatalf("expected DIMENSION header in output: %q", out)
	}
	if !strings.Contains(out, "METRIC") {
		t.Fatalf("expected METRIC header in output: %q", out)
	}
	if !strings.Contains(out, "COMPATIBLE") {
		t.Fatalf("expected COMPATIBLE in output: %q", out)
	}
}

func TestExecute_AnalyticsCheckCompatibility_MissingDimensionsAndMetrics(t *testing.T) {
	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"analytics", "check-compatibility",
		"--property", "456",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestReadJSONFromFlag_Direct(t *testing.T) {
	result, err := readJSONFromFlag(`{"test": "value"}`, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"test": "value"}` {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestReadJSONFromFlag_File(t *testing.T) {
	// Create a temp file with JSON content
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.json")
	if err := os.WriteFile(tmpFile, []byte(`{"test": "file"}`), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	result, err := readJSONFromFlag("@"+tmpFile, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"test": "file"}` {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestReadJSONFromFlag_Empty(t *testing.T) {
	_, err := readJSONFromFlag("", "test")
	if err == nil {
		t.Fatalf("expected error for empty value")
	}
}
