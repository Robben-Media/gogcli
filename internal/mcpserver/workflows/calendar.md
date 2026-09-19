# Google calendar workflow

1. Call `accounts_list` and use one opaque `account_id` for every Calendar call.
2. List candidate calendars with `calendar_list`, then use their IDs rather than display names. For events, pass one `calendar_id` with explicit RFC 3339 `time_min` and `time_max`; repeat the primitive for another calendar.
3. Use `calendar_freebusy` only for availability across a bounded list of calendars. Preserve all-day events, recurrence, offsets, and timezone fields rather than normalizing them to unspecified local time.
4. Treat per-calendar `partial_failures` as unavailable calendars, never as free time. Report the requested interval and each failed calendar; a result with no busy intervals is free only when all requested calendars succeeded.
