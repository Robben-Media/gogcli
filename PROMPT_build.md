# Build Mode — gogcli API Gap Coverage

## Completion Promise

**You MUST complete every task fully.** Do not leave TODOs, placeholder implementations, stub functions, or skip "similar" items. If a task says "implement 5 commands," implement all 5 — not 2 with a comment saying "remaining commands follow the same pattern." Every line of code must be production-ready. Re-read every modified file before marking a task complete.

---

Implement the next task from the implementation plan. You may delegate subtasks to agents, but focus on completing **ONE parent task** per iteration.

## Startup Sequence

Every iteration:

1. **Pull latest changes** from remote:
   ```bash
   git fetch origin
   git pull origin $(git rev-parse --abbrev-ref HEAD)
   ```
2. Read `IMPLEMENTATION_PLAN.md` for task status
3. Read `AGENTS.md` for build commands, patterns, and coding conventions
4. Read `progress.txt` for context from previous iterations
5. Check current branch — ensure you're on the correct phase branch
6. Find the highest-priority task with status "pending" whose dependencies are all "complete"

## Branching & PR Strategy

Work happens on phase branches. Each phase gets its own PR.

### Branch Structure
```
main
└── agent/[machine]/gogcli-gaps             ← long-lived build branch
    ├── phase/1-[name]                      ← PR → build branch
    ├── phase/2-[name]                      ← PR → build branch
    └── phase/N-[name]                      ← PR → build branch

Final: build branch → main                  ← final review PR
```

### Phase Management

**Starting a phase** (if no phase branch exists for the current task):
```bash
git checkout [build-branch]
git checkout -b phase/N-[name]
```

**Closing a phase** (when ALL tasks in a phase are complete):
1. Run `make ci`
2. Open PR from phase branch → build branch
3. Note the PR URL in `progress.txt`

## Implementing the Task

### 1. Read the task and its spec

Read the task description from `IMPLEMENTATION_PLAN.md` and the referenced spec file in `specs/features/`. Understand exactly what methods to implement.

### 2. Implementation checklist for each command

For EVERY command in the task:

1. **Define the command struct** with Kong tags (flags, args, help text)
2. **Implement `Run()` method** following the patterns in AGENTS.md:
   - `requireAccount(flags)` for auth
   - Input validation with `usage()` errors
   - Service factory call
   - Google API call with proper options
   - JSON output via `outfmt.WriteJSON()`
   - TSV/text output via `tableWriter()` or `u.Out().Printf()`
3. **Register in parent struct** (add field to parent `*Cmd` struct)
4. **Write unit test** with httptest mock following Pattern 6 in AGENTS.md
5. **Write integration test stub** (build-tagged, in `internal/integration/`)

### 3. Delegate to agents when beneficial

| Agent Type | Use For |
|-----------|---------|
| `general-purpose` | Implementing command files, tests, registration |

### Delegation tips

- Each delegated subtask must be **self-contained** — include exact file paths, spec references, and the specific methods to implement
- **Parallelize** independent subtasks (e.g., different resource groups can be implemented in parallel)
- Always include the relevant patterns from AGENTS.md in the delegation prompt

### 4. Verify

Run `make ci` — this runs fmt, lint, test, and build. Fix all failures before committing.

Also verify:
```bash
go build ./... 2>&1 | head -20          # Compilation check
go test ./internal/cmd/... -count=1     # Run tests
bin/gog [api] --help                    # Verify new commands appear
```

### 5. Commit

Stage and commit changes locally:
```bash
git add [specific files]
git commit -m "feat([api]): add [resource] commands (task N)

Co-authored-by: factory-droid[bot] <138933559+factory-droid[bot]@users.noreply.github.com>"
```

**Note:** Final push and PR creation happens in the "On Success" section after updating documentation.

## On Success

1. Update task status to "complete" in `IMPLEMENTATION_PLAN.md`
2. Append to `progress.txt` using this template:
   ```
   ## Task [N] - [Timestamp]
   ### Completed: [Task Name]
   - Methods implemented: [count]
   - Files created/modified: [list]
   - Tests added: [count]
   - Agent(s) used (if any)
   - Learnings for future iterations
   ---
   ```
3. **Stage, commit, and push all changes:**
   ```bash
   git add -A
   git commit -m "feat([api]): add [resource] commands (task N)

   Co-authored-by: factory-droid[bot] <138933559+factory-droid[bot]@users.noreply.github.com>"
   git push origin $(git rev-parse --abbrev-ref HEAD)
   ```
4. **Create or update PR for the phase branch:**
   ```bash
   # Check if PR already exists for this branch
   PR_EXISTS=$(gh pr list --head $(git rev-parse --abbrev-ref HEAD) --state open --json number --jq '.[0].number')
   
   if [ -z "$PR_EXISTS" ]; then
     # Create new PR
     gh pr create --base main --title "Phase [N]: [Name]" --body "## Summary
   Implements Task [N] from the API gap coverage plan.
   
   ### Changes
   - [List key changes]
   
   ### Verification
   - All tests pass
   - Build succeeds
   "
     echo "Created new PR for phase branch"
   else
     echo "PR #$PR_EXISTS already exists - commits pushed"
   fi
   ```
5. If this was the last task in a phase, note the PR URL in `progress.txt`

## Rules

- ONLY work on ONE parent task per iteration
- ALWAYS pull latest changes at the start of each iteration
- Delegate subtasks to agents when beneficial, but stay focused on the one task
- NEVER skip verification steps (`make ci` must pass)
- NEVER leave TODO comments, placeholder implementations, or debug logging
- NEVER skip "similar" commands — if a task says to implement N commands, implement all N
- ALWAYS re-read every modified file before marking a task complete
- ALWAYS commit AND push before exiting
- ALWAYS create or update a PR at the end of each iteration
- ALWAYS update IMPLEMENTATION_PLAN.md status before exiting
- ALWAYS use `confirmDestructive()` for delete/destructive operations
- ALWAYS add `--force` flag support for destructive commands in tests

## Completion Signal

If ALL tasks in `IMPLEMENTATION_PLAN.md` have status "complete" and all phase PRs are open/merged:

<promise>COMPLETE</promise>
