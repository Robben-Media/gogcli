package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	keepapi "google.golang.org/api/keep/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// KeepPermissionsCmd is the parent command for Keep permission operations.
type KeepPermissionsCmd struct {
	BatchCreate KeepPermissionsBatchCreateCmd `cmd:"" name:"batch-create" help:"Share a note with multiple users"`
	BatchDelete KeepPermissionsBatchDeleteCmd `cmd:"" name:"batch-delete" help:"Remove sharing permissions from a note"`
}

// KeepPermissionsBatchCreateCmd shares a note with multiple users in a single batch request.
type KeepPermissionsBatchCreateCmd struct {
	Parent  string   `arg:"" name:"parent" help:"Note resource name (e.g. notes/abc123)"`
	Members []string `name:"members" required:"" help:"Email addresses to share with"`
	Role    string   `name:"role" help:"Permission role" default:"WRITER"`
}

func (c *KeepPermissionsBatchCreateCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	u := ui.FromContext(ctx)

	if len(c.Members) == 0 {
		return usage("at least one --members is required")
	}

	// Keep API only supports WRITER role
	role := strings.TrimSpace(c.Role)
	if role == "" {
		role = "WRITER"
	}
	if role != "WRITER" {
		return usagef("Keep API only supports WRITER role, got %q", role)
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("parent cannot be empty")
	}
	if !strings.HasPrefix(parent, "notes/") {
		parent = "notes/" + parent
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	// Build batch request
	requests := make([]*keepapi.CreatePermissionRequest, len(c.Members))
	for i, email := range c.Members {
		email = strings.TrimSpace(email)
		if email == "" {
			return usagef("member email at index %d cannot be empty", i)
		}
		requests[i] = &keepapi.CreatePermissionRequest{
			Parent: parent,
			Permission: &keepapi.Permission{
				Email: email,
				Role:  role,
			},
		}
	}

	req := &keepapi.BatchCreatePermissionsRequest{
		Requests: requests,
	}

	resp, err := svc.Notes.Permissions.BatchCreate(parent, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("batch create permissions: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"permissions": resp.Permissions,
		})
	}

	// Text output
	if len(resp.Permissions) == 0 {
		u.Err().Println("No permissions created")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "MEMBER\tROLE\tNAME")
	for _, perm := range resp.Permissions {
		fmt.Fprintf(w, "%s\t%s\t%s\n", perm.Email, perm.Role, perm.Name)
	}
	return nil
}

// KeepPermissionsBatchDeleteCmd removes sharing permissions from a note in a single batch request.
type KeepPermissionsBatchDeleteCmd struct {
	Parent          string   `arg:"" name:"parent" help:"Note resource name (e.g. notes/abc123)"`
	PermissionNames []string `name:"permission-names" required:"" help:"Permission resource names to delete (e.g. notes/abc123/permissions/xyz)"`
}

func (c *KeepPermissionsBatchDeleteCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	u := ui.FromContext(ctx)

	if len(c.PermissionNames) == 0 {
		return usage("at least one --permission-names is required")
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("parent cannot be empty")
	}
	if !strings.HasPrefix(parent, "notes/") {
		parent = "notes/" + parent
	}

	// Validate permission names belong to this parent
	for i, name := range c.PermissionNames {
		name = strings.TrimSpace(name)
		if name == "" {
			return usagef("permission name at index %d cannot be empty", i)
		}
		// Allow short format (just permission ID) and convert to full name
		if !strings.Contains(name, "/permissions/") {
			c.PermissionNames[i] = parent + "/permissions/" + name
		}
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove %d permission(s) from note %s", len(c.PermissionNames), parent)); err != nil {
		return err
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	req := &keepapi.BatchDeletePermissionsRequest{
		Names: c.PermissionNames,
	}

	if _, err := svc.Notes.Permissions.BatchDelete(parent, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("batch delete permissions: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"count":   len(c.PermissionNames),
			"parent":  parent,
			"names":   c.PermissionNames,
		})
	}

	u.Err().Printf("Removed %d permission(s) from %s", len(c.PermissionNames), parent)
	return nil
}
