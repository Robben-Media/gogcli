package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ---------------------------------------------------------------------------
// Properties
// ---------------------------------------------------------------------------

// --- create ---

type AAPropertiesCreateCmd struct {
	GAAccount        string `name:"ga-account" required:"" help:"GA account ID (e.g. 123456 or accounts/123456)"`
	DisplayName      string `name:"display-name" required:"" help:"Human-readable display name"`
	TimeZone         string `name:"timezone" help:"Reporting time zone (IANA)" default:"America/Chicago"`
	CurrencyCode     string `name:"currency" help:"Currency code (ISO 4217)" default:"USD"`
	IndustryCategory string `name:"industry" help:"Industry category (e.g. TECHNOLOGY, REAL_ESTATE)"`
}

func (c *AAPropertiesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	acctID := normalizeAccountID(c.GAAccount)

	prop := &analyticsadmin.GoogleAnalyticsAdminV1betaProperty{
		Parent:       acctID,
		DisplayName:  c.DisplayName,
		TimeZone:     c.TimeZone,
		CurrencyCode: c.CurrencyCode,
	}
	if c.IndustryCategory != "" {
		prop.IndustryCategory = c.IndustryCategory
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.Create(prop).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"property": resp})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created property: %s", resp.Name)
	u.Out().Printf("Display name: %s", resp.DisplayName)
	return nil
}

// --- list ---

type AAPropertiesListCmd struct {
	GAAccount string `name:"ga-account" required:"" help:"GA account ID (e.g. 123456 or accounts/123456)"`
	PageSize  int64  `name:"page-size" help:"Max results per page" default:"50"`
	PageToken string `name:"page-token" help:"Page token for pagination"`
}

func (c *AAPropertiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	acctID := normalizeAccountID(c.GAAccount)

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Properties.List().Filter("parent:" + acctID)
	if c.PageSize > 0 {
		call = call.PageSize(c.PageSize)
	}
	if c.PageToken != "" {
		call = call.PageToken(c.PageToken)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"properties":    resp.Properties,
			"nextPageToken": resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Properties) == 0 {
		u.Err().Println("No properties")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY_NAME\tTIMEZONE\tCURRENCY\tINDUSTRY")
	for _, p := range resp.Properties {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.DisplayName, p.TimeZone, p.CurrencyCode, p.IndustryCategory)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const analyticsAccountPrefix = "accounts/"

// normalizeAccountID ensures the account ID has the "accounts/" prefix.
func normalizeAccountID(id string) string {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, analyticsAccountPrefix) {
		return analyticsAccountPrefix + id
	}
	return id
}
