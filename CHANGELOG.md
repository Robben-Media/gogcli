# Changelog

## Unreleased

Current capabilities of the native Google MCP server (`gog-mcp`). This section describes the tree as it stands; there is no versioned release for these notes yet.

### Server

- Native Go MCP server (`gog-mcp`) speaking MCP over stdio by default, with optional stateless Streamable HTTP (`--http-addr` + authenticated caller file, endpoint `/mcp`).
- Explicit account model: `accounts_list` reports connected Google accounts with per-account capabilities; every Google operation takes an explicit `account_id`. No ambient default account.
- Startup grant model: operation allow-lists (`--allow-operations` / `--grants-file`), account/client/action grants, and config policies. stdio reloads grants and policies on SIGHUP; HTTP callers are a validated startup snapshot.
- Bounded execution: per-request Google API attempt budget (1–256, default 32, including retries), per-tool deadline (default 30s), server-wide concurrency limit (default 32), and 8 MiB body bounds. Truncated responses say so.

### Tools and discovery

- Curated read-only operations for Gmail (search, message, thread), Drive (search, metadata), Google Docs (bounded text extraction), Calendar (lists, events, free/busy), GA4 (properties, metadata, reports), Search Console (sites, queries), and Sheets (metadata, range reads). All are `SafeRead`.
- Compact discovery (`--discovery=compact`): `capabilities_search`, `capabilities_describe`, and `capabilities_execute` gateways, so clients discover granted operations on demand.
- Extended API catalog (`--api-catalog`, requires compact discovery): 1,108 pinned Google discovery methods across 29 services executed as JSON REST, plus Business Profile account/location reads. Catalog access follows the same grants as everything else; media upload/download transports are excluded.
- Guided authoring workflows: formatted mail prepare/draft/send, and structured creation for Docs, Slides, and Sheets, each with explicit call caps.
- Five read-only workflow recipe resources (`gog://workflows/v1/{mail,documents,calendar,reporting,sheets}`). Resources, not native MCP Skills.

### Media

- Bounded Gmail attachment reads and Drive download/export operations returning temporary account-bound artifact references (`gog://media/{id}`) that expire after 15 minutes, with optional inline delivery capped at 1 MiB default / 2 MiB maximum.
- Bounded Drive content writes (`drive_create_file`, `drive_update_file`) including optimistic `expected_version` replacement.

### Accounts and safety

- Browser OAuth onboarding through the loopback account connect page (`--connect-addr`): connect, reconnect, disconnect, and status, using the app-owned OAuth client bucket. No service account keys.
- Writes disabled by default; enabled only with `--enable-writes` plus explicit write grants. Ambiguous writes surface `outcome_unknown` and must be reconciled before repeating.
- Tokens stored in the OS keyring or encrypted keyring file; HTTP callers authenticate with SHA-256 token digests. Credentials, tokens, and registries stay outside Git.

### Deployment

- Container package in `deploy/google-mcp/` (Dockerfile, Compose, smoke test) for a single Google MCP service behind an existing ingress; see `deploy/google-mcp/README.md`.
