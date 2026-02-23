# Specs — gogcli API Gap Coverage

588 missing Google API methods across 19 APIs. Full implementation with tests.

## Design System

- **[CLI_DESIGN_SYSTEM.md](CLI_DESIGN_SYSTEM.md)** — Command naming, global flags, output contract, error taxonomy, pagination, 10 golden commands. All specs conform to this.

## Linter

```bash
./specs/lint-specs.sh          # Lint all specs
./specs/lint-specs.sh specs/features/gmail-gaps.md  # Lint one
```

## Feature Specs (by gap count)

| Spec | API | Gaps | Lines |
|------|-----|------|-------|
| [tagmanager-gaps.md](features/tagmanager-gaps.md) | Tag Manager v2 | 99 | 406 |
| [youtube-gaps.md](features/youtube-gaps.md) | YouTube v3 | 77 | 504 |
| [cloudidentity-gaps.md](features/cloudidentity-gaps.md) | Cloud Identity v1 | 60 | 327 |
| [analyticsadmin-gaps.md](features/analyticsadmin-gaps.md) | Analytics Admin v1beta | 53 | 604 |
| [classroom-gaps.md](features/classroom-gaps.md) | Classroom v1 | 51 | 412 |
| [bigquery-gaps.md](features/bigquery-gaps.md) | BigQuery v2 | 42 | 459 |
| [drive-gaps.md](features/drive-gaps.md) | Drive v3 | 40 | 461 |
| [gmail-gaps.md](features/gmail-gaps.md) | Gmail v1 | 34 | 447 |
| [chat-gaps.md](features/chat-gaps.md) | Google Chat v1 | 32 | 410 |
| [calendar-gaps.md](features/calendar-gaps.md) | Calendar v3 | 26 | 365 |
| [mybusinessaccountmanagement-gaps.md](features/mybusinessaccountmanagement-gaps.md) | Business Account Mgmt v1 | 15 | 203 |
| [mybusinessbusinessinformation-gaps.md](features/mybusinessbusinessinformation-gaps.md) | Business Info v1 | 13 | 211 |
| [people-gaps.md](features/people-gaps.md) | People v1 | 13 | 184 |
| [analyticsdata-gaps.md](features/analyticsdata-gaps.md) | Analytics Data v1beta | 8 | 131 |
| [sheets-gaps.md](features/sheets-gaps.md) | Sheets v4 | 8 | 138 |
| [searchconsole-gaps.md](features/searchconsole-gaps.md) | Search Console v1 | 6 | 102 |
| [tasks-gaps.md](features/tasks-gaps.md) | Google Tasks v1 | 6 | 103 |
| [keep-gaps.md](features/keep-gaps.md) | Google Keep v1 | 4 | 81 |
| [docs-gaps.md](features/docs-gaps.md) | Google Docs v1 | 1 | 36 |
| **Total** | **19 APIs** | **588** | **5,584** |

## Source Data

- Gap report: `docs/reports/cli-vs-api-gap-report-2026-02-22.json`
- Summary: `docs/reports/cli-vs-api-gap-summary-2026-02-22.tsv`

## Next Steps

```bash
cd gogcli
./loop.sh plan    # Generate IMPLEMENTATION_PLAN.md
./loop.sh 50      # Build (50-iteration limit recommended for 588 methods)
```
