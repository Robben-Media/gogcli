# Google Sheets workflow

1. Call `accounts_list` and use one opaque `account_id` for both Sheets calls.
2. If the exact spreadsheet ID and range are already known, read directly. Otherwise call `sheets_get_metadata` to identify spreadsheet ID, title, timezone, and sheet dimensions. Use exact sheet titles in A1 ranges and quote a title when it contains spaces or punctuation.
3. Read the smallest bounded range that answers the task with `sheets_read_range`. Select `FORMULA` only when formulas are requested; otherwise use formatted or unformatted values deliberately.
4. Preserve sparse rows and empty cells in row order. Report the returned range, render option, source spreadsheet ID, and any truncation. Do not infer values from formatting or fill missing cells with guesses.

For authorized creation, discover `sheets_create_spreadsheet` when available. Supply explicitly typed cells; `text` stays literal even when it begins with `=`, while `formula` requests evaluation. The workflow creates cells and optional header formatting in one call. For existing spreadsheets, inspect the exact update schema and send only the required range or batched changes. Do not read the entire workbook to update a known range.

Keep pagination and retries within the returned API attempt budget. HTTP call counts are not Google quota units. After an unknown write outcome, reconcile the exact spreadsheet before retrying.
