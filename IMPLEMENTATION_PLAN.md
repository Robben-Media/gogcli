# Implementation Plan — gogcli API Gap Coverage

Generated: 2026-02-22
Last Updated: 2026-02-22

## Summary

Adding 588 missing Google API methods to gogcli across 19 APIs to achieve full Discovery API parity. Plan is organized into 5 phases (small → massive) with 73 tasks, each covering one resource group.

## Architecture Decisions

1. **File organization**: One Go file per resource group (e.g., `bigquery_models.go` + `bigquery_models_test.go`). Keeps files focused and reviewable.
2. **Kong struct nesting**: New sub-resources register as fields in their parent API's cmd struct (e.g., `BigqueryCmd.Models`). New top-level commands (like `IdentityCmd` for Cloud Identity devices) register in `root.go` CLI struct.
3. **Existing service factories**: All 19 APIs already have service factories in `internal/googleapi/`. No new factories needed — only new service enum entries if missing from `internal/googleauth/service.go`.
4. **Long-running operations**: Cloud Identity device operations return `Operation` objects. Implement a shared `pollOperation()` helper in a new `internal/cmd/operations.go` file, reusable across APIs.
5. **Upload commands**: YouTube and Chat have multipart upload methods. Use `media.Media(reader)` pattern from the Google API client. Create a shared `openFileArg()` helper if one doesn't exist.
6. **Batch operations**: People API and Keep API have batch endpoints. Accept JSON via `--*-json` flags with `@filepath` support. Reuse existing `readJSONInput()` helper or create one.
7. **`--no-wait` flag**: For Cloud Identity LRO methods, support `--no-wait` to return the operation name without polling.

## Blockers / Questions

1. **MyBusiness command prefix**: The spec uses `gog mybusiness` and `gog mybusiness-info` as separate top-level commands. Current root.go has `BusinessProfile` — need to verify whether mybusiness account management and business information are separate or unified under `BusinessProfileCmd`.
2. **Tag Manager path vs ID flags**: The existing GTM commands use `--account-id`/`--container-id`/`--workspace-id` flags. New commands also accept full `path` positional args. Both interfaces should work — the path arg takes precedence when provided.
3. **YouTube `part` auto-computation**: The YouTube API requires explicit `part` parameters. The CLI should auto-compute `part` from which flags are provided (e.g., `--title` → `snippet`, `--privacy-status` → `status`).
4. **BigQuery jobs.insert complexity**: The spec suggests breaking into subcommands (`submit-query`, `submit-load`, `submit-extract`, `submit-copy`) for better UX. This is recommended over a monolithic `jobs submit --type` command.

---

## Phase 1: Small APIs (33 methods, 9 tasks)
> Why: Quick wins to build momentum and validate patterns. Each API has ≤8 gaps.
> Branch: `phase/1-small-apis` | PR → build branch

### Task 1: Google Docs — documents.create
- **Status**: completed
- **Depends on**: none
- **Spec**: specs/features/docs-gaps.md
- **Description**: Add `gog docs create` command. Creates a new Google Doc with optional title. Returns document ID and URL.
- **Files**:
  - `internal/cmd/docs_create.go` — modified (existing DocsCreateCmd in docs.go)
  - `internal/cmd/docs_create_test.go` — created
- **Methods**: documents.create (1)
- **Verification**: `make ci` passes, `gog docs create --help` works

### Task 2: Google Keep — Notes CRUD
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/keep-gaps.md
- **Description**: Add `gog keep notes create` (text notes and list notes via mutually exclusive flags) and `gog keep notes delete` (with confirmDestructive). Both require `requireWorkspaceAccount()`.
- **Files**:
  - `internal/cmd/keep_notes_edit.go` — create
  - `internal/cmd/keep_notes_edit_test.go` — create
- **Methods**: notes.create, notes.delete (2)
- **Verification**: `make ci` passes, `gog keep notes create --help`, `gog keep notes delete --help`

### Task 3: Google Keep — Permissions batch operations
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/keep-gaps.md
- **Description**: Add `gog keep permissions batch-create` (share note with multiple members) and `gog keep permissions batch-delete` (remove sharing, with confirmDestructive). Both require Workspace accounts.
- **Files**:
  - `internal/cmd/keep_permissions.go` — create
  - `internal/cmd/keep_permissions_test.go` — create
- **Methods**: notes.permissions.batchCreate, notes.permissions.batchDelete (2)
- **Verification**: `make ci` passes, `gog keep permissions batch-create --help`

### Task 4: Google Tasks — Task list management
- **Status**: completed
- **Depends on**: none
- **Spec**: specs/features/tasks-gaps.md
- **Description**: Add `gog tasks tasklists delete` (confirmDestructive, warns about cascading task deletion), `gog tasks tasklists get`, `gog tasks tasklists patch` (partial with flagProvided), and `gog tasks tasklists update` (full replace with PUT).
- **Files**:
  - `internal/cmd/tasks_tasklists_edit.go` — create
  - `internal/cmd/tasks_tasklists_edit_test.go` — create
- **Methods**: tasklists.delete, tasklists.get, tasklists.patch, tasklists.update (4)
- **Verification**: `make ci` passes, `gog tasks tasklists get --help`

### Task 5: Google Tasks — Task move and update
- **Status**: completed
- **Depends on**: none
- **Spec**: specs/features/tasks-gaps.md
- **Description**: Add `gog tasks move` (POST with query params for parent/previous positioning) and `gog tasks replace` (full PUT replace, contrast with existing `tasks update` which uses PATCH). Added `patch` alias to `update` for clarity.
- **Files**:
  - `internal/cmd/tasks_edit.go` — create
  - `internal/cmd/tasks_edit_test.go` — create
  - `internal/cmd/tasks.go` — modified (added Move, Replace, updated help text, added patch alias)
- **Methods**: tasks.move, tasks.update (2)
- **Verification**: `make ci` passes, `gog tasks move --help`, `gog tasks replace --help`

### Task 6: Search Console — Sitemaps and URL inspection
- **Status**: completed
- **Depends on**: none
- **Spec**: specs/features/searchconsole-gaps.md
- **Description**: Add sitemap CRUD (`gog gsc sitemaps delete/get/list/submit`) and URL inspection (`gog gsc url inspect`). The mobile-friendly test may also be in this spec. Sitemaps use the `webmasters` prefix in the API path.
- **Files**:
  - `internal/cmd/searchconsole_sitemaps.go` — create
  - `internal/cmd/searchconsole_sitemaps_test.go` — create
  - `internal/cmd/searchconsole_inspect.go` — create
  - `internal/cmd/searchconsole_inspect_test.go` — create
- **Methods**: sitemaps.delete, sitemaps.get, sitemaps.list, sitemaps.submit, urlInspection.index.inspect, urlTestingTools.mobileFriendlyTest.run (6)
- **Verification**: `make ci` passes, `gog gsc sitemaps list --help`

### Task 7: Analytics Data — Report operations
- **Status**: completed
- **Depends on**: none
- **Spec**: specs/features/analyticsdata-gaps.md
- **Description**: Add report commands: `gog analytics pivot-report`, `gog analytics batch-reports`, `gog analytics batch-pivot-reports`, `gog analytics check-compatibility`. These accept complex JSON request bodies via `--pivots-json`, `--requests-json`, or `--filter-json` flags. All support `@filepath` syntax for reading JSON from files.
- **Files**:
  - `internal/cmd/analytics_reports.go` — created
  - `internal/cmd/analytics_reports_test.go` — created
  - `internal/cmd/analytics.go` — modified (added new commands to AnalyticsCmd struct)
- **Methods**: properties.runPivotReport, properties.batchRunReports, properties.batchRunPivotReports, properties.checkCompatibility (4)
- **Verification**: `make ci` passes, `gog analytics pivot-report --help`, `gog analytics batch-reports --help`

### Task 8: Analytics Data — Audience exports
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/analyticsdata-gaps.md
- **Description**: Add audience export commands: `gog analytics data audience-exports create/get/list/query`. Create returns an operation; query returns exported user data.
- **Files**:
  - `internal/cmd/analyticsdata_audience.go` — create
  - `internal/cmd/analyticsdata_audience_test.go` — create
- **Methods**: properties.audienceExports.create, .get, .list, .query (3 — verify exact count from spec)
- **Verification**: `make ci` passes, `gog analytics data audience-exports list --help`

### Task 9: Sheets — All 8 gap methods
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/sheets-gaps.md
- **Description**: Add developer metadata commands (`gog sheets metadata get/search`), spreadsheet filter operations (`gog sheets get-by-filter`), sheet copy (`gog sheets copy-to`), and values operations (`gog sheets values batch-clear`, `batch-clear-by-filter`, `batch-get-by-filter`, `batch-update-by-filter`). DataFilter methods accept `--filters-json` with `@filepath` support.
- **Files**:
  - `internal/cmd/sheets_metadata.go` — create
  - `internal/cmd/sheets_metadata_test.go` — create
  - `internal/cmd/sheets_filter.go` — create
  - `internal/cmd/sheets_filter_test.go` — create
  - `internal/cmd/sheets_values_filter.go` — create
  - `internal/cmd/sheets_values_filter_test.go` — create
- **Methods**: developerMetadata.get, .search, spreadsheets.getByDataFilter, sheets.copyTo, values.batchClear, .batchClearByDataFilter, .batchGetByDataFilter, .batchUpdateByDataFilter (8)
- **Verification**: `make ci` passes, `gog sheets metadata get --help`, `gog sheets copy-to --help`

---

## Phase 2: Core APIs (145 methods, 18 tasks)
> Why: Highest user value. Gmail, Calendar, Drive, People, and Chat are the most-used APIs.
> Branch: `phase/2-core-apis` | PR → build branch

### Task 10: People — Contact groups CRUD
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/people-gaps.md
- **Description**: Add `gog contacts groups batch-get/create/delete/get/list/update`. Delete requires confirmDestructive. List supports pagination.
- **Files**:
  - `internal/cmd/people_contact_groups.go` — create
  - `internal/cmd/people_contact_groups_test.go` — create
- **Methods**: contactGroups.batchGet, .create, .delete, .get, .list, .update (6)
- **Verification**: `make ci` passes, `gog contacts groups list --help`

### Task 11: People — Contact group members modify
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/people-gaps.md
- **Description**: Add `gog contacts groups members modify` with `--add` and `--remove` flags for member resource names. At least one must be provided.
- **Files**:
  - `internal/cmd/people_group_members.go` — create
  - `internal/cmd/people_group_members_test.go` — create
- **Methods**: contactGroups.members.modify (1)
- **Verification**: `make ci` passes, `gog contacts groups members modify --help`

### Task 12: People — Batch contact operations
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/people-gaps.md
- **Description**: Add `gog contacts batch-create`, `batch-delete` (confirmDestructive), `batch-update`, and `batch-get`. Batch create/update accept `--contacts-json` with `@filepath` support.
- **Files**:
  - `internal/cmd/people_batch.go` — create
  - `internal/cmd/people_batch_test.go` — create
- **Methods**: people.batchCreateContacts, .batchDeleteContacts, .batchUpdateContacts, .getBatchGet (4)
- **Verification**: `make ci` passes, `gog contacts batch-create --help`

### Task 13: People — Photo management
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/people-gaps.md
- **Description**: Add `gog contacts delete-photo` (confirmDestructive) and `gog contacts update-photo` (reads image file, base64-encodes, sends as photoBytes).
- **Files**:
  - `internal/cmd/people_photo.go` — create
  - `internal/cmd/people_photo_test.go` — create
- **Methods**: people.deleteContactPhoto, people.updateContactPhoto (2)
- **Verification**: `make ci` passes, `gog contacts update-photo --help`

### Task 14: Gmail — Labels management
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/gmail-gaps.md
- **Description**: Add `gog gmail labels create/delete/patch/update`. Delete requires confirmDestructive. Patch uses flagProvided. Update is full replace.
- **Files**:
  - `internal/cmd/gmail_labels_edit.go` — create
  - `internal/cmd/gmail_labels_edit_test.go` — create
- **Methods**: labels.create, .delete, .patch, .update (4)
- **Verification**: `make ci` passes, `gog gmail labels create --help`

### Task 15: Gmail — Threads and history
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/gmail-gaps.md
- **Description**: Add thread operations (`gog gmail threads delete/get/list/modify/trash/untrash`) and history list (`gog gmail history list`).
- **Files**:
  - `internal/cmd/gmail_threads.go` — create
  - `internal/cmd/gmail_threads_test.go` — create
  - `internal/cmd/gmail_history.go` — create
  - `internal/cmd/gmail_history_test.go` — create
- **Methods**: threads.delete, .get, .list, .modify, .trash, .untrash, history.list (7)
- **Verification**: `make ci` passes, `gog gmail threads list --help`

### Task 16: Gmail — Message operations
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/gmail-gaps.md
- **Description**: Add message operations: `gog gmail messages send`, `gog gmail messages import`, `gog gmail messages insert`, `gog gmail messages modify`, `gog gmail messages trash/untrash`, `gog gmail messages batch-delete` (confirmDestructive), `gog gmail messages batch-modify`. Send handles MIME encoding.
- **Files**:
  - `internal/cmd/gmail_messages_edit.go` — create
  - `internal/cmd/gmail_messages_edit_test.go` — create
- **Methods**: messages.send, .import, .insert, .modify, .trash, .untrash, .batchDelete, .batchModify (8)
- **Verification**: `make ci` passes, `gog gmail messages send --help`

### Task 17: Gmail — Settings (delegates, filters, forwarding)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/gmail-gaps.md
- **Description**: Add settings commands: delegates CRUD, filters CRUD, forwarding addresses CRUD, send-as CRUD, auto-forwarding get/update, IMAP/POP get/update, vacation get/update, language get/update.
- **Files**:
  - `internal/cmd/gmail_settings.go` — create
  - `internal/cmd/gmail_settings_test.go` — create
  - `internal/cmd/gmail_settings_sendas.go` — create
  - `internal/cmd/gmail_settings_sendas_test.go` — create
- **Methods**: ~15 settings methods (delegates.create/delete/get/list, filters.create/delete/get/list, forwardingAddresses.create/delete/get/list, getAutoForwarding, updateAutoForwarding, getImap/updateImap, getPop/updatePop, getVacation/updateVacation, getLanguage/updateLanguage)
- **Verification**: `make ci` passes, `gog gmail settings delegates list --help`

### Task 18: Gmail — CSE (Client-Side Encryption)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/gmail-gaps.md
- **Description**: Add CSE commands for enterprise Gmail: `gog gmail cse identities create/delete/get/list/patch` and `gog gmail cse keypairs create/disable/enable/get/list/obliterate`. These are enterprise-only features.
- **Files**:
  - `internal/cmd/gmail_cse.go` — create
  - `internal/cmd/gmail_cse_test.go` — create
- **Methods**: cse.identities CRUD + cse.keypairs CRUD (~10 methods — verify from spec)
- **Verification**: `make ci` passes, `gog gmail cse identities list --help`

### Task 19: Calendar — ACL management
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/calendar-gaps.md
- **Description**: Add `gog calendar acl delete/get/insert/list/patch/watch`. ACL rules control calendar sharing. Watch implements webhook registration.
- **Files**:
  - `internal/cmd/calendar_acl.go` — create
  - `internal/cmd/calendar_acl_test.go` — create
- **Methods**: acl.delete, .get, .insert, .list, .patch, .watch (6)
- **Verification**: `make ci` passes, `gog calendar acl list --help`

### Task 20: Calendar — Calendar list and settings
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/calendar-gaps.md
- **Description**: Add calendar list operations (`gog calendar calendars delete/get/insert/patch/update/watch`) and settings (`gog calendar settings list/get/watch`). Also freebusy query (`gog calendar freebusy query`).
- **Files**:
  - `internal/cmd/calendar_list_edit.go` — create
  - `internal/cmd/calendar_list_edit_test.go` — create
  - `internal/cmd/calendar_settings.go` — create
  - `internal/cmd/calendar_settings_test.go` — create
- **Methods**: calendarList.delete, .get, .insert, .list (if missing), .patch, .update, .watch, settings.get, .list, .watch, freebusy.query (~11 methods)
- **Verification**: `make ci` passes, `gog calendar calendars get --help`

### Task 21: Calendar — Event watch and channels
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/calendar-gaps.md
- **Description**: Add event watch commands (`gog calendar events watch`, `gog calendar events import`, `gog calendar events instances`, `gog calendar events quick-add`, `gog calendar events move`) and channel stop (`gog calendar channels stop`). Watch commands share `watchFlags` pattern.
- **Files**:
  - `internal/cmd/calendar_events_edit.go` — create
  - `internal/cmd/calendar_events_edit_test.go` — create
  - `internal/cmd/calendar_channels.go` — create
  - `internal/cmd/calendar_channels_test.go` — create
- **Methods**: events.watch, .import, .instances, .quickAdd, .move, channels.stop (~6-9 methods)
- **Verification**: `make ci` passes, `gog calendar events watch --help`

### Task 22: Drive — Comments and replies
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/drive-gaps.md
- **Description**: Add `gog drive comments create/delete/get/list/update` and `gog drive replies create/delete/get/list/update`. Standard CRUD with file ID as parent.
- **Files**:
  - `internal/cmd/drive_comments.go` — create
  - `internal/cmd/drive_comments_test.go` — create
  - `internal/cmd/drive_replies.go` — create
  - `internal/cmd/drive_replies_test.go` — create
- **Methods**: comments.create, .delete, .get, .list, .update, replies.create, .delete, .get, .list, .update (10)
- **Verification**: `make ci` passes, `gog drive comments list --help`

### Task 23: Drive — Permissions management
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/drive-gaps.md
- **Description**: Add `gog drive permissions create/delete/get/list/update`. Manage file/folder sharing. Includes `--transfer-ownership` flag for ownership transfers.
- **Files**:
  - `internal/cmd/drive_permissions.go` — create
  - `internal/cmd/drive_permissions_test.go` — create
- **Methods**: permissions.create, .delete, .get, .list, .update (5)
- **Verification**: `make ci` passes, `gog drive permissions list --help`

### Task 24: Drive — Revisions and about
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/drive-gaps.md
- **Description**: Add `gog drive revisions delete/get/list/update`, `gog drive about get`, and `gog drive changes list/get-start-page-token/watch`. Also `gog drive channels stop`.
- **Files**:
  - `internal/cmd/drive_revisions.go` — create
  - `internal/cmd/drive_revisions_test.go` — create
  - `internal/cmd/drive_about.go` — create
  - `internal/cmd/drive_about_test.go` — create
  - `internal/cmd/drive_changes.go` — create
  - `internal/cmd/drive_changes_test.go` — create
- **Methods**: revisions.delete, .get, .list, .update, about.get, changes.list, .getStartPageToken, .watch, channels.stop (~9 methods)
- **Verification**: `make ci` passes, `gog drive revisions list --help`

### Task 25: Drive — Remaining operations (files, teamdrives, labels)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/drive-gaps.md
- **Description**: Add remaining Drive gaps: file operations (watch, generateIds, emptyTrash, export if missing), teamdrives/drives CRUD (deprecated but in Discovery API), and label management if specified. Check spec for exact remaining methods.
- **Files**:
  - `internal/cmd/drive_files_edit.go` — create/modify
  - `internal/cmd/drive_files_edit_test.go` — create
  - `internal/cmd/drive_teamdrives.go` — create
  - `internal/cmd/drive_teamdrives_test.go` — create
- **Methods**: Remaining ~16 Drive methods
- **Verification**: `make ci` passes, `gog drive --help` shows all new subcommands

### Task 26: Chat — Custom emojis and media
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/chat-gaps.md
- **Description**: Add `gog chat emoji create/delete/get/list` and `gog chat media download/upload`. All require Workspace accounts. Media upload uses multipart. Media download streams binary to file/stdout.
- **Files**:
  - `internal/cmd/chat_emoji.go` — create
  - `internal/cmd/chat_emoji_test.go` — create
  - `internal/cmd/chat_media.go` — create
  - `internal/cmd/chat_media_test.go` — create
- **Methods**: customEmojis.create, .delete, .get, .list, media.download, media.upload (6)
- **Verification**: `make ci` passes, `gog chat emoji list --help`

### Task 27: Chat — Spaces management
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/chat-gaps.md
- **Description**: Add `gog chat spaces complete-import/create/delete/find-dm/get/patch/search`. Patch uses flagProvided for updateMask. All require Workspace.
- **Files**:
  - `internal/cmd/chat_spaces_edit.go` — create
  - `internal/cmd/chat_spaces_edit_test.go` — create
- **Methods**: spaces.completeImport, .create, .delete, .findDirectMessage, .get, .patch, .search (7)
- **Verification**: `make ci` passes, `gog chat spaces search --help`

### Task 28: Chat — Members, messages, attachments, reactions, events, read states
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/chat-gaps.md
- **Description**: Add remaining Chat commands: members CRUD (`create/delete/get/list/patch`), messages operations (`delete/get/patch/update`), attachments get, reactions (`create/delete/list`), events (`get/list`), notification settings (`get/patch`), thread-read-state get, space-read-state update. This is a larger task covering the remaining 19 Chat methods.
- **Files**:
  - `internal/cmd/chat_members.go` — create
  - `internal/cmd/chat_members_test.go` — create
  - `internal/cmd/chat_messages_edit.go` — create
  - `internal/cmd/chat_messages_edit_test.go` — create
  - `internal/cmd/chat_reactions.go` — create
  - `internal/cmd/chat_reactions_test.go` — create
  - `internal/cmd/chat_events.go` — create
  - `internal/cmd/chat_events_test.go` — create
  - `internal/cmd/chat_readstate.go` — create
  - `internal/cmd/chat_readstate_test.go` — create
- **Methods**: members.create, .delete, .get, .list, .patch, messages.delete, .get, .patch, .update, attachments.get, reactions.create, .delete, .list, spaceEvents.get, .list, notificationSettings.get, .patch, threadReadState.get, spaceReadState.update (19)
- **Verification**: `make ci` passes, `gog chat members list --help`, `gog chat reactions list --help`

---

## Phase 3: Business APIs (28 methods, 5 tasks)
> Why: Niche but small — complete them in a focused sprint.
> Branch: `phase/3-business-apis` | PR → build branch

### Task 29: MyBusiness Account Management — Accounts
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/mybusinessaccountmanagement-gaps.md
- **Description**: Add `gog mybusiness accounts create/get/patch`. Patch uses flagProvided for updateMask.
- **Files**:
  - `internal/cmd/mybusiness_accounts.go` — create
  - `internal/cmd/mybusiness_accounts_test.go` — create
- **Methods**: accounts.create, .get, .patch (3)
- **Verification**: `make ci` passes, `gog mybusiness accounts get --help`

### Task 30: MyBusiness Account Management — Account admins and invitations
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/mybusinessaccountmanagement-gaps.md
- **Description**: Add account admins (`gog mybusiness account-admins create/delete/list/patch`) and invitations (`gog mybusiness account-invitations accept/decline/list`). No pagination on admin list or invitation list endpoints.
- **Files**:
  - `internal/cmd/mybusiness_account_admins.go` — create
  - `internal/cmd/mybusiness_account_admins_test.go` — create
  - `internal/cmd/mybusiness_invitations.go` — create
  - `internal/cmd/mybusiness_invitations_test.go` — create
- **Methods**: admins.create, .delete, .list, .patch, invitations.accept, .decline, .list (7)
- **Verification**: `make ci` passes, `gog mybusiness account-admins list --help`

### Task 31: MyBusiness Account Management — Location admins and transfer
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/mybusinessaccountmanagement-gaps.md
- **Description**: Add `gog mybusiness location-admins create/delete/list/patch` and `gog mybusiness locations transfer` (confirmDestructive — transfers ownership). No pagination on location admin list.
- **Files**:
  - `internal/cmd/mybusiness_location_admins.go` — create
  - `internal/cmd/mybusiness_location_admins_test.go` — create
- **Methods**: locations.admins.create, .delete, .list, .patch, locations.transfer (5)
- **Verification**: `make ci` passes, `gog mybusiness location-admins list --help`

### Task 32: MyBusiness Business Information — Categories, chains, locations
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/mybusinessbusinessinformation-gaps.md
- **Description**: Add reference data commands (`gog mybusiness-info categories batch-get/list`, `gog mybusiness-info chains get/search`, `gog mybusiness-info google-locations search`) and location operations (`gog mybusiness-info locations create/delete/get-google-updated/patch`).
- **Files**:
  - `internal/cmd/mybusiness_info_categories.go` — create
  - `internal/cmd/mybusiness_info_categories_test.go` — create
  - `internal/cmd/mybusiness_info_chains.go` — create
  - `internal/cmd/mybusiness_info_chains_test.go` — create
  - `internal/cmd/mybusiness_info_locations.go` — create
  - `internal/cmd/mybusiness_info_locations_test.go` — create
- **Methods**: categories.batchGet, .list, chains.get, .search, googleLocations.search, accounts.locations.create, locations.delete, .getGoogleUpdated, .patch (9 — verify)
- **Verification**: `make ci` passes, `gog mybusiness-info categories list --help`

### Task 33: MyBusiness Business Information — Attributes
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/mybusinessbusinessinformation-gaps.md
- **Description**: Add attributes commands: `gog mybusiness-info attributes list`, `gog mybusiness-info location-attributes get/get-google-updated/update`. Update accepts `--attributes-json` with `@filepath` support.
- **Files**:
  - `internal/cmd/mybusiness_info_attributes.go` — create
  - `internal/cmd/mybusiness_info_attributes_test.go` — create
- **Methods**: attributes.list, locations.getAttributes, locations.attributes.getGoogleUpdated, locations.updateAttributes (4)
- **Verification**: `make ci` passes, `gog mybusiness-info attributes list --help`

---

## Phase 4: Large APIs (146 methods, 16 tasks)
> Why: More methods per API, but well-structured resource groups keep tasks manageable.
> Branch: `phase/4-large-apis` | PR → build branch

### Task 34: BigQuery — Datasets CRUD
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/bigquery-gaps.md
- **Description**: Add `gog bigquery datasets get/create/delete/patch/update/undelete`. Delete requires confirmDestructive with `--delete-contents` flag. Restructure existing `BigqueryDatasetsCmd` to support subcommands alongside existing list.
- **Files**:
  - `internal/cmd/bigquery_datasets_admin.go` — create
  - `internal/cmd/bigquery_datasets_admin_test.go` — create
- **Methods**: datasets.get, .insert, .delete, .patch, .update, .undelete (6)
- **Verification**: `make ci` passes, `gog bigquery datasets get --help`

### Task 35: BigQuery — Jobs operations
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/bigquery-gaps.md
- **Description**: Add `gog bigquery jobs get/cancel/delete/results` and `gog bigquery jobs submit-query/submit-load/submit-extract/submit-copy` (split into subcommands per job type). Jobs.insert is the most complex — each type has different flags.
- **Files**:
  - `internal/cmd/bigquery_jobs_admin.go` — create
  - `internal/cmd/bigquery_jobs_admin_test.go` — create
- **Methods**: jobs.get, .cancel, .delete, .getQueryResults, .insert (5)
- **Verification**: `make ci` passes, `gog bigquery jobs get --help`, `gog bigquery jobs submit-query --help`

### Task 36: BigQuery — Models and projects
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/bigquery-gaps.md
- **Description**: Add `gog bigquery models list/get/patch/delete` and `gog bigquery projects list/service-account`. Standard CRUD patterns.
- **Files**:
  - `internal/cmd/bigquery_models.go` — create
  - `internal/cmd/bigquery_models_test.go` — create
  - `internal/cmd/bigquery_projects.go` — create
  - `internal/cmd/bigquery_projects_test.go` — create
- **Methods**: models.list, .get, .patch, .delete, projects.list, .getServiceAccount (6)
- **Verification**: `make ci` passes, `gog bigquery models list --help`

### Task 37: BigQuery — Routines
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/bigquery-gaps.md
- **Description**: Add `gog bigquery routines list/get/create/update/delete` plus IAM operations (`get-iam-policy/set-iam-policy/test-iam-permissions`). Routines are UDFs/stored procedures.
- **Files**:
  - `internal/cmd/bigquery_routines.go` — create
  - `internal/cmd/bigquery_routines_test.go` — create
- **Methods**: routines.list, .get, .insert, .update, .delete, .getIamPolicy, .setIamPolicy, .testIamPermissions (8)
- **Verification**: `make ci` passes, `gog bigquery routines list --help`

### Task 38: BigQuery — Row access policies
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/bigquery-gaps.md
- **Description**: Add `gog bigquery row-access-policies list/get/create/update/delete/batch-delete` plus IAM operations. Row-level security for BigQuery tables.
- **Files**:
  - `internal/cmd/bigquery_row_policies.go` — create
  - `internal/cmd/bigquery_row_policies_test.go` — create
- **Methods**: rowAccessPolicies.list, .get, .insert, .update, .delete, .batchDelete, .getIamPolicy, .testIamPermissions (8)
- **Verification**: `make ci` passes, `gog bigquery row-access-policies list --help`

### Task 39: BigQuery — Table data and tables admin
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/bigquery-gaps.md
- **Description**: Add `gog bigquery tabledata list/insert` (insert supports stdin) and `gog bigquery tables create/patch/update/delete` plus table IAM. Table create has complex schema flags.
- **Files**:
  - `internal/cmd/bigquery_tabledata.go` — create
  - `internal/cmd/bigquery_tabledata_test.go` — create
  - `internal/cmd/bigquery_tables_admin.go` — create
  - `internal/cmd/bigquery_tables_admin_test.go` — create
- **Methods**: tabledata.list, .insertAll, tables.insert, .patch, .update, .delete, .getIamPolicy, .setIamPolicy, .testIamPermissions (9)
- **Verification**: `make ci` passes, `gog bigquery tabledata list --help`

### Task 40: Classroom — Course aliases and course updates
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/classroom-gaps.md
- **Description**: Add `gog classroom aliases create/delete/list`, `gog classroom courses update` (full replace), and grading period settings (`gog classroom courses grading-periods get/update`).
- **Files**:
  - `internal/cmd/classroom_aliases.go` — create
  - `internal/cmd/classroom_aliases_test.go` — create
  - `internal/cmd/classroom_courses_edit.go` — create
  - `internal/cmd/classroom_courses_edit_test.go` — create
- **Methods**: aliases.create, .delete, .list, courses.update, .getGradingPeriodSettings, .updateGradingPeriodSettings (6)
- **Verification**: `make ci` passes, `gog classroom aliases list --help`

### Task 41: Classroom — Add-on attachments
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/classroom-gaps.md
- **Description**: Add `gog classroom addons create/delete/get/list/patch` with `--item-type` flag routing to 4 parent types (announcement, coursework, material, post). Also add `gog classroom addons context` for getAddOnContext. Shared implementation with type-based routing.
- **Files**:
  - `internal/cmd/classroom_addons.go` — create
  - `internal/cmd/classroom_addons_test.go` — create
- **Methods**: 20 addon attachment methods (5 ops × 4 parents) + 4 getAddOnContext methods = 24
- **Verification**: `make ci` passes, `gog classroom addons create --help`

### Task 42: Classroom — Add-on student submissions
- **Status**: pending
- **Depends on**: Task 41
- **Spec**: specs/features/classroom-gaps.md
- **Description**: Add `gog classroom addons submissions get/patch` with `--item-type` routing (coursework, post). Also add `gog classroom submissions modify-attachments`.
- **Files**:
  - `internal/cmd/classroom_addon_submissions.go` — create
  - `internal/cmd/classroom_addon_submissions_test.go` — create
- **Methods**: studentSubmissions.get ×2, .patch ×2, submissions.modifyAttachments (5)
- **Verification**: `make ci` passes, `gog classroom addons submissions get --help`

### Task 43: Classroom — Rubrics
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/classroom-gaps.md
- **Description**: Add `gog classroom rubrics create/delete/get/list/patch`. Create accepts `--criteria` as JSON array. Standard CRUD under courseWork parent.
- **Files**:
  - `internal/cmd/classroom_rubrics.go` — create
  - `internal/cmd/classroom_rubrics_test.go` — create
- **Methods**: rubrics.create, .delete, .get, .list, .patch (5)
- **Verification**: `make ci` passes, `gog classroom rubrics list --help`

### Task 44: Classroom — Student groups, registrations, guardian invitation patch
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/classroom-gaps.md
- **Description**: Add student groups (`gog classroom student-groups create/delete/list/patch`), group members (`gog classroom student-groups members create/delete/list`), registrations (`gog classroom registrations create/delete`), and guardian invitation patch (`gog classroom guardian-invitations patch`).
- **Files**:
  - `internal/cmd/classroom_student_groups.go` — create
  - `internal/cmd/classroom_student_groups_test.go` — create
  - `internal/cmd/classroom_registrations.go` — create
  - `internal/cmd/classroom_registrations_test.go` — create
- **Methods**: studentGroups.create, .delete, .list, .patch, members.create, .delete, .list, registrations.create, .delete, guardianInvitations.patch (10 — verify)
- **Verification**: `make ci` passes, `gog classroom student-groups list --help`

### Task 45: Analytics Admin — Accounts
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/analyticsadmin-gaps.md
- **Description**: Add `gog analytics admin accounts get/delete/patch/data-sharing/provision-ticket/access-report/change-history`. Add `AnalyticsAdminCmd` struct and register in `AnalyticsCmd`. Normalize input to prepend `accounts/` if missing.
- **Files**:
  - `internal/cmd/analytics_admin.go` — create
  - `internal/cmd/analytics_admin_test.go` — create
- **Methods**: accounts.get, .delete, .patch, .getDataSharingSettings, .provisionAccountTicket, .runAccessReport, .searchChangeHistoryEvents (7)
- **Verification**: `make ci` passes, `gog analytics admin accounts get --help`

### Task 46: Analytics Admin — Properties
- **Status**: pending
- **Depends on**: Task 45
- **Spec**: specs/features/analyticsadmin-gaps.md
- **Description**: Add `gog analytics admin properties create/delete/get/list/patch/acknowledge-data/access-report/data-retention get/update`. Properties are the core GA4 resource.
- **Files**:
  - `internal/cmd/analytics_admin_properties.go` — create
  - `internal/cmd/analytics_admin_properties_test.go` — create
- **Methods**: properties.create, .delete, .get, .list, .patch, .acknowledgeUserDataCollection, .runAccessReport, .getDataRetentionSettings, .updateDataRetentionSettings (9)
- **Verification**: `make ci` passes, `gog analytics admin properties list --help`

### Task 47: Analytics Admin — Conversion events, custom dimensions, custom metrics
- **Status**: pending
- **Depends on**: Task 45
- **Spec**: specs/features/analyticsadmin-gaps.md
- **Description**: Add conversion events CRUD+patch, custom dimensions CRUD+archive, custom metrics CRUD+archive. All nested under properties. Archive operations are irreversible (use confirmDestructive).
- **Files**:
  - `internal/cmd/analytics_admin_events.go` — create
  - `internal/cmd/analytics_admin_events_test.go` — create
  - `internal/cmd/analytics_admin_dimensions.go` — create
  - `internal/cmd/analytics_admin_dimensions_test.go` — create
  - `internal/cmd/analytics_admin_metrics.go` — create
  - `internal/cmd/analytics_admin_metrics_test.go` — create
- **Methods**: conversionEvents ×5, customDimensions ×5, customMetrics ×5 (15)
- **Verification**: `make ci` passes, `gog analytics admin conversion-events list --help`

### Task 48: Analytics Admin — Data streams, MP secrets, Firebase links, Google Ads links, key events
- **Status**: pending
- **Depends on**: Task 45
- **Spec**: specs/features/analyticsadmin-gaps.md
- **Description**: Add data streams CRUD (5), measurement protocol secrets CRUD (5), Firebase links create/delete/list (3), Google Ads links CRUD (4), key events CRUD+patch (5). All nested under properties.
- **Files**:
  - `internal/cmd/analytics_admin_streams.go` — create
  - `internal/cmd/analytics_admin_streams_test.go` — create
  - `internal/cmd/analytics_admin_links.go` — create
  - `internal/cmd/analytics_admin_links_test.go` — create
  - `internal/cmd/analytics_admin_keyevents.go` — create
  - `internal/cmd/analytics_admin_keyevents_test.go` — create
- **Methods**: dataStreams ×5, mpSecrets ×5, firebaseLinks ×3, googleAdsLinks ×4, keyEvents ×5 (22)
- **Verification**: `make ci` passes, `gog analytics admin data-streams list --help`

---

## Phase 5: Massive APIs (236 methods, 25 tasks)
> Why: Largest gap counts. Tag Manager (99), YouTube (77), Cloud Identity (60) require the most tasks.
> Branch: `phase/5-massive-apis` | PR → build branch

### Task 49: Tag Manager — Accounts and containers
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm accounts get/update` and `gog gtm containers create/delete/get/update/combine/lookup/move-tag-id/snippet`. Containers have several specialized operations beyond basic CRUD.
- **Files**:
  - `internal/cmd/tagmanager_accounts.go` — create
  - `internal/cmd/tagmanager_accounts_test.go` — create
  - `internal/cmd/tagmanager_containers.go` — create
  - `internal/cmd/tagmanager_containers_test.go` — create
- **Methods**: accounts.get, .update, containers.create, .delete, .get, .update, .combine, .lookup, .move_tag_id, .snippet (10)
- **Verification**: `make ci` passes, `gog gtm accounts get --help`

### Task 50: Tag Manager — Container versions
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm versions delete/get/live/publish/set-latest/undelete/update`. Version operations control the GTM publishing workflow.
- **Files**:
  - `internal/cmd/tagmanager_versions.go` — create
  - `internal/cmd/tagmanager_versions_test.go` — create
- **Methods**: versions.delete, .get, .live, .publish, .set_latest, .undelete, .update (7)
- **Verification**: `make ci` passes, `gog gtm versions get --help`

### Task 51: Tag Manager — Version headers and environments
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm version-headers latest` and environments CRUD (`gog gtm environments create/delete/get/list/reauthorize/update`).
- **Files**:
  - `internal/cmd/tagmanager_version_headers.go` — create
  - `internal/cmd/tagmanager_version_headers_test.go` — create
  - `internal/cmd/tagmanager_environments.go` — create
  - `internal/cmd/tagmanager_environments_test.go` — create
- **Methods**: versionHeaders.latest, environments.create, .delete, .get, .list, .reauthorize, .update (7)
- **Verification**: `make ci` passes, `gog gtm environments list --help`

### Task 52: Tag Manager — Destinations and workspaces
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm destinations get/link/list` and workspace management (`gog gtm workspaces create/delete/get/status/list/update/quick-preview/resolve-conflict/sync/create-version/bulk-update`). Workspaces have many specialized operations.
- **Files**:
  - `internal/cmd/tagmanager_destinations.go` — create
  - `internal/cmd/tagmanager_destinations_test.go` — create
  - `internal/cmd/tagmanager_workspaces.go` — create
  - `internal/cmd/tagmanager_workspaces_test.go` — create
- **Methods**: destinations.get, .link, .list, workspaces.create, .delete, .get, .getStatus, .list, .update, .quick_preview, .resolve_conflict, .sync, .create_version, .bulk_update (14)
- **Verification**: `make ci` passes, `gog gtm workspaces list --help`

### Task 53: Tag Manager — Tags (create/delete/revert/update)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm tags create/delete/revert/update`. Tags already have get/list. Create accepts type, firing-trigger-id, blocking-trigger-id, and parameter key=value pairs.
- **Files**:
  - `internal/cmd/tagmanager_tags_edit.go` — create
  - `internal/cmd/tagmanager_tags_edit_test.go` — create
- **Methods**: tags.create, .delete, .revert, .update (4)
- **Verification**: `make ci` passes, `gog gtm tags create --help`

### Task 54: Tag Manager — Triggers (CRUD + revert)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm triggers create/delete/get/revert/update`. Triggers already have list. Create accepts type and filter specs.
- **Files**:
  - `internal/cmd/tagmanager_triggers_edit.go` — create
  - `internal/cmd/tagmanager_triggers_edit_test.go` — create
- **Methods**: triggers.create, .delete, .get, .revert, .update (5)
- **Verification**: `make ci` passes, `gog gtm triggers create --help`

### Task 55: Tag Manager — Variables (CRUD + revert)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm variables create/delete/get/revert/update`. Variables already have list. Parameter key=value parsing.
- **Files**:
  - `internal/cmd/tagmanager_variables_edit.go` — create
  - `internal/cmd/tagmanager_variables_edit_test.go` — create
- **Methods**: variables.create, .delete, .get, .revert, .update (5)
- **Verification**: `make ci` passes, `gog gtm variables create --help`

### Task 56: Tag Manager — Built-in variables
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm built-in-variables create/delete/list/revert`. Built-in variables use type enums instead of individual IDs. Create/delete accept arrays of types.
- **Files**:
  - `internal/cmd/tagmanager_builtins.go` — create
  - `internal/cmd/tagmanager_builtins_test.go` — create
- **Methods**: built_in_variables.create, .delete, .list, .revert (4)
- **Verification**: `make ci` passes, `gog gtm built-in-variables list --help`

### Task 57: Tag Manager — Folders
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm folders create/delete/get/list/entities/move-entities/revert/update`. Entities command returns tags/triggers/variables in a folder. Move-entities relocates items between folders.
- **Files**:
  - `internal/cmd/tagmanager_folders.go` — create
  - `internal/cmd/tagmanager_folders_test.go` — create
- **Methods**: folders.create, .delete, .get, .list, .entities, .move_entities_to_folder, .revert, .update (8)
- **Verification**: `make ci` passes, `gog gtm folders list --help`

### Task 58: Tag Manager — Templates
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm templates create/delete/get/list/revert/update/import-from-gallery`. Template data is read from file or stdin. Import-from-gallery requires gallery reference fields.
- **Files**:
  - `internal/cmd/tagmanager_templates.go` — create
  - `internal/cmd/tagmanager_templates_test.go` — create
- **Methods**: templates.create, .delete, .get, .list, .revert, .update, .import_from_gallery (7 — verify)
- **Verification**: `make ci` passes, `gog gtm templates list --help`

### Task 59: Tag Manager — Transformations, clients, zones, gtag config
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add transformations CRUD+revert (6), clients CRUD+revert (6), zones CRUD+revert (6), gtag-config CRUD (5, no revert). All server-side container resources follow the same pattern.
- **Files**:
  - `internal/cmd/tagmanager_transformations.go` — create
  - `internal/cmd/tagmanager_transformations_test.go` — create
  - `internal/cmd/tagmanager_clients.go` — create
  - `internal/cmd/tagmanager_clients_test.go` — create
  - `internal/cmd/tagmanager_zones.go` — create
  - `internal/cmd/tagmanager_zones_test.go` — create
  - `internal/cmd/tagmanager_gtag_config.go` — create
  - `internal/cmd/tagmanager_gtag_config_test.go` — create
- **Methods**: transformations ×6, clients ×6, zones ×6, gtag_config ×5 (23)
- **Verification**: `make ci` passes, `gog gtm transformations list --help`

### Task 60: Tag Manager — User permissions
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/tagmanager-gaps.md
- **Description**: Add `gog gtm user-permissions create/delete/get/list/update`. Container access uses nested JSON for role assignment per container.
- **Files**:
  - `internal/cmd/tagmanager_permissions.go` — create
  - `internal/cmd/tagmanager_permissions_test.go` — create
- **Methods**: user_permissions.create, .delete, .get, .list, .update (5)
- **Verification**: `make ci` passes, `gog gtm user-permissions list --help`

### Task 61: YouTube — Videos (upload, delete, rate, update, getRating, reportAbuse)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add `gog yt videos upload` (multipart upload with progress), `gog yt videos delete` (confirmDestructive), `gog yt videos rate`, `gog yt videos update` (flagProvided + auto-computed part), `gog yt videos get-rating`, `gog yt videos report-abuse`. Upload is the most complex — handles video files with resumable upload.
- **Files**:
  - `internal/cmd/youtube_videos_edit.go` — create
  - `internal/cmd/youtube_videos_edit_test.go` — create
- **Methods**: videos.delete, .getRating, .insert, .rate, .reportAbuse, .update (6)
- **Verification**: `make ci` passes, `gog yt videos upload --help`

### Task 62: YouTube — Comments and comment threads
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add `gog yt comments delete/insert/list/mark-as-spam/set-moderation-status/update` and `gog yt comment-threads insert/update`.
- **Files**:
  - `internal/cmd/youtube_comments.go` — create
  - `internal/cmd/youtube_comments_test.go` — create
- **Methods**: comments.delete, .insert, .list, .markAsSpam, .setModerationStatus, .update, commentThreads.insert, .update (8)
- **Verification**: `make ci` passes, `gog yt comments list --help`

### Task 63: YouTube — Captions, channel banners, thumbnails, watermarks
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add captions CRUD (`gog yt captions delete/download/upload/list/update`), channel banners upload, thumbnails set, watermarks set/unset. Multiple multipart upload commands.
- **Files**:
  - `internal/cmd/youtube_captions.go` — create
  - `internal/cmd/youtube_captions_test.go` — create
  - `internal/cmd/youtube_media.go` — create
  - `internal/cmd/youtube_media_test.go` — create
- **Methods**: captions.delete, .download, .insert, .list, .update, channelBanners.insert, thumbnails.set, watermarks.set, .unset (9)
- **Verification**: `make ci` passes, `gog yt captions list --help`

### Task 64: YouTube — Channels, channel sections, playlists, playlist items, playlist images
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add `gog yt channels update`, channel sections CRUD, playlists insert/delete/update (list exists), playlist items insert/delete/update (list exists), playlist images CRUD.
- **Files**:
  - `internal/cmd/youtube_channels_edit.go` — create
  - `internal/cmd/youtube_channels_edit_test.go` — create
  - `internal/cmd/youtube_playlists_edit.go` — create
  - `internal/cmd/youtube_playlists_edit_test.go` — create
  - `internal/cmd/youtube_playlist_images.go` — create
  - `internal/cmd/youtube_playlist_images_test.go` — create
- **Methods**: channels.update, channelSections.delete, .insert, .list, .update, playlists.delete, .insert, .update, playlistItems.delete, .insert, .update, playlistImages.delete, .insert, .list, .update (15)
- **Verification**: `make ci` passes, `gog yt playlists insert --help`

### Task 65: YouTube — Subscriptions
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add `gog yt subscriptions delete/insert/list`. List has multiple filter options (mine, channel-id, for-channel-id).
- **Files**:
  - `internal/cmd/youtube_subscriptions.go` — create
  - `internal/cmd/youtube_subscriptions_test.go` — create
- **Methods**: subscriptions.delete, .insert, .list (3)
- **Verification**: `make ci` passes, `gog yt subscriptions list --help`

### Task 66: YouTube — Live broadcasts and streams
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add live broadcast commands (`gog yt live-broadcasts bind/delete/insert/insert-cuepoint/list/transition/update`) and live stream commands (`gog yt live-streams delete/insert/list/update`). Broadcasts have state machine transitions.
- **Files**:
  - `internal/cmd/youtube_live_broadcasts.go` — create
  - `internal/cmd/youtube_live_broadcasts_test.go` — create
  - `internal/cmd/youtube_live_streams.go` — create
  - `internal/cmd/youtube_live_streams_test.go` — create
- **Methods**: liveBroadcasts.bind, .delete, .insert, .insertCuepoint, .list, .transition, .update, liveStreams.delete, .insert, .list, .update (11)
- **Verification**: `make ci` passes, `gog yt live-broadcasts list --help`

### Task 67: YouTube — Live chat (bans, messages, moderators)
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add live chat bans (`insert/delete`), messages (`delete/insert/list/transition`), moderators (`delete/insert/list`), and the live chat stream command (long-polling).
- **Files**:
  - `internal/cmd/youtube_live_chat.go` — create
  - `internal/cmd/youtube_live_chat_test.go` — create
- **Methods**: liveChatBans.delete, .insert, liveChatMessages.delete, .insert, .list, .transition, liveChatModerators.delete, .insert, .list, liveChatStream (10)
- **Verification**: `make ci` passes, `gog yt live-chat-messages list --help`

### Task 68: YouTube — Reference data and remaining methods
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/youtube-gaps.md
- **Description**: Add members list, membershipsLevels list, superChatEvents list, i18nLanguages list, i18nRegions list, videoAbuseReportReasons list, videoCategories list, abuseReports insert, activities list, thirdPartyLinks CRUD, tests insert, videoTrainability get.
- **Files**:
  - `internal/cmd/youtube_reference.go` — create
  - `internal/cmd/youtube_reference_test.go` — create
  - `internal/cmd/youtube_misc.go` — create
  - `internal/cmd/youtube_misc_test.go` — create
- **Methods**: members.list, membershipsLevels.list, superChatEvents.list, i18nLanguages.list, i18nRegions.list, videoAbuseReportReasons.list, videoCategories.list, abuseReports.insert, activities.list, thirdPartyLinks CRUD (4), tests.insert, videoTrainability.get (15)
- **Verification**: `make ci` passes, `gog yt members list --help`

### Task 69: Cloud Identity — Groups CRUD and search
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/cloudidentity-gaps.md
- **Description**: Add `gog groups create/delete/get/list/update/search/security-settings/update-security-settings`. Extends existing GroupsCmd. Search uses CEL query syntax.
- **Files**:
  - `internal/cmd/groups_edit.go` — create
  - `internal/cmd/groups_edit_test.go` — create
- **Methods**: groups.create, .delete, .get, .list, .patch, .search, .getSecuritySettings, .updateSecuritySettings (8)
- **Verification**: `make ci` passes, `gog groups create --help`

### Task 70: Cloud Identity — Memberships
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/cloudidentity-gaps.md
- **Description**: Add `gog groups memberships check-transitive/create/delete/get/graph/lookup/modify-roles/search-direct/search-transitive-memberships`. Extends existing memberships. Graph returns operation (LRO).
- **Files**:
  - `internal/cmd/groups_memberships_edit.go` — create
  - `internal/cmd/groups_memberships_edit_test.go` — create
- **Methods**: memberships.checkTransitiveMembership, .create, .delete, .get, .getMembershipGraph, .lookup, .modifyMembershipRoles, .searchDirectGroups, .searchTransitiveMemberships (9)
- **Verification**: `make ci` passes, `gog groups memberships create --help`

### Task 71: Cloud Identity — Devices and operation polling
- **Status**: pending
- **Depends on**: none
- **Spec**: specs/features/cloudidentity-gaps.md
- **Description**: Add `gog identity devices cancel-wipe/create/delete/get/list/wipe` with long-running operation polling. Create shared `pollOperation()` helper. Wipe requires extra-strong confirmDestructive message. Add `--no-wait` flag. Register new `IdentityCmd` in root.go.
- **Files**:
  - `internal/cmd/operations.go` — create (shared LRO helper)
  - `internal/cmd/identity.go` — create (IdentityCmd struct)
  - `internal/cmd/identity_devices.go` — create
  - `internal/cmd/identity_devices_test.go` — create
  - `internal/cmd/root.go` — modify (register IdentityCmd)
- **Methods**: devices.cancelWipe, .create, .delete, .get, .list, .wipe (6)
- **Verification**: `make ci` passes, `gog identity devices list --help`

### Task 72: Cloud Identity — Device users and client states
- **Status**: pending
- **Depends on**: Task 71
- **Spec**: specs/features/cloudidentity-gaps.md
- **Description**: Add `gog identity device-users approve/block/cancel-wipe/delete/get/list/lookup/wipe` (all LRO-returning methods use shared pollOperation) and `gog identity client-states get/list/update`.
- **Files**:
  - `internal/cmd/identity_device_users.go` — create
  - `internal/cmd/identity_device_users_test.go` — create
  - `internal/cmd/identity_client_states.go` — create
  - `internal/cmd/identity_client_states_test.go` — create
- **Methods**: deviceUsers.approve, .block, .cancelWipe, .delete, .get, .list, .lookup, .wipe, clientStates.get, .list, .patch (11)
- **Verification**: `make ci` passes, `gog identity device-users list --help`

### Task 73: Cloud Identity — Invitations, SSO profiles, SSO assignments, policies
- **Status**: pending
- **Depends on**: Task 71
- **Spec**: specs/features/cloudidentity-gaps.md
- **Description**: Add customer invitations (`cancel/get/check/list/send`), OIDC SSO profiles CRUD (5), SAML SSO profiles CRUD (5), SAML IDP credentials (add/delete/get/list), SSO assignments CRUD+patch (5), policies (get/list). Several operations return LROs.
- **Files**:
  - `internal/cmd/identity_invitations.go` — create
  - `internal/cmd/identity_invitations_test.go` — create
  - `internal/cmd/identity_sso.go` — create
  - `internal/cmd/identity_sso_test.go` — create
  - `internal/cmd/identity_policies.go` — create
  - `internal/cmd/identity_policies_test.go` — create
- **Methods**: invitations ×5, oidcProfiles ×5, samlProfiles ×5, samlCredentials ×4, ssoAssignments ×5, policies ×2 (26)
- **Verification**: `make ci` passes, `gog identity oidc-profiles list --help`

---

## Summary

| Phase | APIs | Methods | Tasks |
|-------|------|---------|-------|
| 1: Small APIs | docs, keep, tasks, searchconsole, analyticsdata, sheets | 33 | 9 |
| 2: Core APIs | gmail, calendar, drive, people, chat | 145 | 18 |
| 3: Business APIs | mybusinessaccountmanagement, mybusinessbusinessinformation | 28 | 5 |
| 4: Large APIs | bigquery, classroom, analyticsadmin | 146 | 16 |
| 5: Massive APIs | youtube, tagmanager, cloudidentity | 236 | 25 |
| **Total** | **19 APIs** | **588** | **73** |

## Recommended Starting Point

Begin with **Phase 1, Task 1** (Google Docs `documents.create`) — it's a single method, validating the full cycle: struct definition → Run() → Kong registration → test → make ci. Then proceed sequentially through Phase 1 to build momentum.

## Estimated Effort

- Phase 1: ~9 iterations (1 per task)
- Phase 2: ~20 iterations (some tasks like Chat members and Gmail settings are larger)
- Phase 3: ~5 iterations
- Phase 4: ~18 iterations (Classroom add-ons and BigQuery jobs are complex)
- Phase 5: ~28 iterations (Tag Manager alone has 12 tasks)
- **Total: ~80 iterations** (recommended limit: `./loop.sh 80`)
