# Google Docs v1 -- Gap Coverage Spec

**API**: Google Docs v1 (`docs/v1`)
**Current coverage**: 2 methods (documents.batchUpdate, documents.get)
**Gap**: 1 missing method
**Service factory**: `newDocsService` (existing)

## Overview

Adding 1 missing method (documents.create) to the Google Docs v1 CLI commands. Covers document creation to achieve full Discovery API parity.

All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Text output uses TSV-formatted columns (DOCUMENT_ID, TITLE, REVISION_ID). Error handling includes test coverage for invalid requests and missing required fields.

---

## Documents

### `gog docs create`

- **API method**: `documents.create`
- **Struct**: `DocsCreateCmd`
- **Args/Flags**:
  - `--title` (required string): document title
- **Behavior**: Creates a new empty Google Doc with the given title. Returns the full document resource including the generated `documentId`. The document is created in the user's My Drive root folder.
- **Output**: JSON object with `documentId`, `title`, `revisionId`, `body`, `headers`, `footers`, etc.; text shows DOCUMENT_ID, TITLE, REVISION_ID
- **Test**: httptest mock POST `/v1/documents`, assert `title` in request body, assert response contains documentId

---

## Implementation Notes

1. **Simple command**: This is the only missing method. The documents.create endpoint takes a minimal request body with just the title. All other document content is added via subsequent batchUpdate calls.
2. **No Workspace requirement**: Google Docs API works with both consumer and Workspace accounts.
3. **Drive integration**: The created document appears in the user's My Drive root. If the user wants it in a specific folder, they would use `gog drive move` afterward.
4. **Response structure**: The API returns the full Document resource, which can be large (includes empty body structure, default styles, etc.). The JSON output includes everything; the text output shows only the key identifiers.
5. Total new test count: 1 test function (plus one for error handling).
