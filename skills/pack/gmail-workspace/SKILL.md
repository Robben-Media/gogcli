---
name: gmail-workspace
description: "Manage workspace Gmail (jeremy@robbenmedia.com). Search, read, send, and triage emails via gogcli."
allowed-tools: "Bash(gog:*)"
---

# Gmail (Workspace)

Use `gog gmail` for workspace email operations (`jeremy@robbenmedia.com`).

## Non-negotiables

- Prefer explicit account on every command: `--account jeremy@robbenmedia.com` or verified alias `--account business`
- Bare `gog` default is **business continuity-critical**. Never set personal as default / `GOG_ACCOUNT`
- Auth backend is **encrypted file keyring** (`keyring_backend: file`). Shell/agent env should have `GOG_KEYRING_PASSWORD` (from `~/.config/gogcli/keyring_password` or existing env). Do **not** use macOS Keychain for gog tokens.
- Dual-account is live: personal exists as `jdjb78@gmail.com` / alias `personal` — keep it out of workspace workflows

## Quick Start

```bash
# Search inbox threads
gog --json --no-input --account jeremy@robbenmedia.com gmail search "is:unread" --max 10

# Alias (after verified)
gog --json --no-input --account business gmail search "is:unread" --max 10

# Read specific message
gog --json --no-input --account jeremy@robbenmedia.com gmail get MESSAGE_ID

# Send email
gog --json --no-input --account jeremy@robbenmedia.com gmail send --to "user@example.com" --subject "Re: Quote" --body "Hi, here's the quote..."

# List labels
gog --json --no-input --account jeremy@robbenmedia.com gmail labels list
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `search <query>` | Search threads using Gmail query syntax | `--max`, `--page` |
| `get <messageId>` | Get a message (full/metadata/raw) | `--format` |
| `messages list` | List messages | `--max`, `--label`, `--query` |
| `send` | Send an email | `--to`, `--subject`, `--body`, `--cc`, `--bcc`, `--html`, `--thread` |
| `labels list` | List all labels | |
| `labels create <name>` | Create a label | |
| `thread get <threadId>` | Get full thread | |
| `thread modify <threadId>` | Modify thread labels | `--add-labels`, `--remove-labels` |
| `batch archive` | Archive messages in batch | `--query` |
| `drafts list` | List drafts | `--max` |
| `drafts create` | Create a draft | `--to`, `--subject`, `--body` |
| `attachment <msgId> <attachId>` | Download attachment | `--output` |

## Gmail Search Syntax

```
is:unread                     Unread messages
from:user@example.com         From specific sender
subject:"invoice"             Subject contains word
has:attachment                Has attachments
newer_than:7d                 Last 7 days
after:2024/01/01              After date
label:important               Has label
```

## Output Format

With `--json` flag, gog outputs raw Google API JSON (not our `{success, data, error, meta}` wrapper).

Output modes: `--json` (scripting), `--plain` (TSV), or default (human-readable table).

## Common Workflows

### Email Triage
```bash
gog --json --no-input --account jeremy@robbenmedia.com gmail search "is:unread" --max 20
gog --no-input --account jeremy@robbenmedia.com gmail thread modify THREAD_ID --remove-labels INBOX
```

### Reply to Client Thread
```bash
gog --json --no-input --account jeremy@robbenmedia.com gmail search "from:client@example.com"
gog --json --no-input --account jeremy@robbenmedia.com gmail thread get THREAD_ID
gog --no-input --account jeremy@robbenmedia.com gmail send --to "client@example.com" --subject "Re: Project" --body "..." --thread THREAD_ID
```

## Configuration

- Account: `jeremy@robbenmedia.com`
- Alias: `business` → `jeremy@robbenmedia.com`
- Keyring: gogcli encrypted file backend (`keyring_backend: file`; `GOG_KEYRING_PASSWORD` required)
- Default account must remain business (set via `gog auth manage` → Set default).  
  Note: `gog auth alias set default ...` is **reserved/invalid** on current gog build.

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
