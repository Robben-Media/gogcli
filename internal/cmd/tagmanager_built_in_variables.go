package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TagManagerBuiltInVariablesCmd struct {
	Create TagManagerBuiltInVariablesCreateCmd `cmd:"" name:"create" help:"Enable built-in variables"`
	Delete TagManagerBuiltInVariablesDeleteCmd `cmd:"" name:"delete" help:"Disable built-in variables"`
	List   TagManagerBuiltInVariablesListCmd   `cmd:"" name:"list" help:"List enabled built-in variables"`
	Revert TagManagerBuiltInVariablesRevertCmd `cmd:"" name:"revert" help:"Revert a built-in variable"`
}

type TagManagerBuiltInVariablesCreateCmd struct {
	tagManagerWorkspaceFlags
	Types []string `name:"type" required:"" sep:"none" help:"Built-in variable type (repeatable)"`
}

func (c *TagManagerBuiltInVariablesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, parent, types, err := tagManagerBuiltInRequest(flags, c.tagManagerWorkspaceFlags, c.Types)
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Accounts.Containers.Workspaces.BuiltInVariables.Create(parent).Type(types...).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerBuiltInVariables(ctx, resp.BuiltInVariable, "")
}

type TagManagerBuiltInVariablesDeleteCmd struct {
	tagManagerWorkspaceFlags
	Types []string `name:"type" required:"" sep:"none" help:"Built-in variable type (repeatable)"`
}

func (c *TagManagerBuiltInVariablesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, parent, types, err := tagManagerBuiltInRequest(flags, c.tagManagerWorkspaceFlags, c.Types)
	if err != nil {
		return err
	}
	path := parent + "/built_in_variables"
	if err = confirmDestructive(ctx, flags, "disable GTM built-in variables "+strings.Join(types, ", ")); err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	if err = svc.Accounts.Containers.Workspaces.BuiltInVariables.Delete(path).Type(types...).Context(ctx).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"deleted": true, "path": path, "types": types})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "DELETED\tPATH\tTYPES")
		fmt.Fprintf(w, "true\t%s\t%s\n", path, strings.Join(types, ","))
		return nil
	}
	ui.FromContext(ctx).Out().Printf("Disabled built-in variables: %s", strings.Join(types, ", "))
	return nil
}

type TagManagerBuiltInVariablesListCmd struct {
	tagManagerWorkspaceFlags
	Page string `name:"page" help:"Page token"`
}

func (c *TagManagerBuiltInVariablesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := c.parent()
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	call := svc.Accounts.Containers.Workspaces.BuiltInVariables.List(parent).Context(ctx)
	if page := strings.TrimSpace(c.Page); page != "" {
		call = call.PageToken(page)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, resp.BuiltInVariable)
	}
	return writeTagManagerBuiltInVariables(ctx, resp.BuiltInVariable, resp.NextPageToken)
}

type TagManagerBuiltInVariablesRevertCmd struct {
	tagManagerWorkspaceFlags
	Types []string `name:"type" required:"" sep:"none" help:"Built-in variable type (exactly one)"`
}

func (c *TagManagerBuiltInVariablesRevertCmd) Run(ctx context.Context, flags *RootFlags) error {
	if len(c.Types) != 1 {
		return usage("revert requires exactly one --type")
	}
	account, parent, types, err := tagManagerBuiltInRequest(flags, c.tagManagerWorkspaceFlags, c.Types)
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Accounts.Containers.Workspaces.BuiltInVariables.Revert(parent).Type(types[0]).Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"type": types[0], "enabled": resp.Enabled})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "TYPE\tENABLED")
		fmt.Fprintf(w, "%s\t%t\n", types[0], resp.Enabled)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("Reverted built-in variable %s (enabled: %t)", types[0], resp.Enabled)
	return nil
}

func tagManagerBuiltInRequest(flags *RootFlags, workspace tagManagerWorkspaceFlags, values []string) (string, string, []string, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", "", nil, err
	}
	parent, err := workspace.parent()
	if err != nil {
		return "", "", nil, err
	}
	types := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return "", "", nil, usage("--type must not be blank")
		}
		types = append(types, value)
	}
	if len(types) == 0 {
		return "", "", nil, usage("at least one --type is required")
	}
	return account, parent, types, nil
}

func writeTagManagerBuiltInVariables(ctx context.Context, variables []*tagmanager.BuiltInVariable, nextPageToken string) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"builtInVariables": variables, "nextPageToken": nextPageToken})
	}
	if len(variables) == 0 {
		ui.FromContext(ctx).Err().Println("No built-in variables")
		return nil
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "TYPE\tNAME")
		for _, variable := range variables {
			fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(variable.Type), sanitizeTab(variable.Name))
		}
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TYPE\tNAME")
	for _, variable := range variables {
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(variable.Type), sanitizeTab(variable.Name))
	}
	printNextPageHint(ui.FromContext(ctx), nextPageToken)
	return nil
}
