---
name: Development Workflow and Operating Rules
description: Rules for Agent operations, branch strategy, testing, and Definition of Done.
---

# Development Workflow

## 1. Branch Strategy (Trunk-Based Development)
Keep `main` as the sole integration branch. Feature branches must be short-lived.

**Branch Naming Convention:**
- Use Conventional Commits prefixes: `feat/`, `fix/`, `chore/`, `docs/`, `refactor/`.
- Include the issue number: `<type>/<issue-number>-<short-desc>` (e.g., `feat/160-small-medals`).

**Safe Branching Procedure:**
Never branch directly from another feature branch unless explicitly stacking PRs.
1. `git checkout main`
2. `git pull --ff-only origin main`
3. `git checkout -b <type>/<issue-number>-<short-description>`

**Merge Strategy & PR Titles:**
- Always use **Squash and Merge**.
- Because of Squash and Merge, **the PR Title MUST strictly follow Conventional Commits** (e.g., `feat: implement small medals`). This ensures the `main` branch history remains pristine and automatically parsable.

## 2. Issue and PR Workflow
- **No substantial work without an Issue:** Do not begin substantial implementation from an informal request without an Issue.
- **Templates:** Check `.github/ISSUE_TEMPLATE` and `.github/PULL_REQUEST_TEMPLATE.md`. Every Issue **must** use the provided repository Issue template. Every PR **must** use the repository PR template and all checkboxes must be honestly verified. Do not bypass templates.
- **Study Existing Implementations:** Before generating new logic from scratch, actively search the codebase (using `fd`, `grep`, or IDE tools) for existing features that solve similar problems. Adopt the same architectural patterns, variable naming conventions, and file structures.
- **Transparent Tool Usage:** When analyzing codebases, prefer native tools (`view_file`, `grep_search`). Do NOT execute complex or opaque bash scripts (like `sed`, `awk`, or `perl` one-liners) to parse code without explicitly explaining your intent to the user first. Ensure transparency in your actions.
- **Bounded Tasks:** Select a bounded task. If the requested work is too large, split it into smaller Issues; preserve independently testable acceptance criteria; do not silently expand the current Issue.

## 3. TDD and Local Verification (Tiered Strategy)
For non-trivial behavior:
1. **Prepare Environment:** Run `make up` to ensure the local DB and Cache are running. This is required for fast integration testing and MCP tool access.
2. Identify acceptance criteria and write/update tests.
3. Implement the smallest change satisfying the tests.
4. **Inner Loop:** Run fast host tests continuously using `make test` (or `make test-integration`).
5. **Outer Loop:** Auto-format code with `make fmt`, then run unified local verification with `make check` (runs format check, `go vet`, host test suite, and fast smoke build).

## 4. Definition of Done
A ticket is complete only when applicable:
- Acceptance criteria are satisfied.
- Behavior is covered by tests.
- Unified local checks (`make check`) pass completely.
- Architecture remains valid and no unrelated changes were introduced.
- Documentation/status is updated when necessary.
- PR template requirements are satisfied.

## 5. Token-Budget Strategy
Optimize for **tested, reviewable, mergeable functionality**, not maximum generated code.
Prefer: `one small Issue` + `tests` + `minimum implementation` + `one focused PR`.
If implementation reveals a significant architectural question, stop the local implementation and create an appropriate decision Issue rather than consuming the token budget on speculative redesign.

## 6. Dependency and License Gate
Before adding a dependency:
1. Determine why it is needed.
2. Check whether the standard library or existing dependencies are sufficient.
3. Inspect its license (and transitive licenses).
4. Verify compatibility with the project's licensing strategy.
