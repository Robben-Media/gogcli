# Google documents workflow

1. Call `accounts_list` and use one opaque `account_id` for every Drive or Docs call in the task.
2. Find candidates with `drive_search`. Use the returned file ID and `web_view_link` as source identifiers, including results from shared drives.
3. Confirm ownership and metadata with `drive_get_file`, then extract text with `docs_get_text` only for a Google Doc. Pass a bounded `max_bytes` and an explicit `tab_id` only when the task names a tab.
4. Quote or summarize only returned text. Report document ID, title or link when available, and say what was truncated. Permission or not-found failures mean that source was unavailable; do not replace it with an unrequested file.
