package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newTagManagerService = googleapi.NewTagManager

type TagManagerCmd struct {
	Accounts   TagManagerAccountsCmd   `cmd:"" name:"accounts" group:"Read" help:"List GTM accounts"`
	Containers TagManagerContainersCmd `cmd:"" name:"containers" group:"Read" help:"List containers in an account"`
	Tags       TagManagerTagsCmd       `cmd:"" name:"tags" group:"Read" help:"List tags in a workspace"`
	Tag        TagManagerTagCmd        `cmd:"" name:"tag" group:"Read" help:"Get a single tag by path"`
	Triggers   TagManagerTriggersCmd   `cmd:"" name:"triggers" group:"Read" help:"List triggers in a workspace"`
	Variables  TagManagerVariablesCmd  `cmd:"" name:"variables" group:"Read" help:"List variables in a workspace"`
	Versions   TagManagerVersionsCmd   `cmd:"" name:"versions" group:"Read" help:"List container version headers"`
}

func gtmWorkspacePath(accountID, containerID, workspaceID string) string {
	return "accounts/" + accountID + "/containers/" + containerID + "/workspaces/" + workspaceID
}

// --- accounts ---

type TagManagerAccountsCmd struct{}

func (c *TagManagerAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.List().Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"accounts": resp.Account,
		})
	}

	if len(resp.Account) == 0 {
		u.Err().Println("No accounts")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ACCOUNT_ID\tNAME")
	for _, a := range resp.Account {
		fmt.Fprintf(w, "%s\t%s\n", a.AccountId, a.Name)
	}
	return nil
}

// --- containers ---

type TagManagerContainersCmd struct {
	AccountID string `name:"account-id" required:"" help:"GTM account ID"`
}

func (c *TagManagerContainersCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	accountID := strings.TrimSpace(c.AccountID)
	if accountID == "" {
		return usage("--account-id required")
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.Containers.List("accounts/" + accountID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"containers": resp.Container,
		})
	}

	if len(resp.Container) == 0 {
		u.Err().Println("No containers")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CONTAINER_ID\tNAME\tPUBLIC_ID")
	for _, ct := range resp.Container {
		fmt.Fprintf(w, "%s\t%s\t%s\n", ct.ContainerId, ct.Name, ct.PublicId)
	}
	return nil
}

// --- tags ---

type TagManagerTagsCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
	WorkspaceID string `name:"workspace-id" help:"GTM workspace ID (default: 0)" default:"0"`
}

func (c *TagManagerTagsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.AccountID) == "" {
		return usage("--account-id required")
	}
	if strings.TrimSpace(c.ContainerID) == "" {
		return usage("--container-id required")
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	parent := gtmWorkspacePath(c.AccountID, c.ContainerID, c.WorkspaceID)
	resp, err := svc.Accounts.Containers.Workspaces.Tags.List(parent).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"tags": resp.Tag,
		})
	}

	if len(resp.Tag) == 0 {
		u.Err().Println("No tags")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TAG_ID\tNAME\tTYPE")
	for _, tag := range resp.Tag {
		fmt.Fprintf(w, "%s\t%s\t%s\n", tag.TagId, tag.Name, tag.Type)
	}
	return nil
}

// --- tag (single) ---

type TagManagerTagCmd struct {
	TagPath string `arg:"" name:"tagPath" help:"Full GTM tag path (e.g. accounts/123/containers/456/workspaces/0/tags/789)"`
}

func (c *TagManagerTagCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tagPath := strings.TrimSpace(c.TagPath)
	if tagPath == "" {
		return usage("tagPath required")
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	tag, err := svc.Accounts.Containers.Workspaces.Tags.Get(tagPath).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"tag": tag})
	}

	u.Out().Printf("tagId\t%s", tag.TagId)
	u.Out().Printf("name\t%s", tag.Name)
	u.Out().Printf("type\t%s", tag.Type)
	if len(tag.FiringTriggerId) > 0 {
		u.Out().Printf("firingTriggerIds\t%s", strings.Join(tag.FiringTriggerId, ", "))
	}
	if len(tag.BlockingTriggerId) > 0 {
		u.Out().Printf("blockingTriggerIds\t%s", strings.Join(tag.BlockingTriggerId, ", "))
	}
	if len(tag.Parameter) > 0 {
		u.Out().Printf("parameters\t(%d parameters)", len(tag.Parameter))
		for _, p := range tag.Parameter {
			u.Out().Printf("  %s\t%s", p.Key, p.Value)
		}
	}
	return nil
}

// --- triggers ---

type TagManagerTriggersCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
	WorkspaceID string `name:"workspace-id" help:"GTM workspace ID (default: 0)" default:"0"`
}

func (c *TagManagerTriggersCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.AccountID) == "" {
		return usage("--account-id required")
	}
	if strings.TrimSpace(c.ContainerID) == "" {
		return usage("--container-id required")
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	parent := gtmWorkspacePath(c.AccountID, c.ContainerID, c.WorkspaceID)
	resp, err := svc.Accounts.Containers.Workspaces.Triggers.List(parent).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"triggers": resp.Trigger,
		})
	}

	if len(resp.Trigger) == 0 {
		u.Err().Println("No triggers")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TRIGGER_ID\tNAME\tTYPE")
	for _, tr := range resp.Trigger {
		fmt.Fprintf(w, "%s\t%s\t%s\n", tr.TriggerId, tr.Name, tr.Type)
	}
	return nil
}

// --- variables ---

type TagManagerVariablesCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
	WorkspaceID string `name:"workspace-id" help:"GTM workspace ID (default: 0)" default:"0"`
}

func (c *TagManagerVariablesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.AccountID) == "" {
		return usage("--account-id required")
	}
	if strings.TrimSpace(c.ContainerID) == "" {
		return usage("--container-id required")
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	parent := gtmWorkspacePath(c.AccountID, c.ContainerID, c.WorkspaceID)
	resp, err := svc.Accounts.Containers.Workspaces.Variables.List(parent).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"variables": resp.Variable,
		})
	}

	if len(resp.Variable) == 0 {
		u.Err().Println("No variables")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "VARIABLE_ID\tNAME\tTYPE")
	for _, v := range resp.Variable {
		fmt.Fprintf(w, "%s\t%s\t%s\n", v.VariableId, v.Name, v.Type)
	}
	return nil
}

// --- versions ---

type TagManagerVersionsCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
}

func (c *TagManagerVersionsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.AccountID) == "" {
		return usage("--account-id required")
	}
	if strings.TrimSpace(c.ContainerID) == "" {
		return usage("--container-id required")
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	parent := "accounts/" + c.AccountID + "/containers/" + c.ContainerID
	resp, err := svc.Accounts.Containers.VersionHeaders.List(parent).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"versionHeaders": resp.ContainerVersionHeader,
		})
	}

	if len(resp.ContainerVersionHeader) == 0 {
		u.Err().Println("No versions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "VERSION_ID\tNAME\tTAGS\tTRIGGERS\tVARIABLES")
	for _, vh := range resp.ContainerVersionHeader {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			vh.ContainerVersionId, vh.Name, vh.NumTags, vh.NumTriggers, vh.NumVariables)
	}
	return nil
}

// Ensure tagmanager.Service is used to avoid import cycle lint errors.
var _ *tagmanager.Service
