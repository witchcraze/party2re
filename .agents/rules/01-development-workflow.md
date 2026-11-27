---
name: Development Workflow and Operating Rules
description: Rules for branches, Issues and PRs, related tickets, scope, verification, tests and Definition of Done.
---

# Development Workflow

## 1. Branches
- `main` is the only integration branch. Feature branches are short-lived and named `<type>/<issue-number>-<short-desc>` (`feat/`, `fix/`, `chore/`, `docs/`, `refactor/`).
- Before branching: keep the working tree clean (stash or commit; never a blind `reset --hard`), then `git checkout main && git pull --ff-only origin main`.
- Always squash-merge; PR titles follow Conventional Commits.
- After merge:
  1. `git checkout main && git pull --ff-only origin main`;
  2. delete the local branch;
  3. delete the merged remote head branch unless retention was requested (`gh pr merge --delete-branch` or `git push origin --delete <branch>`), and confirm `git ls-remote --heads origin <branch>` returns nothing; pruning alone does not delete it;
  4. `git fetch origin --prune`.

## 2. Issue and PR Workflow

### Issues
- Substantial work MUST have an Issue. Issues and PRs MUST use the repository templates in `.github/`, with every checkbox honestly verified.
- Titles: `[<Type>] <Domain>: <Specific Action / Target>`; vague titles are banned.
- Bodies describe the work independently of tools or executors, and MUST name the primary affected component and database tables.
- Exactly one size label, revisable as investigation clarifies (explain material changes):
  - `size/small`: localized work on established contracts and patterns;
  - `size/medium`: investigation, implementation and verification across multiple cases with a clear target;
  - `size/large`: substantial design uncertainty, broad impact or complex migration/transaction coordination.
  Line counts, layers, DDL and transactions are signals, not ceilings.
- `status/needs-spec` marks unresolved specifications or designs; implementation waits for them. Ordering dependencies use native links (`gh issue edit <n> --add-blocked-by <m>`; parents via `--add-sub-issue`/`--parent`), never a `status/blocked` label. Native links are the only record of relationships and their state; bodies explain why a prerequisite is needed and what to adopt and verify, without relationship lists or progress.
- Triage candidates: `gh issue list --search "is:open -is:blocked"`, excluding `status/needs-spec` and `priority: low`.
- Before creating an Issue, search open Issues by domain/entity keywords; refine an existing one instead of duplicating.
- For security or cross-cutting work, verify that prerequisite infrastructure exists; if not, document it with concrete options and mark that specification alignment is required.

### Related tickets
- After a change, assess open Issues that depend on it or reference the changed contract; widen discovery only for a concrete shared-model, storage, behavior or prerequisite change. Do not scan all Issues.
- MUST correct an affected open Issue body (`gh issue edit --body-file`) when it would otherwise mislead: changed contracts, obsolete targets or prerequisites, duplicated completed work or changed remaining scope. Comments alone are not corrections. Completion, new dependencies or reusable code alone need no edit.
- Edit only affected sections but reconcile contradictions elsewhere in the body; preserve evidence and still-valid requirements. Use `> [!NOTE]` only for an implementation caution; never completion notes or status lists.
- Call an Issue unblocked only after checking its native blockers. Resolving a blocker MUST NOT clear `status/needs-spec`.

### Working practice
- Reuse existing implementations and patterns before writing new logic.
- Prefer dedicated file reading and search tools when the agent provides them. Before running a non-trivial shell command that parses or rewrites code (such as `sed`, `awk` or `perl` one-liners), state its intent in visible output; MUST NOT hide what a command does.
- Apply the [auth wrapper rules](06-security.md#3-mandatory-auth-helper-and-common-wrapper-anti-bypass-rule) and [legacy dispatch reconciliation](00-migration-constraints.md#4-legacy-cgi-actions-array--subroutine-reconciliation) to affected work.
- Before committing and opening a PR, re-read the active Issue and every other Issue the PR closes, and verify every acceptance criterion and requirement against code, tests and documents.

### Scope (1 Issue = 1 PR = 1 Mergeable Unit)
- **Cohesive Objective:** an Issue is one reviewable, independently verifiable objective. Unrelated objectives MUST NOT be bundled merely for convenience.
- **Decomposition:** consider splitting for independent objectives, broad impact, independently useful sub-features or separable prerequisites; reassess when investigation finds more work.
- **Coupled Changes:** cross-layer, schema and refactoring changes MAY stay together when they serve the same acceptance criteria; record the reason in the Issue or PR when decomposition is a concern.
- **Related and Discovered Issues:** a PR MAY also resolve open Issues on the same code targets, legacy routines or contracts, and defects or missing implementation found during the work, when their specification is resolved and combining them avoids rework, conflicting edits or duplicate verification. Register each discovered defect as an Issue first (de-duplication applies). The PR MUST close each with its own `Closes #<number>` and state why it was combined. MUST NOT combine `status/needs-spec` or `priority: low` Issues, or work that would make the change `size/large`.
- **Capped Files:** a file near a size ceiling or ratchet does not by itself force a split, but ceilings and boundaries MUST still hold.

## 3. TDD and Verification

Commands, budgets and examples: [`docs/development/testing.md`](../../docs/development/testing.md).

- **By change type:**
  - pure/unit logic: focused and affected-package tests on host Go; no services needed;
  - DB/Valkey: services up (`make up`), migrations applied (`make db-migrate`), connection variables set (`PARTY2_DB_DSN`, `PARTY2_TEST_DB_ADMIN_DSN`, `PARTY2_VALKEY_ADDR`), and the isolation rules below;
  - documentation only: check changed claims, links and examples against sources; no services or runtime suites.
- **Inner loop:** write or update tests with the code, implement the smallest passing change, run `go test -count=1 ./<package> -run '<test>'`, then the affected packages. MUST NOT run the full suite on every edit.
- **Before PR:** `make fmt`. The pre-push hook checks formatting, `go vet` and OpenAPI sync and runs the affected-package tests (`make check-changed`). CI's full suite on the PR head is the merge gate; MUST NOT present local checks as CI results. `make check` (full local run) is optional, for example after migration or shared test-infrastructure changes; run it with a command limit above its 30-minute test deadline or in the background, and never treat an agent timeout as a result.

### Test Database Ownership and Isolation
- DB tests use the environment's shared test DB unless the criteria below require a dedicated one.
- Environment-wide reset is allowed only before tests start or after all users stop; never while tests or the application use it.
- Tests on the shared DB MUST delete or reset only data they created, including dependent rows; no table-wide deletes, broad fixture predicates or snapshot restores. Serialization is not ownership.
- Temporary tables and single-transaction rollback MAY isolate assertions; rollback is not isolation for independent commits, implicit-commit DDL or other connections.
- When shared data or concurrency prevents accurate verification, use a dedicated DB, touching only that DB; MUST NOT silently fall back to the shared DB.
- CI's disposable environment does not waive these rules. Procedure: [`testing.md` §5.1](../../docs/development/testing.md#51-database-ownership-and-isolation).

### Whole-Action Admission Regression Coverage
Changes to whole-action admission MUST cover the affected representative cases:
- **Rejection and permission:** unregistered reachable descendants and paths unsupported in the known state reject before execution; a later slot unsupported in the actual state rejects at execution with battle state and callback context restored. Supported nonlethal and permitted no-effect paths are protected from rejection for context used only by unreachable side effects. Rejected, accepted-no-effect and applied outcomes are distinguished.
- **Pure validation:** admission leaves caller state, context, counters, RNG and nested shared references unchanged on accept and reject, observed independently of shallow snapshots. It MUST NOT call external `Apply` hooks or persistence; external transitions use the owner's validation/projection on isolated state.
- **Ordered composition:** when earlier effects change later resources, stats, consumption, life state, targets or eligibility, test the sequence at the enclosing operation boundary. Mutually exclusive alternatives are not accumulated. Execution-time rejection or external failure commits no partial effects; assert the restored state.

No exhaustive combination matrix or tests of unrelated behavior.

### Battle Test Selection
Apply the [selection procedure](../../docs/development/testing.md#53-battle-test-selection):
- Identify equivalence classes, boundaries and shared-state interactions; test at the responsible layers. Pairwise sampling MUST NOT replace ordered cases for death/revival, reflection/cover, one-use defenses, equipment reset or external state changes affecting continuation. The full job × skill × status product is not required.
- Shared helpers or synthetic catalogs MUST NOT certify untested entries; changed entries are checked for binding, parameters/cost, discovery and execution through their invocation paths. Unsupported descendants stay whole-action gates.
- Positive fixtures satisfy state and context contracts; missing facts are explicit negative cases. MUST NOT narrow production targets or random pools to pass tests.
- Expected values come from independently verified vectors or invariants; projection-vs-execution comparison is supplementary only. Changed persisted facts are checked across save/reload.
- Generated failures keep replayable seeds and a minimized sequence. Budget generated cases without capping production behavior. No new generic runner, dependency or CI tier without a demonstrated gap.

### Unit & Integration Test Time Budget Policy vs. Benchmarking
- Functional tests MUST NOT assert sub-second latency. Use 2–5s dead-man bounds only for deadlock/hang detection; DB integration tests allow about 60s under package concurrency and `-race`, and cancellation releases barriers and joins workers. Avoid serial multi-second waits with safe `t.Parallel()` or injected timeouts without weakening deadlock headroom or real TTL checks. Report timing with `t.Logf` ([budgets](../../docs/development/testing.md#54-execution-budgets-and-stop-detection)).
- Performance is verified with `testing.B` benchmarks via `make bench` ([benchmarking](../../docs/development/testing.md#7-continuous-performance-verification--benchmarking)).

## 4. Definition of Done
- Acceptance criteria of every closed Issue are met and re-checked (§2).
- Behavior is covered by tests; shared-resource or P2P mutations have a paired stress test (`05-database-and-caching.md` §6).
- CI passes on the PR head.
- Architecture remains valid; no unrelated changes beyond Issues the PR closes.
- Superseded methods, interfaces, constants and DTO fields are removed (`deadcode_lint_test.go`, `unused_definitions_lint_test.go`; intentional compatibility fields use `//lint:ignore <reason>`).
- Repository, store, container and currency errors are never discarded with `_`; best-effort calls are annotated `//lint:ignore error-swallow <reason>` (`error_swallow_lint_test.go`).
- Owning documents are current under `02-documentation-sync.md`; PR template requirements are met.

## 5. Token Budget
Optimize for tested, reviewable, mergeable work, not generated volume. When implementation reveals a significant architectural question, stop and open a decision Issue instead of redesigning speculatively.

## 6. Dependency and License Gate
Before adding a dependency, state why it is needed, confirm the standard library and existing dependencies are insufficient, and check its license (including transitive) against the project's licensing strategy.
