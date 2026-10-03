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

**Safe Branching & Working Tree Hygiene:**
1. Check working tree cleanliness: `rtk git status --porcelain`. Stash or commit uncommitted changes before switching branches. Never use `reset --hard` blindly.
2. Checkout and sync main: `rtk git checkout main && rtk git pull --ff-only origin main`.
3. Create feature branch: `rtk git checkout -b <type>/<issue-number>-<short-description>`.

**Merge Strategy & PR Titles:**
- Always **Squash and Merge**. PR Titles MUST follow Conventional Commits (e.g., `feat: implement small medals`).

**Post-Merge Local Cleanup:**
1. Return to synced main: `rtk git checkout main && rtk git pull --ff-only origin main`.
2. Delete local branch: `rtk git branch -D <branch-name> 2>/dev/null || true`.
3. Prune remote tracking branches: `rtk git fetch origin --prune`.

## 2. Issue and PR Workflow
- **No substantial work without an Issue:** Do not begin substantial implementation from an informal request without an Issue.
- **Templates:** Check `.github/ISSUE_TEMPLATE` and `.github/PULL_REQUEST_TEMPLATE.md`. Every Issue **must** use the provided repository Issue template. Every PR **must** use the repository PR template and all checkboxes must be honestly verified. Do not bypass templates.
- **Searchable Issue Naming, Sizing, and Deterministic Labeling Rules (SSOT):**
  - **Issue Titles:** MUST strictly follow `[<Type>] <Domain>: <Specific Action / Target>` (e.g., `[Bug] Home: Return 404 in GET /homes/{id}/companion/phrases`, `[Feature] HTTP: Guard action endpoints against active sleep penalty`). Prohibit vague titles like `Fix bug` or `Update system`.
  - **Deterministic Size Classification (Both Creation & Triage):**
    Issue bodies MUST describe work independently of implementation tools or providers; do not add execution-strategy or executor-recommendation sections. Size labels MUST describe work complexity only.
    Every issue MUST be classified into exactly one size label upon creation, enabling zero-token triage:
    1. **`size/large` (Large / Interactive)**: Requires DB migration (DDL), cross-package transactions, 500-line ceiling/1122-line ratchet refactoring, or interactive user design decisions.
    2. **`size/medium` (Medium: diff <= 150 lines)**: Standard single-package feature or refactoring, or adding/modifying endpoints requiring OpenAPI (`paths/<pkg>.json`) synchronization.
    3. **`size/small` (Small: diff <= 50 lines)**: Localized bug fix, formula/cap adjustment, single-handler guard, or unit test addition.
  - **Issue Status & Dependency Management Rules (明確な使い分けルール):**
    - **`status/needs-spec` (Label)**: Use when legacy specifications, formulas, or architectural designs are incomplete, undecided, or pending discussion/RFC. Implementation cannot proceed until specs are clarified.
    - **Native Issue Dependencies (`blocked by` / `blocking`)**: Use GitHub native dependencies instead of manual labels when the specification is clear, but another ticket/PR must be executed first (e.g., foundational infrastructure/models, security/auth guards on overlapping handlers, sequential PRs, or prerequisite domain logic).
      - **Native Registration**: When an issue depends on another issue, link it natively via `gh issue edit <issue-number> --add-blocked-by <predecessor-number>`. Do NOT apply a manual `status/blocked` label (`status/blocked` is deprecated).
      - **Sub-issues / Parent Hierarchy**: When decomposing large features or epics under the Anti-Fat-Issue rule, link child issues to the tracking issue via `gh issue edit <parent-number> --add-sub-issue <child-number>` (or `--parent <parent-number>`).
      - **Zero-Token Triage Exclusion**: When querying candidate issues, filter out blocked tickets directly using GitHub native search syntax (`gh issue list --search "is:open -is:blocked" --json number,title,labels`) and exclude `status/needs-spec` and `priority: low`.
      - **Automatic Unblocking on Merge**: When a preceding ticket is merged and closed, GitHub automatically resolves the blocker (0 open blockers). Subsequent triage workflows immediately observe the unblocked ticket without manual label-stripping overhead.
  - The Issue body MUST explicitly state the primary affected component/package and database tables (e.g., `Affected Component: internal/home`, `Database Tables: character_letters`).
- **Pre-Registration De-duplication Check:**
  - Before creating any new Issue, agents MUST search existing open issues using targeted domain/entity keywords (`gh issue list --state open --search "<domain-or-keyword>" --json number,title,labels`).
  - If an open issue already tracks the problem, update or refine the existing issue rather than registering a duplicate.
- **Related Tickets Synchronization (Body-First Update Rule):**
  - When a PR modifies shared models, storage contracts, bugs, or premises affecting other open issues, agents MUST update affected open issue bodies directly (`gh issue edit <number> --body ...`). Never rely solely on comments (not shown by `gh issue view` by default).
  - Prepend a standardized `> [!NOTE]` callout at the top of `## Problem` detailing resolved issue #, PR #, changes made, and remaining active scope.
  - If dependent issues were blocked, verify that GitHub native dependency reflects all blockers resolved (automatic upon PR merge/issue close). Prepend the standardized `> [!NOTE]` callout noting that prerequisites are satisfied and the issue is unblocked.
- **Prerequisite & Feasibility Verification:**
  - When creating issues for security, auth, or cross-cutting features, verify whether required underlying infrastructure/models (e.g., Admin role, RBAC, config keys) already exist in the codebase.
  - If prerequisites are missing, explicitly document them in the Issue body along with concrete architectural options (e.g. Option A, Option B) and note that specification alignment is required before implementation.
- **Study Existing Implementations:** Before generating new logic from scratch, actively search the codebase (using `fd`, `grep`, or IDE tools) for existing features that solve similar problems. Adopt the same architectural patterns, variable naming conventions, and file structures.
- **Transparent Tool Usage:** When analyzing codebases, prefer native tools (`view_file`, `grep_search`). Do NOT execute complex or opaque bash scripts (like `sed`, `awk`, or `perl` one-liners) to parse code without explicitly explaining your intent to the user first. Ensure transparency in your actions.
- **Mandatory Auth Wrappers**: All HTTP handlers exposing character actions or mutating gameplay entities must use standard auth wrappers (`withAuthenticatedCharacter`, `withAuthenticatedCharacterAndJSON`). Never decode `req.CharacterID` directly without verifying player ownership. The mechanical AST linter (`internal/api/http/auth_lint_test.go`) enforces this check.
- **Legacy `@actions` Reconciliation**: When migrating or reviewing a feature originating from a legacy CGI script (`/home/witchcraze/dev/party2`), extract the full list of actions/subroutines and verify 1:1 mapping against new Go domain methods & HTTP endpoints. Any omitted or superseded action must have documented rationale.
- **Pre-Commit Active Issue Re-Check (対応漏れ再確認)**:
  - Before committing changes and opening a PR upon task completion, agents and developers MUST re-inspect the active issue (`gh issue view <issue-number>`).
  - Cross-check every item in Acceptance Criteria, Scope, and specific requirements against the implemented code, tests, and documentation to verify zero omissions.
  - Do not proceed to commit changes or open a PR until all acceptance criteria and issue requirements are verified as completely satisfied.
- **Bounded Tasks & Anti-Fat-Issue Rule (1 Issue = 1 PR = 1 Mergeable Unit):**
  - **Single Responsibility Principle:** An issue MUST represent a cohesive, reviewable, and independently testable unit of work that maps directly to a single PR. Never create monolithic "umbrella" issues combining multiple unrelated domain fixes, cross-cutting layers, and sub-features.
  - **4 Mandatory Split Triggers (必須分割判定ルール):** If an issue meets ANY of the following criteria, it is **FAT** and MUST be decomposed into smaller, atomic sub-issues before starting implementation:
    1. **Multi-Package Trigger:** The scope spans 2 or more independent domain/feature packages (e.g., `internal/home` AND `internal/rescue`). Each package must have its own issue.
    2. **Cross-Layer Trigger:** The scope mixes domain/core business logic with cross-cutting HTTP middleware/auth routing or database schema migrations. HTTP-wide guards and schema migrations must be isolated into dedicated tickets.
    3. **Independent Verifiability Trigger:** Sub-features can be tested, reviewed, and merged independently without functional dependencies.
    4. **Blast Radius & Ratchet Trigger:** The scope risks touching ratchet-capped files (e.g., `internal/api/http/handler.go` capped at 1122 lines) or production files near the 500-line ceiling (`file_size_lint_test.go`).
  - **Sizing Check:** When creating or refining an issue, verify that the ticket is strictly restricted to a single primary package or layer. If scope creeps, split immediately.

## 3. TDD and Local Verification (Tiered Strategy)
For non-trivial behavior:
1. **Prepare Environment:** Run `make up` to ensure the local DB and Cache are running. This is required for fast integration testing and MCP tool access.
2. Identify acceptance criteria and write/update tests.
3. Implement the smallest change satisfying the tests.
4. **Inner Loop:** Run fast host tests continuously using `make test` (or `make test-integration`).
5. **Outer Loop:** Auto-format code with `make fmt`, then run unified local verification with `make check` (runs format check, `go vet`, host test suite, and fast smoke build).

### Unit & Integration Test Time Budget Policy vs. Benchmarking
- **No Sub-Second Assertions in Unit/Integration Tests:** Functional tests (`go test`) MUST NOT enforce sub-second wall-clock thresholds (fragile in CI/virtualized environments). Use liberal dead-man bounds (2.0s–5.0s) strictly to catch deadlocks/hanging goroutines. Emit informational timing via `t.Logf`.
- **Dedicated Benchmarks for Performance:** Verify throughput and latency regressions via Go benchmarks (`testing.B`) using `scripts/benchmark.sh` / `make bench`. SSOT: [`docs/development/benchmarking.md`](../../docs/development/benchmarking.md).

## 4. Definition of Done
A ticket is complete only when applicable:
- Acceptance criteria are satisfied.
- Active issue is re-checked prior to commit and all requirements/acceptance criteria are verified with zero omissions.
- Behavior is covered by tests.
- For features touching P2P or shared-resource state mutations (Bank, Auction, Flea Market, Delivery, Guild, Boss), a paired concurrency stress test using the standardized test harness (`testutil.RunConcurrentStressTest` or `testutil.RunRace`) is implemented and passes with zero deadlocks and conserved assets.
- Unified local checks (`make check`) pass completely.
- Architecture remains valid and no unrelated changes were introduced.
- Dead Code and Orphaned Definitions Elimination: When replacing, refactoring, or superseding domain methods, repository interfaces, constants, or DTOs, all unused precursors MUST be eliminated. Leaving obsolete or zero-caller methods, unused top-level constants, or dead DTO fields behind is prohibited. Mechanically enforced by Go AST linters (`internal/architecture/deadcode_lint_test.go`, `internal/architecture/unused_definitions_lint_test.go`). Intentional schema/OpenAPI compatibility fields and enum sets must be annotated with `//lint:ignore <reason>`.
- Silent Error Suppression Prohibition: Database repository, persistent store, container mutation, and currency mutation calls must never discard errors via blank identifiers (`_ =`, `_, _ =`, `val, _ :=`) or unassigned expression statements. All errors must be propagated to the caller or handled within transactional rollback boundaries. If a call is strictly best-effort or compensatory (e.g., defer rollback, post-commit cache eviction), it must be annotated with `//lint:ignore error-swallow <reason>`. Mechanically enforced by Go AST linter (`internal/architecture/error_swallow_lint_test.go`; see [`docs/development/ast-linters.md`](../../docs/development/ast-linters.md)).
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
