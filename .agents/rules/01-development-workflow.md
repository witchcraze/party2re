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
- **Searchable Issue Naming and Complexity Labeling Rules (SSOT):**
  - **Issue Titles:** MUST strictly follow `[<Type>] <Domain>: <Specific Action / Target>` (e.g., `[Bug] Home: Return 404 in GET /homes/{id}/companion/phrases`, `[Feature] HTTP: Guard action endpoints against active sleep penalty`). Prohibit vague titles like `Fix bug` or `Update system`.
  - **Size Estimates (Creation, Refinement & Triage):**
    Issue bodies MUST describe work independently of implementation tools or providers; do not add execution-strategy or executor-recommendation sections. Size labels MUST describe work complexity only.
    Every issue MUST have exactly one size label upon creation. Use the following as initial estimates of the expected work:
    1. **`size/small`**: Localized work that can be completed using established contracts and patterns.
    2. **`size/medium`**: Work requiring investigation, implementation and verification across multiple cases, with a clear responsibility and completion target.
    3. **`size/large`**: Work with substantial design uncertainty, broad impact, or complex migration/transaction coordination.
    Choose the estimate that best fits the work as a whole; these descriptions are judgment aids, not automatic triggers. Reviewers and implementers MAY revise the label as investigation clarifies complexity; maintain exactly one size label and briefly explain material changes in the Issue or PR.
    Use line counts, package/layer count, DDL, transactions and capped-file edits as signals of review effort and risk; none alone imposes an issue-diff ceiling, a size label or decomposition. Account for the nature of tests, documentation, generated artifacts and code moves when assessing review effort.
  - **Issue Status & Dependency Management Rules (明確な使い分けルール):**
    - **`status/needs-spec` (Label)**: Use when legacy specifications, formulas, or architectural designs are incomplete, undecided, or pending discussion/RFC. Implementation cannot proceed until specs are clarified.
    - **Native Issue Dependencies (`blocked by` / `blocking`)**: Use GitHub native dependencies instead of manual labels when the specification is clear, but another ticket/PR must be executed first (e.g., foundational infrastructure/models, security/auth guards on overlapping handlers, sequential PRs, or prerequisite domain logic).
      - **Native Registration**: When an issue depends on another issue, link it natively via `gh issue edit <issue-number> --add-blocked-by <predecessor-number>`. Do NOT apply a manual `status/blocked` label (`status/blocked` is deprecated).
      - **Sub-issues / Parent Hierarchy**: When decomposing features or epics under the bounded-task guidance below, link child issues to the tracking issue via `gh issue edit <parent-number> --add-sub-issue <child-number>` (or `--parent <parent-number>`).
      - **Native Relationships and Issue Bodies**: MUST use GitHub native relationships as the sole source of dependency/parent-child relationships and their resolution state. Issue bodies MUST explain why a prerequisite is needed and specify the requirements/contracts to adopt and validation conditions, referencing the prerequisite Issue or owning document. MUST NOT duplicate relationship-only lists, progress/resolution state, or full shared specifications in the body.
      - **Zero-Token Triage Exclusion**: When querying candidate issues, filter out blocked tickets directly using GitHub native search syntax (`gh issue list --search "is:open -is:blocked" --json number,title,labels`) and exclude `status/needs-spec` and `priority: low`.
      - **Automatic Unblocking on Merge**: When a preceding ticket is merged and closed, GitHub automatically resolves the blocker (0 open blockers). Subsequent triage workflows immediately observe the unblocked ticket without manual label-stripping overhead.
  - The Issue body MUST explicitly state the primary affected component/package and database tables (e.g., `Affected Component: internal/home`, `Database Tables: character_letters`).
- **Pre-Registration De-duplication Check:**
  - Before creating any new Issue, agents MUST search existing open issues using targeted domain/entity keywords (`gh issue list --state open --search "<domain-or-keyword>" --json number,title,labels`).
  - If an open issue already tracks the problem, update or refine the existing issue rather than registering a duplicate.
- **Related Tickets Synchronization (Body-First Update Rule):**
  - Agents MUST assess related-ticket impact starting with direct dependent issues and known issue references to the changed contract. Expand targeted discovery when a concrete shared-model, storage, behavior or prerequisite change may affect additional open issues; do not routinely scan all open issues or recursively follow every related link.
  - Agents MUST update affected open issue bodies when unchanged instructions would mislead implementation or verification: changed requirements/contracts, obsolete code targets or prerequisite claims, duplicated completed work, or changed remaining scope. Apply corrections directly in the owning sections with relevant Issue/PR references (`gh issue edit <number> --body-file ...`); never rely solely on comments for required corrections (not shown by `gh issue view` by default).
  - Adding or resolving a dependency, completion, newly reusable implementations or clarification in a linked specification alone MUST NOT require a body edit unless they make the issue's instructions inaccurate or insufficient. Update the owning body sections when dependency reasons, adoption requirements/contracts or validation conditions change.
  - Keep edits scoped to materially affected sections, but MUST reconcile contradictory claims elsewhere in the same body. Preserve evidence links and still-valid, independently usable requirements, specifications and decisions. Do not use synchronization to add unrelated requirements or reopen issue scope without concrete impact.
  - Add or update a `> [!NOTE]` callout only when an implementation caution needs emphasis. NEVER add completion-only notes or cumulative implementation-status lists. Replace or consolidate existing callouts only for the changed premises; unrelated valid notes are outside mandatory cleanup.
  - When dependency relationships change or an issue is described as unblocked, MUST verify remaining open blockers through GitHub native dependencies. Update explicit prerequisite claims in the body when they become misleading; do not maintain a parallel list of satisfied blockers. State that the issue is unblocked only when all native blockers are resolved. Resolving an implementation blocker MUST NOT clear independent `status/needs-spec` labels or unresolved specification decisions.
- **Prerequisite & Feasibility Verification:**
  - When creating issues for security, auth, or cross-cutting features, verify whether required underlying infrastructure/models (e.g., Admin role, RBAC, config keys) already exist in the codebase.
  - If prerequisites are missing, explicitly document them in the Issue body along with concrete architectural options (e.g. Option A, Option B) and note that specification alignment is required before implementation.
- **Study Existing Implementations:** Before generating new logic from scratch, actively search the codebase (using `fd`, `grep`, or IDE tools) for existing features that solve similar problems. Adopt the same architectural patterns, variable naming conventions, and file structures.
- **Transparent Tool Usage:** When analyzing codebases, prefer native tools (`view_file`, `grep_search`). Do NOT execute complex or opaque bash scripts (like `sed`, `awk`, or `perl` one-liners) to parse code without explicitly explaining your intent to the user first. Ensure transparency in your actions.
- **HTTP Authorization / Legacy Reconciliation**: For affected work, MUST apply the [auth wrapper rules](06-security.md#3-mandatory-auth-helper-and-common-wrapper-anti-bypass-rule) and [legacy dispatch reconciliation](00-migration-constraints.md#4-legacy-cgi-actions-array--subroutine-reconciliation); those files own the requirements.
- **Pre-Commit Active Issue Re-Check (対応漏れ再確認)**:
  - Before committing changes and opening a PR upon task completion, agents and developers MUST re-inspect the active issue (`gh issue view <issue-number>`).
  - Cross-check every item in Acceptance Criteria, Scope, and specific requirements against the implemented code, tests, and documentation to verify zero omissions.
  - Do not proceed to commit changes or open a PR until all acceptance criteria and issue requirements are verified as completely satisfied.
- **Bounded Tasks (1 Issue = 1 PR = 1 Mergeable Unit):**
  - **Cohesive Objective:** An issue MUST represent one cohesive, reviewable and independently verifiable objective that maps to a single PR. Unrelated objectives MUST NOT be bundled merely for convenience.
  - **Decomposition Judgment:** When creating, refining or implementing an issue, assess whether splitting would make the work easier to understand, verify and merge. Independent objectives, broad impact, independently useful sub-features and separable prerequisites are reasons to consider decomposition. Reassess the scope when investigation reveals additional work.
  - **Coupled Changes:** Changes across packages/layers, schema changes and refactoring MAY remain together when they support the same acceptance criteria and keeping them together improves understanding, verification or consistency. When retaining work that raises decomposition concerns, briefly record the reason in the Issue or PR. This rationale does not introduce a separate approval gate; apply existing requirements for unresolved specifications and significant architectural decisions.
  - **Capped Files:** Touching a file near a production line ceiling or ratchet MUST NOT by itself force a split. Assess whether the required restructuring is cohesive with the objective or would be easier to review as a prerequisite. Existing architecture boundaries, production-file ceilings and ratchets MUST still be satisfied.

## 3. TDD and Local Verification (Tiered Strategy)
For non-trivial behavior:
1. **Prepare Environment:** Run `make up` to ensure the local DB and Cache are running. This is required for fast integration testing and MCP tool access.
2. Identify acceptance criteria and write/update tests.
3. Implement the smallest change satisfying the tests.
4. **Inner Loop:** Run fast host tests continuously using `make test` (or `make test-integration`).
5. **Outer Loop:** Auto-format code with `make fmt`, then run unified local verification with `make check` (runs format check, `go vet`, host test suite, and fast smoke build).

### Test Database Ownership and Isolation

- DB-backed tests MUST use the environment's shared test DB by default, choosing a dedicated DB when the isolation criteria below require it.
- Environment-wide initialization/disposal MAY reset or destroy test data before tests start or after all users of that environment stop. Explicit local resets and temporary development data MAY be used; MUST NOT reset an environment while tests or application processes are using it.
- Individual tests using the shared DB MUST normally limit deletion/reset to data they created, including dependent rows. MUST NOT commit destructive changes to unrelated data, including table-wide deletes, broad fixture predicates or restoring an old shared-data snapshot. Serialization alone MUST NOT be treated as data ownership.
- Connection-local temporary tables and guaranteed rollback within a single transaction MAY isolate assertions when they leave unrelated data unchanged and do not compromise other tests. MUST NOT treat rollback as isolation for independent commits, implicit-commit DDL or multiple connections outside that transaction.
- When shared data or concurrent tests prevent accurate verification, tests MUST use a dedicated DB. Inside that DB they MAY freely manipulate data; creation/cleanup MUST target only their own database. MUST NOT silently fall back to the shared DB when required isolation is unavailable.
- CI's disposable environment MUST NOT waive ownership/isolation rules for individual tests within a run. Operating procedures and current examples: [`docs/development/testing.md`](../../docs/development/testing.md#51-database-ownership-and-isolation).

### Whole-Action Admission Regression Coverage

Changes to whole-action admission MUST cover representative cases relevant to the affected behavior:

- **Rejection:** Reachable unsupported paths MUST reject before execution, with state, counters and RNG consumption unchanged.
- **Accepted No-Effect:** When the contract permits a no-effect outcome (e.g., caps or zero power), admission MUST NOT require context used only by skipped or unreachable side effects.
- **Ordered Composition:** When earlier side effects can change later target resolution or eligibility, tests MUST include such a sequence at the enclosing operation boundary.

These criteria MUST NOT be interpreted as requiring an exhaustive combination matrix or tests for unrelated behavior.

### Unit & Integration Test Time Budget Policy vs. Benchmarking
- **No Sub-Second Assertions in Unit/Integration Tests:** Functional tests (`go test`) MUST NOT enforce sub-second wall-clock thresholds (fragile in CI/virtualized environments). Use liberal dead-man bounds (2.0s–5.0s) strictly to catch deadlocks/hanging goroutines. Emit informational timing via `t.Logf`.
- **Dedicated Benchmarks for Performance:** Verify throughput and latency regressions via Go benchmarks (`testing.B`) using `scripts/benchmark.sh` / `make bench`. SSOT: [`docs/development/benchmarking.md`](../../docs/development/benchmarking.md).

## 4. Definition of Done
A ticket is complete only when applicable:
- Acceptance criteria are satisfied.
- Active issue re-check above is complete.
- Behavior is covered by tests.
- For features touching P2P or shared-resource state mutations (Bank, Auction, Flea Market, Delivery, Guild, Boss), a paired concurrency stress test using the standardized test harness (`testutil.RunConcurrentStressTest` or `testutil.RunRace`) is implemented and passes with zero deadlocks and conserved assets.
- Unified local checks (`make check`) pass completely.
- Architecture remains valid and no unrelated changes were introduced.
- Dead Code and Orphaned Definitions Elimination: When replacing, refactoring, or superseding domain methods, repository interfaces, constants, or DTOs, all unused precursors MUST be eliminated. Leaving obsolete or zero-caller methods, unused top-level constants, or dead DTO fields behind is prohibited. Mechanically enforced by Go AST linters (`internal/architecture/deadcode_lint_test.go`, `internal/architecture/unused_definitions_lint_test.go`). Intentional schema/OpenAPI compatibility fields and enum sets must be annotated with `//lint:ignore <reason>`.
- Silent Error Suppression Prohibition: Database repository, persistent store, container mutation, and currency mutation calls must never discard errors via blank identifiers (`_ =`, `_, _ =`, `val, _ :=`) or unassigned expression statements. All errors must be propagated to the caller or handled within transactional rollback boundaries. If a call is strictly best-effort or compensatory (e.g., defer rollback, post-commit cache eviction), it must be annotated with `//lint:ignore error-swallow <reason>`. Mechanically enforced by Go AST linter (`internal/architecture/error_swallow_lint_test.go`; see [`docs/development/ast-linters.md`](../../docs/development/ast-linters.md)).
- Owning documentation is current and affected explanations are consolidated under [`02-documentation-sync.md`](02-documentation-sync.md).
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
