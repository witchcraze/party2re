---
name: Development Workflow and Operating Rules
description: Rules for Agent operations, branch strategy, testing, and Definition of Done.
---

# Development Workflow

## 1. Branch Strategy
Keep `main` as the integration branch. Do not implement Issue work directly on `main`.
Before editing:
1. Update a clean local `main` with `git pull --ff-only origin main`.
2. Create one new branch for the Issue (e.g., `feature/issue-4-character-persistence`).
3. Commit and push only the Issue's changes to its branch. Open a PR from that branch to `main`.
4. **Merge Strategy:** Always use **Squash and Merge** (or a single standardized method chosen by the project) to keep the `main` branch history clean and readable. 

## 2. Issue and PR Workflow
- **No substantial work without an Issue:** Do not begin substantial implementation from an informal request without an Issue.
- **Templates:** Every Issue **must use a repository Issue template**. Every PR **must use the repository PR template**. Do not bypass templates.
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
