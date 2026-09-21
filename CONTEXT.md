# gog-mcp

gog-mcp is a native Google MCP server that connects agents to Google accounts over stdio or Streamable HTTP while enforcing explicit grants and bounded execution.

## Language

**Capability discovery**:
The tools a client uses to find granted operations: `capabilities_search` and `capabilities_describe` in compact mode, or the expanded per-operation tool list. Discovery reflects grants and write enablement; it never reveals more than the caller may execute.
_Avoid_: Tool dump, schema listing

**Curated read operation**:
A `SafeRead` Google operation from the default catalog (for example `gmail_search` or `calendar_list_events`), retryable and bounded. The default discovery mode exposes these as individual MCP tools.
_Avoid_: Treating a capability as an unrestricted API proxy

**API catalog**:
The opt-in extended operation set (`--discovery=compact --api-catalog`) built from a pinned Google discovery snapshot plus Business Profile reads, media operations, and authoring workflows. Loading the catalog changes discoverability, never authorization.
_Avoid_: Full API coverage, parity

**Write gate**:
The rule that write operations run only when the server enables writes and the caller holds an explicit write grant. Without both, write operations are invisible and return `forbidden_operation`.
_Avoid_: Confirmation prompt

**Non-replayable write**:
A write operation that must never be retried blindly after an ambiguous failure. The typed `outcome_unknown` error category marks these cases; callers reconcile with reads before repeating.
_Avoid_: Idempotent retry

**Grant**:
A startup-time binding of principal, account IDs, client names, and operations that authorizes a caller to use specific operations against specific accounts. Empty grants authorize nothing.
_Avoid_: Permission bump, runtime escalation

**Media artifact**:
A temporary, account-bound reference (`gog://media/{id}`) returned by bounded Gmail/Drive media reads. It expires after 15 minutes, is never enumerated, and is read back with fresh authorization.
_Avoid_: File download, local path

**Account connect page**:
The loopback web surface (`--connect-addr`) where a human authorizes Google accounts through the app-owned OAuth client. The deployed service keeps no public account-page listener.
_Avoid_: Admin UI, dashboard

**Workflow resource**:
A versioned, read-only recipe document at `gog://workflows/v1/{slug}` describing bounded patterns for one service. Resources are documentation; they do not execute and are not native MCP Skills.
_Avoid_: Skill, automation
