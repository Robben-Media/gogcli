package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TagManagerUserPermissionsCmd struct {
	Create TagManagerUserPermissionsCreateCmd `cmd:"" name:"create" help:"Create a user permission"`
	Delete TagManagerUserPermissionsDeleteCmd `cmd:"" name:"delete" help:"Delete a user permission"`
	Get    TagManagerUserPermissionsGetCmd    `cmd:"" name:"get" help:"Get a user permission"`
	List   TagManagerUserPermissionsListCmd   `cmd:"" name:"list" help:"List user permissions"`
	Update TagManagerUserPermissionsUpdateCmd `cmd:"" name:"update" help:"Update a user permission"`
}

const (
	tagManagerAccountAccessNoAccess = "noAccess"
	tagManagerAccountAccessUser     = "user"
	tagManagerAccountAccessAdmin    = "admin"
)

func tagManagerAccountParent(accountID string) (string, error) {
	id := strings.TrimSpace(accountID)
	if id == "" {
		return "", usage("--account-id required")
	}
	if strings.Contains(id, "/") {
		return "", usage("--account-id must be a GTM account ID")
	}
	return "accounts/" + id, nil
}

func tagManagerUserPermissionPath(value string) (string, error) {
	path := strings.TrimSpace(value)
	parts := strings.Split(path, "/")
	if len(parts) != 4 || parts[0] != "accounts" || strings.TrimSpace(parts[1]) == "" || parts[2] != "user_permissions" || strings.TrimSpace(parts[3]) == "" {
		return "", usage("path must be accounts/ACCOUNT_ID/user_permissions/PERMISSION_ID")
	}
	return path, nil
}

func tagManagerAccountAccess(value string) (*tagmanager.AccountAccess, error) {
	permission := strings.TrimSpace(value)
	switch permission {
	case tagManagerAccountAccessNoAccess, tagManagerAccountAccessUser, tagManagerAccountAccessAdmin:
		return &tagmanager.AccountAccess{Permission: permission}, nil
	default:
		return nil, usagef("invalid --account-access-type %q (expected noAccess, user, or admin)", permission)
	}
}

func tagManagerContainerAccess(values []string) ([]*tagmanager.ContainerAccess, error) {
	access := make([]*tagmanager.ContainerAccess, 0, len(values))
	for _, value := range values {
		var item tagmanager.ContainerAccess
		if err := json.Unmarshal([]byte(value), &item); err != nil {
			return nil, usagef("invalid --container-access JSON: %v", err)
		}
		if strings.TrimSpace(item.ContainerId) == "" {
			return nil, usage("--container-access requires containerId")
		}
		access = append(access, &item)
	}
	return access, nil
}

type TagManagerUserPermissionsCreateCmd struct {
	AccountID       string   `name:"account-id" required:"" help:"GTM account ID"`
	Email           string   `name:"email" required:"" help:"User email address"`
	AccountAccess   string   `name:"account-access-type" help:"Account access: noAccess, user, or admin"`
	ContainerAccess []string `name:"container-access" sep:"none" help:"Container access JSON object (repeatable)"`
}

func (c *TagManagerUserPermissionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := tagManagerAccountParent(c.AccountID)
	if err != nil {
		return err
	}
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("--email required")
	}
	permission := &tagmanager.UserPermission{EmailAddress: email}
	if strings.TrimSpace(c.AccountAccess) != "" {
		permission.AccountAccess, err = tagManagerAccountAccess(c.AccountAccess)
		if err != nil {
			return err
		}
	}
	if len(c.ContainerAccess) > 0 {
		permission.ContainerAccess, err = tagManagerContainerAccess(c.ContainerAccess)
		if err != nil {
			return err
		}
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	created, err := svc.Accounts.UserPermissions.Create(parent, permission).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerUserPermission(ctx, "Created", created)
}

type TagManagerUserPermissionsDeleteCmd struct {
	Path string `arg:"" name:"path" help:"User permission path"`
}

func (c *TagManagerUserPermissionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	path, err := tagManagerUserPermissionPath(c.Path)
	if err != nil {
		return err
	}
	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete GTM user permission %s", path)); err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	if err = svc.Accounts.UserPermissions.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, outfmt.DirectResult(map[string]any{"deleted": true, "path": path}))
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "DELETED\tPATH")
		fmt.Fprintf(w, "true\t%s\n", path)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("Deleted user permission: %s", path)
	return nil
}

type TagManagerUserPermissionsGetCmd struct {
	Path string `arg:"" name:"path" help:"User permission path"`
}

func (c *TagManagerUserPermissionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	path, err := tagManagerUserPermissionPath(c.Path)
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	permission, err := svc.Accounts.UserPermissions.Get(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerUserPermission(ctx, "User", permission)
}

type TagManagerUserPermissionsListCmd struct {
	AccountID string `name:"account-id" required:"" help:"GTM account ID"`
	Page      string `name:"page" help:"Page token"`
}

func (c *TagManagerUserPermissionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := tagManagerAccountParent(c.AccountID)
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	call := svc.Accounts.UserPermissions.List(parent).Context(ctx)
	if page := strings.TrimSpace(c.Page); page != "" {
		call = call.PageToken(page)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, outfmt.PrimaryResult(map[string]any{"userPermissions": resp.UserPermission, "nextPageToken": resp.NextPageToken}, resp.UserPermission))
	}
	u := ui.FromContext(ctx)
	if len(resp.UserPermission) == 0 {
		u.Err().Println("No user permissions")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PERMISSION_ID\tEMAIL\tACCOUNT_ACCESS")
	for _, permission := range resp.UserPermission {
		accountAccess := ""
		if permission.AccountAccess != nil {
			accountAccess = permission.AccountAccess.Permission
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", tagManagerPermissionID(permission.Path), permission.EmailAddress, accountAccess)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type TagManagerUserPermissionsUpdateCmd struct {
	Path            string   `arg:"" name:"path" help:"User permission path"`
	AccountAccess   string   `name:"account-access-type" help:"Account access: noAccess, user, or admin"`
	ContainerAccess []string `name:"container-access" sep:"none" help:"Container access JSON object (repeatable)"`
}

func (c *TagManagerUserPermissionsUpdateCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	path, err := tagManagerUserPermissionPath(c.Path)
	if err != nil {
		return err
	}
	var accountAccess *tagmanager.AccountAccess
	var containerAccess []*tagmanager.ContainerAccess
	changed := false
	if flagProvided(kctx, "account-access-type") {
		accountAccess, err = tagManagerAccountAccess(c.AccountAccess)
		if err != nil {
			return err
		}
		changed = true
	}
	if flagProvided(kctx, "container-access") {
		containerAccess, err = tagManagerContainerAccess(c.ContainerAccess)
		if err != nil {
			return err
		}
		changed = true
	}
	if !changed {
		return usage("at least one of --account-access-type or --container-access is required")
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	permission, err := svc.Accounts.UserPermissions.Get(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	if accountAccess != nil {
		permission.AccountAccess = accountAccess
	}
	if containerAccess != nil {
		permission.ContainerAccess = containerAccess
	}
	updated, err := svc.Accounts.UserPermissions.Update(path, permission).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerUserPermission(ctx, "Updated", updated)
}

func tagManagerPermissionID(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

func writeTagManagerUserPermission(ctx context.Context, action string, permission *tagmanager.UserPermission) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, outfmt.PrimaryResult(map[string]any{"userPermission": permission}, permission))
	}
	accountAccess := ""
	if permission.AccountAccess != nil {
		accountAccess = permission.AccountAccess.Permission
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "PATH\tEMAIL\tACCOUNT_ACCESS")
		fmt.Fprintf(w, "%s\t%s\t%s\n", permission.Path, permission.EmailAddress, accountAccess)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("%s user permission: %s", action, permission.Path)
	return nil
}
