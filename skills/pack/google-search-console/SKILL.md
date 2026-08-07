---

name: google-search-console
description: "Google Search Console performance and indexing CLI"
allowed-tools: "Bash(gog:*)"

---

# Google Search Console

Use `gog search-console` (aliases: `gog gsc`, `gog sc`) for search performance data, indexing status, and URL inspection.

## Quick Start

```bash
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com search-console sites
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com search-console query --site-url "sc-domain:example.com" --start-date 2026-01-01 --end-date 2026-01-31 --dimensions query,page
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com search-console inspect --site-url "sc-domain:example.com" --url "https://example.com/page"
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `sites` | List verified sites | |
| `query` | Query search analytics data | `--site-url`, `--start-date`, `--end-date`, `--dimensions`, `--row-limit` |
| `sitemaps` | List sitemaps for a site | `--site-url` |
| `submit-sitemap` | Submit a sitemap | `--site-url`, `--sitemap-url` |
| `inspect` | Inspect URL indexing status | `--site-url`, `--url` |

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
