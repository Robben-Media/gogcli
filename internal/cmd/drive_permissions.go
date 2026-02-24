package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Permission type constants
const (
	permTypeUser   = "user"
	permTypeGroup  = "group"
	permTypeDomain = "domain"
	permTypeAnyone = "anyone"
)

// Permission role constants
const (
	permRoleOwner     = "owner"
	permRoleWriter    = "writer"
	permRoleCommenter = "commenter"
	permRoleReader    = "reader"
)

// DrivePermissionsCmd is the parent command for permissions subcommands
type DrivePermissionsCmd struct {
	List   DrivePermissionsListCmd   `cmd:"" name:"list" help:"List permissions on a file"`
	Get    DrivePermissionsGetCmd    `cmd:"" name:"get" help:"Get a specific permission"`
	Create DrivePermissionsCreateCmd `cmd:"" name:"create" help:"Create a permission (share a file)"`
	Update DrivePermissionsUpdateCmd `cmd:"" name:"update" help:"Update a permission"`
	Delete DrivePermissionsDeleteCmd `cmd:"" name:"delete" help:"Delete a permission (unshare)"`
}

// DrivePermissionsListCmd lists permissions on a file
type DrivePermissionsListCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page   string `name:"page" help:"Page token"`
}

func (c *DrivePermissionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Permissions.List(fileID).
		SupportsAllDrives(true).
		Fields("nextPageToken, permissions(id, type, role, emailAddress, domain, allowFileDiscovery, expirationTime, deleted)").
		Context(ctx)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if strings.TrimSpace(c.Page) != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"fileId":          fileID,
			"permissions":     resp.Permissions,
			"permissionCount": len(resp.Permissions),
			"nextPageToken":   resp.NextPageToken,
		})
	}
	if len(resp.Permissions) == 0 {
		u.Err().Println("No permissions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTYPE\tROLE\tEMAIL\tDOMAIN\tDISCOVERABLE")
	for _, p := range resp.Permissions {
		email := p.EmailAddress
		if email == "" {
			email = "-"
		}
		domain := p.Domain
		if domain == "" {
			domain = "-"
		}
		discoverable := "-"
		if p.Type == permTypeAnyone || p.Type == permTypeDomain {
			if p.AllowFileDiscovery {
				discoverable = "yes"
			} else {
				discoverable = "no"
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", p.Id, p.Type, p.Role, email, domain, discoverable)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// DrivePermissionsGetCmd gets a specific permission
type DrivePermissionsGetCmd struct {
	FileID       string `arg:"" name:"fileId" help:"File ID"`
	PermissionID string `arg:"" name:"permissionId" help:"Permission ID"`
}

func (c *DrivePermissionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	permissionID := strings.TrimSpace(c.PermissionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if permissionID == "" {
		return usage("empty permissionId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	perm, err := svc.Permissions.Get(fileID, permissionID).
		SupportsAllDrives(true).
		Fields("id, type, role, emailAddress, domain, allowFileDiscovery, expirationTime, deleted, pendingOwner, view").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"permission": perm})
	}

	u.Out().Printf("id\t%s", perm.Id)
	u.Out().Printf("type\t%s", perm.Type)
	u.Out().Printf("role\t%s", perm.Role)
	if perm.EmailAddress != "" {
		u.Out().Printf("email\t%s", perm.EmailAddress)
	}
	if perm.Domain != "" {
		u.Out().Printf("domain\t%s", perm.Domain)
	}
	if perm.AllowFileDiscovery {
		u.Out().Printf("discoverable\t%t", perm.AllowFileDiscovery)
	}
	if perm.ExpirationTime != "" {
		u.Out().Printf("expires\t%s", perm.ExpirationTime)
	}
	if perm.PendingOwner {
		u.Out().Printf("pending_owner\t%t", perm.PendingOwner)
	}
	if perm.View != "" {
		u.Out().Printf("view\t%s", perm.View)
	}
	return nil
}

// DrivePermissionsCreateCmd creates a new permission (shares a file)
type DrivePermissionsCreateCmd struct {
	FileID            string `arg:"" name:"fileId" help:"File ID"`
	Type              string `name:"type" help:"Permission type: user|group|domain|anyone"`
	Role              string `name:"role" help:"Permission role: owner|writer|commenter|reader"`
	Email             string `name:"email" help:"User or group email address (for user/group type)"`
	Domain            string `name:"domain" help:"Domain name (for domain type)"`
	Discoverable      bool   `name:"discoverable" help:"Allow file discovery in search (anyone/domain only)"`
	SendNotification  bool   `name:"notify" help:"Send notification email" default:"false"`
	EmailMessage      string `name:"message" help:"Custom message for notification email"`
	ExpirationTime    string `name:"expires" help:"Expiration time (RFC 3339, e.g., 2024-12-31T23:59:59Z)"`
	TransferOwnership bool   `name:"transfer-ownership" help:"Transfer file ownership (requires --email)"`
}

func (c *DrivePermissionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	permType := strings.TrimSpace(c.Type)
	role := strings.TrimSpace(c.Role)

	if permType == "" {
		return usage("missing --type (user|group|domain|anyone)")
	}
	if role == "" {
		return usage("missing --role (owner|writer|commenter|reader)")
	}

	// Validate type-specific requirements
	email := strings.TrimSpace(c.Email)
	domain := strings.TrimSpace(c.Domain)
	switch permType {
	case permTypeUser, permTypeGroup:
		if email == "" {
			return usage(fmt.Sprintf("--email is required for type %q", permType))
		}
	case permTypeDomain:
		if domain == "" {
			return usage("--domain is required for type 'domain'")
		}
	case permTypeAnyone:
		// No additional requirements
	default:
		return usage(fmt.Sprintf("invalid --type %q", permType))
	}

	// Ownership transfer requires extra confirmation
	if c.TransferOwnership {
		if email == "" {
			return usage("--email is required for ownership transfer")
		}
		if role != permRoleOwner {
			return usage("--role must be 'owner' when using --transfer-ownership")
		}
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("transfer ownership of file %s to %s", fileID, email)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	perm := &drive.Permission{
		Type: permType,
		Role: role,
	}
	if email != "" {
		perm.EmailAddress = email
	}
	if domain != "" {
		perm.Domain = domain
	}
	if permType == permTypeAnyone || permType == permTypeDomain {
		perm.AllowFileDiscovery = c.Discoverable
	}
	if c.ExpirationTime != "" {
		perm.ExpirationTime = c.ExpirationTime
	}

	call := svc.Permissions.Create(fileID, perm).
		SupportsAllDrives(true).
		SendNotificationEmail(c.SendNotification).
		Fields("id, type, role, emailAddress, domain, allowFileDiscovery, expirationTime").
		Context(ctx)

	if c.TransferOwnership {
		call = call.TransferOwnership(true)
	}
	if c.EmailMessage != "" {
		call = call.EmailMessage(c.EmailMessage)
	}

	created, err := call.Do()
	if err != nil {
		return err
	}

	link, err := driveWebLink(ctx, svc, fileID)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"link":         link,
			"permissionId": created.Id,
			"permission":   created,
		})
	}

	u.Out().Printf("link\t%s", link)
	u.Out().Printf("permission_id\t%s", created.Id)
	return nil
}

// DrivePermissionsUpdateCmd updates an existing permission
type DrivePermissionsUpdateCmd struct {
	FileID            string `arg:"" name:"fileId" help:"File ID"`
	PermissionID      string `arg:"" name:"permissionId" help:"Permission ID"`
	Role              string `name:"role" help:"New role: owner|writer|commenter|reader"`
	ExpirationTime    string `name:"expires" help:"Expiration time (RFC 3339, e.g., 2024-12-31T23:59:59Z)"`
	RemoveExpiration  bool   `name:"remove-expiration" help:"Remove the expiration time"`
	TransferOwnership bool   `name:"transfer-ownership" help:"Transfer file ownership (requires --role owner)"`
}

func (c *DrivePermissionsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	permissionID := strings.TrimSpace(c.PermissionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if permissionID == "" {
		return usage("empty permissionId")
	}

	role := strings.TrimSpace(c.Role)

	// Check if at least one update field is provided
	if role == "" && c.ExpirationTime == "" && !c.RemoveExpiration {
		return usage("at least one of --role, --expires, or --remove-expiration is required")
	}

	// Ownership transfer requires extra confirmation
	if c.TransferOwnership {
		if role != permRoleOwner {
			return usage("--role must be 'owner' when using --transfer-ownership")
		}
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("transfer ownership of file %s", fileID)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	perm := &drive.Permission{}
	if role != "" {
		perm.Role = role
	}
	if c.RemoveExpiration {
		perm.ExpirationTime = ""
		// ForceSendFields ensures the empty string is sent to clear expiration
		perm.ForceSendFields = append(perm.ForceSendFields, "ExpirationTime")
	} else if c.ExpirationTime != "" {
		perm.ExpirationTime = c.ExpirationTime
	}

	call := svc.Permissions.Update(fileID, permissionID, perm).
		SupportsAllDrives(true).
		Fields("id, type, role, emailAddress, domain, allowFileDiscovery, expirationTime").
		Context(ctx)

	if c.TransferOwnership {
		call = call.TransferOwnership(true)
	}

	updated, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"permission": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("type\t%s", updated.Type)
	u.Out().Printf("role\t%s", updated.Role)
	if updated.EmailAddress != "" {
		u.Out().Printf("email\t%s", updated.EmailAddress)
	}
	if updated.ExpirationTime != "" {
		u.Out().Printf("expires\t%s", updated.ExpirationTime)
	}
	return nil
}

// DrivePermissionsDeleteCmd deletes a permission (unshares a file)
type DrivePermissionsDeleteCmd struct {
	FileID       string `arg:"" name:"fileId" help:"File ID"`
	PermissionID string `arg:"" name:"permissionId" help:"Permission ID"`
}

func (c *DrivePermissionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	permissionID := strings.TrimSpace(c.PermissionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if permissionID == "" {
		return usage("empty permissionId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove permission %s from file %s", permissionID, fileID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Permissions.Delete(fileID, permissionID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"removed":      true,
			"fileId":       fileID,
			"permissionId": permissionID,
		})
	}

	u.Out().Printf("removed\ttrue")
	u.Out().Printf("file_id\t%s", fileID)
	u.Out().Printf("permission_id\t%s", permissionID)
	return nil
}
