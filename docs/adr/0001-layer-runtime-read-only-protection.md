# Layer runtime read-only protection

Runtime read-only mode is an opt-in, fail-closed outbound-request guard. When `--readonly` or `GOG_READONLY=1` is active, it permits GET, HEAD, OPTIONS, and a small reviewed allowlist of semantic-read POST endpoints; it blocks every other POST, PUT, PATCH, and DELETE before dispatch. The guard applies to shared Google API clients and service-account Keep clients.

## Consequences

The public interface remains compatible with upstream `--readonly` and `GOG_READONLY=1`. Read-only mode is an absolute runtime backstop for Google API mutations: prompts, `--force`, and existing destructive-command confirmations cannot grant an exception. Independent command policies run before execution and remain separate. Local auth/config persistence is intentionally outside this transport boundary so `auth add --readonly` retains upstream-compatible readonly OAuth-scope behavior. MCP and schema exposure are not applicable to #154: this repository has no MCP server or schema runtime.
