# Google Tasks v1 -- Gap Coverage Spec

**API**: Google Tasks v1 (`tasks/v1`)
**Current coverage**: 8 methods (tasklists.insert/list, tasks.clear/delete/get/insert/list/patch)
**Gap**: 6 missing methods
**Service factory**: `newTasksService` (existing)

## Overview

Adding 6 missing methods to the Google Tasks CLI commands. Covers task list management (delete/get/patch/update) and task operations (move/update) to achieve full Discovery API parity.

**Pagination**: The existing `tasklists list` command uses `--max` (default 10) and `--page` flags for pagination. JSON output includes `nextPageToken`. The new gap commands are single-resource operations and do not require pagination flags.

**Error handling**: All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations require `confirmDestructive()` with a warning about cascading task deletion.

---

## Task Lists

### `gog tasks tasklists delete`

- **API method**: `tasklists.delete`
- **Struct**: `TasksTasklistsDeleteCmd`
- **Args/Flags**:
  - `tasklistId` (required arg): task list ID
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Permanently deletes the task list and all tasks within it.
- **Output**: Empty on success; stderr shows "Deleted task list {tasklistId}"
- **Test**: httptest mock DELETE `/tasks/v1/users/@me/lists/{tasklistId}`

### `gog tasks tasklists get`

- **API method**: `tasklists.get`
- **Struct**: `TasksTasklistsGetCmd`
- **Args/Flags**:
  - `tasklistId` (required arg): task list ID
- **Output**: JSON object with `id`, `title`, `updated`, `selfLink`; text shows ID, TITLE, UPDATED
- **Test**: httptest mock GET `/tasks/v1/users/@me/lists/{tasklistId}`

### `gog tasks tasklists patch`

- **API method**: `tasklists.patch`
- **Struct**: `TasksTasklistsPatchCmd`
- **Args/Flags**:
  - `tasklistId` (required arg): task list ID
  - `--title` (string): new title
- **Behavior**: `flagProvided()` for partial update. Only send changed fields.
- **Output**: JSON object of updated task list
- **Test**: httptest mock PATCH `/tasks/v1/users/@me/lists/{tasklistId}`, assert only changed fields in body

### `gog tasks tasklists update`

- **API method**: `tasklists.update`
- **Struct**: `TasksTasklistsUpdateCmd`
- **Args/Flags**:
  - `tasklistId` (required arg): task list ID
  - `--title` (required string): title (full replace)
- **Behavior**: Full replace semantics. All writable fields must be provided.
- **Output**: JSON object of updated task list
- **Test**: httptest mock PUT `/tasks/v1/users/@me/lists/{tasklistId}`, assert full body

---

## Tasks

### `gog tasks move`

- **API method**: `tasks.move`
- **Struct**: `TasksMoveCmd`
- **Args/Flags**:
  - `tasklistId` (required arg): task list ID
  - `taskId` (required arg): task ID to move
  - `--parent` (string): new parent task ID (makes it a subtask; omit to move to top level)
  - `--previous` (string): task ID to insert after (omit to place at beginning)
- **Behavior**: Moves a task within a task list. Can change parent (nesting) and position (ordering). Both `--parent` and `--previous` are optional; at least one should be provided for a meaningful move, but the API allows calling with neither (moves to top of list).
- **Output**: JSON object of moved task; text shows ID, TITLE, PARENT, POSITION
- **Test**: httptest mock POST `/tasks/v1/lists/{tasklistId}/tasks/{taskId}/move?parent={parent}&previous={previous}`, assert query params

### `gog tasks update`

- **API method**: `tasks.update`
- **Struct**: `TasksUpdateCmd`
- **Args/Flags**:
  - `tasklistId` (required arg): task list ID
  - `taskId` (required arg): task ID
  - `--title` (required string): task title
  - `--notes` (string): task notes/description
  - `--status` (string): "needsAction" or "completed"
  - `--due` (string): due date RFC3339
- **Behavior**: Full replace (PUT). All writable fields sent. Omitted optional fields are cleared. Contrast with existing `tasks patch` which uses PATCH semantics.
- **Output**: JSON object of updated task
- **Test**: httptest mock PUT `/tasks/v1/lists/{tasklistId}/tasks/{taskId}`, assert full body

---

## Implementation Notes

1. **patch vs update**: The Tasks API has both PATCH and PUT for tasks and task lists. The existing `tasks patch` command uses PATCH (partial update with `flagProvided()`). The new `tasks update` command should use PUT (full replace). Consider adding help text to clarify the difference.
2. **tasks.move**: This is a POST with query parameters, not a body. The parent and previous parameters control nesting and ordering respectively. If neither is provided, the task moves to the top of the root level.
3. **tasklists.delete**: Deleting a task list also deletes all tasks in it. The confirmation message should warn about this cascading deletion.
4. **Task list IDs**: These are opaque strings returned by the list endpoint. The default task list is accessible but has a system-generated ID.
5. **Naming conflicts**: The existing command structure likely uses `TasksCmd` as the top-level. The new `tasks update` (full replace) may conflict with the existing naming if there's already an `update` subcommand. Check the existing Kong struct and decide on naming: `update` for PUT and `patch`/`edit` for PATCH, or `full-update` for PUT.
6. Total new test count: minimum 6 test functions, including move with various parent/previous combinations.
