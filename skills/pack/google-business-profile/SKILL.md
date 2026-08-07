---

name: google-business-profile
description: "Google Business Profile management CLI"
allowed-tools: "Bash(gog:*)"

---

# Google Business Profile

Use `gog business-profile` (aliases: `gog gbp`, `gog business`) for managing Google Business Profile listings.

## Quick Start

```bash
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com business-profile accounts
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com business-profile locations "accounts/12345"
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com business-profile get "locations/67890"
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `accounts` | List GBP accounts | |
| `locations` | List business locations for an account | `<account>` (positional), `--page-size`, `--read-mask` |
| `get` | Get a specific location | `<locationName>` (positional), `--read-mask` |

## Output Modes

| Flag | Description |
|------|-------------|
| `--json` | Raw Google API JSON output |
| `--plain` | TSV for piping to other tools |
| (default) | Human-readable table |

## Configuration

Auth: `GOG_KEYRING_PASSWORD=cli-tools` with `--account jeremy@robbenmedia.com`

## Study @learnings/LEARNINGS.md

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
