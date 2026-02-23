package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveAboutCmd gets information about the user's Drive account
type DriveAboutCmd struct {
	// No arguments needed - returns info about the authenticated user's Drive
}

func (c *DriveAboutCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	about, err := svc.About.Get().
		Fields("user(displayName, emailAddress, me, permissionId, photoLink), storageQuota(limit, usage, usageInDrive, usageInDriveTrash), kind").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"about": about})
	}

	// User info
	if about.User != nil {
		u.Out().Printf("user_name\t%s", about.User.DisplayName)
		u.Out().Printf("user_email\t%s", about.User.EmailAddress)
		u.Out().Printf("user_id\t%s", about.User.PermissionId)
		if about.User.PhotoLink != "" {
			u.Out().Printf("user_photo\t%s", about.User.PhotoLink)
		}
	}

	// Storage quota
	if about.StorageQuota != nil {
		limit := about.StorageQuota.Limit
		usage := about.StorageQuota.Usage
		driveUsage := about.StorageQuota.UsageInDrive
		trashUsage := about.StorageQuota.UsageInDriveTrash

		u.Out().Printf("storage_limit\t%s", formatDriveSize(limit))
		u.Out().Printf("storage_used\t%s", formatDriveSize(usage))
		u.Out().Printf("drive_used\t%s", formatDriveSize(driveUsage))
		u.Out().Printf("trash_used\t%s", formatDriveSize(trashUsage))

		if limit > 0 {
			percentUsed := float64(usage) / float64(limit) * 100
			u.Out().Printf("storage_percent\t%.1f%%", percentUsed)
		}
	}

	return nil
}

// DriveAboutFields represents commonly used fields for about output
type DriveAboutFields struct {
	UserName     string
	UserEmail    string
	UserPhoto    string
	StorageLimit int64
	StorageUsed  int64
	DriveUsed    int64
	TrashUsed    int64
}

// FormatAboutAsTable formats about info as a table row (used by commands that need this)
func FormatAboutAsTable(fields DriveAboutFields) string {
	var b strings.Builder
	b.WriteString("User: ")
	b.WriteString(fields.UserName)
	b.WriteString(" <")
	b.WriteString(fields.UserEmail)
	b.WriteString(">\n")
	b.WriteString("Storage: ")
	b.WriteString(formatDriveSize(fields.StorageUsed))
	b.WriteString(" / ")
	b.WriteString(formatDriveSize(fields.StorageLimit))
	b.WriteString(" used\n")
	return b.String()
}
