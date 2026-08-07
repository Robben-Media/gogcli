package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1alpha"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AAAccessBindingsCmd struct {
	List   AAAccessBindingsListCmd   `cmd:"" name:"list" help:"List access bindings"`
	Create AAAccessBindingsCreateCmd `cmd:"" name:"create" help:"Grant a user access"`
	Patch  AAAccessBindingsPatchCmd  `cmd:"" name:"patch" help:"Update an access binding's roles"`
	Delete AAAccessBindingsDeleteCmd `cmd:"" name:"delete" help:"Delete an access binding"`
}

func analyticsAccessBindingParent(value string) (string, error) {
	parent := strings.TrimSpace(value)
	parts := strings.Split(parent, "/")
	if len(parts) != 2 || (parts[0] != "accounts" && parts[0] != "properties") || strings.TrimSpace(parts[1]) == "" {
		return "", usage("parent must be accounts/ACCOUNT_ID or properties/PROPERTY_ID")
	}
	return parent, nil
}

func analyticsAccessBindingName(value string) (string, error) {
	name := strings.TrimSpace(value)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || (parts[0] != "accounts" && parts[0] != "properties") || strings.TrimSpace(parts[1]) == "" || parts[2] != "accessBindings" || strings.TrimSpace(parts[3]) == "" {
		return "", usage("name must be accounts/ACCOUNT_ID/accessBindings/BINDING_ID or properties/PROPERTY_ID/accessBindings/BINDING_ID")
	}
	return name, nil
}

func analyticsAccessBindingRoles(value string) ([]string, error) {
	valid := map[string]string{
		"viewer":          "predefinedRoles/viewer",
		"analyst":         "predefinedRoles/analyst",
		"editor":          "predefinedRoles/editor",
		"admin":           "predefinedRoles/admin",
		"no-cost-data":    "predefinedRoles/no-cost-data",
		"no-revenue-data": "predefinedRoles/no-revenue-data",
	}
	parts := strings.Split(value, ",")
	roles := make([]string, 0, len(parts))
	for _, part := range parts {
		role := strings.TrimSpace(part)
		if role == "" {
			return nil, usage("--roles must contain one or more comma-separated roles")
		}
		short := strings.TrimPrefix(role, "predefinedRoles/")
		normalized, ok := valid[short]
		if !ok {
			return nil, usagef("unsupported role %q (expected viewer, analyst, editor, admin, no-cost-data, or no-revenue-data)", role)
		}
		roles = append(roles, normalized)
	}
	return roles, nil
}

func listAnalyticsAccessBindings(svc *analyticsadmin.Service, parent string, maxResults int64, page string) (*analyticsadmin.GoogleAnalyticsAdminV1alphaListAccessBindingsResponse, error) {
	if strings.HasPrefix(parent, "accounts/") {
		call := svc.Accounts.AccessBindings.List(parent).PageSize(maxResults)
		if page != "" {
			call = call.PageToken(page)
		}
		return call.Do()
	}
	call := svc.Properties.AccessBindings.List(parent).PageSize(maxResults)
	if page != "" {
		call = call.PageToken(page)
	}
	return call.Do()
}

type AAAccessBindingsListCmd struct {
	Parent string `arg:"" name:"parent" help:"Parent resource (accounts/123 or properties/456)"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page   string `name:"page" help:"Page token"`
}

func (c *AAAccessBindingsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := analyticsAccessBindingParent(c.Parent)
	if err != nil {
		return err
	}
	svc, err := newAnalyticsAdminAlphaService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := listAnalyticsAccessBindings(svc, parent, c.Max, strings.TrimSpace(c.Page))
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"accessBindings": resp.AccessBindings, "nextPageToken": resp.NextPageToken})
	}
	u := ui.FromContext(ctx)
	if len(resp.AccessBindings) == 0 {
		u.Err().Println("No access bindings")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tUSER\tROLES")
	for _, binding := range resp.AccessBindings {
		fmt.Fprintf(w, "%s\t%s\t%s\n", binding.Name, binding.User, strings.Join(binding.Roles, ","))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type AAAccessBindingsCreateCmd struct {
	Parent string `arg:"" name:"parent" help:"Parent resource (accounts/123 or properties/456)"`
	Email  string `name:"email" required:"" help:"User email address"`
	Roles  string `name:"roles" required:"" help:"Comma-separated roles: viewer, analyst, editor, admin, no-cost-data, no-revenue-data"`
}

func (c *AAAccessBindingsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := analyticsAccessBindingParent(c.Parent)
	if err != nil {
		return err
	}
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("--email required")
	}
	roles, err := analyticsAccessBindingRoles(c.Roles)
	if err != nil {
		return err
	}
	svc, err := newAnalyticsAdminAlphaService(ctx, account)
	if err != nil {
		return err
	}
	binding := &analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding{User: email, Roles: roles}
	var created *analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding
	if strings.HasPrefix(parent, "accounts/") {
		created, err = svc.Accounts.AccessBindings.Create(parent, binding).Do()
	} else {
		created, err = svc.Properties.AccessBindings.Create(parent, binding).Do()
	}
	if err != nil {
		return err
	}
	return writeAnalyticsAccessBinding(ctx, "Created", created)
}

type AAAccessBindingsPatchCmd struct {
	Name  string `arg:"" name:"name" help:"Access binding resource name"`
	Roles string `name:"roles" required:"" help:"Comma-separated roles"`
}

func (c *AAAccessBindingsPatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	name, err := analyticsAccessBindingName(c.Name)
	if err != nil {
		return err
	}
	roles, err := analyticsAccessBindingRoles(c.Roles)
	if err != nil {
		return err
	}
	svc, err := newAnalyticsAdminAlphaService(ctx, account)
	if err != nil {
		return err
	}
	binding := &analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding{Roles: roles}
	var updated *analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding
	if strings.HasPrefix(name, "accounts/") {
		updated, err = svc.Accounts.AccessBindings.Patch(name, binding).Do()
	} else {
		updated, err = svc.Properties.AccessBindings.Patch(name, binding).Do()
	}
	if err != nil {
		return err
	}
	return writeAnalyticsAccessBinding(ctx, "Updated", updated)
}

type AAAccessBindingsDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Access binding resource name"`
}

func (c *AAAccessBindingsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	name, err := analyticsAccessBindingName(c.Name)
	if err != nil {
		return err
	}
	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete analytics access binding %s", name)); err != nil {
		return err
	}
	svc, err := newAnalyticsAdminAlphaService(ctx, account)
	if err != nil {
		return err
	}
	if strings.HasPrefix(name, "accounts/") {
		_, err = svc.Accounts.AccessBindings.Delete(name).Do()
	} else {
		_, err = svc.Properties.AccessBindings.Delete(name).Do()
	}
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "name": name})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "DELETED\tNAME")
		fmt.Fprintf(w, "true\t%s\n", name)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("Deleted access binding: %s", name)
	return nil
}

func writeAnalyticsAccessBinding(ctx context.Context, action string, binding *analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"accessBinding": binding})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "NAME\tUSER\tROLES")
		fmt.Fprintf(w, "%s\t%s\t%s\n", binding.Name, binding.User, strings.Join(binding.Roles, ","))
		return nil
	}
	ui.FromContext(ctx).Out().Printf("%s access binding: %s", action, binding.Name)
	return nil
}
