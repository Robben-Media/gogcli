# Drive v3 -- Gap Coverage Spec

## Overview

**API**: Google Drive API v3
**Go package**: `google.golang.org/api/drive/v3`
**Service factory**: `newDriveService` (existing in `drive.go`)
**Currently implemented**: 17 methods -- comments (create/delete/get/list/update), drives.list, files (copy/create/delete/export/get/list/update), permissions (create/delete/list), replies.create
**Missing methods**: 40

## Why

Drive is one of the most commonly used Google APIs. Missing methods prevent users from managing shared drives, tracking file changes (watch/webhooks), managing file revisions, accessing file labels, downloading binary files (vs export), handling access proposals/approvals, and managing reply threads -- all needed for complete Drive administration and automation workflows.

---

## Resource: About (1 method)

### about.get

| Field | Value |
|-------|-------|
| CLI | `gog drive about` |
| API | `About.Get().Fields("*")` |
| Flags | `--fields` (optional, comma-separated; default: user,storageQuota) |
| Output JSON | `{"about": {...}}` |
| Output TSV | `user.displayName`, `user.emailAddress`, `storageQuota.limit`, `storageQuota.usage`, `storageQuota.usageInDrive`, `storageQuota.usageInDriveTrash` |
| Notes | Must specify fields parameter; Drive API returns empty without it |

---

## Resource: Access Proposals (3 methods)

Path pattern: `files/{fileId}/accessProposals/{proposalId}`

### accessproposals.list

| Field | Value |
|-------|-------|
| CLI | `gog drive access-proposals list <fileId>` |
| API | `Files.AccessProposals.List(fileId)` |
| Args | `fileId` (positional, required) |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"accessProposals": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `REQUESTER`, `ROLE`, `CREATE_TIME` |

### accessproposals.get

| Field | Value |
|-------|-------|
| CLI | `gog drive access-proposals get <fileId> <proposalId>` |
| Output JSON | `{"accessProposal": {...}}` |

### accessproposals.resolve

| Field | Value |
|-------|-------|
| CLI | `gog drive access-proposals resolve <fileId> <proposalId>` |
| Flags | `--action` (required: ACCEPT, DENY), `--role` (required if accepting: reader, commenter, writer), `--send-notification` (bool) |
| Output JSON | `{"resolved": true}` |

---

## Resource: Approvals (2 methods)

Path pattern: `files/{fileId}/approvals/{approvalId}`

### approvals.get

| Field | Value |
|-------|-------|
| CLI | `gog drive approvals get <fileId> <approvalId>` |
| Output JSON | `{"approval": {...}}` |

### approvals.list

| Field | Value |
|-------|-------|
| CLI | `gog drive approvals list <fileId>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"approvals": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `STATUS`, `REQUESTER`, `CREATE_TIME` |

---

## Resource: Apps (2 methods)

### apps.get

| Field | Value |
|-------|-------|
| CLI | `gog drive apps get <appId>` |
| API | `Apps.Get(appId)` |
| Output JSON | `{"app": {...}}` |
| Output TSV | `id`, `name`, `supportsCreate`, `supportsImport`, `primaryMimeTypes` |

### apps.list

| Field | Value |
|-------|-------|
| CLI | `gog drive apps list` |
| API | `Apps.List()` |
| Flags | `--app-filter-extensions`, `--app-filter-mime-types` |
| Output JSON | `{"apps": [...]}` |
| Output TSV | `ID`, `NAME`, `INSTALLED` |

---

## Resource: Changes (3 methods)

### changes.getStartPageToken

| Field | Value |
|-------|-------|
| CLI | `gog drive changes token` |
| API | `Changes.GetStartPageToken()` |
| Flags | `--drive-id` (optional, for shared drives), `--supports-all-drives` (bool) |
| Output JSON | `{"startPageToken": "..."}` |
| Output TSV | prints token value |

### changes.list

| Field | Value |
|-------|-------|
| CLI | `gog drive changes list --token <pageToken>` |
| API | `Changes.List(pageToken)` |
| Flags | `--token` (required, from getStartPageToken), `--page-size`, `--include-removed` (bool), `--restrict-to-my-drive` (bool), `--spaces` (drive, appDataFolder), `--supports-all-drives`, `--include-items-from-all-drives` |
| Output JSON | `{"changes": [...], "newStartPageToken": "...", "nextPageToken": "..."}` |
| Output TSV | `FILE_ID`, `CHANGE_TYPE`, `REMOVED`, `TIME`, `FILE_NAME` |
| Notes | Save `newStartPageToken` for next poll; if present, no more pages |

### changes.watch

| Field | Value |
|-------|-------|
| CLI | `gog drive changes watch --token <pageToken>` |
| API | `Changes.Watch(pageToken, &Channel{...})` |
| Flags | `--token` (required), `--channel-id` (required, unique string), `--webhook-url` (required, HTTPS URL), `--type` (default: web_hook), `--expiration` (RFC3339 or Unix ms), `--supports-all-drives` |
| Output JSON | `{"channel": {...}}` |
| Output TSV | `id`, `resourceId`, `resourceUri`, `expiration` |
| Notes | Requires publicly accessible HTTPS webhook endpoint |

---

## Resource: Channels (1 method)

### channels.stop

| Field | Value |
|-------|-------|
| CLI | `gog drive channels stop --channel-id <id> --resource-id <id>` |
| API | `Channels.Stop(&Channel{Id: channelId, ResourceId: resourceId})` |
| Flags | `--channel-id` (required), `--resource-id` (required) |
| Output JSON | `{"stopped": true}` |
| Notes | Stops push notifications for a channel created by watch |

---

## Resource: Drives (6 methods)

### drives.create

| Field | Value |
|-------|-------|
| CLI | `gog drive drives create --name <name>` |
| API | `Drives.Create(requestId, &Drive{Name: name})` |
| Flags | `--name` (required), `--request-id` (auto-generated UUID if not provided) |
| Output JSON | `{"drive": {...}}` |
| Output TSV | `id`, `name`, `createdTime` |

### drives.get

| Field | Value |
|-------|-------|
| CLI | `gog drive drives get <driveId>` |
| API | `Drives.Get(driveId)` |
| Output JSON | `{"drive": {...}}` |
| Output TSV | `id`, `name`, `createdTime`, `hidden` |

### drives.delete

| Field | Value |
|-------|-------|
| CLI | `gog drive drives delete <driveId>` |
| Guard | `confirmDestructive()` with `--force` |
| Output JSON | `{"deleted": true, "driveId": "..."}` |
| Notes | Drive must be empty before deletion |

### drives.update

| Field | Value |
|-------|-------|
| CLI | `gog drive drives update <driveId>` |
| Flags | `--name`, `--color-rgb`, `--theme-id`, `--restrictions` (JSON object or individual flags: `--restrict-admin-managed-restrictions`, `--restrict-copy-requires-writer`, `--restrict-domain-users-only`, `--restrict-drive-members-only`) |
| Patch logic | `flagProvided()` |
| Output JSON | `{"drive": {...}}` |

### drives.hide

| Field | Value |
|-------|-------|
| CLI | `gog drive drives hide <driveId>` |
| API | `Drives.Hide(driveId)` |
| Output JSON | `{"drive": {...}}` |

### drives.unhide

| Field | Value |
|-------|-------|
| CLI | `gog drive drives unhide <driveId>` |
| API | `Drives.Unhide(driveId)` |
| Output JSON | `{"drive": {...}}` |

---

## Resource: Files -- Additional Methods (6 methods)

### files.download

| Field | Value |
|-------|-------|
| CLI | `gog drive download <fileId>` |
| Notes | Already partially implemented via existing `download` command. This covers the v3 `files.download` method for downloading blob content (media). Verify existing implementation covers this or extend it. |

### files.emptyTrash

| Field | Value |
|-------|-------|
| CLI | `gog drive empty-trash` |
| API | `Files.EmptyTrash()` |
| Flags | `--drive-id` (optional, for shared drives), `--force` |
| Guard | `confirmDestructive(ctx, flags, "permanently delete all files in trash")` |
| Output JSON | `{"emptied": true}` |

### files.generateIds

| Field | Value |
|-------|-------|
| CLI | `gog drive generate-ids` |
| API | `Files.GenerateIds()` |
| Flags | `--count` (default: 10), `--space` (drive, appDataFolder), `--type` (files, shortcuts) |
| Output JSON | `{"ids": [...], "space": "...", "kind": "..."}` |
| Output TSV | one ID per line |

### files.listLabels

| Field | Value |
|-------|-------|
| CLI | `gog drive labels list <fileId>` |
| API | `Files.ListLabels(fileId)` |
| Flags | `--max-results`, `--page-token` |
| Output JSON | `{"labels": [...], "nextPageToken": "..."}` |
| Output TSV | `LABEL_ID`, `REVISION_ID`, `FIELDS` |

### files.modifyLabels

| Field | Value |
|-------|-------|
| CLI | `gog drive labels modify <fileId>` |
| API | `Files.ModifyLabels(fileId, &ModifyLabelsRequest{...})` |
| Flags | `--add-label` (labelId), `--remove-label` (labelId), `--set-field` (labelId.fieldId=value, repeatable) |
| Output JSON | `{"modifiedLabels": [...]}` |

### files.watch

| Field | Value |
|-------|-------|
| CLI | `gog drive files watch <fileId>` |
| API | `Files.Watch(fileId, &Channel{...})` |
| Flags | `--channel-id` (required), `--webhook-url` (required), `--type` (default: web_hook), `--expiration` |
| Output JSON | `{"channel": {...}}` |
| Notes | Watches a specific file for changes |

---

## Resource: Operations (1 method)

### operations.get

| Field | Value |
|-------|-------|
| CLI | `gog drive operations get <operationId>` |
| API | `Operations.Get(operationId)` |
| Output JSON | `{"operation": {...}}` |
| Output TSV | `name`, `done`, `error` (if present) |
| Notes | For long-running operations (e.g., large file downloads) |

---

## Resource: Permissions -- Additional Methods (2 methods)

### permissions.get

| Field | Value |
|-------|-------|
| CLI | `gog drive permissions get <fileId> <permissionId>` |
| API | `Permissions.Get(fileId, permissionId)` |
| Flags | `--supports-all-drives` |
| Output JSON | `{"permission": {...}}` |
| Output TSV | `id`, `type`, `role`, `emailAddress`, `displayName` |

### permissions.update

| Field | Value |
|-------|-------|
| CLI | `gog drive permissions update <fileId> <permissionId>` |
| API | `Permissions.Update(fileId, permissionId, &Permission{...})` |
| Flags | `--role` (required: owner, organizer, fileOrganizer, writer, commenter, reader), `--transfer-ownership` (bool), `--remove-expiration` (bool), `--expiration-time` (RFC3339) |
| Patch logic | `flagProvided()` |
| Output JSON | `{"permission": {...}}` |

---

## Resource: Replies -- Additional Methods (4 methods)

Path pattern: `files/{fileId}/comments/{commentId}/replies/{replyId}`

### replies.delete

| Field | Value |
|-------|-------|
| CLI | `gog drive replies delete <fileId> <commentId> <replyId>` |
| Guard | `confirmDestructive()` with `--force` |

### replies.get

| Field | Value |
|-------|-------|
| CLI | `gog drive replies get <fileId> <commentId> <replyId>` |
| Flags | `--include-deleted` (bool) |
| Output JSON | `{"reply": {...}}` |

### replies.list

| Field | Value |
|-------|-------|
| CLI | `gog drive replies list <fileId> <commentId>` |
| Flags | `--page-size`, `--page-token`, `--include-deleted` |
| Output JSON | `{"replies": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `AUTHOR`, `CONTENT`, `CREATED_TIME`, `MODIFIED_TIME` |

### replies.update

| Field | Value |
|-------|-------|
| CLI | `gog drive replies update <fileId> <commentId> <replyId>` |
| Flags | `--content` (required) |
| Output JSON | `{"reply": {...}}` |

---

## Resource: Revisions (4 methods)

Path pattern: `files/{fileId}/revisions/{revisionId}`

### revisions.list

| Field | Value |
|-------|-------|
| CLI | `gog drive revisions list <fileId>` |
| API | `Revisions.List(fileId)` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"revisions": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `MODIFIED_TIME`, `LAST_MODIFYING_USER`, `SIZE`, `KEEP_FOREVER` |

### revisions.get

| Field | Value |
|-------|-------|
| CLI | `gog drive revisions get <fileId> <revisionId>` |
| Flags | `--acknowledge-abuse` (bool, for files flagged as abusive) |
| Output JSON | `{"revision": {...}}` |

### revisions.update

| Field | Value |
|-------|-------|
| CLI | `gog drive revisions update <fileId> <revisionId>` |
| Flags | `--keep-forever` (bool), `--publish-auto` (bool), `--published` (bool), `--published-link` |
| Patch logic | `flagProvided()` |
| Output JSON | `{"revision": {...}}` |

### revisions.delete

| Field | Value |
|-------|-------|
| CLI | `gog drive revisions delete <fileId> <revisionId>` |
| Guard | `confirmDestructive()` with `--force` |
| Notes | Cannot delete the last remaining revision |

---

## Resource: Team Drives (deprecated) (5 methods)

### Implementation note

Team Drives are deprecated in favor of Drives (shared drives). These methods exist in the Discovery API for backward compatibility. Implement with deprecation warnings.

### teamdrives.create / get / list / update / delete

| Field | Value |
|-------|-------|
| CLI | `gog drive teamdrives <action>` |
| Notes | Mirror the Drives commands but use the TeamDrives service. Print deprecation warning to stderr: `"Warning: Team Drives API is deprecated. Use 'gog drive drives' instead."` |
| Pattern | Same flags/output as corresponding Drives methods |

---

## Kong Struct Layout

```go
type DriveCmd struct {
    // ... existing fields ...
    About           DriveAboutCmd           `cmd:"" name:"about" help:"Get Drive account info"`
    AccessProposals DriveAccessProposalsCmd `cmd:"" name:"access-proposals" help:"Access proposal operations"`
    Approvals       DriveApprovalsCmd       `cmd:"" name:"approvals" help:"File approval operations"`
    Apps            DriveAppsCmd            `cmd:"" name:"apps" help:"Connected app operations"`
    Changes         DriveChangesCmd         `cmd:"" name:"changes" help:"Change tracking"`
    Channels        DriveChannelsCmd        `cmd:"" name:"channels" help:"Push notification channels"`
    Drives          DriveDrivesCmd          `cmd:"" name:"drives" help:"Shared drive operations"` // extend existing
    EmptyTrash      DriveEmptyTrashCmd      `cmd:"" name:"empty-trash" help:"Permanently delete all trashed files"`
    GenerateIds     DriveGenerateIdsCmd     `cmd:"" name:"generate-ids" help:"Generate file IDs"`
    Labels          DriveLabelsCmd          `cmd:"" name:"labels" help:"File label operations"`
    Operations      DriveOperationsCmd      `cmd:"" name:"operations" help:"Long-running operation status"`
    Replies         DriveRepliesCmd         `cmd:"" name:"replies" help:"Comment reply operations"` // extend existing
    Revisions       DriveRevisionsCmd       `cmd:"" name:"revisions" help:"File revision operations"`
    TeamDrives      DriveTeamDrivesCmd      `cmd:"" name:"teamdrives" hidden:"" help:"(Deprecated) Team drive operations"`
}
```

---

## Test Requirements

### Test patterns

1. **About**: Verify fields parameter is sent; mock quota response
2. **Changes**: Test pagination with `newStartPageToken` vs `nextPageToken`; test watch channel creation
3. **Channels stop**: Verify request body contains both channelId and resourceId
4. **Drives CRUD**: Standard pattern; test hide/unhide toggle
5. **Empty trash**: Verify `confirmDestructive()` guard; test with `--drive-id`
6. **Revisions**: Test keep-forever flag; verify cannot delete last revision (error handling)
7. **Permissions update**: Test role change, ownership transfer flag
8. **Team Drives**: Verify deprecation warning printed to stderr
9. **Watch methods**: Verify channel object in request body, verify webhook URL validation

### Factory injection

Use existing: `var newDriveService = googleapi.NewDrive`

### Test file organization

- `drive_about_test.go` -- about.get
- `drive_changes_test.go` -- changes token/list/watch, channels stop
- `drive_drives_admin_test.go` -- drives CRUD, hide/unhide
- `drive_files_extra_test.go` -- emptyTrash, generateIds, labels, watch
- `drive_permissions_extra_test.go` -- permissions get/update
- `drive_replies_extra_test.go` -- replies CRUD
- `drive_revisions_test.go` -- revisions CRUD
- `drive_access_proposals_test.go` -- access proposals and approvals
- `drive_teamdrives_test.go` -- deprecated team drives with warning
