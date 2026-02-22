package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/searchconsole/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newSearchConsoleService = googleapi.NewSearchConsole

type SearchConsoleCmd struct {
	Sites         SearchConsoleSitesCmd         `cmd:"" name:"sites" group:"Read" help:"List Search Console sites"`
	Query         SearchConsoleQueryCmd         `cmd:"" name:"query" group:"Read" help:"Query search analytics data"`
	Sitemaps      SearchConsoleSitemapsCmd      `cmd:"" name:"sitemaps" group:"Read" help:"List sitemaps for a site"`
	SubmitSitemap SearchConsoleSubmitSitemapCmd `cmd:"" name:"submit-sitemap" group:"Write" help:"Submit a sitemap for a site"`
	Inspect       SearchConsoleInspectCmd       `cmd:"" name:"inspect" group:"Read" help:"Inspect a URL's index status"`
}

// --- sites ---

type SearchConsoleSitesCmd struct{}

func (c *SearchConsoleSitesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Sites.List().Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"sites": resp.SiteEntry,
		})
	}

	if len(resp.SiteEntry) == 0 {
		u.Err().Println("No sites")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SITE_URL\tPERMISSION")
	for _, site := range resp.SiteEntry {
		fmt.Fprintf(w, "%s\t%s\n", site.SiteUrl, site.PermissionLevel)
	}
	return nil
}

// --- query ---

type SearchConsoleQueryCmd struct {
	SiteURL    string `name:"site-url" required:"" help:"Site URL (e.g. https://example.com/ or sc-domain:example.com)"`
	StartDate  string `name:"start-date" required:"" help:"Start date (YYYY-MM-DD)"`
	EndDate    string `name:"end-date" required:"" help:"End date (YYYY-MM-DD)"`
	Dimensions string `name:"dimensions" help:"Comma-separated dimensions: query,page,country,device,date" default:""`
	RowLimit   int64  `name:"row-limit" help:"Max rows to return" default:"25"`
}

func (c *SearchConsoleQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("--site-url required")
	}
	startDate := strings.TrimSpace(c.StartDate)
	if startDate == "" {
		return usage("--start-date required")
	}
	endDate := strings.TrimSpace(c.EndDate)
	if endDate == "" {
		return usage("--end-date required")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: startDate,
		EndDate:   endDate,
		RowLimit:  c.RowLimit,
	}

	if dims := strings.TrimSpace(c.Dimensions); dims != "" {
		req.Dimensions = strings.Split(dims, ",")
	}

	resp, err := svc.Searchanalytics.Query(siteURL, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"rows": resp.Rows,
		})
	}

	if len(resp.Rows) == 0 {
		u.Err().Println("No data")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "KEYS\tCLICKS\tIMPRESSIONS\tCTR\tPOSITION")
	for _, row := range resp.Rows {
		keys := strings.Join(row.Keys, ", ")
		fmt.Fprintf(w, "%s\t%.0f\t%.0f\t%.1f%%\t%.1f\n",
			keys, row.Clicks, row.Impressions, row.Ctr*100, row.Position)
	}
	return nil
}

// --- sitemaps ---

type SearchConsoleSitemapsCmd struct {
	SiteURL string `name:"site-url" required:"" help:"Site URL"`
}

func (c *SearchConsoleSitemapsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("--site-url required")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Sitemaps.List(siteURL).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"sitemaps": resp.Sitemap,
		})
	}

	if len(resp.Sitemap) == 0 {
		u.Err().Println("No sitemaps")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tLAST_SUBMITTED\tPENDING\tWARNINGS\tERRORS")
	for _, sm := range resp.Sitemap {
		fmt.Fprintf(w, "%s\t%s\t%t\t%d\t%d\n",
			sm.Path, sm.LastSubmitted, sm.IsPending, sm.Warnings, sm.Errors)
	}
	return nil
}

// --- submit-sitemap ---

type SearchConsoleSubmitSitemapCmd struct {
	SiteURL    string `name:"site-url" required:"" help:"Site URL"`
	SitemapURL string `name:"sitemap-url" required:"" help:"Sitemap URL to submit"`
}

func (c *SearchConsoleSubmitSitemapCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("--site-url required")
	}
	sitemapURL := strings.TrimSpace(c.SitemapURL)
	if sitemapURL == "" {
		return usage("--sitemap-url required")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Sitemaps.Submit(siteURL, sitemapURL).Do(); err != nil {
		return err
	}

	u.Err().Println("Sitemap submitted successfully")
	return nil
}

// --- inspect ---

type SearchConsoleInspectCmd struct {
	SiteURL string `name:"site-url" required:"" help:"Site URL"`
	URL     string `name:"url" required:"" help:"URL to inspect"`
}

func (c *SearchConsoleInspectCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("--site-url required")
	}
	inspectURL := strings.TrimSpace(c.URL)
	if inspectURL == "" {
		return usage("--url required")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.UrlInspection.Index.Inspect(&searchconsole.InspectUrlIndexRequest{
		InspectionUrl: inspectURL,
		SiteUrl:       siteURL,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"inspectionResult": resp.InspectionResult,
		})
	}

	result := resp.InspectionResult
	if result == nil || result.IndexStatusResult == nil {
		u.Err().Println("No inspection result available")
		return nil
	}

	idx := result.IndexStatusResult
	u.Out().Printf("verdict\t%s", idx.Verdict)
	u.Out().Printf("coverageState\t%s", idx.CoverageState)
	u.Out().Printf("indexingState\t%s", idx.IndexingState)
	u.Out().Printf("pageFetchState\t%s", idx.PageFetchState)
	u.Out().Printf("crawledAs\t%s", idx.CrawledAs)
	u.Out().Printf("lastCrawlTime\t%s", idx.LastCrawlTime)
	return nil
}

// Ensure searchconsole.Service is used to avoid lint.
var _ *searchconsole.Service
