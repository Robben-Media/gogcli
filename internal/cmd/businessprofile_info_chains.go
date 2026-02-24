package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// BusinessProfileInfoChainsCmd is a parent for chain subcommands.
type BusinessProfileInfoChainsCmd struct {
	Get    BusinessProfileInfoChainsGetCmd    `cmd:"" name:"get" help:"Get a chain by name"`
	Search BusinessProfileInfoChainsSearchCmd `cmd:"" name:"search" help:"Search for chains"`
}

// BusinessProfileInfoChainsGetCmd gets a chain by resource name.
type BusinessProfileInfoChainsGetCmd struct {
	Name string `arg:"" name:"name" help:"Chain resource name (e.g. 'chains/123' or '123')"`
}

func (c *BusinessProfileInfoChainsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: chain name")
	}
	if !strings.HasPrefix(name, "chains/") {
		name = "chains/" + name
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Chains.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"chain": resp})
	}

	u.Out().Printf("name\t%s", resp.Name)
	if len(resp.ChainNames) > 0 {
		u.Out().Printf("chainName\t%s", resp.ChainNames[0].DisplayName)
	}
	u.Out().Printf("locationCount\t%d", resp.LocationCount)
	if len(resp.Websites) > 0 {
		var uris []string
		for _, w := range resp.Websites {
			uris = append(uris, w.Uri)
		}
		u.Out().Printf("websites\t%s", strings.Join(uris, ", "))
	}
	return nil
}

// BusinessProfileInfoChainsSearchCmd searches for chains by name.
type BusinessProfileInfoChainsSearchCmd struct {
	ChainName string `name:"chain-name" required:"" help:"Chain name to search for"`
	Max       int64  `name:"max" help:"Max results" default:"10"`
}

func (c *BusinessProfileInfoChainsSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Chains.Search().
		ChainName(c.ChainName).
		PageSize(c.Max).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"chains": resp.Chains,
		})
	}

	if len(resp.Chains) == 0 {
		u.Err().Println("No chains found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tCHAIN_NAME\tLOCATION_COUNT")
	for _, ch := range resp.Chains {
		chainName := ""
		if len(ch.ChainNames) > 0 {
			chainName = ch.ChainNames[0].DisplayName
		}
		fmt.Fprintf(w, "%s\t%s\t%d\n", ch.Name, chainName, ch.LocationCount)
	}
	return nil
}
