---

name: google-sheets
description: "Google Sheets spreadsheet management via gogcli"
allowed-tools: "Bash(gog:*)"

---

# Google Sheets

Use `gog sheets` for reading, writing, and managing Google Sheets spreadsheets.

**Requires**: `GOG_KEYRING_PASSWORD=cli-tools` env var for all commands.

## Quick Start

```bash
# Read cell values
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com sheets get SPREADSHEET_ID "Sheet1!A1:D10"

# Write/update values
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com sheets update SPREADSHEET_ID "A1:B2" '["val1","val2"]' '["val3","val4"]'

# Append rows
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com sheets append SPREADSHEET_ID "Sheet1!A:D" '["new1","new2","new3","new4"]'
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `get <spreadsheetId> <range>` | Read cell values | `--formula` (show formulas) |
| `update <spreadsheetId> <range> [values...]` | Write values to cells | `--raw` (no parse) |
| `append <spreadsheetId> <range> [values...]` | Append rows | |
| `info <spreadsheetId>` | Get spreadsheet metadata | |
| `create <title>` | Create a new spreadsheet | |

Values are passed as positional JSON arrays, one per row.

## Output Format

With `--json`: raw Google Sheets API JSON. With `--plain`: TSV (great for piping sheet data). Default: human-readable table.

## Configuration

Auth: gogcli file keyring (`GOG_KEYRING_PASSWORD=cli-tools`). Account: `--account jeremy@robbenmedia.com`.

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
