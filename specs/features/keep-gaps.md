# Google Keep v1 -- Gap Coverage Spec

**API**: Google Keep v1 (`keep/v1`)
**Current coverage**: 3 methods (media.download, notes.get, notes.list)
**Gap**: 4 missing methods
**Service factory**: `newKeepService` (existing)
**Workspace-only**: Yes -- Google Keep API is restricted to Google Workspace accounts

## Overview

Adding 4 missing methods to the Google Keep v1 CLI commands. Covers note creation and deletion, plus permission batch operations (batch-create/batch-delete) to achieve full Discovery API parity.

All commands require Workspace accounts via `requireWorkspaceAccount(account)`. Commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations require `confirmDestructive()`. The existing `notes.list` command supports `--max` (default 10) and `--page` flags for pagination; JSON output includes `nextPageToken`. Text output uses TSV-formatted tables.

---

## Notes

### `gog keep notes create`

- **API method**: `notes.create`
- **Struct**: `KeepNotesCreateCmd`
- **Args/Flags**:
  - `--title` (string): note title (optional -- Keep notes can be untitled)
  - `--body` (string): note text body content
  - `--body-from-file` (string): read body from file path (mutually exclusive with --body)
  - `--list-items` ([]string): create a list note with these items (mutually exclusive with --body/--body-from-file)
  - `--checked` ([]int): indices of list items to mark as checked (0-based)
- **Behavior**: Creates either a text note or a list note depending on flags. If `--list-items` is provided, creates a list note with ListContent. Otherwise creates a text note with TextContent. At least one of `--body`, `--body-from-file`, or `--list-items` is required.
- **Output**: JSON object of created note (name, title, body/listContent, createTime, updateTime); text shows NAME, TITLE, TYPE (text/list), CREATE_TIME
- **Test**: httptest mock POST `/v1/notes`, assert body content type (text vs list), assert title if provided

### `gog keep notes delete`

- **API method**: `notes.delete`
- **Struct**: `KeepNotesDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): note resource name (e.g., `notes/abc123`)
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Permanently deletes the note (moves to trash, then permanently deleted after 30 days by Google).
- **Output**: Empty on success; stderr shows "Deleted note {name}"
- **Test**: httptest mock DELETE `/v1/notes/{id}`, verify --force bypasses prompt, verify non-force with --no-input returns ExitError code 2

---

## Note Permissions

### `gog keep permissions batch-create`

- **API method**: `notes.permissions.batchCreate`
- **Struct**: `KeepPermissionsBatchCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): note resource name (e.g., `notes/abc123`)
  - `--members` (required []string): email addresses to share with
  - `--role` (string, default "WRITER"): permission role (WRITER is the only supported role for Keep)
- **Behavior**: Creates permissions for multiple users in a single call. Each member gets the specified role on the note. Builds a batch request with one Permission per member.
- **Output**: JSON object with `permissions` array of created permissions; text shows MEMBER, ROLE, NAME for each
- **Test**: httptest mock POST `/v1/{parent}/permissions:batchCreate`, assert members in request body

### `gog keep permissions batch-delete`

- **API method**: `notes.permissions.batchDelete`
- **Struct**: `KeepPermissionsBatchDeleteCmd`
- **Args/Flags**:
  - `parent` (required arg): note resource name
  - `--permission-names` (required []string): permission resource names to delete (e.g., `notes/abc/permissions/xyz`)
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Removes sharing permissions from the note for the specified users.
- **Output**: Empty on success; stderr shows "Removed N permissions from {parent}"
- **Test**: httptest mock POST `/v1/{parent}/permissions:batchDelete`, assert permission names in body

---

## Implementation Notes

1. **Workspace-only**: All Keep API commands must call `requireWorkspaceAccount(account)` before creating the service. Consumer `@gmail.com` accounts will be rejected.
2. **Text vs List notes**: Keep supports two content types: TextContent (plain text body) and ListContent (checklist items). The create command must handle both via mutually exclusive flag groups.
3. **Permission roles**: The Keep API only supports the WRITER role for permissions. The `--role` flag is included for forward compatibility but defaults to WRITER.
4. **Note resource names**: Follow the pattern `notes/{noteId}`. Permission names follow `notes/{noteId}/permissions/{permissionId}`.
5. **Delete behavior**: The API delete marks the note as trashed. Google permanently deletes trashed notes after approximately 30 days. The CLI should mention this in the confirmation prompt.
6. Total new test count: minimum 4 test functions, plus edge cases for text vs list note creation and Workspace account validation.
