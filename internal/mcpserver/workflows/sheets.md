# Google Sheets workflow

1. Call `accounts_list` and use one opaque `account_id` for both Sheets calls.
2. Call `sheets_get_metadata` first to identify spreadsheet ID, title, timezone, and sheet dimensions. Use exact sheet titles in A1 ranges and quote a title when it contains spaces or punctuation.
3. Read the smallest bounded range that answers the task with `sheets_read_range`. Select `FORMULA` only when formulas are requested; otherwise use formatted or unformatted values deliberately.
4. Preserve sparse rows and empty cells in row order. Report the returned range, render option, source spreadsheet ID, and any truncation. Do not infer values from formatting or fill missing cells with guesses.
