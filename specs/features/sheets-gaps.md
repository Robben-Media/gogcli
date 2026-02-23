# Sheets v4 -- Gap Coverage Spec

**API**: Google Sheets v4 (`sheets/v4`)
**Current coverage**: 9 methods (spreadsheets.batchUpdate/create/get, values.append/batchGet/batchUpdate/clear/get/update)
**Gap**: 8 missing methods
**Service factory**: `newSheetsService` (existing)

## Overview

Adding 8 missing methods to the Sheets CLI commands. Covers developer metadata (get/search), data-filter-based operations (getByDataFilter, batchGetByDataFilter, batchUpdateByDataFilter, batchClearByDataFilter), sheet copying (copyTo), and batch-clear to achieve full Discovery API parity.

---

## Developer Metadata

### `gog sheets metadata get`

- **API method**: `spreadsheets.developerMetadata.get`
- **Struct**: `SheetsMetadataGetCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `metadataId` (required arg): developer metadata ID (integer)
- **Output**: JSON object with `metadataId`, `metadataKey`, `metadataValue`, `location`, `visibility`; text shows ID, KEY, VALUE, LOCATION, VISIBILITY
- **Test**: httptest mock GET `/v4/spreadsheets/{spreadsheetId}/developerMetadata/{metadataId}`

### `gog sheets metadata search`

- **API method**: `spreadsheets.developerMetadata.search`
- **Struct**: `SheetsMetadataSearchCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `--key` (string): metadata key to search for
  - `--value` (string): metadata value to match
  - `--location-type` (string): ROW, COLUMN, SHEET, or SPREADSHEET
  - `--visibility` (string): DOCUMENT or PROJECT
- **Behavior**: Builds a DataFilter from provided flags. At least one filter criterion must be specified.
- **Output**: JSON object with `matchedDeveloperMetadata` array; text table ID, KEY, VALUE, LOCATION
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}/developerMetadata:search`, assert dataFilters in body

---

## Spreadsheets

### `gog sheets get-by-filter`

- **API method**: `spreadsheets.getByDataFilter`
- **Struct**: `SheetsGetByFilterCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `--filters-json` (required string): JSON array of DataFilter objects, or @filepath
  - `--include-grid-data` (bool): include cell data in response
- **Behavior**: Retrieves spreadsheet data matching the given data filters (DeveloperMetadataLookup or GridRange). More targeted than a full `spreadsheets.get`.
- **Output**: JSON object with filtered spreadsheet data (sheets, properties, namedRanges)
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}:getByDataFilter`, assert dataFilters and includeGridData in body

---

## Sheets

### `gog sheets copy-to`

- **API method**: `spreadsheets.sheets.copyTo`
- **Struct**: `SheetsCopyToCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): source spreadsheet ID
  - `sheetId` (required arg): source sheet ID (integer, the tab ID)
  - `--destination-spreadsheet-id` (required string): target spreadsheet ID
- **Behavior**: Copies a single sheet (tab) from one spreadsheet to another. The copied sheet gets a new ID in the destination.
- **Output**: JSON object with `sheetId`, `title`, `index`, `sheetType` of the new sheet in destination; text shows SHEET_ID, TITLE, INDEX
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}/sheets/{sheetId}:copyTo`, assert destinationSpreadsheetId in body

---

## Values -- Batch Clear

### `gog sheets values batch-clear`

- **API method**: `spreadsheets.values.batchClear`
- **Struct**: `SheetsValuesBatchClearCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `--ranges` (required []string): A1 notation ranges to clear (e.g., "Sheet1!A1:B10", "Sheet2!C:C")
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Clears values (not formatting) from multiple ranges in a single call.
- **Output**: JSON object with `spreadsheetId` and `clearedRanges` array; text shows "Cleared N ranges"
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}/values:batchClear`, assert ranges in body

### `gog sheets values batch-clear-by-filter`

- **API method**: `spreadsheets.values.batchClearByDataFilter`
- **Struct**: `SheetsValuesBatchClearByFilterCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `--filters-json` (required string): JSON array of DataFilter objects, or @filepath
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Same as batch-clear but uses DataFilter instead of A1 notation.
- **Output**: JSON object with `spreadsheetId` and `clearedRanges`
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}/values:batchClearByDataFilter`, assert dataFilters in body

### `gog sheets values batch-get-by-filter`

- **API method**: `spreadsheets.values.batchGetByDataFilter`
- **Struct**: `SheetsValuesBatchGetByFilterCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `--filters-json` (required string): JSON array of DataFilter objects, or @filepath
  - `--major-dimension` (string): ROWS or COLUMNS (default: ROWS)
  - `--value-render` (string): FORMATTED_VALUE, UNFORMATTED_VALUE, FORMULA (default: FORMATTED_VALUE)
  - `--date-time-render` (string): SERIAL_NUMBER or FORMATTED_STRING
- **Behavior**: Retrieves values matching DataFilter criteria. Alternative to batchGet when using metadata or grid ranges.
- **Output**: JSON object with `valueRanges` array; text output as formatted table per range
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}/values:batchGetByDataFilter`, assert dataFilters in body

### `gog sheets values batch-update-by-filter`

- **API method**: `spreadsheets.values.batchUpdateByDataFilter`
- **Struct**: `SheetsValuesBatchUpdateByFilterCmd`
- **Args/Flags**:
  - `spreadsheetId` (required arg): spreadsheet ID
  - `--data-json` (required string): JSON array of DataFilterValueRange objects, or @filepath
  - `--value-input` (string): RAW or USER_ENTERED (default: USER_ENTERED)
  - `--include-values-in-response` (bool): return updated values
  - `--response-value-render` (string): FORMATTED_VALUE, UNFORMATTED_VALUE, FORMULA
  - `--response-date-time-render` (string): SERIAL_NUMBER or FORMATTED_STRING
- **Behavior**: Updates values in ranges identified by DataFilters. The data-json contains an array of objects with `dataFilter` and `values`.
- **Output**: JSON object with update metadata (updatedRows, updatedColumns, updatedCells, etc.)
- **Test**: httptest mock POST `/v4/spreadsheets/{spreadsheetId}/values:batchUpdateByDataFilter`, assert data array in body

---

## Implementation Notes

1. **DataFilter variants**: The `ByDataFilter` methods use `DataFilter` objects instead of A1 notation ranges. A DataFilter can be either a `DeveloperMetadataLookup` (match by metadata key/value) or a `GridRange` (by sheet ID + row/column indices). These are too complex for individual flags, so they accept JSON input.
2. **Developer metadata**: Metadata is attached to spreadsheet elements (rows, columns, sheets, or the whole spreadsheet). It is invisible to regular users and used by applications to store state.
3. **sheets.copyTo**: The `sheetId` is the numeric tab ID (visible in URL `#gid=123`), not the tab name. The destination spreadsheet must be accessible by the authenticated user.
4. **Destructive operations**: `batchClear` and `batchClearByDataFilter` clear cell values but preserve formatting. Still require `confirmDestructive()` since data loss is involved.
5. **JSON input pattern**: All `--*-json` flags should support both inline JSON strings and `@filepath` syntax consistent with other gogcli commands.
6. Total new test count: minimum 8 test functions plus DataFilter validation edge cases.
