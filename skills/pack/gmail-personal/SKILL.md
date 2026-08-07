---
name: gmail-personal
description: "Personal Gmail (jdjb78@gmail.com) via gogcli — readonly by default."
allowed-tools: "Bash(gog:*)"
---

# Gmail Personal

Use `gog gmail` for **personal** mail only: `jdjb78@gmail.com`.

## Non-negotiables

- Always pass an explicit account: `--account jdjb78@gmail.com` or verified alias `--account personal`
- Never rely on bare `gog` / ambient default for personal mail
- Never set personal as the gog default account
- Do **not** set `GOG_ACCOUNT=personal` globally
- Auth backend is **encrypted file keyring** (`keyring_backend: file`). Env needs `GOG_KEYRING_PASSWORD` (same as business). Do **not** use macOS Keychain for gog tokens.
- Current OAuth scope is **Gmail readonly**. Do not send/draft until scopes are widened (Phase 5)

## Quick Start

```bash
# Search inbox threads
gog --json --no-input --account jdjb78@gmail.com gmail search "is:unread" --max 20

# Alias (after verified)
gog --json --no-input --account personal gmail search "is:unread" --max 20

# Read a message
gog --json --no-input --account jdjb78@gmail.com gmail get MESSAGE_ID
```

## Available Commands (readonly phase)

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `search <query>` | Search threads | `--max` |
| `get <messageId>` | Read an email by ID | `--format` |
| `messages list` | List messages | `--max`, `--label` |
| `labels list` | List labels | |
| `thread get <threadId>` | Get thread | |

## Out of scope until Phase 5

- `gmail send`
- `drafts create` / send draft
- any write that needs full Gmail scope

## Configuration

- Account: `jdjb78@gmail.com`
- Alias: `personal` → `jdjb78@gmail.com`
- Keyring: gogcli encrypted file backend (`keyring_backend: file`; `GOG_KEYRING_PASSWORD` required)
- Business default must remain `jeremy@robbenmedia.com` (set via `gog auth manage` → Set default)

## Study @learnings/LEARNINGS.md


## Command accuracy

Prefer `gog <group> --help` and subcommand `--help` when flags are uncertain; the live binary is the source of truth over any table in this skill.

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
