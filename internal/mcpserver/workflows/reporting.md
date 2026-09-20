# Google reporting workflow

1. Call `accounts_list` and use one opaque `account_id` for all reporting calls.
2. For GA4, identify the property with `analytics_list_properties`. Check `analytics_metadata` before composing `analytics_report`; report dimensions and metrics exactly as returned. Use inclusive `start_date` and `end_date`, bounded rows, `offset` for additional rows, and preserve property timezone and totals.
3. For Search Console, identify the exact `site_url` with `searchconsole_list_sites`, then query with inclusive dates and bounded rows. Search Console dates follow its Pacific-time convention; do not relabel them as GA4 property dates.
4. Preserve metric names and meanings. Zero rows from a successful query are valid empty data. Quota, scope, and access failures are errors, not zero traffic.
