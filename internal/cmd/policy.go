package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

type PolicyCmd struct {
	Create PolicyCreateCmd `cmd:"" help:"Create a persisted safety policy"`
	Get    PolicyGetCmd    `cmd:"" help:"Show one policy"`
	List   PolicyListCmd   `cmd:"" help:"List policies"`
	Delete PolicyDeleteCmd `cmd:"" help:"Delete a policy"`
}

type PolicyCreateCmd struct {
	Name   string `arg:"" help:"Policy name"`
	Allow  string `name:"allow" help:"Allowed action IDs (comma-separated)"`
	Deny   string `name:"deny" help:"Denied action IDs (comma-separated)"`
	Reason string `name:"reason" help:"Why this policy exists"`
}

func (c *PolicyCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	policy := config.Policy{
		Name:    c.Name,
		Account: flags.Account,
		Client:  flags.Client,
		Allow:   normalizePolicyInputs(splitCSV(c.Allow)),
		Deny:    normalizePolicyInputs(splitCSV(c.Deny)),
		Reason:  c.Reason,
	}
	if err := validatePolicyActions(policy); err != nil {
		return err
	}
	if err := config.UpsertPolicy(&cfg, policy, flags != nil && flags.Force); err != nil {
		return usage(err.Error())
	}
	if err := config.WriteConfig(cfg); err != nil {
		return err
	}

	saved, _ := config.GetPolicy(cfg, c.Name)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"created": true,
			"policy":  saved,
		})
	}
	fmt.Fprintf(os.Stdout, "Saved policy %s\n", saved.Name)
	return nil
}

type PolicyGetCmd struct {
	Name string `arg:"" help:"Policy name"`
}

func (c *PolicyGetCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	policy, ok := config.GetPolicy(cfg, c.Name)
	if !ok {
		return usagef("policy %q not found", c.Name)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"policy": policy})
	}
	fmt.Fprintf(os.Stdout, "name\t%s\n", policy.Name)
	if policy.Account != "" {
		fmt.Fprintf(os.Stdout, "account\t%s\n", policy.Account)
	}
	if policy.Client != "" {
		fmt.Fprintf(os.Stdout, "client\t%s\n", policy.Client)
	}
	if len(policy.Allow) > 0 {
		fmt.Fprintf(os.Stdout, "allow\t%s\n", joinCSV(policy.Allow))
	}
	if len(policy.Deny) > 0 {
		fmt.Fprintf(os.Stdout, "deny\t%s\n", joinCSV(policy.Deny))
	}
	if policy.Reason != "" {
		fmt.Fprintf(os.Stdout, "reason\t%s\n", policy.Reason)
	}
	return nil
}

type PolicyListCmd struct{}

func (c *PolicyListCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"policies": cfg.Policies})
	}
	if len(cfg.Policies) == 0 {
		fmt.Fprintln(os.Stdout, "No policies")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "NAME\tACCOUNT\tCLIENT\tALLOW\tDENY")
	for _, policy := range cfg.Policies {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n", policy.Name, policy.Account, policy.Client, len(policy.Allow), len(policy.Deny))
	}
	return nil
}

type PolicyDeleteCmd struct {
	Name string `arg:"" help:"Policy name"`
}

func (c *PolicyDeleteCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if err := config.DeletePolicy(&cfg, c.Name); err != nil {
		return usage(err.Error())
	}
	if err := config.WriteConfig(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"name":    c.Name,
		})
	}
	fmt.Fprintf(os.Stdout, "Deleted policy %s\n", c.Name)
	return nil
}
