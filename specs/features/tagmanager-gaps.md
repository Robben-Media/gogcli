# Tag Manager v2 Gap Spec

## Overview

The Tag Manager v2 API has 99 missing methods. Currently implemented: `accounts.list`, `containers.list`, `versionHeaders.list`, `workspaces.tags.get`, `workspaces.tags.list`, `workspaces.triggers.list`, `workspaces.variables.list`.

All Tag Manager resources use a deeply nested path-based hierarchy:
```
accounts/{accountId}
accounts/{accountId}/containers/{containerId}
accounts/{accountId}/containers/{containerId}/environments/{environmentId}
accounts/{accountId}/containers/{containerId}/versions/{versionId}
accounts/{accountId}/containers/{containerId}/version_headers/{versionHeaderId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/tags/{tagId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/triggers/{triggerId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/variables/{variableId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/built_in_variables
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/folders/{folderId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/templates/{templateId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/transformations/{transformationId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/clients/{clientId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/zones/{zoneId}
accounts/{accountId}/containers/{containerId}/workspaces/{workspaceId}/gtag_config/{gtagConfigId}
accounts/{accountId}/user_permissions/{userPermissionId}
accounts/{accountId}/containers/{containerId}/destinations/{destinationId}
```

Service factory: `newTagManagerService` in `internal/cmd/tagmanager.go`, returns `*tagmanager.Service`.

Helper: `gtmWorkspacePath(accountID, containerID, workspaceID)` builds the workspace parent path.

---

## Shared Flags

All GTM workspace-scoped commands share:
- `--account-id` (required): GTM account ID
- `--container-id` (required): GTM container ID
- `--workspace-id` (default "0"): GTM workspace ID

Container-scoped commands share:
- `--account-id` (required): GTM account ID
- `--container-id` (required): GTM container ID

Account-scoped commands use:
- `--account-id` (required): GTM account ID

Many resources also accept a full `path` positional arg (e.g. `accounts/123/containers/456/workspaces/0/tags/789`) as an alternative to individual ID flags for GET/DELETE/UPDATE operations.

---

## Resource Groups

### 1. Accounts

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **get** | `gog gtm accounts get <path>` | `path` (positional): full account path `accounts/{accountId}` | JSON object, TSV: ACCOUNT_ID, NAME, SHARE_DATA, FINGERPRINT |
| **update** | `gog gtm accounts update <path>` | `path` (positional); `--name` (optional); `--share-data` (optional bool). Uses `flagProvided()` for partial update. | JSON object of updated account |

**Test requirements:**
- `get`: httptest mock returning single account JSON; verify JSON output contains accountId, name
- `get`: text output prints ACCOUNT_ID/NAME fields
- `update`: mock PATCH returning updated account; verify only provided fields are sent
- `update`: error when no flags provided ("no updates provided")

---

### 2. Containers

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm containers create` | `--account-id` (required); `--name` (required); `--usage-context` (required, one of: web, android, ios, ampEmail, server); `--domain-name` (optional); `--notes` (optional) | JSON object of created container |
| **delete** | `gog gtm containers delete <path>` | `path` (positional): full container path. `--force` bypasses confirmation. Uses `confirmDestructive()`. | JSON `{"deleted": true, "path": "..."}` |
| **get** | `gog gtm containers get <path>` | `path` (positional): full container path | JSON object, TSV: CONTAINER_ID, NAME, PUBLIC_ID, USAGE_CONTEXT |
| **update** | `gog gtm containers update <path>` | `path` (positional); `--name`, `--usage-context`, `--domain-name`, `--notes` (all optional). Uses `flagProvided()`. | JSON object of updated container |
| **combine** | `gog gtm containers combine` | `--account-id` (required); `--container-id` (required); `--setting-source` (optional) | JSON object of combined container |
| **lookup** | `gog gtm containers lookup` | `--destination-id` (required) | JSON object of container found by destination link |
| **move_tag_id** | `gog gtm containers move-tag-id` | `--account-id` (required); `--container-id` (required); `--tag-id` (optional); `--tag-name` (optional); `--copy-users` (optional bool); `--copy-settings` (optional bool) | JSON object of container |
| **snippet** | `gog gtm containers snippet` | `--account-id` (required); `--container-id` (required) | JSON object with `snippet` and `noScriptSnippet` fields; text mode prints raw HTML |

**Test requirements:**
- `create`: mock POST, verify request body has name + usageContext
- `delete`: mock DELETE, verify `confirmDestructive()` called; verify `--force` bypasses
- `get`: mock GET, verify JSON and text output
- `update`: mock PUT, verify `flagProvided()` partial update behavior
- `snippet`: mock GET, verify text mode outputs raw HTML strings
- `lookup`: mock GET with `destinationId` query param
- `combine`: mock POST, verify settingSource in request
- `move_tag_id`: mock POST, verify body fields

---

### 3. Container Versions

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog gtm versions delete <path>` | `path` (positional): full version path. `--force` flag. Uses `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm versions get <path>` | `path` (positional): full version path | JSON object with full version details (tags, triggers, variables, folders) |
| **live** | `gog gtm versions live` | `--account-id` (required); `--container-id` (required) | JSON object of currently live version |
| **publish** | `gog gtm versions publish <path>` | `path` (positional): full version path; `--fingerprint` (optional) | JSON object of published version |
| **set_latest** | `gog gtm versions set-latest <path>` | `path` (positional): full version path | JSON object of version set as latest |
| **undelete** | `gog gtm versions undelete <path>` | `path` (positional): full version path | JSON object of restored version |
| **update** | `gog gtm versions update <path>` | `path` (positional); `--name`, `--description` (optional). Uses `flagProvided()`. | JSON object of updated version |

**Test requirements:**
- `delete`: confirm destructive, mock DELETE
- `get`: mock full version payload with nested tags/triggers/variables
- `live`: mock GET on `live` endpoint
- `publish`: mock POST, verify fingerprint header if provided
- `set_latest`/`undelete`: mock POST for each
- `update`: verify partial update with flagProvided

---

### 4. Container Version Headers

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **latest** | `gog gtm version-headers latest` | `--account-id` (required); `--container-id` (required) | JSON object, TSV: VERSION_ID, NAME, NUM_TAGS, NUM_TRIGGERS, NUM_VARIABLES |

**Test requirements:**
- Mock GET returning latest version header; verify JSON and text output

---

### 5. Environments

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm environments create` | `--account-id`, `--container-id` (required); `--name` (required); `--description` (optional); `--type` (optional: latest, live, workspace, custom); `--url` (optional) | JSON object |
| **delete** | `gog gtm environments delete <path>` | `path` (positional). `--force` flag. Uses `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm environments get <path>` | `path` (positional) | JSON object, TSV: ENVIRONMENT_ID, NAME, TYPE, URL |
| **list** | `gog gtm environments list` | `--account-id`, `--container-id` (required); `--max`, `--page` | JSON array, TSV: ENVIRONMENT_ID, NAME, TYPE |
| **reauthorize** | `gog gtm environments reauthorize <path>` | `path` (positional) | JSON object with new authorization code |
| **update** | `gog gtm environments update <path>` | `path` (positional); `--name`, `--description`, `--url` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle tests for each method
- `reauthorize`: verify POST to reauthorize endpoint
- `list`: pagination with --max/--page

---

### 6. Destinations

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **get** | `gog gtm destinations get <path>` | `path` (positional) | JSON object |
| **link** | `gog gtm destinations link` | `--account-id`, `--container-id` (required); `--destination` (required body) | JSON object |
| **list** | `gog gtm destinations list` | `--account-id`, `--container-id` (required) | JSON array, TSV: DESTINATION_ID, NAME, LINK |

**Test requirements:**
- `get`/`list`/`link`: httptest mock for each

---

### 7. Workspaces

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm workspaces create` | `--account-id`, `--container-id` (required); `--name` (required); `--description` (optional) | JSON object |
| **delete** | `gog gtm workspaces delete <path>` | `path` (positional). `--force`. Uses `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm workspaces get <path>` | `path` (positional) | JSON object, TSV: WORKSPACE_ID, NAME, DESCRIPTION |
| **getStatus** | `gog gtm workspaces status <path>` | `path` (positional) | JSON object with workspace entity counts and change status |
| **list** | `gog gtm workspaces list` | `--account-id`, `--container-id` (required); `--max`, `--page` | JSON array, TSV: WORKSPACE_ID, NAME |
| **update** | `gog gtm workspaces update <path>` | `path` (positional); `--name`, `--description` (optional). Uses `flagProvided()`. | JSON object |
| **quick_preview** | `gog gtm workspaces quick-preview <path>` | `path` (positional) | JSON object with preview info and sync status |
| **resolve_conflict** | `gog gtm workspaces resolve-conflict <path>` | `path` (positional); `--fingerprint` (required); entity body via stdin or `--entity-json` | JSON (empty on success) |
| **sync** | `gog gtm workspaces sync <path>` | `path` (positional) | JSON object with sync status and merge conflicts |
| **create_version** | `gog gtm workspaces create-version <path>` | `path` (positional); `--name` (optional); `--notes` (optional) | JSON object containing new container version |
| **bulk_update** | `gog gtm workspaces bulk-update <path>` | `path` (positional); entity body via stdin or `--entities-json` | JSON object |

**Test requirements:**
- Full CRUD cycle for create/delete/get/list/update
- `getStatus`: mock with workspace status payload
- `quick_preview`/`sync`: mock POST operations
- `resolve_conflict`: verify fingerprint and entity body sent
- `create_version`: verify name/notes in request body
- `bulk_update`: verify entity body sent via stdin

---

### 8. Workspace Tags

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm tags create` | `--account-id`, `--container-id`, `--workspace-id` (workspace flags); `--name` (required); `--type` (required); `--firing-trigger-id` (repeatable); `--blocking-trigger-id` (repeatable); `--parameter` (repeatable key=value) | JSON object |
| **delete** | `gog gtm tags delete <path>` | `path` (positional). `--force`. Uses `confirmDestructive()`. | JSON `{"deleted": true}` |
| **revert** | `gog gtm tags revert <path>` | `path` (positional) | JSON object with reverted tag |
| **update** | `gog gtm tags update <path>` | `path` (positional); `--name`, `--type`, `--firing-trigger-id`, `--blocking-trigger-id`, `--parameter` (all optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- `create`: verify request body has name, type, firingTriggerId
- `delete`: `confirmDestructive()` + `--force` bypass
- `revert`: mock POST to revert endpoint
- `update`: partial update with `flagProvided()`

---

### 9. Workspace Triggers

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm triggers create` | workspace flags; `--name` (required); `--type` (required, e.g. pageview, click, formSubmit, customEvent, etc.); `--filter` (repeatable); `--custom-event-filter` (repeatable) | JSON object |
| **delete** | `gog gtm triggers delete <path>` | `path` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm triggers get <path>` | `path` (positional) | JSON object, TSV: TRIGGER_ID, NAME, TYPE |
| **revert** | `gog gtm triggers revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm triggers update <path>` | `path` (positional); `--name`, `--type`, `--filter` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD + revert cycle; verify trigger type enum values

---

### 10. Workspace Variables

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm variables create` | workspace flags; `--name` (required); `--type` (required); `--parameter` (repeatable key=value) | JSON object |
| **delete** | `gog gtm variables delete <path>` | `path` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm variables get <path>` | `path` (positional) | JSON object, TSV: VARIABLE_ID, NAME, TYPE |
| **revert** | `gog gtm variables revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm variables update <path>` | `path` (positional); `--name`, `--type`, `--parameter` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD + revert cycle; verify parameter key=value parsing

---

### 11. Workspace Built-in Variables

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm built-in-variables create` | workspace flags; `--type` (required, repeatable: e.g. pageUrl, pageHostname, pagePath, referrer, event, etc.) | JSON object with list of created built-in variables |
| **delete** | `gog gtm built-in-variables delete` | workspace flags; `--type` (required, repeatable). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **list** | `gog gtm built-in-variables list` | workspace flags | JSON array, TSV: TYPE, NAME |
| **revert** | `gog gtm built-in-variables revert` | workspace flags; `--type` (required) | JSON object |

**Notes:** Built-in variables use `type` enum instead of individual IDs. Create/delete take arrays of types.

**Test requirements:**
- `create`: verify type array in request body
- `delete`: confirmDestructive + verify type array
- `list`: mock response with multiple built-in variable types

---

### 12. Workspace Folders

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm folders create` | workspace flags; `--name` (required); `--notes` (optional) | JSON object |
| **delete** | `gog gtm folders delete <path>` | `path` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm folders get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm folders list` | workspace flags; `--max`, `--page` | JSON array, TSV: FOLDER_ID, NAME |
| **entities** | `gog gtm folders entities <path>` | `path` (positional) | JSON object with tags, triggers, variables in folder |
| **move-entities** | `gog gtm folders move-entities <path>` | `path` (positional); `--tag-id` (repeatable); `--trigger-id` (repeatable); `--variable-id` (repeatable) | JSON `{"moved": true}` |
| **revert** | `gog gtm folders revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm folders update <path>` | `path` (positional); `--name`, `--notes` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- `entities`: verify nested response with tags/triggers/variables arrays
- `move-entities`: verify tag/trigger/variable ID arrays in request
- Full CRUD + revert cycle

---

### 13. Workspace Templates

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm templates create` | workspace flags; `--name` (required); `--template-data` (required, file path or stdin) | JSON object |
| **delete** | `gog gtm templates delete <path>` | `path` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm templates get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm templates list` | workspace flags; `--max`, `--page` | JSON array, TSV: TEMPLATE_ID, NAME |
| **revert** | `gog gtm templates revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm templates update <path>` | `path` (positional); `--name`, `--template-data` (optional). Uses `flagProvided()`. | JSON object |
| **import-from-gallery** | `gog gtm templates import-from-gallery` | workspace flags; `--gallery-reference` (required: repository + owner + version) | JSON object |

**Test requirements:**
- `create`: verify template-data read from file or stdin
- `import-from-gallery`: verify gallery reference fields in request
- Full CRUD + revert cycle

---

### 14. Workspace Transformations

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm transformations create` | workspace flags; `--name` (required); `--type` (required); `--parameter` (repeatable key=value) | JSON object |
| **delete** | `gog gtm transformations delete <path>` | `path`. `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm transformations get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm transformations list` | workspace flags; `--max`, `--page` | JSON array, TSV: TRANSFORMATION_ID, NAME, TYPE |
| **revert** | `gog gtm transformations revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm transformations update <path>` | `path`; `--name`, `--type`, `--parameter` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD + revert cycle

---

### 15. Workspace Clients

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm clients create` | workspace flags; `--name` (required); `--type` (required); `--parameter` (repeatable key=value); `--priority` (optional int) | JSON object |
| **delete** | `gog gtm clients delete <path>` | `path`. `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm clients get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm clients list` | workspace flags; `--max`, `--page` | JSON array, TSV: CLIENT_ID, NAME, TYPE |
| **revert** | `gog gtm clients revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm clients update <path>` | `path`; `--name`, `--type`, `--parameter`, `--priority` (optional). Uses `flagProvided()`. | JSON object |

**Note:** Clients are server-side container resources only.

**Test requirements:**
- Full CRUD + revert cycle; verify server container context

---

### 16. Workspace Zones

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm zones create` | workspace flags; `--name` (required); `--type-restriction` (optional); `--child-container` (repeatable); `--boundary` (optional JSON) | JSON object |
| **delete** | `gog gtm zones delete <path>` | `path`. `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm zones get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm zones list` | workspace flags; `--max`, `--page` | JSON array, TSV: ZONE_ID, NAME |
| **revert** | `gog gtm zones revert <path>` | `path` (positional) | JSON object |
| **update** | `gog gtm zones update <path>` | `path`; `--name`, `--type-restriction`, `--child-container`, `--boundary` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD + revert cycle; verify zone boundary/child container structures

---

### 17. Workspace Gtag Config

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm gtag-config create` | workspace flags; `--type` (required); `--parameter` (repeatable key=value) | JSON object |
| **delete** | `gog gtm gtag-config delete <path>` | `path`. `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm gtag-config get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm gtag-config list` | workspace flags; `--max`, `--page` | JSON array, TSV: GTAG_CONFIG_ID, TYPE |
| **update** | `gog gtm gtag-config update <path>` | `path`; `--type`, `--parameter` (optional). Uses `flagProvided()`. | JSON object |

**Note:** No revert method for gtag_config.

**Test requirements:**
- CRUD cycle (no revert); verify parameter key=value parsing

---

### 18. User Permissions

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog gtm user-permissions create` | `--account-id` (required); `--email` (required); `--account-access-type` (optional: noAccess, user, admin); `--container-access` (optional repeatable JSON) | JSON object |
| **delete** | `gog gtm user-permissions delete <path>` | `path` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog gtm user-permissions get <path>` | `path` (positional) | JSON object |
| **list** | `gog gtm user-permissions list` | `--account-id` (required); `--max`, `--page` | JSON array, TSV: PERMISSION_ID, EMAIL, ACCOUNT_ACCESS |
| **update** | `gog gtm user-permissions update <path>` | `path`; `--account-access-type`, `--container-access` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle; verify containerAccess nested structure

---

## Edge Cases

1. **Deeply nested paths**: Tags, triggers, variables, and other workspace resources have paths 5 levels deep (accounts/containers/workspaces/resource/id). All commands accepting a `path` positional arg must validate the path has the correct number of segments.

2. **Workspace ID default**: The existing pattern defaults `--workspace-id` to `"0"` (the default workspace). New commands must preserve this convention.

3. **Path vs ID flags**: GET/DELETE/UPDATE commands accept full resource paths as positional args. LIST/CREATE commands use `--account-id`/`--container-id`/`--workspace-id` flags to build the parent path.

4. **Fingerprint optimistic locking**: Several operations (publish, resolve_conflict, update) accept a `--fingerprint` for optimistic concurrency. When provided, the API rejects requests with stale fingerprints.

5. **Server-side containers**: Clients, transformations, and zones are only available in server-side containers. Commands should document this in their help text.

6. **Built-in variables use type enums**: Unlike other resources that have string IDs, built-in variables are identified by their type enum values. Create/delete accept arrays of types.

7. **Workspace sync conflicts**: `sync` may return merge conflicts that require `resolve_conflict` to be called. The CLI should print actionable guidance when conflicts are detected.

8. **Container combine**: This is a cross-container operation that merges two containers. The `--setting-source` flag controls which container's settings win.

---

## Test Requirements Summary

Every method requires at minimum:
1. **JSON output test**: httptest mock server returning representative JSON; verify `outfmt.IsJSON` path produces valid JSON with correct structure
2. **Text output test**: Verify TSV table output (for LIST) or key-value output (for GET) via `captureStdout`
3. **Error test**: Verify proper error messages for missing required args
4. **Delete tests**: Must verify `confirmDestructive()` is called AND that `--force` bypasses confirmation

All tests use the pattern:
```go
origNew := newTagManagerService
t.Cleanup(func() { newTagManagerService = origNew })
// ... swap with httptest-backed service
```

Total test count estimate: ~200 tests (2 per method average across 99 methods).
