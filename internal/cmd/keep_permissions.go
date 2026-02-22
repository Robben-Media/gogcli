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

// KeepPermissionsCmd contains subcommands for note permissions.
type KeepPermissionsCmd struct {
	BatchCreate KeepPermissionsBatchCreateCmd `cmd:"" name:"batch-create" help:"Share a note with multiple users"`
	BatchDelete KeepPermissionsBatchDeleteCmd `cmd:"" name:"batch-delete" help:"Remove sharing from a note"`
}

// KeepPermissionsBatchCreateCmd shares a note with multiple users.
type KeepPermissionsBatchCreateCmd struct {
	Parent  string   `arg:"" name:"parent" help:"Note resource name (e.g. notes/abc123)"`
	Members []string `name:"members" help:"Email addresses to share with" required:""`
	Role    string   `name:"role" help:"Permission role (only WRITER is supported)" default:"WRITER"`
}

func (c *KeepPermissionsBatchCreateCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("empty parent")
	}
	if !strings.HasPrefix(parent, "notes/") {
		parent = "notes/" + parent
	}

	if len(c.Members) == 0 {
		return usage("at least one --members email is required")
	}

	role := strings.TrimSpace(c.Role)
	if role == "" {
		role = "WRITER"
	}
	if role != "WRITER" {
		return usage("only WRITER role is supported for Keep permissions")
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	// Build batch create request
	req := &keepapi.BatchCreatePermissionsRequest{
		Requests: make([]*keepapi.CreatePermissionRequest, len(c.Members)),
	}
	for i, email := range c.Members {
		email = strings.TrimSpace(email)
		if email == "" {
			return usage("empty email in --members")
		}
		req.Requests[i] = &keepapi.CreatePermissionRequest{
			Parent: parent,
			Permission: &keepapi.Permission{
				Email: email,
				Role:  role,
			},
		}
	}

	resp, err := svc.Notes.Permissions.BatchCreate(parent, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"permissions": resp.Permissions})
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "MEMBER\tROLE\tNAME")
	for _, p := range resp.Permissions {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Email, p.Role, p.Name)
	}
	return nil
}

// KeepPermissionsBatchDeleteCmd removes sharing from a note.
type KeepPermissionsBatchDeleteCmd struct {
	Parent          string   `arg:"" name:"parent" help:"Note resource name (e.g. notes/abc123)"`
	PermissionNames []string `name:"permission-names" help:"Permission resource names to delete (e.g. notes/abc/permissions/xyz)" required:""`
}

func (c *KeepPermissionsBatchDeleteCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	u := ui.FromContext(ctx)

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("empty parent")
	}
	if !strings.HasPrefix(parent, "notes/") {
		parent = "notes/" + parent
	}

	if len(c.PermissionNames) == 0 {
		return usage("at least one --permission-names is required")
	}

	// Validate and normalize permission names
	names := make([]string, len(c.PermissionNames))
	for i, name := range c.PermissionNames {
		name = strings.TrimSpace(name)
		if name == "" {
			return usage("empty permission name in --permission-names")
		}
		// If short form provided (just the ID), expand to full name
		if !strings.Contains(name, "/permissions/") {
			name = parent + "/permissions/" + name
		}
		names[i] = name
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove %d permission(s) from note %s", len(names), parent)); confErr != nil {
		return confErr
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	req := &keepapi.BatchDeletePermissionsRequest{
		Names: names,
	}

	if _, delErr := svc.Notes.Permissions.BatchDelete(parent, req).Context(ctx).Do(); delErr != nil {
		return delErr
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"parent":  parent,
			"count":   len(names),
		})
	}
	u.Err().Printf("Removed %d permission(s) from %s", len(names), parent)
	return nil
}
