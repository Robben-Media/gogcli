---

name: google-analytics
description: "Google Analytics reporting CLI"
allowed-tools: "Bash(gog:*)"

---

# Google Analytics

Use `gog analytics` (aliases: `gog ga`, `gog ga4`) for querying GA4 reports, metrics, and dimensions.

## Quick Start

```bash
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com analytics properties
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com analytics report --property 12345 --metrics "sessions,users" --start-date 28daysAgo --end-date today
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com analytics realtime --property 12345
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `report` | Run a GA4 report | `--property`, `--metrics`, `--dimensions`, `--start-date`, `--end-date`, `--limit` |
| `realtime` | Get realtime active users | `--property`, `--metrics`, `--dimensions` |
| `properties` | List available GA4 properties | `--page-size` |
| `accounts` | List GA4 accounts | `--page-size` |
| `dimensions` | List available dimensions for a property | `--property` |
| `metrics` | List available metrics for a property | `--property` |

## Property ID Format

Property IDs can be passed with or without prefix:
- `--property 12345` (bare number)
- `--property properties/12345` (full format)

## Output Modes

| Flag | Description |
|------|-------------|
| `--json` | Raw Google API JSON output |
| `--plain` | TSV for piping to other tools |
| (default) | Human-readable table |

## Configuration

Auth: `GOG_KEYRING_PASSWORD=cli-tools` with `--account jeremy@robbenmedia.com`

## Study @learnings/LEARNINGS.md


## Command accuracy

Prefer `gog <group> --help` and subcommand `--help` when flags are uncertain; the live binary is the source of truth over any table in this skill.

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
