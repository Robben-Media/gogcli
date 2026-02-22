# Google Chat v1 -- Gap Coverage Spec

**API**: Google Chat v1 (`chat/v1`)
**Current coverage**: 5 methods (spaces.list, spaces.messages.create, spaces.messages.list, spaces.setup, users.spaces.getSpaceReadState)
**Gap**: 32 missing methods
**Service factory**: `newChatService` in `chat_services.go`
**Workspace-only**: Yes -- all commands must call `requireWorkspaceAccount(account)`

## Overview

Adding 32 missing methods to the Google Chat v1 CLI commands. Covers custom emojis (CRUD), media (download/upload), spaces (complete-import/create/delete/find-dm/get/patch/search), members (CRUD), messages (delete/get/patch/update), attachments (get), reactions (create/delete/list), space events (get/list), notification settings (get/patch), thread read state (get), and space read state (update) to achieve full Discovery API parity.

All commands require Workspace accounts via `requireWorkspaceAccount(account)`. Commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations require `confirmDestructive()`. List commands include `--max` and `--page` flags for pagination. JSON output includes `nextPageToken`. Text output uses TSV-formatted tables.

---

## Custom Emojis

### `gog chat emoji create`

- **API method**: `customEmojis.create`
- **Struct**: `ChatEmojiCreateCmd`
- **Args/Flags**:
  - `--shortcode` (required string): emoji shortcode without colons
  - `--image` (required string): path to local image file (PNG/GIF, max 256KB)
- **Behavior**: Reads image file, uploads as customEmoji. Returns created resource JSON.
- **Output**: JSON object with `name`, `uid`, `shortcode`, `payload`
- **Test**: httptest mock POST `/v1/customEmojis`, assert request body contains shortcode, assert response parsed

### `gog chat emoji delete`

- **API method**: `customEmojis.delete`
- **Struct**: `ChatEmojiDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): custom emoji resource name (e.g., `customEmojis/abc123`)
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Calls DELETE on the resource.
- **Output**: Empty response on success, prints confirmation message to stderr
- **Test**: httptest mock DELETE `/v1/customEmojis/{id}`, verify --force bypasses prompt, verify non-force returns ExitError code 2 with --no-input

### `gog chat emoji get`

- **API method**: `customEmojis.get`
- **Struct**: `ChatEmojiGetCmd`
- **Args/Flags**:
  - `name` (required arg): custom emoji resource name
- **Output**: JSON object with emoji details; text mode shows NAME, SHORTCODE, UID columns
- **Test**: httptest mock GET `/v1/customEmojis/{id}`, assert JSON output

### `gog chat emoji list`

- **API method**: `customEmojis.list`
- **Struct**: `ChatEmojiListCmd`
- **Args/Flags**:
  - `--max` (int64, default 100, alias `limit`): page size
  - `--page` (string): page token
- **Output**: JSON array `{"customEmojis": [...], "nextPageToken": "..."}`. Text mode: NAME, SHORTCODE, UID columns with TSV table.
- **Test**: httptest mock GET `/v1/customEmojis`, assert pagination params forwarded, assert table output

---

## Media

### `gog chat media download`

- **API method**: `media.download`
- **Struct**: `ChatMediaDownloadCmd`
- **Args/Flags**:
  - `name` (required arg): media resource name (e.g., `spaces/xxx/messages/yyy/attachments/zzz`)
  - `--output` / `-o` (string): output file path (default: stdout)
- **Behavior**: Streams binary content to file or stdout. Use `resp.Body` directly for large files.
- **Output**: Binary data to file/stdout
- **Test**: httptest mock GET `/v1/media/{name}?alt=media`, assert binary content written

### `gog chat media upload`

- **API method**: `media.upload`
- **Struct**: `ChatMediaUploadCmd`
- **Args/Flags**:
  - `parent` (required arg): space resource name (e.g., `spaces/xxx`)
  - `--file` (required string): local file path
  - `--filename` (string): override filename in upload metadata
- **Behavior**: Reads local file, uploads via multipart. Returns attachment metadata.
- **Output**: JSON object with uploaded attachment resource
- **Test**: httptest mock POST `/upload/v1/media/{parent}`, assert multipart body

---

## Spaces

### `gog chat spaces complete-import`

- **API method**: `spaces.completeImport`
- **Struct**: `ChatSpacesCompleteImportCmd`
- **Args/Flags**:
  - `name` (required arg): space resource name
- **Behavior**: POST to complete import of a space. Returns updated space.
- **Output**: JSON object of the completed space
- **Test**: httptest mock POST `/v1/{name}:completeImport`

### `gog chat spaces create`

- **API method**: `spaces.create`
- **Struct**: `ChatSpacesCreateCmd` (already exists -- verify coverage or extend)
- **Args/Flags**:
  - `--display-name` (required string): space display name
  - `--type` (string, default "SPACE"): SPACE or GROUP_CHAT
  - `--description` (string): space description
  - `--external-allowed` (bool): allow external users
  - `--threading` (string): THREADED_MESSAGES or UNTHREADED_MESSAGES
- **Note**: Check if existing `ChatSpacesCreateCmd` covers this. If it uses `spaces.setup` instead, add a separate `spaces.create` path.
- **Test**: httptest mock POST `/v1/spaces`, assert request body fields

### `gog chat spaces delete`

- **API method**: `spaces.delete`
- **Struct**: `ChatSpacesDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): space resource name
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/spaces/{id}`, verify --force flag, verify destructive confirmation

### `gog chat spaces find-dm`

- **API method**: `spaces.findDirectMessage`
- **Struct**: `ChatSpacesFindDmCmd`
- **Args/Flags**:
  - `--user` (required string): user resource name or email
- **Behavior**: GET with query parameter. Returns the DM space if one exists.
- **Output**: JSON object of the space; text shows RESOURCE, NAME, TYPE columns
- **Test**: httptest mock GET `/v1/spaces:findDirectMessage?name={user}`

### `gog chat spaces get`

- **API method**: `spaces.get`
- **Struct**: `ChatSpacesGetCmd`
- **Args/Flags**:
  - `name` (required arg): space resource name
- **Output**: JSON object; text shows RESOURCE, NAME, TYPE, THREADING, URI
- **Test**: httptest mock GET `/v1/spaces/{id}`

### `gog chat spaces patch`

- **API method**: `spaces.patch`
- **Struct**: `ChatSpacesPatchCmd`
- **Args/Flags**:
  - `name` (required arg): space resource name
  - `--display-name` (string): new display name
  - `--description` (string): new description
  - `--external-allowed` (bool pointer): allow external users
- **Behavior**: Uses `flagProvided()` to detect changed fields. Builds updateMask from provided flags.
- **Output**: JSON object of updated space
- **Test**: httptest mock PATCH `/v1/spaces/{id}`, assert updateMask query param, assert only changed fields in body

### `gog chat spaces search`

- **API method**: `spaces.search`
- **Struct**: `ChatSpacesSearchCmd`
- **Args/Flags**:
  - `--query` (required string): search query (e.g., `spaceType = "SPACE"`)
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
- **Output**: JSON array of matching spaces; text table
- **Test**: httptest mock GET `/v1/spaces:search?query=...&pageSize=...`

---

## Space Members

### `gog chat members create`

- **API method**: `spaces.members.create`
- **Struct**: `ChatMembersCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): space resource name
  - `--user` (required string): user resource name (e.g., `users/123` or email)
  - `--role` (string, default "ROLE_MEMBER"): ROLE_MEMBER or ROLE_MANAGER
- **Output**: JSON object of created membership
- **Test**: httptest mock POST `/v1/{parent}/members`

### `gog chat members delete`

- **API method**: `spaces.members.delete`
- **Struct**: `ChatMembersDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): membership resource name
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{name}`

### `gog chat members get`

- **API method**: `spaces.members.get`
- **Struct**: `ChatMembersGetCmd`
- **Args/Flags**:
  - `name` (required arg): membership resource name
- **Output**: JSON object; text shows NAME, USER, ROLE, STATE
- **Test**: httptest mock GET `/v1/{name}`

### `gog chat members list`

- **API method**: `spaces.members.list`
- **Struct**: `ChatMembersListCmd`
- **Args/Flags**:
  - `parent` (required arg): space resource name
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
  - `--filter` (string): member filter (e.g., `role = "ROLE_MANAGER"`)
- **Output**: JSON array; text table NAME, USER, ROLE, STATE
- **Test**: httptest mock GET `/v1/{parent}/members`

### `gog chat members patch`

- **API method**: `spaces.members.patch`
- **Struct**: `ChatMembersPatchCmd`
- **Args/Flags**:
  - `name` (required arg): membership resource name
  - `--role` (string): new role
- **Behavior**: `flagProvided()` for updateMask.
- **Output**: JSON object of updated membership
- **Test**: httptest mock PATCH `/v1/{name}`, assert updateMask

---

## Space Messages

### `gog chat messages delete`

- **API method**: `spaces.messages.delete`
- **Struct**: `ChatMessagesDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): message resource name
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{name}`

### `gog chat messages get`

- **API method**: `spaces.messages.get`
- **Struct**: `ChatMessagesGetCmd`
- **Args/Flags**:
  - `name` (required arg): message resource name
- **Output**: JSON object; text shows NAME, SENDER, TEXT (truncated), CREATE_TIME
- **Test**: httptest mock GET `/v1/{name}`

### `gog chat messages patch`

- **API method**: `spaces.messages.patch`
- **Struct**: `ChatMessagesPatchCmd`
- **Args/Flags**:
  - `name` (required arg): message resource name
  - `--text` (string): new message text
  - `--cards-v2` (string): JSON string for cards
- **Behavior**: `flagProvided()` for updateMask.
- **Output**: JSON object of updated message
- **Test**: httptest mock PATCH `/v1/{name}`, assert updateMask, assert body

### `gog chat messages update`

- **API method**: `spaces.messages.update`
- **Struct**: `ChatMessagesUpdateCmd`
- **Args/Flags**:
  - `name` (required arg): message resource name
  - `--text` (required string): full replacement text
  - `--cards-v2` (string): JSON string for cards (full replace)
- **Behavior**: Full replace (no updateMask filtering). All writable fields sent.
- **Output**: JSON object of updated message
- **Test**: httptest mock PUT `/v1/{name}`, assert full body sent

---

## Message Attachments

### `gog chat attachments get`

- **API method**: `spaces.messages.attachments.get`
- **Struct**: `ChatAttachmentsGetCmd`
- **Args/Flags**:
  - `name` (required arg): attachment resource name
- **Output**: JSON object with attachment metadata (name, contentName, contentType, downloadUri, source)
- **Test**: httptest mock GET `/v1/{name}`

---

## Message Reactions

### `gog chat reactions create`

- **API method**: `spaces.messages.reactions.create`
- **Struct**: `ChatReactionsCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): message resource name
  - `--emoji` (required string): unicode emoji string or custom emoji resource name
- **Output**: JSON object of created reaction
- **Test**: httptest mock POST `/v1/{parent}/reactions`

### `gog chat reactions delete`

- **API method**: `spaces.messages.reactions.delete`
- **Struct**: `ChatReactionsDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): reaction resource name
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{name}`

### `gog chat reactions list`

- **API method**: `spaces.messages.reactions.list`
- **Struct**: `ChatReactionsListCmd`
- **Args/Flags**:
  - `parent` (required arg): message resource name
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
  - `--filter` (string): reaction filter
- **Output**: JSON array; text table NAME, EMOJI, USER
- **Test**: httptest mock GET `/v1/{parent}/reactions`

---

## Space Events

### `gog chat events get`

- **API method**: `spaces.spaceEvents.get`
- **Struct**: `ChatEventsGetCmd`
- **Args/Flags**:
  - `name` (required arg): space event resource name
- **Output**: JSON object with event details (name, eventType, eventTime, payload)
- **Test**: httptest mock GET `/v1/{name}`

### `gog chat events list`

- **API method**: `spaces.spaceEvents.list`
- **Struct**: `ChatEventsListCmd`
- **Args/Flags**:
  - `parent` (required arg): space resource name
  - `--filter` (required string): event filter (e.g., event types)
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
- **Output**: JSON array; text table NAME, EVENT_TYPE, TIME
- **Test**: httptest mock GET `/v1/{parent}/spaceEvents`

---

## User Space Notification Settings

### `gog chat notification-settings get`

- **API method**: `users.spaces.getSpaceNotificationSetting`
- **Struct**: `ChatNotificationSettingsGetCmd`
- **Args/Flags**:
  - `name` (required arg): notification setting resource name (e.g., `users/me/spaces/xxx/spaceNotificationSetting`)
- **Output**: JSON object with muteSetting
- **Test**: httptest mock GET `/v1/{name}`

### `gog chat notification-settings patch`

- **API method**: `users.spaces.updateSpaceNotificationSetting`
- **Struct**: `ChatNotificationSettingsPatchCmd`
- **Args/Flags**:
  - `name` (required arg): notification setting resource name
  - `--mute-setting` (string): MUTE or UNMUTE
- **Behavior**: `flagProvided()` for updateMask.
- **Output**: JSON object of updated settings
- **Test**: httptest mock PATCH `/v1/{name}`, assert updateMask

---

## User Space Threads

### `gog chat thread-read-state get`

- **API method**: `users.spaces.threads.getThreadReadState`
- **Struct**: `ChatThreadReadStateGetCmd`
- **Args/Flags**:
  - `name` (required arg): thread read state resource name (e.g., `users/me/spaces/xxx/threads/yyy/threadReadState`)
- **Output**: JSON object with name, lastReadTime
- **Test**: httptest mock GET `/v1/{name}`

---

## User Spaces

### `gog chat space-read-state update`

- **API method**: `users.spaces.updateSpaceReadState`
- **Struct**: `ChatSpaceReadStateUpdateCmd`
- **Args/Flags**:
  - `name` (required arg): space read state resource name (e.g., `users/me/spaces/xxx/spaceReadState`)
  - `--last-read-time` (string): RFC3339 timestamp to mark as read up to
- **Behavior**: `flagProvided()` for updateMask.
- **Output**: JSON object of updated read state
- **Test**: httptest mock PATCH `/v1/{name}`, assert updateMask

---

## Implementation Notes

1. All commands require Workspace accounts. Consumer `@gmail.com` accounts must be rejected before service creation (see existing `requireWorkspaceAccount` pattern).
2. Service factory override pattern: `origNew := newChatService; t.Cleanup(func() { newChatService = origNew })` in every test.
3. Group related commands under sub-structs in the Kong routing tree (e.g., `ChatCmd.Emoji`, `ChatCmd.Members`, `ChatCmd.Reactions`, `ChatCmd.Events`).
4. For media download/upload, use streaming I/O -- do not buffer entire file in memory.
5. For patch commands, collect `updateMask` from `flagProvided()` calls and pass as query parameter.
6. Total new test count: minimum 32 test functions (one per method), plus edge cases for destructive confirmations and pagination.
