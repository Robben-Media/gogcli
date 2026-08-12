# gogcli

gogcli provides command-line access to Google services while preserving explicit safety boundaries for human and automated use.

## Language

**Runtime read-only mode**:
An invocation-wide, opt-in safety mode enabled by `--readonly` or `GOG_READONLY=1`. It blocks mutating Google API requests at the HTTP transport boundary before dispatch. GET, HEAD, OPTIONS, and reviewed semantic-read POST endpoints remain available.
_Avoid_: Dry run, promptable mode

**Read-only transport guard**:
The fail-closed HTTP wrapper installed on Google API clients while runtime read-only mode is active. Requests that are not an allowed read are blocked before their base transport is called.
_Avoid_: Command policy

**Command policy**:
An independent command-level safety control. Policies are evaluated before command execution and are not overridden by runtime read-only mode, `--force`, or destructive-command confirmations.
_Avoid_: Read-only transport guard

**Operational state**:
Incidental local state maintained while commands run, such as refreshed credentials, caches, and bookkeeping needed to continue reading.
_Avoid_: CLI configuration
