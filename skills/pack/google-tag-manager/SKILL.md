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
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com gtm workspaces create-version accounts/12345/containers/67890/workspaces/7 --name "Release"
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com gtm versions publish accounts/12345/containers/67890/versions/42
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com gtm triggers create --account-id 12345 --container-id 67890 --workspace-id 7 --name "Checkout" --type customEvent --custom-event-filter '{"type":"equals","parameter":[{"type":"template","key":"arg0","value":"{{Event}}"},{"type":"template","key":"arg1","value":"checkout"}]}'
GOG_KEYRING_PASSWORD=cli-tools gog --json --no-input --account jeremy@robbenmedia.com gtm built-in-variables create --account-id 12345 --container-id 67890 --workspace-id 7 --type pageUrl --type clickId
```

## Available Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `accounts` | List GTM accounts | |
| `containers` | List containers in an account | `--account-id` |
| `tags` | List tags in a container | `--account-id`, `--container-id`, `--workspace-id` |
| `tag` | Get a single tag by path | `<tagPath>` (positional) |
| `triggers` | List triggers in a workspace | `--account-id`, `--container-id`, `--workspace-id` |
| `triggers create` | Create a trigger | workspace flags, `--name`, `--type`; repeatable Condition JSON flags |
| `triggers delete/get/revert/update` | Mutate a trigger by full path | `<path>`; update field flags; delete uses `--force` |
| `variables` | List variables in a workspace | `--account-id`, `--container-id`, `--workspace-id` |
| `variables create` | Create a custom variable | workspace flags, `--name`, `--type`, repeatable `--parameter` JSON |
| `variables delete/get/revert/update` | Mutate a custom variable by full path | `<path>`; update field flags; delete uses `--force` |
| `built-in-variables create/delete/list/revert` | Manage workspace built-in variables | workspace flags, repeatable `--type`; delete uses `--force` |
| `versions` | List container versions | `--account-id`, `--container-id` |
| `versions publish` | Publish a container version | `<path>`, `--fingerprint` |
| `workspaces create-version` | Create a container version from a workspace | `<path>`, `--name`, `--notes` |

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


## Command accuracy

Prefer `gog <group> --help` and subcommand `--help` when flags are uncertain; the live binary is the source of truth over any table in this skill.

## Keeping this skill current

This skill is part of the **gog companion skill pack**. When `gog` reports an update is available, run:

```bash
gog update
```

That refreshes the binary and any installed pack skills. Locally edited skills are skipped (agent-safe). If a skill is skipped, tell the user the path; only overwrite with `gog skills update --overwrite-local` or `gog update --force-skills` when they ask.
