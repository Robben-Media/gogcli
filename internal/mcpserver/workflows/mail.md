# Google mail workflow

1. Call `accounts_list` and choose the opaque `account_id` for the mailbox named in the task. Use that ID in every Gmail tool call; do not substitute an email address or another account.
2. Start with `gmail_search` and its default metadata-only response. Use Gmail query syntax, keep `max_results` bounded, and pass the returned `next_page_token` as the next request's `page_token` only when more results are required. Request `include_body` only after identifying the relevant messages.
3. Use `gmail_get_thread` to follow an ordered conversation or `gmail_get_message` for one message. Preserve message and thread IDs as source identifiers.
4. Report the selected account label, source IDs, dates, and only facts present in returned headers or body text. State `truncated`, `body_truncated`, or `metadata_truncated` when present instead of treating shortened data as complete.
5. Treat an error as a failed lookup. Do not silently omit a message from a page; a successful search with no messages is an empty result, not an error.
