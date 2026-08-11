package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type tagManagerWorkspaceFlags struct {
	AccountID   string `name:"account-id" required:"" help:"GTM account ID"`
	ContainerID string `name:"container-id" required:"" help:"GTM container ID"`
	WorkspaceID string `name:"workspace-id" help:"GTM workspace ID" default:"0"`
}

func tagManagerResourceRequest(flags *RootFlags, value, resource, placeholder string) (string, string, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", "", err
	}
	path, err := tagManagerWorkspaceEntityPath(value, resource, placeholder)
	if err != nil {
		return "", "", err
	}
	return account, path, nil
}

func (f tagManagerWorkspaceFlags) parent() (string, error) {
	accountID := strings.TrimSpace(f.AccountID)
	containerID := strings.TrimSpace(f.ContainerID)
	workspaceID := strings.TrimSpace(f.WorkspaceID)
	if accountID == "" || strings.Contains(accountID, "/") {
		return "", usage("--account-id must be a GTM account ID")
	}
	if containerID == "" || strings.Contains(containerID, "/") {
		return "", usage("--container-id must be a GTM container ID")
	}
	if workspaceID == "" || strings.Contains(workspaceID, "/") {
		return "", usage("--workspace-id must be a GTM workspace ID")
	}

	return gtmWorkspacePath(accountID, containerID, workspaceID), nil
}

func tagManagerWorkspaceEntityPath(value, resource, placeholder string) (string, error) {
	path := strings.TrimSpace(value)
	parts := strings.Split(path, "/")
	if len(parts) != 8 || parts[0] != tagManagerAccountsPathSegment || strings.TrimSpace(parts[1]) == "" ||
		parts[2] != "containers" || strings.TrimSpace(parts[3]) == "" ||
		parts[4] != "workspaces" || strings.TrimSpace(parts[5]) == "" ||
		parts[6] != resource || strings.TrimSpace(parts[7]) == "" {
		return "", usagef("path must be accounts/ACCOUNT_ID/containers/CONTAINER_ID/workspaces/WORKSPACE_ID/%s/%s", resource, placeholder)
	}

	return path, nil
}

func parseTagManagerJSONObjects[T any](flagName string, values []string) ([]*T, error) {
	items := make([]*T, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "{") {
			return nil, usagef("--%s must be a JSON object", flagName)
		}
		var item T
		if err := json.Unmarshal([]byte(value), &item); err != nil {
			return nil, usagef("invalid --%s JSON: %v", flagName, err)
		}
		items = append(items, &item)
	}

	return items, nil
}

func parseTagManagerJSONObject[T any](flagName, value string) (*T, error) {
	items, err := parseTagManagerJSONObjects[T](flagName, []string{value})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

func writeTagManagerDelete(ctx context.Context, resource, path string) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"deleted": true, "path": path})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "DELETED\tPATH")
		fmt.Fprintf(w, "true\t%s\n", path)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("Deleted %s: %s", resource, path)
	return nil
}
