package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TagManagerVariablesCreateCmd struct {
	tagManagerWorkspaceFlags
	Name       string   `name:"name" required:"" help:"Variable name"`
	Type       string   `name:"type" required:"" help:"GTM variable type"`
	Parameters []string `name:"parameter" sep:"none" help:"GTM Parameter JSON object (repeatable)"`
}

func (c *TagManagerVariablesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := c.parent()
	if err != nil {
		return err
	}
	name, variableType := strings.TrimSpace(c.Name), strings.TrimSpace(c.Type)
	if name == "" || variableType == "" {
		return usage("--name and --type must not be empty")
	}
	parameters, err := parseTagManagerJSONObjects[tagmanager.Parameter]("parameter", c.Parameters)
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	variable, err := svc.Accounts.Containers.Workspaces.Variables.Create(parent, &tagmanager.Variable{
		Name: name, Type: variableType, Parameter: parameters,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerVariable(ctx, "Created", variable)
}

type TagManagerVariablesGetCmd struct {
	Path string `arg:"" name:"path" help:"Full GTM variable path"`
}

func (c *TagManagerVariablesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "variables", "VARIABLE_ID")
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	variable, err := svc.Accounts.Containers.Workspaces.Variables.Get(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerVariable(ctx, "Variable", variable)
}

type TagManagerVariablesDeleteCmd struct {
	Path string `arg:"" name:"path" help:"Full GTM variable path"`
}

func (c *TagManagerVariablesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "variables", "VARIABLE_ID")
	if err != nil {
		return err
	}
	if err = confirmDestructive(ctx, flags, "delete GTM variable "+path); err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	if err = svc.Accounts.Containers.Workspaces.Variables.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	return writeTagManagerDelete(ctx, "variable", path)
}

type TagManagerVariablesRevertCmd struct {
	Path string `arg:"" name:"path" help:"Full GTM variable path"`
}

func (c *TagManagerVariablesRevertCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "variables", "VARIABLE_ID")
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Accounts.Containers.Workspaces.Variables.Revert(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerVariable(ctx, "Reverted", resp.Variable)
}

type TagManagerVariablesUpdateCmd struct {
	Path       string   `arg:"" name:"path" help:"Full GTM variable path"`
	Name       string   `name:"name" help:"Variable name"`
	Type       string   `name:"type" help:"GTM variable type"`
	Parameters []string `name:"parameter" sep:"none" help:"GTM Parameter JSON object (repeatable)"`
}

func (c *TagManagerVariablesUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "variables", "VARIABLE_ID")
	if err != nil {
		return err
	}
	if !flagProvidedAny(kctx, "name", "type", "parameter") {
		return usage("at least one of --name, --type, or --parameter is required")
	}
	if flagProvided(kctx, "name") && strings.TrimSpace(c.Name) == "" {
		return usage("--name must not be blank")
	}
	if flagProvided(kctx, "type") && strings.TrimSpace(c.Type) == "" {
		return usage("--type must not be blank")
	}
	var parameters []*tagmanager.Parameter
	if flagProvided(kctx, "parameter") {
		parameters, err = parseTagManagerJSONObjects[tagmanager.Parameter]("parameter", c.Parameters)
		if err != nil {
			return err
		}
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	variable, err := svc.Accounts.Containers.Workspaces.Variables.Get(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	if flagProvided(kctx, "name") {
		variable.Name = strings.TrimSpace(c.Name)
	}
	if flagProvided(kctx, "type") {
		variable.Type = strings.TrimSpace(c.Type)
	}
	if flagProvided(kctx, "parameter") {
		variable.Parameter = parameters
	}
	updated, err := svc.Accounts.Containers.Workspaces.Variables.Update(path, variable).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerVariable(ctx, "Updated", updated)
}

func writeTagManagerVariable(ctx context.Context, action string, variable *tagmanager.Variable) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"variable": variable})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "VARIABLE_ID\tNAME\tTYPE")
		if variable != nil {
			fmt.Fprintf(w, "%s\t%s\t%s\n", sanitizeTab(variable.VariableId), sanitizeTab(variable.Name), sanitizeTab(variable.Type))
		}
		return nil
	}
	if variable == nil {
		ui.FromContext(ctx).Out().Printf("%s variable: no published version", action)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("%s variable: %s", action, variable.Path)
	return nil
}
