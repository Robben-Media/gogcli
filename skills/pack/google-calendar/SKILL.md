---

name: google-calendar
description: "Google Calendar event management via gogcli"
allowed-tools: "Bash(gog:*)"

---

# Google Calendar

Use `gog calendar` for managing calendar events, scheduling, and availability.

**Requires**: `GOG_KEYRING_PASSWORD=cli-tools` env var for all commands.

## Quick Start

```bash
# List upcoming events (today)
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com calendar events --today

# List next 7 days
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com calendar events --days 7

# Create an event
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com calendar create primary --summary "Meeting" --start "2026-02-10T10:00:00" --end "2026-02-10T11:00:00"

# Check free/busy
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com calendar freebusy primary --from "2026-02-10" --to "2026-02-11"
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `events [calendarId]` | List events | `--today`, `--tomorrow`, `--week`, `--days`, `--from`, `--to`, `--max`, `--all` |
| `event <calendarId> <eventId>` | Get event details | |
| `create <calendarId>` | Create a new event | `--summary`, `--start`, `--end`, `--location`, `--description` |
| `update <calendarId> <eventId>` | Update an event | `--summary`, `--start`, `--end` |
| `delete <calendarId> <eventId>` | Delete an event | |
| `search <query>` | Search events | `--from`, `--to` |
| `freebusy <calendarIds>` | Check free/busy times | `--from`, `--to` |
| `calendars` | List available calendars | |
| `conflicts` | Find scheduling conflicts | `--from`, `--to` |
| `respond <calendarId> <eventId>` | RSVP to an event | `--status` (accepted/declined/tentative) |

Default calendarId is `primary`.

## Output Format

With `--json`: raw Google Calendar API JSON. With `--plain`: TSV. Default: human-readable table with times.

## Configuration

Auth: gogcli file keyring (`GOG_KEYRING_PASSWORD=cli-tools`). Account: `--account jeremy@robbenmedia.com`.

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
