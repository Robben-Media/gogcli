# Layer runtime read-only protection

Runtime read-only mode is an opt-in, fail-closed outbound-request guard. When `--readonly` or `GOG_READONLY=1` is active, it permits GET, HEAD, OPTIONS, and a small reviewed allowlist of semantic-read POST endpoints; it blocks every other POST, PUT, PATCH, and DELETE before dispatch. BigQuery query POSTs are intentionally blocked because their payload can execute DML and DDL. The guard applies to shared Google API clients and service-account Keep clients.

## Consequences

The public interface remains compatible with upstream `--readonly` and `GOG_READONLY=1`. Read-only mode is an absolute runtime backstop for Google API mutations: prompts, `--force`, and existing destructive-command confirmations cannot grant an exception. Independent command policies run before execution and remain separate. Local auth/config persistence is intentionally outside this transport boundary so `auth add --readonly` retains upstream-compatible readonly OAuth-scope behavior. This repository has no MCP server. Its `gog schema` command is a local, read-only CLI discovery surface that reports effective runtime readonly state but does not participate in outbound request enforcement.
