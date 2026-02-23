package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// --- audience-exports parent command ---

type AnalyticsAudienceExportsCmd struct {
	Create AnalyticsAudienceExportsCreateCmd `cmd:"" name:"create" help:"Create a new audience export"`
	Get    AnalyticsAudienceExportsGetCmd    `cmd:"" name:"get" help:"Get an audience export by name"`
	List   AnalyticsAudienceExportsListCmd   `cmd:"" name:"list" help:"List audience exports for a property"`
	Query  AnalyticsAudienceExportsQueryCmd  `cmd:"" name:"query" help:"Query rows from an audience export"`
}

// --- audience-exports create ---

type AnalyticsAudienceExportsCreateCmd struct {
	Property   string   `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Audience   string   `name:"audience" required:"" help:"Audience resource name (e.g. properties/123456/audiences/789)"`
	Dimensions []string `name:"dimensions" help:"Dimension names to include (e.g. deviceId,userId)"`
}

func (c *AnalyticsAudienceExportsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)
	if propID == analyticsPropertyPrefix {
		return usage("--property required")
	}

	audience := strings.TrimSpace(c.Audience)
	if audience == "" {
		return usage("--audience required")
	}

	req := &analyticsdata.AudienceExport{
		Audience: audience,
	}

	// Parse dimensions
	for _, d := range c.Dimensions {
		d = strings.TrimSpace(d)
		if d != "" {
			req.Dimensions = append(req.Dimensions, &analyticsdata.V1betaAudienceDimension{
				DimensionName: d,
			})
		}
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	parent := propID
	resp, err := svc.Properties.AudienceExports.Create(parent, req).Do()
	if err != nil {
		return err
	}

	// The create method returns an Operation. When done, the response contains the AudienceExport.
	// For now, we just return the operation metadata.
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"operation": map[string]any{
				"name":     resp.Name,
				"done":     resp.Done,
				"metadata": resp.Metadata,
			},
		})
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintf(w, "OPERATION_NAME\t%s\n", resp.Name)
	fmt.Fprintf(w, "DONE\t%v\n", resp.Done)

	return nil
}

// --- audience-exports get ---

type AnalyticsAudienceExportsGetCmd struct {
	Name string `name:"name" required:"" help:"Audience export resource name (e.g. properties/123456/audienceExports/789)"`
}

func (c *AnalyticsAudienceExportsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("--name required")
	}

	// Normalize name if needed
	if !strings.HasPrefix(name, analyticsPropertyPrefix) {
		name = analyticsPropertyPrefix + name
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.AudienceExports.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintf(w, "NAME\t%s\n", resp.Name)
	fmt.Fprintf(w, "AUDIENCE\t%s\n", resp.Audience)
	if resp.AudienceDisplayName != "" {
		fmt.Fprintf(w, "AUDIENCE_NAME\t%s\n", resp.AudienceDisplayName)
	}
	fmt.Fprintf(w, "STATE\t%s\n", resp.State)
	fmt.Fprintf(w, "ROW_COUNT\t%d\n", resp.RowCount)
	if resp.BeginCreatingTime != "" {
		fmt.Fprintf(w, "CREATION_TIME\t%s\n", resp.BeginCreatingTime)
	}
	if resp.PercentageCompleted > 0 {
		fmt.Fprintf(w, "PERCENTAGE_COMPLETED\t%.1f%%\n", resp.PercentageCompleted)
	}
	if resp.ErrorMessage != "" {
		fmt.Fprintf(w, "ERROR\t%s\n", resp.ErrorMessage)
	}

	return nil
}

// --- audience-exports list ---

type AnalyticsAudienceExportsListCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Max      int64  `name:"max" help:"Max results per page" default:"100"`
	Page     string `name:"page" help:"Page token for pagination"`
}

func (c *AnalyticsAudienceExportsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	call := svc.Properties.AudienceExports.List(propID)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"audienceExports": resp.AudienceExports,
			"nextPageToken":   resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.AudienceExports) == 0 {
		u.Err().Println("No audience exports")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "NAME\tAUDIENCE\tSTATE\tROW_COUNT")
	for _, ae := range resp.AudienceExports {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", ae.Name, ae.Audience, ae.State, ae.RowCount)
	}
	printNextPageHint(u, resp.NextPageToken)

	return nil
}

// --- audience-exports query ---

type AnalyticsAudienceExportsQueryCmd struct {
	Name   string `name:"name" required:"" help:"Audience export resource name (e.g. properties/123456/audienceExports/789)"`
	Offset int64  `name:"offset" help:"Starting row offset" default:"0"`
	Limit  int64  `name:"limit" help:"Max rows to return" default:"10000"`
}

func (c *AnalyticsAudienceExportsQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("--name required")
	}

	// Normalize name if needed
	if !strings.HasPrefix(name, analyticsPropertyPrefix) {
		name = analyticsPropertyPrefix + name
	}

	req := &analyticsdata.QueryAudienceExportRequest{
		Offset: c.Offset,
		Limit:  c.Limit,
	}

	svc, err := newAnalyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.AudienceExports.Query(name, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"audienceExport": resp.AudienceExport,
			"audienceRows":   resp.AudienceRows,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.AudienceRows) == 0 {
		u.Err().Println("No audience rows")
		return nil
	}

	// Get dimension names from the audience export metadata
	var dimensionNames []string
	if resp.AudienceExport != nil {
		for _, d := range resp.AudienceExport.Dimensions {
			dimensionNames = append(dimensionNames, d.DimensionName)
		}
	}

	w, flush := tableWriter(ctx)
	defer flush()

	// Print header row
	if len(dimensionNames) > 0 {
		fmt.Fprintln(w, strings.Join(dimensionNames, "\t"))
	} else {
		fmt.Fprintln(w, "DIMENSION_VALUES")
	}

	// Print data rows
	for _, row := range resp.AudienceRows {
		var vals []string
		for _, dv := range row.DimensionValues {
			vals = append(vals, dv.Value)
		}
		if len(vals) > 0 {
			fmt.Fprintln(w, strings.Join(vals, "\t"))
		}
	}

	return nil
}
