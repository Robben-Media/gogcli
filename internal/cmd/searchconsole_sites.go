package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/searchconsole/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// --- sites get ---

type SearchConsoleSitesGetCmd struct {
	SiteURL string `arg:"" name:"siteUrl" help:"Site URL (e.g. https://example.com/ or sc-domain:example.com)"`
}

func (c *SearchConsoleSitesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	site, err := svc.Sites.Get(siteURL).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"site": site})
	}

	// Text output
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SITE_URL\tPERMISSION_LEVEL")
	fmt.Fprintf(w, "%s\t%s\n", site.SiteUrl, site.PermissionLevel)
	return nil
}

// --- sites add ---

type SearchConsoleSitesAddCmd struct {
	SiteURL string `arg:"" name:"siteUrl" help:"Site URL to add (e.g. https://example.com/ or sc-domain:example.com)"`
}

func (c *SearchConsoleSitesAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	// sites.add uses PUT (idempotent) with no request body
	if err := svc.Sites.Add(siteURL).Context(ctx).Do(); err != nil {
		return err
	}

	u.Err().Printf("Site added: %s. Verify ownership to access data.", siteURL)
	return nil
}

// --- sites delete ---

type SearchConsoleSitesDeleteCmd struct {
	SiteURL string `arg:"" name:"siteUrl" help:"Site URL to remove"`
}

func (c *SearchConsoleSitesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove site %s from Search Console", siteURL)); confErr != nil {
		return confErr
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	if delErr := svc.Sites.Delete(siteURL).Context(ctx).Do(); delErr != nil {
		return delErr
	}

	u.Err().Println("Site removed")
	return nil
}

// --- mobile-friendly-test ---

type SearchConsoleMobileFriendlyTestCmd struct {
	URL               string `name:"url" required:"" help:"URL to test for mobile friendliness"`
	RequestScreenshot bool   `name:"request-screenshot" help:"Include a screenshot in the response"`
}

func (c *SearchConsoleMobileFriendlyTestCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	testURL := strings.TrimSpace(c.URL)
	if testURL == "" {
		return usage("empty --url")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	req := &searchconsole.RunMobileFriendlyTestRequest{
		Url:               testURL,
		RequestScreenshot: c.RequestScreenshot,
	}

	resp, err := svc.UrlTestingTools.MobileFriendlyTest.Run(req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"testResult": resp})
	}

	// Text output
	w, flush := tableWriter(ctx)
	defer flush()

	// Test status
	testStatus := "UNKNOWN"
	if resp.TestStatus != nil {
		testStatus = resp.TestStatus.Status
	}
	fmt.Fprintln(w, "URL\tSTATUS\tMOBILE_FRIENDLY\tISSUES")
	fmt.Fprintf(w, "%s\t%s\t", testURL, testStatus)

	// Mobile friendliness
	mobileFriendly := "no"
	if resp.MobileFriendliness == "MOBILE_FRIENDLY" {
		mobileFriendly = "yes"
	}
	fmt.Fprintf(w, "%s\t", mobileFriendly)

	// Issues summary
	var issues []string
	if resp.MobileFriendlyIssues != nil {
		for _, issue := range resp.MobileFriendlyIssues {
			if issue.Rule != "" {
				issues = append(issues, issue.Rule)
			}
		}
	}
	if len(issues) == 0 {
		fmt.Fprintln(w, "none")
	} else {
		fmt.Fprintln(w, strings.Join(issues, ", "))
	}

	return nil
}
