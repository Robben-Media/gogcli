package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// --- sitemaps get ---

type SearchConsoleSitemapsGetCmd struct {
	SiteURL  string `arg:"" name:"siteUrl" help:"Site URL (e.g. https://example.com/ or sc-domain:example.com)"`
	Feedpath string `arg:"" name:"feedpath" help:"Sitemap URL (e.g. https://example.com/sitemap.xml)"`
}

func (c *SearchConsoleSitemapsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}
	feedpath := strings.TrimSpace(c.Feedpath)
	if feedpath == "" {
		return usage("empty feedpath")
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	sm, err := svc.Sitemaps.Get(siteURL, feedpath).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"sitemap": sm})
	}

	// Text output
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tTYPE\tLAST_SUBMITTED\tLAST_DOWNLOADED\tWARNINGS\tERRORS")
	smType := sm.Type
	if smType == "" {
		smType = "unknown"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n",
		sm.Path, smType, sm.LastSubmitted, sm.LastDownloaded, sm.Warnings, sm.Errors)
	return nil
}

// --- sitemaps delete ---

type SearchConsoleSitemapsDeleteCmd struct {
	SiteURL  string `arg:"" name:"siteUrl" help:"Site URL (e.g. https://example.com/ or sc-domain:example.com)"`
	Feedpath string `arg:"" name:"feedpath" help:"Sitemap URL to delete (e.g. https://example.com/sitemap.xml)"`
}

func (c *SearchConsoleSitemapsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}
	feedpath := strings.TrimSpace(c.Feedpath)
	if feedpath == "" {
		return usage("empty feedpath")
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete sitemap %s from site %s", feedpath, siteURL)); confErr != nil {
		return confErr
	}

	svc, err := newSearchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	if delErr := svc.Sitemaps.Delete(siteURL, feedpath).Context(ctx).Do(); delErr != nil {
		return delErr
	}

	u.Err().Println("Sitemap deleted")
	return nil
}
