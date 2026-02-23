package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// --- pivot-report ---

type AnalyticsPivotReportCmd struct {
	Property     string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Dimensions   string `name:"dimensions" required:"" help:"Comma-separated dimension names (e.g. country,browser)"`
	Metrics      string `name:"metrics" required:"" help:"Comma-separated metric names (e.g. sessions,users)"`
	PivotsJSON   string `name:"pivots-json" required:"" help:"JSON array of pivot definitions, or @filepath to read from file"`
	DateFrom     string `name:"date-from" help:"Start date (YYYY-MM-DD or relative like 28daysAgo)" default:"28daysAgo"`
	DateTo       string `name:"date-to" help:"End date (YYYY-MM-DD or relative like today)" default:"today"`
	FilterJSON   string `name:"filter-json" help:"JSON dimension filter expression, or @filepath"`
	KeepEmpty    bool   `name:"keep-empty" help:"Include rows with all zero metric values"`
	CurrencyCode string `name:"currency-code" help:"Currency code in ISO4217 format (e.g. USD)"`
	Limit        int64  `name:"limit" help:"Max rows per pivot (default 10000)" default:"0"`
}

func (c *AnalyticsPivotReportCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	// Parse dimensions
	dimensions := parseAnalyticsDimensions(c.Dimensions)
	if len(dimensions) == 0 {
		return usage("--dimensions required")
	}

	// Parse metrics
	metrics := parseAnalyticsMetrics(c.Metrics)
	if len(metrics) == 0 {
		return usage("--metrics required")
	}

	// Parse pivots from JSON
	pivotsJSON, err := readJSONFromFlag(c.PivotsJSON, "pivots")
	if err != nil {
		return err
	}
	var pivots []*analyticsdata.Pivot
	if unmarshalErr := json.Unmarshal([]byte(pivotsJSON), &pivots); unmarshalErr != nil {
		return fmt.Errorf("invalid pivots JSON: %w", unmarshalErr)
	}
	if len(pivots) == 0 {
		return usage("--pivots-json must contain at least one pivot")
	}

	req := &analyticsdata.RunPivotReportRequest{
		Property:   propID,
		Dimensions: dimensions,
		Metrics:    metrics,
		Pivots:     pivots,
		DateRanges: []*analyticsdata.DateRange{{StartDate: c.DateFrom, EndDate: c.DateTo}},
	}
	if c.KeepEmpty {
		req.KeepEmptyRows = true
	}
	if c.CurrencyCode != "" {
		req.CurrencyCode = c.CurrencyCode
	}

	// Parse dimension filter if provided
	if c.FilterJSON != "" {
		filterJSON, filterErr := readJSONFromFlag(c.FilterJSON, "filter")
		if filterErr != nil {
			return filterErr
		}
		var filter analyticsdata.FilterExpression
		if unmarshalErr := json.Unmarshal([]byte(filterJSON), &filter); unmarshalErr != nil {
			return fmt.Errorf("invalid filter JSON: %w", unmarshalErr)
		}
		req.DimensionFilter = &filter
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.RunPivotReport(propID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"pivotHeaders":     resp.PivotHeaders,
			"dimensionHeaders": resp.DimensionHeaders,
			"metricHeaders":    resp.MetricHeaders,
			"rows":             resp.Rows,
			"metadata":         resp.Metadata,
			"propertyQuota":    resp.PropertyQuota,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Rows) == 0 {
		u.Err().Println("No data")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	// Build header row from dimension headers and metric headers
	var headers []string
	for _, dh := range resp.DimensionHeaders {
		headers = append(headers, dh.Name)
	}
	for _, ph := range resp.PivotHeaders {
		for _, pdh := range ph.PivotDimensionHeaders {
			for _, dv := range pdh.DimensionValues {
				headers = append(headers, dv.Value)
			}
		}
	}
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Build data rows
	for _, row := range resp.Rows {
		var vals []string
		for _, dv := range row.DimensionValues {
			vals = append(vals, dv.Value)
		}
		for _, mv := range row.MetricValues {
			vals = append(vals, mv.Value)
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}

	return nil
}

// --- batch-reports ---

type AnalyticsBatchReportsCmd struct {
	Property     string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	RequestsJSON string `name:"requests-json" required:"" help:"JSON array of report requests (max 5), or @filepath"`
}

func (c *AnalyticsBatchReportsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	// Parse requests from JSON
	requestsJSON, err := readJSONFromFlag(c.RequestsJSON, "requests")
	if err != nil {
		return err
	}
	var requests []*analyticsdata.RunReportRequest
	if unmarshalErr := json.Unmarshal([]byte(requestsJSON), &requests); unmarshalErr != nil {
		return fmt.Errorf("invalid requests JSON: %w", unmarshalErr)
	}
	if len(requests) == 0 {
		return usage("--requests-json must contain at least one request")
	}
	if len(requests) > 5 {
		return usage("--requests-json can contain at most 5 requests")
	}

	req := &analyticsdata.BatchRunReportsRequest{
		Requests: requests,
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.BatchRunReports(propID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"reports": resp.Reports,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Reports) == 0 {
		u.Err().Println("No reports")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	for i, report := range resp.Reports {
		if i > 0 {
			fmt.Fprintln(w, "---")
		}
		fmt.Fprintf(w, "Report %d: %d rows\n", i+1, report.RowCount)
		if len(report.DimensionHeaders) > 0 || len(report.MetricHeaders) > 0 {
			var headers []string
			for _, dh := range report.DimensionHeaders {
				headers = append(headers, dh.Name)
			}
			for _, mh := range report.MetricHeaders {
				headers = append(headers, mh.Name)
			}
			fmt.Fprintln(w, strings.Join(headers, "\t"))
			for _, row := range report.Rows {
				var vals []string
				for _, dv := range row.DimensionValues {
					vals = append(vals, dv.Value)
				}
				for _, mv := range row.MetricValues {
					vals = append(vals, mv.Value)
				}
				fmt.Fprintln(w, strings.Join(vals, "\t"))
			}
		}
	}

	return nil
}

// --- batch-pivot-reports ---

type AnalyticsBatchPivotReportsCmd struct {
	Property     string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	RequestsJSON string `name:"requests-json" required:"" help:"JSON array of pivot report requests (max 5), or @filepath"`
}

func (c *AnalyticsBatchPivotReportsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	// Parse requests from JSON
	requestsJSON, err := readJSONFromFlag(c.RequestsJSON, "requests")
	if err != nil {
		return err
	}
	var requests []*analyticsdata.RunPivotReportRequest
	if unmarshalErr := json.Unmarshal([]byte(requestsJSON), &requests); unmarshalErr != nil {
		return fmt.Errorf("invalid requests JSON: %w", unmarshalErr)
	}
	if len(requests) == 0 {
		return usage("--requests-json must contain at least one request")
	}
	if len(requests) > 5 {
		return usage("--requests-json can contain at most 5 requests")
	}

	req := &analyticsdata.BatchRunPivotReportsRequest{
		Requests: requests,
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.BatchRunPivotReports(propID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"pivotReports": resp.PivotReports,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.PivotReports) == 0 {
		u.Err().Println("No pivot reports")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	for i, report := range resp.PivotReports {
		if i > 0 {
			fmt.Fprintln(w, "---")
		}
		fmt.Fprintf(w, "Pivot Report %d\n", i+1)
		if len(report.Rows) > 0 {
			var headers []string
			for _, dh := range report.DimensionHeaders {
				headers = append(headers, dh.Name)
			}
			fmt.Fprintln(w, strings.Join(headers, "\t"))
			for _, row := range report.Rows {
				var vals []string
				for _, dv := range row.DimensionValues {
					vals = append(vals, dv.Value)
				}
				for _, mv := range row.MetricValues {
					vals = append(vals, mv.Value)
				}
				fmt.Fprintln(w, strings.Join(vals, "\t"))
			}
		}
	}

	return nil
}

// --- check-compatibility ---

type AnalyticsCheckCompatibilityCmd struct {
	Property   string   `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Dimensions []string `name:"dimensions" help:"Dimension names to check (e.g. date,country)"`
	Metrics    []string `name:"metrics" help:"Metric names to check (e.g. sessions,users)"`
	FilterJSON string   `name:"filter-json" help:"JSON dimension filter expression, or @filepath"`
}

func (c *AnalyticsCheckCompatibilityCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	if len(c.Dimensions) == 0 && len(c.Metrics) == 0 {
		return usage("at least one of --dimensions or --metrics is required")
	}

	req := &analyticsdata.CheckCompatibilityRequest{}

	// Parse dimensions
	for _, d := range c.Dimensions {
		d = strings.TrimSpace(d)
		if d != "" {
			req.Dimensions = append(req.Dimensions, &analyticsdata.Dimension{Name: d})
		}
	}

	// Parse metrics
	for _, m := range c.Metrics {
		m = strings.TrimSpace(m)
		if m != "" {
			req.Metrics = append(req.Metrics, &analyticsdata.Metric{Name: m})
		}
	}

	// Parse dimension filter if provided
	if c.FilterJSON != "" {
		filterJSON, filterErr := readJSONFromFlag(c.FilterJSON, "filter")
		if filterErr != nil {
			return filterErr
		}
		var filter analyticsdata.FilterExpression
		if unmarshalErr := json.Unmarshal([]byte(filterJSON), &filter); unmarshalErr != nil {
			return fmt.Errorf("invalid filter JSON: %w", unmarshalErr)
		}
		req.DimensionFilter = &filter
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.CheckCompatibility(propID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"dimensionCompatibilities": resp.DimensionCompatibilities,
			"metricCompatibilities":    resp.MetricCompatibilities,
		})
	}

	u := ui.FromContext(ctx)
	w, flush := tableWriter(ctx)
	defer flush()

	if len(resp.DimensionCompatibilities) > 0 {
		fmt.Fprintln(w, "DIMENSION\tCOMPATIBLE")
		for _, dc := range resp.DimensionCompatibilities {
			fmt.Fprintf(w, "%s\t%s\n", dc.DimensionMetadata.ApiName, dc.Compatibility)
		}
	}

	if len(resp.MetricCompatibilities) > 0 {
		if len(resp.DimensionCompatibilities) > 0 {
			fmt.Fprintln(w, "")
		}
		fmt.Fprintln(w, "METRIC\tCOMPATIBLE")
		for _, mc := range resp.MetricCompatibilities {
			fmt.Fprintf(w, "%s\t%s\n", mc.MetricMetadata.ApiName, mc.Compatibility)
		}
	}

	if len(resp.DimensionCompatibilities) == 0 && len(resp.MetricCompatibilities) == 0 {
		u.Err().Println("No compatibility results")
	}

	return nil
}

// --- Helper functions ---

// parseAnalyticsDimensions parses a comma-separated list of dimension names.
func parseAnalyticsDimensions(dimensions string) []*analyticsdata.Dimension {
	dimensions = strings.TrimSpace(dimensions)
	if dimensions == "" {
		return nil
	}
	names := strings.Split(dimensions, ",")
	result := make([]*analyticsdata.Dimension, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			result = append(result, &analyticsdata.Dimension{Name: n})
		}
	}
	return result
}

// parseAnalyticsMetrics parses a comma-separated list of metric names.
func parseAnalyticsMetrics(metrics string) []*analyticsdata.Metric {
	metrics = strings.TrimSpace(metrics)
	if metrics == "" {
		return nil
	}
	names := strings.Split(metrics, ",")
	result := make([]*analyticsdata.Metric, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			result = append(result, &analyticsdata.Metric{Name: n})
		}
	}
	return result
}

// readJSONFromFlag reads JSON input from a flag value.
// If the value starts with @, it reads from the specified file.
// If the value is "-", it reads from stdin.
// Otherwise, it returns the value as-is.
func readJSONFromFlag(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", usagef("--%s-json required", label)
	}

	// Check for @filepath syntax
	if strings.HasPrefix(value, "@") {
		path := strings.TrimPrefix(value, "@")
		var b []byte
		var err error
		if path == "-" {
			b, err = io.ReadAll(os.Stdin)
		} else {
			path, err = config.ExpandPath(path)
			if err != nil {
				return "", err
			}
			b, err = os.ReadFile(path) //nolint:gosec // user-provided path
		}
		if err != nil {
			return "", fmt.Errorf("read %s file: %w", label, err)
		}
		return string(b), nil
	}

	return value, nil
}
