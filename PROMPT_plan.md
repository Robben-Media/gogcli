# Planning Mode — gogcli API Gap Coverage

Study the specifications and create an implementation plan organized into phases.

## Your Task

1. Read all files in `specs/features/` directory
2. Read `AGENTS.md` for project context, build commands, and coding patterns
3. Read `docs/reports/cli-vs-api-gap-report-2026-02-22.json` for authoritative gap data
4. Read the existing codebase to understand what's already built (especially `internal/cmd/root.go` and existing command files)
5. Create `IMPLEMENTATION_PLAN.md` with ordered tasks grouped into phases

## Implementation Plan Format

```markdown
# Implementation Plan — gogcli API Gap Coverage

Generated: [timestamp]
Last Updated: [timestamp]

## Summary
Adding 588 missing Google API methods to gogcli across 19 APIs to achieve full Discovery API parity.

## Architecture Decisions
- [Key technical decisions]

## Blockers / Questions
1. [Any missing information or decisions needed]

---

## Phase 1: [API Group Name]
> Branch: `phase/1-[name]` | PR → build branch

### 1. [Task Name]
- **Status**: pending
- **Depends on**: none | [task numbers]
- **Spec**: specs/features/[api]-gaps.md
- **Description**: [Detailed — what to build, file paths to create/modify]
- **Files**: `internal/cmd/[file].go` — [create/modify]
- **Verification**: `make ci` passes, new commands appear in `gog [api] --help`

### 2. [Next Task]
...
```

## Phasing Strategy

Group tasks by API to minimize context-switching. Recommended phase order:

1. **Small APIs first** (docs, keep, tasks, searchconsole, analyticsdata, sheets) — quick wins, build momentum
2. **Core APIs** (gmail, calendar, drive, people, chat) — most value, well-understood patterns
3. **Business APIs** (mybusinessaccountmanagement, mybusinessbusinessinformation) — niche but small
4. **Large APIs** (bigquery, classroom, analyticsadmin) — more methods, more complexity
5. **Massive APIs** (youtube, tagmanager, cloudidentity) — largest gap counts, most methods

Within each phase, order by:
1. Service factory first (if new API needs one)
2. List/Get commands (read-only, safe to test)
3. Create commands
4. Update/Patch commands
5. Delete commands (destructive, need confirmDestructive)
6. Specialized commands (watch, import, batch operations)

## Task Requirements

- Each task should cover ONE resource group within an API (e.g., "Gmail CSE Identities" or "Calendar ACL management")
- Include exact file paths to create or modify
- Reference specific spec files
- Include verification: `make ci` + specific `gog [api] [command] --help` checks
- Each task completable in ONE iteration
- Do NOT pre-assign agent types or pre-plan subtasks

## Guidelines

- Flag ambiguities in the Blockers section
- Never put all commands for a large API into one mega-task
- Group related commands (e.g., all CRUD for one resource) into one task
- Keep tasks focused — one resource group per task

## Output

After creating `IMPLEMENTATION_PLAN.md`, summarize:
- Total tasks and phase breakdown
- Any blockers or questions
- Recommended starting point

## Completion Signal

After you have:
1. Read all specs in `specs/features/`
2. Created/updated `IMPLEMENTATION_PLAN.md` with tasks for every spec
3. Provided your summary

Output exactly:

<promise>COMPLETE</promise>

This signals the planning loop to exit. Do NOT continue iterating once all specs are planned.
