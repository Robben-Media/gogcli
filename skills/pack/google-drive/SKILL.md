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
# List files in root
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com drive ls --max 20

# List files in a folder
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com drive ls --folder FOLDER_ID

# Search for files
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com drive search "quarterly report"

# Get file info
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com drive get FILE_ID

# Upload a file
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com drive upload ./report.pdf --folder FOLDER_ID
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ls` | List files and folders | `--folder`, `--max`, `--type`, `--page` |
| `search <query>` | Full-text search | `--max`, `--type` |
| `get <fileId>` | Get file metadata | |
| `upload <file>` | Upload a file | `--folder`, `--name` |
| `download <fileId>` | Download a file | `--output` |
| `mkdir <name>` | Create a folder | `--parent` |
| `mv <fileId> <folderId>` | Move a file | |
| `cp <fileId>` | Copy a file | `--name` |
| `rm <fileId>` | Move to trash | |
| `share <fileId>` | Manage permissions | `--email`, `--role` |

## Output Format

With `--json`: raw Google Drive API JSON. With `--plain`: TSV. Default: human-readable table.

## Configuration

Auth: gogcli file keyring (`GOG_KEYRING_PASSWORD=cli-tools`). Account: `--account jeremy@robbenmedia.com`.

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
