---

name: google-docs
description: "Google Docs document management via gogcli"
allowed-tools: "Bash(gog:*)"

---

# Google Docs

Use `gog docs` for reading, creating, and exporting Google Docs documents.

**Requires**: `GOG_KEYRING_PASSWORD=cli-tools` env var for all commands.

## Quick Start

```bash
# Export doc as text
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com docs export DOC_ID --format txt

# Export as PDF
GOG_KEYRING_PASSWORD=cli-tools gog --no-input --account jeremy@robbenmedia.com docs export DOC_ID --format pdf --output ./doc.pdf

# Get doc metadata
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com docs info DOC_ID

# Create a new doc
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com docs create "New Document"
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `export <docId>` | Export to PDF/DOCX/TXT | `--format` (pdf/docx/txt), `--output` |
| `info <docId>` | Get document metadata | |
| `create <title>` | Create a new document | `--folder` |

Note: gogcli exports Docs via Drive API. For reading content, use `export --format txt` to stdout.

## Output Format

With `--json`: raw Google API JSON metadata. Export outputs file content directly.

## Configuration

Auth: gogcli file keyring (`GOG_KEYRING_PASSWORD=cli-tools`). Account: `--account jeremy@robbenmedia.com`.

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
