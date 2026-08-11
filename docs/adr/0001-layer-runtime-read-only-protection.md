# Layer runtime read-only protection

Runtime read-only mode will combine selective command-level declarations with a fail-closed outbound-request guard: all services are protected by default when the mode is active, known read-only requests may proceed, and undeclared mutations are blocked. Declared mutations may proceed only through an explicitly scoped write exception—one operation, a long-running host process, or a persisted target/operation/service rule—because users need both a safe automation default and deliberate Gmail-versus-Sheets flexibility; independent policy denials always remain stricter.

## Consequences

The public interface remains compatible with upstream `--readonly` and `GOG_READONLY=1`, but “read-only” is a protected default rather than an absolute prohibition: prompts may grant writes, and piped standard input may answer them. Persistent exceptions belong to the canonical policy system, default to account-and-client scope, expose broader scopes only through explicit progressive selection, and must be listable and revocable.
