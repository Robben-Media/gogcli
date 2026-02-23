package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveDrivesAdminCmd is the parent command for shared drives admin operations
type DriveDrivesAdminCmd struct {
	Create DriveDrivesCreateCmd `cmd:"" name:"create" help:"Create a shared drive"`
	Update DriveDrivesUpdateCmd `cmd:"" name:"update" help:"Update a shared drive"`
	Delete DriveDrivesDeleteCmd `cmd:"" name:"delete" help:"Delete a shared drive"`
	Hide   DriveDrivesHideCmd   `cmd:"" name:"hide" help:"Hide a shared drive from default view"`
	Unhide DriveDrivesUnhideCmd `cmd:"" name:"unhide" help:"Unhide a shared drive"`
}

// DriveDrivesCreateCmd creates a new shared drive
type DriveDrivesCreateCmd struct {
	Name      string `arg:"" name:"name" help:"Name of the shared drive"`
	RequestID string `name:"request-id" help:"Unique request ID for idempotency (auto-generated if not specified)"`
}

func (c *DriveDrivesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("name is required")
	}

	requestID := strings.TrimSpace(c.RequestID)
	if requestID == "" {
		// Generate a random request ID for idempotency
		requestID = generateRequestID()
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	driveObj := &drive.Drive{
		Name: name,
	}

	created, err := svc.Drives.Create(requestID, driveObj).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"drive": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("created\t%s", formatDateTime(created.CreatedTime))
	return nil
}

// DriveDrivesUpdateCmd updates a shared drive's name or other properties
type DriveDrivesUpdateCmd struct {
	DriveID string `arg:"" name:"driveId" help:"ID of the shared drive"`
	Name    string `name:"name" help:"New name for the shared drive"`
}

func (c *DriveDrivesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	driveID := strings.TrimSpace(c.DriveID)
	if driveID == "" {
		return usage("driveId is required")
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("--name is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	driveObj := &drive.Drive{
		Name: name,
	}

	updated, err := svc.Drives.Update(driveID, driveObj).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"drive": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	return nil
}

// DriveDrivesDeleteCmd deletes a shared drive
type DriveDrivesDeleteCmd struct {
	DriveID string `arg:"" name:"driveId" help:"ID of the shared drive to delete"`
}

func (c *DriveDrivesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	driveID := strings.TrimSpace(c.DriveID)
	if driveID == "" {
		return usage("driveId is required")
	}

	// Get drive info for better confirmation message
	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	driveInfo, err := svc.Drives.Get(driveID).Context(ctx).Do()
	if err != nil {
		return err
	}

	// Confirm destructive action
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete shared drive %q (%s)", driveInfo.Name, driveID)); confirmErr != nil {
		return confirmErr
	}

	if err := svc.Drives.Delete(driveID).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":      driveID,
			"deleted": true,
		})
	}

	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("id\t%s", driveID)
	return nil
}

// DriveDrivesHideCmd hides a shared drive from default view
type DriveDrivesHideCmd struct {
	DriveID string `arg:"" name:"driveId" help:"ID of the shared drive to hide"`
}

func (c *DriveDrivesHideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	driveID := strings.TrimSpace(c.DriveID)
	if driveID == "" {
		return usage("driveId is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Drives.Hide(driveID).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"drive": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	u.Out().Printf("hidden\ttrue")
	return nil
}

// DriveDrivesUnhideCmd unhides a shared drive (makes it visible in default view)
type DriveDrivesUnhideCmd struct {
	DriveID string `arg:"" name:"driveId" help:"ID of the shared drive to unhide"`
}

func (c *DriveDrivesUnhideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	driveID := strings.TrimSpace(c.DriveID)
	if driveID == "" {
		return usage("driveId is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Drives.Unhide(driveID).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"drive": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	u.Out().Printf("hidden\tfalse")
	return nil
}

// generateRequestID creates a unique request ID for idempotency
func generateRequestID() string {
	return uuid.New().String()
}
