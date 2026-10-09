package drive

// drive_share_user_silent is the only Drive permission mutator this package
// exposes. Every parameter that could widen it is fixed in code rather than
// taken from tool input:
//
//   - type is always "user" (no group, domain or anyone grants);
//   - role is one of reader, commenter, writer (never owner/organizer/fileOrganizer);
//   - sendNotificationEmail is always false;
//   - transferOwnership, moveToNewOwnersRoot, useDomainAdminAccess and
//     emailMessage are never sent.
//
// Reference (verified 2026-10-09):
// https://developers.google.com/workspace/drive/api/reference/rest/v3/permissions/create

import (
	"context"
	"net/mail"
	"regexp"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const ShareUserSilentOperation = "drive_share_user_silent"

var (
	shareFileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)
	shareRoles         = map[string]bool{"reader": true, "commenter": true, "writer": true}
)

type ShareUserSilentRequest struct {
	mcpcontract.Selection
	FileID       string `json:"file_id" jsonschema:"Exact Google Drive file ID to share; required"`
	EmailAddress string `json:"email_address" jsonschema:"Exact lowercase email address of the individual user grantee; required"`
	Role         string `json:"role" jsonschema:"One of reader, commenter, writer"`
}

type ShareUserSilentData struct {
	FileID       string `json:"file_id"`
	PermissionID string `json:"permission_id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	EmailAddress string `json:"email_address"`
	Notification bool   `json:"notification_email_sent"`
}

// ShareOperations returns the constrained Drive sharing write. It is opt-in
// (extended catalog) and still requires --enable-writes plus an explicit grant.
func ShareOperations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation(ShareUserSilentOperation, validateShareUserSilent, func(ctx context.Context, id mcpcontract.Identity, in ShareUserSilentRequest) (mcpcontract.Result[ShareUserSilentData], error) {
			client, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: ShareUserSilentOperation, Retry: mcpcontract.NonReplayableWrite})
			if err != nil {
				return mcpcontract.Result[ShareUserSilentData]{}, nativegoogleapi.NativePublicError(err)
			}

			svc, err := drive.NewService(ctx, option.WithHTTPClient(client))
			if err != nil {
				return mcpcontract.Result[ShareUserSilentData]{}, nativegoogleapi.NativePublicError(err)
			}

			permission := &drive.Permission{Type: "user", Role: in.Role, EmailAddress: in.EmailAddress}

			created, err := svc.Permissions.Create(in.FileID, permission).
				SendNotificationEmail(false).
				SupportsAllDrives(true).
				Fields("id,type,role,emailAddress").
				Context(ctx).
				Do()
			if err != nil {
				return mcpcontract.Result[ShareUserSilentData]{}, nativegoogleapi.NativeWritePublicError(err)
			}

			if created == nil || created.Id == "" {
				return mcpcontract.Result[ShareUserSilentData]{}, &mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: "Google returned no permission ID; list the file's permissions before repeating", Retryable: false}
			}

			return mcpcontract.NewResult(id, ShareUserSilentData{
				FileID: in.FileID, PermissionID: created.Id, Type: created.Type, Role: created.Role,
				EmailAddress: created.EmailAddress, Notification: false,
			}), nil
		}),
	}
}

func validateShareUserSilent(in ShareUserSilentRequest) error {
	if !shareFileIDPattern.MatchString(in.FileID) {
		return mcpcontract.Invalid("file_id must be one exact Drive file ID")
	}

	if !shareRoles[in.Role] {
		return mcpcontract.Invalid("role must be reader, commenter or writer")
	}

	addr, err := mail.ParseAddress(in.EmailAddress)
	if err != nil || addr.Address != in.EmailAddress || addr.Name != "" || strings.ToLower(in.EmailAddress) != in.EmailAddress ||
		strings.ContainsAny(in.EmailAddress, "*, <>\"") || !strings.Contains(in.EmailAddress, "@") {
		return mcpcontract.Invalid("email_address must be one exact lowercase user email address")
	}

	return nil
}
