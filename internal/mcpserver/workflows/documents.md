# Google documents workflow

1. Call `accounts_list` and use one opaque `account_id` for every Drive or Docs call in the task.
2. If the task already supplies an exact document ID, read that document directly. Otherwise find candidates with `drive_search`. Use the returned file ID and `web_view_link` as source identifiers, including results from shared drives.
3. Fetch metadata with `drive_get_file` only when needed to resolve file type, identity, or ownership. Extract text with `docs_get_text` only for a Google Doc. Pass a bounded `max_bytes` and an explicit `tab_id` only when the task names a tab.
4. Quote or summarize only returned text. Report document ID, title or link when available, and say what was truncated. Permission or not-found failures mean that source was unavailable; do not replace it with an unrequested file.

When compact discovery is active, search for the task with `capabilities_search`, inspect the selected capability with `capabilities_describe`, and execute its exact schema through `capabilities_execute`. Follow `inspect_paths` to inspect only needed nested schema fields. Discovery and service guidance use no Google calls.

For authorized creation, prefer `docs_create_document` or `slides_create_presentation` when available. Supply structured paragraphs or slides with explicit formatting. Each uses a create call and one batch update. A partial result retains the new resource ID: reconcile that resource instead of creating another. Review slide layout before sharing; text fit is not automatically rendered or checked. Use exact revision controls for edits to existing content when supported.
