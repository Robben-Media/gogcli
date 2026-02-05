package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var (
	newAnalyticsDataService  = googleapi.NewAnalyticsData
	newAnalyticsAdminService = googleapi.NewAnalyticsAdmin
)

type AnalyticsCmd struct {
	Report     AnalyticsReportCmd     `cmd:"" name:"report" group:"Read" help:"Run a report"`
	Realtime   AnalyticsRealtimeCmd   `cmd:"" name:"realtime" group:"Read" help:"Run a realtime report"`
	Properties AnalyticsPropertiesCmd `cmd:"" name:"properties" group:"Read" help:"List properties"`
	Accounts   AnalyticsAccountsCmd   `cmd:"" name:"accounts" group:"Read" help:"List accounts"`
	Dimensions AnalyticsDimensionsCmd `cmd:"" name:"dimensions" group:"Read" help:"List available dimensions for a property"`
	Metrics    AnalyticsMetricsCmd    `cmd:"" name:"metrics" group:"Read" help:"List available metrics for a property"`
}

const analyticsPropertyPrefix = "properties/"

// normalizePropertyID ensures the property ID has the "properties/" prefix.
func normalizePropertyID(id string) string {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, analyticsPropertyPrefix) {
		return analyticsPropertyPrefix + id
	}
	return id
}

// --- report ---

type AnalyticsReportCmd struct {
	Property   string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Metrics    string `name:"metrics" required:"" help:"Comma-separated metric names (e.g. sessions,users)"`
	Dimensions string `name:"dimensions" help:"Comma-separated dimension names (e.g. date,country)"`
	StartDate  string `name:"start-date" help:"Start date (YYYY-MM-DD or relative like 28daysAgo)" default:"28daysAgo"`
	EndDate    string `name:"end-date" help:"End date (YYYY-MM-DD or relative like today)" default:"today"`
	Limit      int64  `name:"limit" help:"Max rows to return" default:"0"`
}

func (c *AnalyticsReportCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	metricNames := strings.Split(c.Metrics, ",")
	if len(metricNames) == 0 || (len(metricNames) == 1 && strings.TrimSpace(metricNames[0]) == "") {
		return usage("--metrics required")
	}

	metrics := make([]*analyticsdata.Metric, 0, len(metricNames))
	for _, m := range metricNames {
		m = strings.TrimSpace(m)
		if m != "" {
			metrics = append(metrics, &analyticsdata.Metric{Name: m})
		}
	}

	var dimensions []*analyticsdata.Dimension
	if c.Dimensions != "" {
		dimNames := strings.Split(c.Dimensions, ",")
		for _, d := range dimNames {
			d = strings.TrimSpace(d)
			if d != "" {
				dimensions = append(dimensions, &analyticsdata.Dimension{Name: d})
			}
		}
	}

	req := &analyticsdata.RunReportRequest{
		Metrics:    metrics,
		Dimensions: dimensions,
		DateRanges: []*analyticsdata.DateRange{
			{StartDate: c.StartDate, EndDate: c.EndDate},
		},
	}
	if c.Limit > 0 {
		req.Limit = c.Limit
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.RunReport(propID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"dimensionHeaders": resp.DimensionHeaders,
			"metricHeaders":    resp.MetricHeaders,
			"rows":             resp.Rows,
			"rowCount":         resp.RowCount,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Rows) == 0 {
		u.Err().Println("No data")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	// Build header row
	var headers []string
	for _, dh := range resp.DimensionHeaders {
		headers = append(headers, dh.Name)
	}
	for _, mh := range resp.MetricHeaders {
		headers = append(headers, mh.Name)
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

// --- realtime ---

type AnalyticsRealtimeCmd struct {
	Property   string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Metrics    string `name:"metrics" help:"Comma-separated metric names" default:"activeUsers"`
	Dimensions string `name:"dimensions" help:"Comma-separated dimension names (e.g. country)"`
}

func (c *AnalyticsRealtimeCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	metricNames := strings.Split(c.Metrics, ",")
	metrics := make([]*analyticsdata.Metric, 0, len(metricNames))
	for _, m := range metricNames {
		m = strings.TrimSpace(m)
		if m != "" {
			metrics = append(metrics, &analyticsdata.Metric{Name: m})
		}
	}

	var dimensions []*analyticsdata.Dimension
	if c.Dimensions != "" {
		dimNames := strings.Split(c.Dimensions, ",")
		for _, d := range dimNames {
			d = strings.TrimSpace(d)
			if d != "" {
				dimensions = append(dimensions, &analyticsdata.Dimension{Name: d})
			}
		}
	}

	req := &analyticsdata.RunRealtimeReportRequest{
		Metrics:    metrics,
		Dimensions: dimensions,
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.RunRealtimeReport(propID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"dimensionHeaders": resp.DimensionHeaders,
			"metricHeaders":    resp.MetricHeaders,
			"rows":             resp.Rows,
			"rowCount":         resp.RowCount,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Rows) == 0 {
		u.Err().Println("No data")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	var headers []string
	for _, dh := range resp.DimensionHeaders {
		headers = append(headers, dh.Name)
	}
	for _, mh := range resp.MetricHeaders {
		headers = append(headers, mh.Name)
	}
	fmt.Fprintln(w, strings.Join(headers, "\t"))

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

// --- properties ---

type AnalyticsPropertiesCmd struct {
	PageSize int64 `name:"page-size" help:"Max results per page" default:"50"`
}

func (c *AnalyticsPropertiesCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.AccountSummaries.List()
	if c.PageSize > 0 {
		call = call.PageSize(c.PageSize)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"accountSummaries": resp.AccountSummaries,
			"nextPageToken":    resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.AccountSummaries) == 0 {
		u.Err().Println("No properties")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ACCOUNT\tACCOUNT_NAME\tPROPERTY\tPROPERTY_NAME")
	for _, acct := range resp.AccountSummaries {
		for _, prop := range acct.PropertySummaries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", acct.Account, acct.DisplayName, prop.Property, prop.DisplayName)
		}
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- accounts ---

type AnalyticsAccountsCmd struct {
	PageSize int64 `name:"page-size" help:"Max results per page" default:"50"`
}

func (c *AnalyticsAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.List()
	if c.PageSize > 0 {
		call = call.PageSize(c.PageSize)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"accounts":      resp.Accounts,
			"nextPageToken": resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Accounts) == 0 {
		u.Err().Println("No accounts")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY_NAME\tCREATED\tUPDATED")
	for _, a := range resp.Accounts {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Name, a.DisplayName, a.CreateTime, a.UpdateTime)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- dimensions ---

type AnalyticsDimensionsCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
}

func (c *AnalyticsDimensionsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	metadataName := propID + "/metadata"
	resp, err := svc.Properties.GetMetadata(metadataName).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"dimensions": resp.Dimensions,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Dimensions) == 0 {
		u.Err().Println("No dimensions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "API_NAME\tDISPLAY_NAME\tDESCRIPTION")
	for _, d := range resp.Dimensions {
		fmt.Fprintf(w, "%s\t%s\t%s\n", d.ApiName, d.UiName, d.Description)
	}
	return nil
}

// --- metrics ---

type AnalyticsMetricsCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
}

func (c *AnalyticsMetricsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	metadataName := propID + "/metadata"
	resp, err := svc.Properties.GetMetadata(metadataName).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"metrics": resp.Metrics,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Metrics) == 0 {
		u.Err().Println("No metrics")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "API_NAME\tDISPLAY_NAME\tDESCRIPTION")
	for _, m := range resp.Metrics {
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.ApiName, m.UiName, m.Description)
	}
	return nil
}

// Ensure service types are used to avoid import cycle lint errors.
var (
	_ *analyticsdata.Service
	_ *analyticsadmin.Service
)
