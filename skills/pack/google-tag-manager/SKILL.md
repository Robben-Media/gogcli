---

name: google-tag-manager
description: "Google Tag Manager container and tag management CLI"
allowed-tools: "Bash(gog:*)"

---

# Google Tag Manager

Use `gog tag-manager` (alias: `gog gtm`) for managing GTM containers, tags, triggers, and variables.

## Preflight

Verify the active binary exposes the GTM command surface before using this skill:

```bash
gog --help | rg 'tag-manager|gtm'
```

Expected help output includes `tag-manager (gtm) <command> [flags]`.

If `gog tag-manager` or `gog gtm` fails with `unexpected argument`, the binary on `PATH` is stale or was built without GTM commands. Fix the installed binary first; stored OAuth scopes alone are not enough to make the CLI command surface exist.

## Quick Start

```bash
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com tag-manager accounts
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com tag-manager containers --account-id 12345
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com tag-manager tags --account-id 12345 --container-id 67890
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `accounts` | List GTM accounts | |
| `containers` | List containers in an account | `--account-id` |
| `tags` | List tags in a container | `--account-id`, `--container-id`, `--workspace-id` |
| `tag` | Get a single tag by path | `<tagPath>` (positional) |
| `triggers` | List triggers in a container | `--account-id`, `--container-id`, `--workspace-id` |
| `variables` | List variables in a container | `--account-id`, `--container-id`, `--workspace-id` |
| `versions` | List container versions | `--account-id`, `--container-id` |

## Resource Paths

GTM uses path-based IDs: `accounts/123/containers/456/workspaces/789/tags/101`

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
