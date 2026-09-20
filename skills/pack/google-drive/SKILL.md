---

name: google-drive
description: "Google Drive file management via gogcli"
allowed-tools: "Bash(gog:*)"

---

# Google Drive

Use `gog drive` for managing Google Drive files, folders, and permissions.

**Requires**: `GOG_KEYRING_PASSWORD=cli-tools` env var for all commands.

## Quick Start

```bash
# List files in root (agents: --wrap-untrusted fences names/descriptions)
GOG_KEYRING_PASSWORD=cli-tools gog --json --wrap-untrusted --no-input --account jeremy@robbenmedia.com drive ls --max 20

# List files in a folder
GOG_KEYRING_PASSWORD=cli-tools gog --json --wrap-untrusted --no-input --account jeremy@robbenmedia.com drive ls --parent FOLDER_ID

# Search for files
GOG_KEYRING_PASSWORD=cli-tools gog --json --wrap-untrusted --no-input --account jeremy@robbenmedia.com drive search "quarterly report"

# Get file info
GOG_KEYRING_PASSWORD=cli-tools gog --json --wrap-untrusted --no-input --account jeremy@robbenmedia.com drive get FILE_ID

# Upload a file
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com drive upload ./report.pdf --parent FOLDER_ID
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ls` | List files and folders | `--parent`, `--max`, `--page` |
| `search <query>` | Full-text search | `--max` |
| `get <fileId>` | Get file metadata | |
| `upload <file>` | Upload a file | `--parent`, `--name` |
| `download <fileId>` | Download a file | `--out` |
| `mkdir <name>` | Create a folder | `--parent` |
| `move <fileId>` | Move a file | `--parent` (required) |
| `copy <fileId>` | Copy a file | `--name`, `--parent` |
| `delete <fileId>` / `rm` | Trash or delete | see `gog drive --help` |
| `permissions` | Manage sharing | see `gog drive permissions --help` |

Prefer `gog drive --help` and subcommand `--help` when unsure — flags drift less often than skill tables.

## Output Format

With `--json`: raw Google Drive API JSON. With `--plain`: TSV. Default: human-readable table.

Agents reading file names/descriptions via JSON should pass `--wrap-untrusted` (or `GOG_WRAP_UNTRUSTED=1`); plain/human output is unchanged.

## Configuration

Auth: gogcli file keyring (`GOG_KEYRING_PASSWORD=cli-tools`). Account: `--account jeremy@robbenmedia.com`.

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
