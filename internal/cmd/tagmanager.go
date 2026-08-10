package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newTagManagerService = googleapi.NewTagManager

const tagManagerParameterTypeList = "list"

type TagManagerCmd struct {
	Accounts         TagManagerAccountsCmd         `cmd:"" name:"accounts" group:"Read" help:"List GTM accounts"`
	BuiltInVariables TagManagerBuiltInVariablesCmd `cmd:"" name:"built-in-variables" group:"Write" help:"Manage built-in variables in a workspace"`
	Containers       TagManagerContainersCmd       `cmd:"" name:"containers" group:"Read" help:"List containers in an account"`
	Tags             TagManagerTagsCmd             `cmd:"" name:"tags" group:"Read" help:"List tags in a workspace"`
	Tag              TagManagerTagCmd              `cmd:"" name:"tag" group:"Read" help:"Get a single tag by path"`
	Triggers         TagManagerTriggersCmd         `cmd:"" name:"triggers" group:"Write" help:"Manage triggers in a workspace"`
	Variables        TagManagerVariablesCmd        `cmd:"" name:"variables" group:"Write" help:"Manage variables in a workspace"`
	Versions         TagManagerVersionsCmd         `cmd:"" name:"versions" group:"Read" help:"List container version headers"`
	Workspaces       TagManagerWorkspacesCmd       `cmd:"" name:"workspaces" group:"Write" help:"Manage GTM workspaces"`
	UserPermissions  TagManagerUserPermissionsCmd  `cmd:"" name:"user-permissions" group:"Admin" help:"Manage GTM user permissions"`
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
	if outfmt.IsPlain(ctx) {
		return writeTagManagerTagPlain(ctx, tag)
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

// writeTagManagerTagPlain emits stable TSV for a single tag detail:
// RECORD_TYPE<TAB>TAG_ID<TAB>KEY<TAB>TYPE<TAB>VALUE
// with one row per metadata field, firing/blocking trigger, and parameter leaf.
func writeTagManagerTagPlain(ctx context.Context, tag *tagmanager.Tag) error {
	w, flush := tableWriter(ctx)
	defer flush()
	writeTableRow(ctx, w, []string{"RECORD_TYPE", "TAG_ID", "KEY", "TYPE", "VALUE"})
	if tag == nil {
		return nil
	}
	tagID := tag.TagId
	writeTableRow(ctx, w, []string{"METADATA", tagID, "name", "", tag.Name})
	writeTableRow(ctx, w, []string{"METADATA", tagID, "type", "", tag.Type})
	for _, id := range tag.FiringTriggerId {
		writeTableRow(ctx, w, []string{"FIRING_TRIGGER", tagID, "", "", id})
	}
	for _, id := range tag.BlockingTriggerId {
		writeTableRow(ctx, w, []string{"BLOCKING_TRIGGER", tagID, "", "", id})
	}
	for _, p := range tag.Parameter {
		writeTagManagerParameterLeaves(ctx, w, tagID, "", p, false)
	}
	return nil
}

func tagManagerParamPath(prefix, segment string) string {
	if segment == "" {
		return prefix
	}
	segment = strings.ReplaceAll(segment, `\`, `\\`)
	segment = strings.ReplaceAll(segment, ".", `\.`)
	segment = strings.ReplaceAll(segment, "[", `\[`)
	segment = strings.ReplaceAll(segment, "]", `\]`)
	if prefix == "" {
		return segment
	}
	return prefix + "." + segment
}

func tagManagerParamListPath(prefix string, index int) string {
	return prefix + "[" + strconv.Itoa(index) + "]"
}

// writeTagManagerParameterLeaves flattens nested GTM parameters into leaf rows.
// Map keys are dot-separated with backslash-escaped `\\.[]`; list indexes use
// brackets, producing unambiguous paths such as list[0].mapKey. ignoreKey is set
// for list children because GTM ignores keys on list values.
func writeTagManagerParameterLeaves(ctx context.Context, w io.Writer, tagID, prefix string, p *tagmanager.Parameter, ignoreKey bool) {
	if p == nil {
		return
	}
	segment := p.Key
	if ignoreKey {
		segment = ""
	}
	path := tagManagerParamPath(prefix, segment)
	paramType := strings.ToLower(p.Type)
	switch {
	case len(p.List) > 0 || paramType == tagManagerParameterTypeList:
		if len(p.List) == 0 {
			writeTableRow(ctx, w, []string{"PARAMETER", tagID, path, p.Type, p.Value})
			return
		}
		for i, child := range p.List {
			childPrefix := tagManagerParamListPath(path, i)
			writeTagManagerParameterLeaves(ctx, w, tagID, childPrefix, child, true)
		}
	case len(p.Map) > 0 || paramType == "map":
		if len(p.Map) == 0 {
			writeTableRow(ctx, w, []string{"PARAMETER", tagID, path, p.Type, p.Value})
			return
		}
		for _, child := range p.Map {
			writeTagManagerParameterLeaves(ctx, w, tagID, path, child, false)
		}
	default:
		writeTableRow(ctx, w, []string{"PARAMETER", tagID, path, p.Type, p.Value})
	}
}

// --- triggers ---

type TagManagerTriggersCmd struct {
	List   TagManagerTriggersListCmd   `cmd:"" default:"withargs" help:"List triggers in a workspace"`
	Create TagManagerTriggersCreateCmd `cmd:"" name:"create" help:"Create a trigger"`
	Delete TagManagerTriggersDeleteCmd `cmd:"" name:"delete" help:"Delete a trigger"`
	Get    TagManagerTriggersGetCmd    `cmd:"" name:"get" help:"Get a trigger"`
	Revert TagManagerTriggersRevertCmd `cmd:"" name:"revert" help:"Revert a trigger"`
	Update TagManagerTriggersUpdateCmd `cmd:"" name:"update" help:"Update a trigger"`
}

type TagManagerTriggersListCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
	WorkspaceID string `name:"workspace-id" help:"GTM workspace ID (default: 0)" default:"0"`
}

func (c *TagManagerTriggersListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	List   TagManagerVariablesListCmd   `cmd:"" default:"withargs" help:"List variables in a workspace"`
	Create TagManagerVariablesCreateCmd `cmd:"" name:"create" help:"Create a variable"`
	Delete TagManagerVariablesDeleteCmd `cmd:"" name:"delete" help:"Delete a variable"`
	Get    TagManagerVariablesGetCmd    `cmd:"" name:"get" help:"Get a variable"`
	Revert TagManagerVariablesRevertCmd `cmd:"" name:"revert" help:"Revert a variable"`
	Update TagManagerVariablesUpdateCmd `cmd:"" name:"update" help:"Update a variable"`
}

type TagManagerVariablesListCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
	WorkspaceID string `name:"workspace-id" help:"GTM workspace ID (default: 0)" default:"0"`
}

func (c *TagManagerVariablesListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	List    TagManagerVersionsListCmd    `cmd:"" default:"withargs" help:"List container version headers"`
	Publish TagManagerVersionsPublishCmd `cmd:"" name:"publish" help:"Publish a container version"`
}

type TagManagerVersionsListCmd struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
}

func (c *TagManagerVersionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
