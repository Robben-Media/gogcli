# OAuth Clients

Use multiple OAuth client credentials (for different Google Cloud projects or brands) without mixing refresh tokens.

## How it works

- Default client name: `default`
- Default credentials file: `$(os.UserConfigDir())/gogcli/credentials.json`
- Named credentials files: `$(os.UserConfigDir())/gogcli/credentials-<client>.json`
- Tokens are stored per client (`token:<client>:<email>`). Default client also writes legacy keys for backwards compatibility.
- Default account is stored per client, with a legacy global fallback for the default client.

## Selecting a client

Use `--client` (or `GOG_CLIENT`) to pick which credentials + token bucket to use:

```
gog --client work auth credentials ~/Downloads/work-client.json
gog --client work auth add you@company.com
gog --client work gmail search "is:unread"
```

When `--client` is not set, `gog` resolves the client in this order:

1) `--client` / `GOG_CLIENT` override
2) `account_clients` map in config
3) `client_domains` map in config
4) Credentials file named after the email domain (e.g. `credentials-example.com.json`)
5) `default`

## Domain auto-map

To auto-select a client for a domain:

```
gog --client work auth credentials ~/Downloads/work.json --domain example.com
```

This writes `client_domains` into `config.json` so any `@example.com` account selects the `work` client.

## Listing stored credentials

```
gog auth credentials list
```

Shows stored credential files plus any configured domain mappings.

## Config example

```
{
  keyring_backend: "auto",
  account_clients: {
    "you@company.com": "work",
  },
  client_domains: {
    "example.com": "work",
  },
}
```

## Migration notes

- Legacy `token:<email>` entries are copied to `token:default:<email>` the first time they are read.
- Legacy `default_account` is still respected for the default client.

## Guided setup

Use `gog auth setup` for first-time Google Cloud + OAuth onboarding (or to resume a partial setup). It pairs a Cloud project with the current `--client` namespace, enables APIs for selected services on **that project only**, guides Console-only Auth Platform steps, installs Desktop OAuth credentials, and can authorize the first account.

```bash
# Interactive (TTY): pick a project, confirm mutations, open Console links
gog auth setup

# Named client
gog --client work auth setup

# Structured discovery only (no Cloud/local mutations)
gog --json --no-input auth setup --discover
gog --plain --no-input auth setup --discover --project my-proj

# Non-interactive mutations require explicit flags + --force
gog --no-input --force auth setup \
  --project my-proj \
  --enable-apis \
  --services gmail,drive,calendar \
  --ack-branding --ack-audience --ack-data-access \
  --credentials ~/Downloads/client_secret.json

# Create a project (never changes gcloud default project/config)
gog --no-input --force auth setup --create-project --project my-new-proj --project-name "My app"
```

Notes:

- Manual Auth Platform stages (branding, audience, data access, Desktop client creation) cannot be fully automated with supported public APIs; setup records **acknowledgments** only (not Google-verified completion).
- Acknowledgments reset if the paired project for the client changes.
- Exit codes: `0` complete or successful discovery, `1` incomplete/manual/blocked after emitting a report, `2` invalid usage.
- Direct `gog auth credentials` / `gog auth add` remain supported for advanced use.

### Machine-readable setup report

`--json` emits one `SetupReport` object on stdout; `--plain` emits a stable TSV report on stdout. Progress and human guidance remain on stderr. Both forms emit a report before exit `1`.

Top-level JSON fields are `complete`, `discovery_only`, `discovery_complete`, `projects_truncated`, `client`, `project_id`, `gcloud_account`, `services`, `service_usage_ids`, `missing_apis`, `projects`, `stages`, `next`, and `continue_command` (empty optional fields are omitted). `complete` means the whole setup is complete. `discovery_complete` means all requested discovery inspections succeeded; it does not mean setup is complete. Project discovery uses one list call with `limit + 1`; `projects_truncated` is true only when an extra project was found.

Each `stages` element contains `id`, `status`, `action_kind`, and optional `summary`, `blocker`, `resumable`, `console_url`, `command`, `next_action`, and `detail`. The defined stage IDs, in execution order, are `gcloud_install`, `gcloud_auth`, `project`, `apis`, `branding`, `audience`, `data_access`, `desktop_client`, `credentials`, and `account`. A full report always lists every stage. After the first incomplete stage, later stages have status `unavailable`: they were not inspected and no prompts, credential reads, OAuth, or mutations occurred.

Statuses are `ok`, `missing`, `blocked`, `failed`, `manual`, `acknowledged`, and `unavailable`. Action kinds are `none`, `command`, and `console`. `blocked` with `resumable: true` denotes an external prerequisite that can be retried after action or propagation; malformed input, invalid credentials, and invariant violations are `failed` and non-resumable. `next` and `continue_command` identify the first required action. Continuation commands preserve retry-defining non-secret inputs; credential paths are included when supplied, but stdin credential content and all secrets are never emitted.

Plain output has the header `STAGE STATUS SUMMARY BLOCKER COMMAND` (tab-separated), one row per stage, then `meta complete <true|false> <project_id> <continue_command>`. Consumers should tolerate new JSON fields and stage IDs. Exit `0` means `complete` or successful `discovery_complete`; exit `1` means a report was emitted but work remains; exit `2` is invalid usage.
