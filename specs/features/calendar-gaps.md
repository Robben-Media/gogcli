# Google Calendar v3 -- Gap Coverage Spec

**API**: Google Calendar v3 (`calendar/v3`)
**Current coverage**: 11 methods (acl.list, calendarList.get/list, colors.get, events create/delete/get/list/patch/instances, freebusy.query)
**Gap**: 26 missing methods
**Service factory**: `newCalendarService` in `calendar_services.go` (or similar)

## Overview

Adding 26 missing methods to the Google Calendar v3 CLI commands. Covers ACL management (delete/get/insert/patch/update/watch), calendar list operations (delete/insert/patch/update/watch), calendar CRUD (clear/delete/get/insert/patch/update), channels (stop), event operations (import/move/quick-add/full-update/watch), and settings (get/list/watch) to achieve full Discovery API parity.

All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete and clear operations require `confirmDestructive()`. List commands include `--max` and `--page` flags for pagination. JSON output includes `nextPageToken`. Text output uses TSV-formatted tables.

---

## ACL

### `gog calendar acl delete`

- **API method**: `acl.delete`
- **Struct**: `CalendarAclDeleteCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `ruleId` (required arg): ACL rule ID
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Calls DELETE.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/calendars/{calendarId}/acl/{ruleId}`

### `gog calendar acl get`

- **API method**: `acl.get`
- **Struct**: `CalendarAclGetCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `ruleId` (required arg): ACL rule ID
- **Output**: JSON object with `id`, `role`, `scope`; text shows ID, ROLE, SCOPE_TYPE, SCOPE_VALUE
- **Test**: httptest mock GET `/calendars/{calendarId}/acl/{ruleId}`

### `gog calendar acl insert`

- **API method**: `acl.insert`
- **Struct**: `CalendarAclInsertCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--role` (required string): freeBusyReader, reader, writer, owner
  - `--scope-type` (required string): default, user, group, domain
  - `--scope-value` (string): email or domain (not needed for scope-type=default)
  - `--send-notifications` (bool, default false): send notification emails
- **Output**: JSON object of created ACL rule
- **Test**: httptest mock POST `/calendars/{calendarId}/acl`, assert request body

### `gog calendar acl patch`

- **API method**: `acl.patch`
- **Struct**: `CalendarAclPatchCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `ruleId` (required arg): ACL rule ID
  - `--role` (string): new role
  - `--send-notifications` (bool pointer): send notification emails
- **Behavior**: `flagProvided()` for partial update. Only send changed fields.
- **Output**: JSON object of updated ACL rule
- **Test**: httptest mock PATCH `/calendars/{calendarId}/acl/{ruleId}`, assert body contains only changed fields

### `gog calendar acl update`

- **API method**: `acl.update`
- **Struct**: `CalendarAclUpdateCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `ruleId` (required arg): ACL rule ID
  - `--role` (required string): role (full replace)
  - `--scope-type` (required string): scope type
  - `--scope-value` (string): scope value
  - `--send-notifications` (bool): send notification emails
- **Behavior**: Full replace -- all fields required.
- **Output**: JSON object of updated ACL rule
- **Test**: httptest mock PUT `/calendars/{calendarId}/acl/{ruleId}`

### `gog calendar acl watch`

- **API method**: `acl.watch`
- **Struct**: `CalendarAclWatchCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--channel-id` (required string): unique channel ID
  - `--address` (required string): webhook URL (HTTPS)
  - `--type` (string, default "web_hook"): channel type
  - `--ttl` (string): time-to-live duration (e.g., "1h", "7d")
  - `--token` (string): verification token
- **Behavior**: Creates a push notification channel. Returns channel resource with `id`, `resourceId`, `expiration`.
- **Output**: JSON object of the channel
- **Test**: httptest mock POST `/calendars/{calendarId}/acl/watch`, assert request body with channel config

---

## Calendar List

### `gog calendar calendar-list delete`

- **API method**: `calendarList.delete`
- **Struct**: `CalendarListDeleteCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Removes calendar from user's list (does not delete the calendar itself).
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/users/me/calendarList/{calendarId}`

### `gog calendar calendar-list insert`

- **API method**: `calendarList.insert`
- **Struct**: `CalendarListInsertCmd`
- **Args/Flags**:
  - `--id` (required string): calendar ID to add
  - `--color-foreground` (string): foreground color hex
  - `--color-background` (string): background color hex
  - `--hidden` (bool): hide calendar
  - `--selected` (bool): show calendar in UI
  - `--default-reminders` (string): JSON array of reminders
- **Output**: JSON object of created calendar list entry
- **Test**: httptest mock POST `/users/me/calendarList`

### `gog calendar calendar-list patch`

- **API method**: `calendarList.patch`
- **Struct**: `CalendarListPatchCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--color-foreground` (string): foreground color hex
  - `--color-background` (string): background color hex
  - `--hidden` (bool pointer): hide calendar
  - `--selected` (bool pointer): show in UI
  - `--summary-override` (string): override display name
- **Behavior**: `flagProvided()` for partial update.
- **Output**: JSON object of updated entry
- **Test**: httptest mock PATCH `/users/me/calendarList/{calendarId}`

### `gog calendar calendar-list update`

- **API method**: `calendarList.update`
- **Struct**: `CalendarListUpdateCmd`
- **Args/Flags**: Same as patch but all fields are full-replace semantics.
- **Output**: JSON object
- **Test**: httptest mock PUT `/users/me/calendarList/{calendarId}`

### `gog calendar calendar-list watch`

- **API method**: `calendarList.watch`
- **Struct**: `CalendarListWatchCmd`
- **Args/Flags**:
  - `--channel-id` (required string): unique channel ID
  - `--address` (required string): webhook URL
  - `--type` (string, default "web_hook"): channel type
  - `--ttl` (string): time-to-live
  - `--token` (string): verification token
- **Output**: JSON channel object
- **Test**: httptest mock POST `/users/me/calendarList/watch`

---

## Calendars

### `gog calendar calendars clear`

- **API method**: `calendars.clear`
- **Struct**: `CalendarCalendarsClearCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Clears all events from primary calendar. Only works for primary.
- **Output**: Empty on success, stderr confirmation message
- **Test**: httptest mock POST `/calendars/{calendarId}/clear`

### `gog calendar calendars delete`

- **API method**: `calendars.delete`
- **Struct**: `CalendarCalendarsDeleteCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Permanently deletes a secondary calendar.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/calendars/{calendarId}`

### `gog calendar calendars get`

- **API method**: `calendars.get`
- **Struct**: `CalendarCalendarsGetCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
- **Output**: JSON object; text shows ID, SUMMARY, DESCRIPTION, TIMEZONE
- **Test**: httptest mock GET `/calendars/{calendarId}`

### `gog calendar calendars insert`

- **API method**: `calendars.insert`
- **Struct**: `CalendarCalendarsInsertCmd`
- **Args/Flags**:
  - `--summary` (required string): calendar name
  - `--description` (string): description
  - `--timezone` (string): IANA timezone (e.g., "America/New_York")
  - `--location` (string): geographic location
- **Output**: JSON object of created calendar
- **Test**: httptest mock POST `/calendars`, assert request body

### `gog calendar calendars patch`

- **API method**: `calendars.patch`
- **Struct**: `CalendarCalendarsPatchCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--summary` (string): calendar name
  - `--description` (string): description
  - `--timezone` (string): timezone
  - `--location` (string): location
- **Behavior**: `flagProvided()` for partial update.
- **Output**: JSON object of updated calendar
- **Test**: httptest mock PATCH `/calendars/{calendarId}`

### `gog calendar calendars update`

- **API method**: `calendars.update`
- **Struct**: `CalendarCalendarsUpdateCmd`
- **Args/Flags**: Same as patch but full-replace.
- **Output**: JSON object
- **Test**: httptest mock PUT `/calendars/{calendarId}`

---

## Channels

### `gog calendar channels stop`

- **API method**: `channels.stop`
- **Struct**: `CalendarChannelsStopCmd`
- **Args/Flags**:
  - `--channel-id` (required string): channel ID from watch response
  - `--resource-id` (required string): resource ID from watch response
- **Behavior**: Stops receiving push notifications for the given channel. No confirmation needed (idempotent).
- **Output**: Empty on success (HTTP 204)
- **Test**: httptest mock POST `/channels/stop`, assert request body with id and resourceId

---

## Events

### `gog calendar events import`

- **API method**: `events.import`
- **Struct**: `CalendarEventsImportCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--ical-uid` (required string): fixed iCalUID for the event
  - `--summary` (required string): event title
  - `--from` (required string): start time RFC3339
  - `--to` (required string): end time RFC3339
  - `--description` (string): description
  - `--location` (string): location
  - `--all-day` (bool): all-day event (date-only in from/to)
- **Behavior**: Imports an event with a pre-set iCalUID. Used for syncing events from external calendars. The iCalUID is permanent and cannot be changed after import.
- **Output**: JSON object of imported event
- **Test**: httptest mock POST `/calendars/{calendarId}/events/import`, assert iCalUID in body

### `gog calendar events move`

- **API method**: `events.move`
- **Struct**: `CalendarEventsMoveCmd`
- **Args/Flags**:
  - `calendarId` (required arg): source calendar ID
  - `eventId` (required arg): event ID
  - `--destination` (required string): destination calendar ID
  - `--send-updates` (string): notification mode (all, externalOnly, none)
- **Behavior**: Moves event between calendars. Event ID remains the same.
- **Output**: JSON object of moved event
- **Test**: httptest mock POST `/calendars/{calendarId}/events/{eventId}/move?destination={dest}`

### `gog calendar events quick-add`

- **API method**: `events.quickAdd`
- **Struct**: `CalendarEventsQuickAddCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `text` (required arg): natural language event text (e.g., "Lunch with Bob tomorrow at noon")
  - `--send-updates` (string): notification mode
- **Behavior**: Google parses the text to create the event. Returns the created event.
- **Output**: JSON object of created event; text shows ID, START, END, SUMMARY
- **Test**: httptest mock POST `/calendars/{calendarId}/events/quickAdd?text={encoded}`, assert text param

### `gog calendar events full-update`

- **API method**: `events.update` (PUT -- full replace)
- **Struct**: `CalendarEventsFullUpdateCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `eventId` (required arg): event ID
  - Same fields as `CalendarCreateCmd` (summary, from, to, description, location, attendees, all-day, recurrence, etc.)
  - `--send-updates` (string): notification mode
- **Behavior**: Full replace semantics. All writable fields must be provided; omitted fields are cleared.
- **Note**: Named `full-update` to differentiate from existing `update` which uses patch semantics.
- **Output**: JSON object of updated event
- **Test**: httptest mock PUT `/calendars/{calendarId}/events/{eventId}`

### `gog calendar events watch`

- **API method**: `events.watch`
- **Struct**: `CalendarEventsWatchCmd`
- **Args/Flags**:
  - `calendarId` (required arg): calendar ID
  - `--channel-id` (required string): unique channel ID
  - `--address` (required string): webhook URL
  - `--type` (string, default "web_hook"): channel type
  - `--ttl` (string): time-to-live
  - `--token` (string): verification token
- **Output**: JSON channel object
- **Test**: httptest mock POST `/calendars/{calendarId}/events/watch`

---

## Settings

### `gog calendar settings get`

- **API method**: `settings.get`
- **Struct**: `CalendarSettingsGetCmd`
- **Args/Flags**:
  - `setting` (required arg): setting key (e.g., "timezone", "dateFieldOrder", "locale")
- **Output**: JSON object with `id` and `value`; text shows KEY, VALUE
- **Test**: httptest mock GET `/users/me/settings/{setting}`

### `gog calendar settings list`

- **API method**: `settings.list`
- **Struct**: `CalendarSettingsListCmd`
- **Args/Flags**:
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
- **Output**: JSON array of settings; text table KEY, VALUE
- **Test**: httptest mock GET `/users/me/settings`

### `gog calendar settings watch`

- **API method**: `settings.watch`
- **Struct**: `CalendarSettingsWatchCmd`
- **Args/Flags**:
  - `--channel-id` (required string): unique channel ID
  - `--address` (required string): webhook URL
  - `--type` (string, default "web_hook"): channel type
  - `--ttl` (string): time-to-live
  - `--token` (string): verification token
- **Output**: JSON channel object
- **Test**: httptest mock POST `/users/me/settings/watch`

---

## Implementation Notes

1. **Watch commands**: All watch commands share the same channel configuration pattern. Consider a shared `watchFlags` struct embedded in each watch command to reduce duplication.
2. **channels.stop**: This is a standalone command, not nested under a resource. Route as `gog calendar channels stop`.
3. **events.import vs events.insert**: Import uses a fixed iCalUID; the CLI command should be `events import` (not `events create`) to make the distinction clear.
4. **events.update vs events.patch**: The existing `update` command uses PATCH semantics. The new full-replace update should be named `full-update` or the existing one renamed to `edit`/`patch` for clarity.
5. **events.quickAdd**: The `text` argument is positional to make it feel natural: `gog calendar events quick-add primary "Lunch tomorrow at noon"`.
6. **calendarList vs calendars**: These are different resources. `calendarList` manages the user's view of calendars; `calendars` manages the calendars themselves. Route as `gog calendar calendar-list` and `gog calendar calendars` respectively.
7. Total new test count: minimum 26 test functions plus watch/channel edge cases.
