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

const (
	unsafeAnalyticsDimensionHeader = "dimension\tname"
	unsafeAnalyticsMetricHeader    = "metric\nname"
	unsafeAnalyticsDimensionValue  = "dimension\rvalue"
	unsafeAnalyticsMetricValue     = "metric\r\nvalue"
	analyticsSafeTSV               = "dimension name\tmetric name\ndimension value\tmetric value\n"
	analyticsBatchSafeTSV          = "REPORT_INDEX\tROW_INDEX\tFIELD_KIND\tFIELD_NAME\tFIELD_VALUE\n" +
		"1\t1\tDIMENSION\tdimension name\tdimension value\n" +
		"1\t1\tMETRIC\tmetric name\tmetric value\n" +
		"3\t1\tDIMENSION\tdevice\tMobile\n" +
		"3\t1\tMETRIC\tviews\t7\n" +
		"3\t2\tDIMENSION\tdevice\tDesktop\n" +
		"3\t2\tMETRIC\tviews\t9\n"
)

func analyticsUnsafeReport() map[string]any {
	return map[string]any{
		"dimensionHeaders": []map[string]any{{"name": unsafeAnalyticsDimensionHeader}},
		"metricHeaders":    []map[string]any{{"name": unsafeAnalyticsMetricHeader, "type": "TYPE_INTEGER"}},
		"rows": []map[string]any{{
			"dimensionValues": []map[string]any{{"value": unsafeAnalyticsDimensionValue}},
			"metricValues":    []map[string]any{{"value": unsafeAnalyticsMetricValue}},
		}},
		"rowCount": 1,
	}
}

func analyticsBatchLongFormReports() []map[string]any {
	return []map[string]any{
		analyticsUnsafeReport(),
		{
			"dimensionHeaders": []map[string]any{{"name": "city"}},
			"metricHeaders":    []map[string]any{{"name": "users", "type": "TYPE_INTEGER"}},
			"rows":             []map[string]any{},
			"rowCount":         0,
		},
		{
			"dimensionHeaders": []map[string]any{{"name": "device"}},
			"metricHeaders":    []map[string]any{{"name": "views", "type": "TYPE_INTEGER"}},
			"rows": []map[string]any{
				{
					"dimensionValues": []map[string]any{{"value": "Mobile"}},
					"metricValues":    []map[string]any{{"value": "7"}},
				},
				{
					"dimensionValues": []map[string]any{{"value": "Desktop"}},
					"metricValues":    []map[string]any{{"value": "9"}},
				},
			},
			"rowCount": 2,
		},
	}
}

func analyticsTSVTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response map[string]any
		switch {
		case r.Method == http.MethodPost &&
			(strings.HasSuffix(r.URL.Path, ":runReport") || strings.HasSuffix(r.URL.Path, ":runRealtimeReport")):
			response = analyticsUnsafeReport()
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":runPivotReport"):
			response = analyticsUnsafeReport()
			response["pivotHeaders"] = []map[string]any{{
				"pivotDimensionHeaders": []map[string]any{{
					"dimensionValues": []map[string]any{{"value": unsafeAnalyticsMetricHeader}},
				}},
			}}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchRunReports"):
			response = map[string]any{"reports": analyticsBatchLongFormReports()}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchRunPivotReports"):
			response = map[string]any{"pivotReports": analyticsBatchLongFormReports()}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":query"):
			response = map[string]any{
				"audienceExport": map[string]any{
					"dimensions": []map[string]any{
						{"dimensionName": unsafeAnalyticsDimensionHeader},
						{"dimensionName": unsafeAnalyticsMetricHeader},
					},
				},
				"audienceRows": []map[string]any{{
					"dimensionValues": []map[string]any{
						{"value": unsafeAnalyticsDimensionValue},
						{"value": unsafeAnalyticsMetricValue},
					},
				}},
			}
		default:
			http.NotFound(w, r)
			return
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
}

func setupAnalyticsTSVService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	original := newAnalyticsDataService
	t.Cleanup(func() { newAnalyticsDataService = original })

	service, err := analyticsdata.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAnalyticsDataService = func(context.Context, string) (*analyticsdata.Service, error) {
		return service, nil
	}
}

func TestAnalyticsReports_PlainTSVPreservesRows(t *testing.T) {
	srv := analyticsTSVTestServer(t)
	defer srv.Close()
	setupAnalyticsTSVService(t, srv)

	tests := []struct {
		name    string
		command []string
		want    string
	}{
		{
			name:    "standard",
			command: []string{"analytics", "report", "--property", "456", "--metrics", "sessions", "--dimensions", "country"},
			want:    analyticsSafeTSV,
		},
		{
			name:    "realtime",
			command: []string{"analytics", "realtime", "--property", "456", "--metrics", "activeUsers", "--dimensions", "country"},
			want:    analyticsSafeTSV,
		},
		{
			name:    "pivot",
			command: []string{"analytics", "pivot-report", "--property", "456", "--dimensions", "country", "--metrics", "sessions", "--pivots-json", `[{"fieldNames":["browser"]}]`},
			want:    analyticsSafeTSV,
		},
		{
			name:    "batch",
			command: []string{"analytics", "batch-reports", "--property", "456", "--requests-json", `[{}]`},
			want:    analyticsBatchSafeTSV,
		},
		{
			name:    "batch pivot",
			command: []string{"analytics", "batch-pivot-reports", "--property", "456", "--requests-json", `[{"pivots":[{"fieldNames":["browser"]}]}]`},
			want:    analyticsBatchSafeTSV,
		},
		{
			name:    "audience",
			command: []string{"analytics", "audience-exports", "query", "--name", "properties/456/audienceExports/1"},
			want:    analyticsSafeTSV,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--plain", "--account", "a@b.com"}, tt.command...)
			out := captureStdout(t, func() {
				_ = captureStderr(t, func() {
					if err := Execute(args); err != nil {
						t.Fatalf("Execute: %v", err)
					}
				})
			})
			if out != tt.want {
				t.Fatalf("plain output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestAnalyticsBatchReports_PlainEmptyResponsesEmitHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchRunReports"):
			_ = json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		case strings.HasSuffix(r.URL.Path, ":batchRunPivotReports"):
			_ = json.NewEncoder(w).Encode(map[string]any{"pivotReports": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	setupAnalyticsTSVService(t, srv)

	commands := [][]string{
		{"analytics", "batch-reports", "--property", "456", "--requests-json", `[{}]`},
		{"analytics", "batch-pivot-reports", "--property", "456", "--requests-json", `[{"pivots":[{"fieldNames":["browser"]}]}]`},
	}
	const want = "REPORT_INDEX\tROW_INDEX\tFIELD_KIND\tFIELD_NAME\tFIELD_VALUE\n"

	for _, command := range commands {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				args := append([]string{"--plain", "--account", "a@b.com"}, command...)
				if err := Execute(args); err != nil {
					t.Fatalf("Execute(%q): %v", command[1], err)
				}
			})
		})
		if out != want {
			t.Fatalf("%s plain output = %q, want %q", command[1], out, want)
		}
	}
}

func TestAnalyticsReport_JSONPreservesUnsafeStrings(t *testing.T) {
	srv := analyticsTSVTestServer(t)
	defer srv.Close()
	setupAnalyticsTSVService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com", "analytics", "report",
				"--property", "456", "--metrics", "sessions", "--dimensions", "country",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var response struct {
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
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if got := response.DimensionHeaders[0].Name; got != unsafeAnalyticsDimensionHeader {
		t.Fatalf("dimension header = %q", got)
	}
	if got := response.MetricHeaders[0].Name; got != unsafeAnalyticsMetricHeader {
		t.Fatalf("metric header = %q", got)
	}
	if got := response.Rows[0].DimensionValues[0].Value; got != unsafeAnalyticsDimensionValue {
		t.Fatalf("dimension value = %q", got)
	}
	if got := response.Rows[0].MetricValues[0].Value; got != unsafeAnalyticsMetricValue {
		t.Fatalf("metric value = %q", got)
	}
}
