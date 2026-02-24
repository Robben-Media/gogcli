package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// BusinessProfileInfoCategoriesCmd is a parent for category subcommands.
type BusinessProfileInfoCategoriesCmd struct {
	List     BusinessProfileInfoCategoriesListCmd     `cmd:"" name:"list" help:"List business categories"`
	BatchGet BusinessProfileInfoCategoriesBatchGetCmd `cmd:"" name:"batch-get" help:"Get multiple categories by name"`
}

// BusinessProfileInfoCategoriesListCmd lists business categories.
type BusinessProfileInfoCategoriesListCmd struct {
	RegionCode   string `name:"region-code" required:"" help:"ISO 3166-1 alpha-2 region code"`
	LanguageCode string `name:"language-code" help:"BCP 47 language code" default:"en"`
	Filter       string `name:"filter" help:"Search filter text (e.g. 'displayName=food')"`
	Max          int64  `name:"max" help:"Max results per page" default:"100"`
	Page         string `name:"page" help:"Page token"`
	View         string `name:"view" help:"View: BASIC or FULL" default:"BASIC"`
}

func (c *BusinessProfileInfoCategoriesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Categories.List().
		RegionCode(c.RegionCode).
		LanguageCode(c.LanguageCode).
		PageSize(c.Max).
		View(c.View)

	if filter := strings.TrimSpace(c.Filter); filter != "" {
		call = call.Filter(filter)
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
			"categories":    resp.Categories,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Categories) == 0 {
		u.Err().Println("No categories")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY_NAME")
	for _, cat := range resp.Categories {
		fmt.Fprintf(w, "%s\t%s\n", cat.Name, cat.DisplayName)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// BusinessProfileInfoCategoriesBatchGetCmd gets multiple categories by name.
type BusinessProfileInfoCategoriesBatchGetCmd struct {
	Names        []string `name:"names" required:"" help:"Category resource names"`
	LanguageCode string   `name:"language-code" help:"BCP 47 language code" default:"en"`
	RegionCode   string   `name:"region-code" help:"ISO 3166-1 alpha-2 region code"`
	View         string   `name:"view" help:"View: BASIC or FULL" default:"BASIC"`
}

func (c *BusinessProfileInfoCategoriesBatchGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Categories.BatchGet().
		Names(c.Names...).
		LanguageCode(c.LanguageCode).
		View(c.View)

	if rc := strings.TrimSpace(c.RegionCode); rc != "" {
		call = call.RegionCode(rc)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"categories": resp.Categories,
		})
	}

	if len(resp.Categories) == 0 {
		u.Err().Println("No categories")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY_NAME")
	for _, cat := range resp.Categories {
		fmt.Fprintf(w, "%s\t%s\n", cat.Name, cat.DisplayName)
	}
	return nil
}
