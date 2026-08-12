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

type TagManagerWorkspacesCmd struct {
	CreateVersion TagManagerWorkspacesCreateVersionCmd `cmd:"" name:"create-version" help:"Create a container version from a workspace"`
}

type TagManagerWorkspacesCreateVersionCmd struct {
	Path  string `arg:"" optional:"" name:"path" help:"Full GTM workspace path"`
	Name  string `name:"name" help:"Container version name"`
	Notes string `name:"notes" help:"Container version notes"`
}

type TagManagerVersionsPublishCmd struct {
	Path        string `arg:"" optional:"" name:"path" help:"Full GTM container version path"`
	Fingerprint string `name:"fingerprint" help:"Expected container version fingerprint"`
}

const tagManagerAccountsPathSegment = "accounts"

func tagManagerWorkspaceResourcePath(value string) (string, error) {
	path := strings.TrimSpace(value)
	parts := strings.Split(path, "/")
	if len(parts) != 6 || parts[0] != tagManagerAccountsPathSegment || strings.TrimSpace(parts[1]) == "" ||
		parts[2] != "containers" || strings.TrimSpace(parts[3]) == "" ||
		parts[4] != "workspaces" || strings.TrimSpace(parts[5]) == "" {
		return "", usage("path must be accounts/ACCOUNT_ID/containers/CONTAINER_ID/workspaces/WORKSPACE_ID")
	}

	return path, nil
}

func tagManagerContainerVersionResourcePath(value string) (string, error) {
	path := strings.TrimSpace(value)
	parts := strings.Split(path, "/")
	if len(parts) != 6 || parts[0] != tagManagerAccountsPathSegment || strings.TrimSpace(parts[1]) == "" ||
		parts[2] != "containers" || strings.TrimSpace(parts[3]) == "" ||
		parts[4] != "versions" || strings.TrimSpace(parts[5]) == "" {
		return "", usage("path must be accounts/ACCOUNT_ID/containers/CONTAINER_ID/versions/VERSION_ID")
	}

	return path, nil
}

func (c *TagManagerWorkspacesCreateVersionCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	path, err := tagManagerWorkspaceResourcePath(c.Path)
	if err != nil {
		return err
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.Containers.Workspaces.CreateVersion(path, &tagmanager.CreateContainerVersionRequestVersionOptions{
		Name:  strings.TrimSpace(c.Name),
		Notes: strings.TrimSpace(c.Notes),
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	versionPath := ""
	versionID := ""
	versionName := ""
	fingerprint := ""
	if resp.ContainerVersion != nil {
		versionPath = resp.ContainerVersion.Path
		versionID = resp.ContainerVersion.ContainerVersionId
		versionName = resp.ContainerVersion.Name
		fingerprint = resp.ContainerVersion.Fingerprint
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, outfmt.DirectResult(map[string]any{
			"compilerError":    resp.CompilerError,
			"containerVersion": resp.ContainerVersion,
			"newWorkspacePath": resp.NewWorkspacePath,
			"path":             versionPath,
		}))
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "PATH\tVERSION_ID\tNAME\tFINGERPRINT\tNEW_WORKSPACE_PATH\tCOMPILER_ERROR")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\n",
			sanitizeTab(versionPath), sanitizeTab(versionID), sanitizeTab(versionName), sanitizeTab(fingerprint),
			sanitizeTab(resp.NewWorkspacePath), resp.CompilerError)

		return nil
	}

	ui.FromContext(ctx).Out().Printf("Created container version: %s", versionPath)

	return nil
}

func (c *TagManagerVersionsPublishCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	path, err := tagManagerContainerVersionResourcePath(c.Path)
	if err != nil {
		return err
	}

	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Containers.Versions.Publish(path).Context(ctx)
	if fingerprint := strings.TrimSpace(c.Fingerprint); fingerprint != "" {
		call = call.Fingerprint(fingerprint)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	versionPath := ""
	versionID := ""
	versionName := ""
	fingerprint := ""
	if resp.ContainerVersion != nil {
		versionPath = resp.ContainerVersion.Path
		versionID = resp.ContainerVersion.ContainerVersionId
		versionName = resp.ContainerVersion.Name
		fingerprint = resp.ContainerVersion.Fingerprint
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, outfmt.DirectResult(map[string]any{
			"compilerError":    resp.CompilerError,
			"containerVersion": resp.ContainerVersion,
			"path":             versionPath,
		}))
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "PATH\tVERSION_ID\tNAME\tFINGERPRINT\tCOMPILER_ERROR")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\n",
			sanitizeTab(versionPath), sanitizeTab(versionID), sanitizeTab(versionName), sanitizeTab(fingerprint), resp.CompilerError)

		return nil
	}

	ui.FromContext(ctx).Out().Printf("Published container version: %s", versionPath)

	return nil
}
