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
      - **Zero-Token Triage Exclusion**: When querying candidate issues, filter out blocked tickets directly using GitHub native search syntax (`gh issue list --search "is:open -is:blocked" --json number,title,labels`) and exclude `status/needs-spec` and `priority: low`.
      - **Automatic Unblocking on Merge**: When a preceding ticket is merged and closed, GitHub automatically resolves the blocker (0 open blockers). Subsequent triage workflows immediately observe the unblocked ticket without manual label-stripping overhead.
  - The Issue body MUST explicitly state the primary affected component/package and database tables (e.g., `Affected Component: internal/home`, `Database Tables: character_letters`).
- **Pre-Registration De-duplication Check:**
  - Before creating any new Issue, agents MUST search existing open issues using targeted domain/entity keywords (`gh issue list --state open --search "<domain-or-keyword>" --json number,title,labels`).
  - If an open issue already tracks the problem, update or refine the existing issue rather than registering a duplicate.
- **Related Tickets Synchronization (Body-First Update Rule):**
  - When a PR modifies shared models, storage contracts, bugs, or premises affecting other open issues, agents MUST update affected open issue bodies directly (`gh issue edit <number> --body ...`). Never rely solely on comments (not shown by `gh issue view` by default).
  - MUST update changed premises directly in their owning sections with relevant Issue/PR references: prerequisite completion in Relationships & Dependencies / Prerequisites & Feasibility, reusable implementations in Scope / Code targets, and behavior or verification changes in Acceptance criteria / Tests. Keep remaining scope accurate, remove or revise superseded state claims throughout the body, and preserve evidence links and still-valid, independently usable requirements, specifications and decisions. Reference shared contracts instead of copying their full contents into each issue.
  - Add or update a `> [!NOTE]` callout only when an implementation caution needs emphasis. NEVER add completion-only notes or cumulative implementation-status lists. Replace or consolidate existing callouts only for the changed premises; unrelated valid notes are outside mandatory cleanup.
  - If dependent issues were blocked, MUST verify the remaining open blockers through GitHub native dependencies and update the body to distinguish satisfied prerequisites from unresolved blockers. State that the issue is unblocked only when all native blockers are resolved. Resolving an implementation blocker MUST NOT clear independent `status/needs-spec` labels or unresolved specification decisions.
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
- Owning documentation is updated when the change makes its explanations outdated, under `02-documentation-sync.md`; no per-ticket history or frozen-inventory append is required.
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
