# gogcli

gogcli provides command-line access to Google services while preserving explicit safety boundaries for human and automated use.

## Language

**Runtime read-only mode**:
An invocation-wide safety mode in which changes to Google data and intentional CLI configuration require explicit prompt confirmation. Incidental operational state required to perform reads may still be persisted.
_Avoid_: Dry run, immutable mode

**Prompt confirmation**:
A response read from standard input that selects a write exception before a declared command operation proceeds. It may be supplied interactively or through piped input and therefore does not prove human presence.
_Avoid_: Human approval

**Declared command operation**:
A user-facing mutation whose purpose and target are identified before execution. It requires an applicable write exception, and undeclared mutations are blocked.
_Avoid_: HTTP request, blanket approval

**Intentional CLI configuration**:
Persistent CLI state that a user explicitly asks to change, including authentication, policy, and configuration records.
_Avoid_: Operational state

**Operational state**:
Incidental local state maintained while commands run, such as refreshed credentials, caches, and bookkeeping needed to continue reading.
_Avoid_: CLI configuration

**Protected service**:
A Google service whose mutations require prompt confirmation while runtime read-only mode is active. Every service is protected by default until an applicable write exception says otherwise.
_Avoid_: Read-only OAuth scope, denied service

**Write exception**:
An explicit authorization for mutations on a particular target, through a particular operation, or within an entire service. It may cover one declared operation, persist across future invocations, or last for a long-running host process; persisted exceptions default to one account and OAuth client, and no exception can override an independent safety control.
_Avoid_: Permission, policy override
