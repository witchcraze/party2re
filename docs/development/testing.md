# Testing and Verification Strategy

## 1. Purpose & Priorities

Tests specify and protect game behavior. The most valuable tests are those that remain valid when implementation details change.

1. **Domain invariants and game rules**: Progression calculations, job requirements, battle outcomes, item ownership/equipment rules, currency invariants, scheduled actions.
2. **Observable component behavior**: Public service contracts and state transitions.
3. **Integration flows**: Cross-package transactions and API contracts.
4. **Regression tests**: A reproducible bug must receive a failing regression test before being fixed.

Avoid tests tightly coupled to private data structures; a refactor preserving behavior should require minimal test churn.

---

## 2. Tiered Verification Strategy

Verification is scoped by change type:

- **Pure / Unit changes** (domain logic, battle calculations, algorithms, or pure helpers without database/cache dependencies): do not require starting local backing services or running all packages. Execute focused tests (`go test -count=1 ./<package> -run '<test>'`) or affected-package tests directly on host Go.
- **DB / Valkey changes**: require live services (`make up`) and applied schema migrations (`make db-migrate`). Run affected-package tests with connection settings (`PARTY2_DB_DSN`, `PARTY2_TEST_DB_ADMIN_DSN`, `PARTY2_VALKEY_ADDR`). Tests requiring dedicated database isolation must have an admin account configured (`PARTY2_TEST_DB_ADMIN_DSN`; see [Dedicated Database Setup](#52-dedicated-database-setup)).
- **Documentation-only changes**: inspect changed requirements, local links/anchors, and examples against their sources. Backing services and full runtime test suites are not required solely for prose. Configured pre-push hooks and CI will run their automated checks.

| Stage | Verification |
| --- | --- |
| TDD iteration | `go test -count=1 ./<package> -run '<test>'` |
| Change ready | `go test -count=1 ./<package>` for affected packages |
| Before commit / ready to merge | `make check`; wait for completion |
| Push | Let the pre-push hook reuse the successful check or verify changed content |
| Clean database verification needed | `make check-clean` resets the database and forces full verification |

For DB/Valkey changes, run affected-package tests with live services and connection settings (`PARTY2_DB_DSN`, `PARTY2_TEST_DB_ADMIN_DSN`, `PARTY2_VALKEY_ADDR`). `make test-integration` supplies local connection settings and tests all packages; start services via `make up` and apply migrations via `make db-migrate`. `make test` runs all packages on host without supplying connection settings; DB-backed tests skip automatically when connection variables are absent.

`make check` runs formatting, static analysis, OpenAPI/architecture checks, migrations, the full test suite and a smoke image build. It and the pre-push hook call the same `scripts/verify.sh`, which reuses a successful result for an unchanged working tree. The verified tree hash is captured after code formatting and OpenAPI sync; if working tree files change before verification completes, the run exits non-zero without updating the stamp, ensuring subsequent runs verify the modified contents. Do not start duplicate checks concurrently or repeat a successful check solely for push or session end. If the verification environment changes, use `FORCE_VERIFY=1 make check`.

For documentation-only changes, inspect the changed requirements, local links/anchors
and examples against their sources. Services and the full runtime suite are not
required solely for prose; a bounded existing test or fault-injection experiment
can substantiate a testing-policy claim. Existing push hooks and CI still run
their configured checks.

---

## 3. Canonical Commands

The [Makefile](../../Makefile) owns command definitions:

- `make up`: Start local MariaDB and Valkey containers in the background.
- `make down`: Stop local services.
- `make db-migrate`: Apply pending SQL schema migrations to the local database.
- `make db-reset`: Drop and recreate the local database from migration files.
- `make test`: Run host test suite without connection settings (pure unit tests run; DB/Valkey tests skip).
- `make test-integration`: Run host test suite with default local MariaDB and Valkey connection settings.
- `make test-docker`: Run the test suite inside the containerized Go test runner.
- `make check`: Run the full unified local verification pipeline (`scripts/verify.sh`).
- `make check-clean`: Reset the database and force full verification (`FORCE_VERIFY=1`).

Environment reset/disposal is described in [development-environment.md](development-environment.md#database-initialization-and-migration).

`make test-stress` runs the full Go suite with `PARTY2_STRESS_ENABLED=1`, so
adaptive concurrency tests are included regardless of package or test name.
The existing harness raises its default load from 15 workers × 10 operations
to 50 × 20; ordinary test runs keep the smaller configuration. This command
waits for local MariaDB/Valkey and applies migrations, then supplies the same
local DB, admin DB and cache defaults as `make test-integration`. Connection
environment overrides are preserved; the admin account must permit dedicated
database creation/deletion. There is no shared-database fallback.

For a focused deep run, pass standard Go test arguments directly to the script,
including the package selector (omitting all arguments defaults to `./...`):

```sh
./scripts/stress_test.sh -run 'TestGvGRewardConcurrentRetryIntegration|TestActionPublication_ConcurrentWagers' ./internal/gvg ./internal/casino
```

Setup or test failure returns nonzero, preserving the Go test exit code. The
entrypoint reads Go's JSON events and requires a passing test without observed
child events, in a package that Go has not reported as having no tests to run.
This excludes the parent-only empty runs reported by Go and observed parents
whose children all skip. It does not independently prove that every requested
subtest or regex alternative exists. With `-json`, stdout contains only Go test
JSON Lines; setup, migration and progress messages go to stderr. Normal runs
and `-json=false` retain human-readable stdout. Use
`go test -list '<pattern>' ./...` separately to inspect selection without running
tests. Full deep runs include ordinary tests as well; elapsed time depends on
the selected suite and local services.

Test connections use the application's normal DB pool defaults (25 open and
25 idle connections per pool). The worker count is independent of the connection
count: 50 workers can queue for a 25-connection pool. Tests that need a different
pool size may configure it explicitly; for environment-configured pools, use
`PARTY2_DB_MAX_OPEN_CONNS` and `PARTY2_DB_MAX_IDLE_CONNS`. The stress entrypoint
preserves these settings. Account for all concurrently running packages and
pools against the server's total connection limit when increasing them.

For local resource pressure, `GOFLAGS=-p=2 make check` limits concurrent package
builds and test binaries using Go's standard flag. It preserves each concurrency
test's worker count and assertions without changing temporary-directory settings.
All full-suite test entrypoints (`make test`, `make test-integration`, `make test-docker`,
and `make check`) execute with an explicit default package-wide
deadline of 30 minutes (`-timeout=30m`), replacing Go's implicit 10-minute timeout.
Callers can override the package budget using `TEST_TIMEOUT=<duration>` (e.g.
`make test TEST_TIMEOUT=45m` or `TEST_TIMEOUT=45m make check`) or standard Go flags
(`GOFLAGS="-timeout=<duration>"`). Unrelated Go flags in `GOFLAGS` (such as `GOFLAGS=-p=2`
or `-v`) are preserved and forwarded.

## 4. Container Roles

- **`Dockerfile.dev` & `compose.yaml`**: Development and integration test environment. Spawns MariaDB and Valkey alongside Go test runner with bind-mounted source and BuildKit cache mounts. The default test command executes with the 30-minute default package timeout, honoring explicit `TEST_TIMEOUT` and standard `GOFLAGS="-timeout=..."` overrides with the same precedence as `Makefile` and `scripts/verify.sh`.
- **`Dockerfile`**: Production distribution image. Multi-stage build compiling a statically linked, stripped binary (`CGO_ENABLED=0`) on `golang:1.26.7-trixie` running on distroless `gcr.io/distroless/static-debian13:nonroot` with CA certificates only.

---

## 5. Test Setup and Fixtures

- **Avoid Premature Test Frameworks**: Do not proactively build complex test frameworks; introduce helpers only when duplication becomes a concrete burden.
- **Explicit Setup over Magic**: Test helpers must not obscure what is being tested. Explicit setup is preferred over implicit abstraction.
- **Reproducible Outcomes**: For exact outcome assertions, inject controlled RNG and clock inputs. Randomized tests should report their seed for replay.
- **Shared Factories**: Use centralized test factories (`CreateTestPlayer`, `CreateTestCharacterWithFunds`, `CreateTestGuildWithLeader`) for integration tests to ensure unique IDs and valid name lengths.

### 5.1 Database Ownership and Isolation

The [workflow rule](../../.agents/rules/01-development-workflow.md#test-database-ownership-and-isolation) defines mandatory constraints. Ordinary DB-backed tests share the database selected by `PARTY2_DB_DSN` within one environment. Unit tests without a database dependency do not need a DB. Shared fixtures can coexist with temporary local development data; neither an empty database nor cleanup of every fixture after every test is a default requirement.

Choose the smallest isolation boundary that preserves the behavior being verified:

| Verification need | Approach |
| --- | --- |
| Assertions can identify the test's own records | Shared DB; use unique fixture identifiers and scope assertions/optional cleanup to those records and their dependents |
| SQL/migration assertions fit entirely within a connection-local temporary table or a guaranteed rollback transaction | Keep that existing isolation when it cannot change unrelated data or compromise concurrent tests |
| Global counts, capacity limits, resets or lifecycle operations cannot be checked reliably alongside unrelated data | Dedicated DB; freely create, update, delete or reset data inside it |
| Real commits or independent worker transactions are part of the behavior | Retain those boundaries; use a dedicated DB if interference would otherwise prevent accurate verification |

Cleanup such as `DELETE ... WHERE character_id = ?` is appropriate when the character and affected dependent rows belong to that fixture. A common town ID, a test-like name prefix, absence of `t.Parallel()` or a cooperating test mutex does not establish ownership of all matching data. Go packages can run concurrently even when individual tests are sequential. Deleting shared rows and later restoring them can erase another test's writes.

The existing repository reset assertion in [`ranking_extra_repository_test.go`](../../internal/database/ranking_extra_repository_test.go) keeps its mutation and reads in one transaction and rolls back before checking the fixture again outside it. This does not verify committed cross-connection behavior. If rollback cannot contain every write, or locking interferes with other tests, use a dedicated DB. Connection-local temporary tables in [`job_points_migration_test.go`](../../internal/database/job_points_migration_test.go) and [`replay_migration_test.go`](../../internal/database/replay_migration_test.go) isolate migration fixtures without clearing persistent tables; their connection boundary must be preserved.

Environment-wide initialization/disposal is separate from per-test setup/cleanup. A fresh CI job may initialize and discard its whole environment; a developer may explicitly reset a local environment between uses. These permissions do not authorize an individual shared-DB test to clear unrelated rows. See [local reset procedures](development-environment.md#database-initialization-and-migration) and [CI environment lifetime](#8-continuous-integration-and-delivery).

Future development with multiple concurrent sessions should isolate each environment's DB, cache and runtime resources. This policy does not add session provisioning or change the current Compose configuration.

### 5.2 Dedicated Database Setup

Dedicated fixtures create uniquely named databases, copy the required schema
including foreign keys, and drop only their own databases. Preserve real
worker/commit boundaries when those are the subject of the test.
`PARTY2_TEST_DB_ADMIN_DSN` must permit database creation/deletion; Compose,
`make test-integration` and `make check` provide it. For direct host tests use
`root:root@tcp(127.0.0.1:3306)/?parseTime=true`. Missing admin configuration fails
DB-enabled dedicated tests rather than falling back to the shared database.

### 5.3 Battle Test Selection

The [workflow rules](../../.agents/rules/01-development-workflow.md#whole-action-admission-regression-coverage)
own admission requirements; [Battle Test Selection](../../.agents/rules/01-development-workflow.md#battle-test-selection)
owns mandatory selection criteria. This section explains how to apply them. It
does not change game rules, supported entries or the boundary between execution,
admission and state projection. [Battle design](../design/battle.md) owns gameplay
contracts; verify expected behavior against the original/approved specification
under the [migration rules](../../.agents/rules/00-migration-constraints.md).

#### Select by changed behavior

Start from the changed entry and its callers. In the PR, identify the relevant
classes and interactions, the layer checking each, and important exclusions with
their reason. Use the following axes as prompts, not a product to enumerate:

| Axes | Split when behavior differs |
| --- | --- |
| Effect and entry | Damage vs HP/MP recovery/drain, stats, status, revival; concrete job/skill/tool/gem identity, binding and direct/custom/random/delegated invocation |
| State and equipment | Normal, temporary and extended states; raw gear/base/current stats, reapplication, one-use defenses and consumed item instances |
| Roles and targets | Actor/requester/target identity; player/NPC, self/ally/enemy; single/area, roster size/order, living/fallen, fallback and mutually exclusive candidates |
| Environment and resources | Field at load vs current field, suppression and external hooks; HP/MP/cost/cap/rounding/probability boundaries, internal resource writes vs enclosing charge and role-specific clamps |
| Time and ownership | Earlier effects and counters, reset/revival/consumption order, external state transitions, frozen command input and save/reload continuation |

Two inputs are equivalent only if they take the same relevant guards, formula,
target resolution and tails with the same context needs. Pick a representative
inside each class and the values immediately below/at/above its boundaries.
Separate zero power, no-effect at a cap, nonlethal and potentially lethal cases:
equal final HP can hide different counters, target tuples or consumed defenses.
Do not assume healing is always monotonic or that HP above MaxHP is malformed;
the owning contract decides which reachable states belong in a fixture.

#### Responsibilities and connections

| Layer | Evidence it provides | Evidence it cannot replace |
| --- | --- | --- |
| Formula/boundary | Hand-derived specification vectors for truncation, caps, thresholds and RNG endpoints | Target resolution, ordered side effects or correct registration |
| Effect helper | Resolved target/returned facts, counters, skipped tails, local state and RNG contract | Complete producer behavior and later operations |
| Enclosing operation | Admission plus execution of ordered single/area/repeated/custom effects; alternative branches and continuation | Concrete entry discovery/binding and caller hydration |
| Concrete entry/registry | Correct identity, arguments, SP/MP/CMP metadata, discovery and execution through affected direct/custom/random/delegated routes | Completeness of other entries sharing the helper |
| Adapter/consumer | Raw gear, job/history/requester facts and owner hooks survive hydration/save/reload; external error/rollback behavior | HTTP command availability, authorization and durable end-to-end lifecycle |

Prove arithmetic once at its owner, then assert caller-specific wiring and
observable results at connections. Repeating every formula vector through every
adapter adds cost without checking a new contract. Conversely, a helper test and
a separately passing registry enumeration do not prove that the registered entry
invokes it correctly. Synthetic catalogs certify only their exercised subset.
The HTTP/context journey and reusable E2E harness are owned by
[#650](https://github.com/witchcraze/party2re/issues/650); battle boundary diagnosis
does not wait for that harness or use it as the numerical oracle.

#### Choose a method

| Change / question | Smallest useful method and required connection |
| --- | --- |
| Formula, cap, probability or resource guard | Deterministic table of class representatives and adjacent boundary values; run through the changed producer for its parameters/tails |
| Many axes with no known shared-state dependency | A small explicit pairwise table, or t-wise table for an identified interaction of t axes; document axes, valid constraints and tuples covered |
| An earlier effect changes later eligibility or target | Deterministic multi-step scenario at the enclosing boundary, with a nearby accepted/no-effect and rejected case where applicable |
| Action and tail have different eligibility predicates | Table the enclosing player/NPC guards separately; exercise skipped action with eligible tail, and excluded tail, through the actual operation |
| Finite projection or large valid numeric range | Bounded property/state-transition test comparing admitted executions with projection plus independent specification vectors |
| Input encoding or a reproducible generated sequence exposes gaps | Go standard fuzz with valid seed corpus, bounded decoding/sequence length and a specific invariant; keep intentional invalid-context cases separate |
| Two executions have a specification-backed relationship | Metamorphic test, e.g. uninterrupted vs save/reload continuation with rebuilt context and the same remaining draw tape |

Pairwise/t-wise tables are a sampling aid, not an independence proof. Gear,
field and category can be sampled this way only after identifying interacting
guards; cover the known threshold combinations explicitly. Ordered death/revival,
reflection/cover, one-hit defense consumption, reset/reapplication and external
HP changes need their actual multi-step sequence, including three or more effects
when an intermediate transition enables the last one. Small finite domains can
be enumerated; no requirement follows to enumerate the full combat catalog.

For property tests, use assertions such as: admitted executions never encounter
`ErrUnsupportedEffect` after a prefix; actual supported outcomes lie within the
projected bounds; validation repeated twice leaves all caller inputs untouched.
These check relationships, not independent correctness of a damage formula.
Keep hand-derived source vectors alongside them. Do not recompute the production
predictor as the expected answer. An admitted external transaction may still fail
under its documented error contract; test that error and rollback at the owner,
separately from admission missing an unsupported descendant.

For metamorphic tests, verify the relationship's preconditions. Roster permutation
is not generally invariant when turn order, fallback or area iteration matters;
adding a target can change selection probabilities. Save/reload equivalence needs
the same command facts and remaining randomness, not a newly seeded replacement
stream or serialized function pointers.

#### Apply the table across domains

These examples illustrate test selection, not additional gameplay specifications:

| Domain / change | Selected checks and existing pattern |
| --- | --- |
| Damage or HP/MP recovery boundary | Independent numeric vectors, full/zero/cap and living/fallen classes; ordered area healing where the first recipient consumes actor tension. See [recovery tests](../../internal/core/battle/recovery_test.go) and [damage composition](../../internal/core/battle/damage_composition_test.go). A recovery helper does not certify MP producers. |
| Status, defeat and revival | Nonlethal acceptance beside lethal rejection; two individually survivable hits that become lethal together; one-use defense then a second hit; revival reset before later target/defense resolution. See [ordered defeat tests](../../internal/core/battle/defeat_ordered_test.go). |
| Job/skill invocation | Each changed concrete binding/cost and its discovery/execution path, unsupported reachable child before any cost/draw, then a prior effect changing the job/table used by the next slot. See [generated invocation tests](../../internal/core/battle/invocation_generated_test.go), [status registry tests](../../internal/core/skill/status_test.go) and [adapter invocation](../../internal/battle/invocation_test.go). Synthetic job tables are not full catalog certification. |
| New resource/custom producers | Derive cases from their owning specifications: internal MP/CMP writes before enclosing charge, signed costs and role-specific clamps where specified; distinguish requested/returned amount from actual delta and assert unaffected HP. Follow earlier slots that change state, roster or caster survival into later slot admission. |
| Equipment tails and persisted consumer | Test the actual player/NPC eligibility boundary and snapshots taken before the action, not only the tail helper. Then verify real-store failure/retry/receipt and reload at the consumer; a Core JSON round trip cannot establish committed retry behavior. |

Apply the same table to planned entries before their bodies exist: derive cases
from the owning specification, label the missing connection, and retain its
adoption gate until real bindings and consumer tests exist. Future work is not
certified by an existing analogous helper or this selection policy.

The healing-order example is grounded in reference revision
`ff97fc549932a146cc945d7edf71b44c868a840e`, `_skill.cgi:9056–9064,9067–9078,9103–9108`:
party iteration calls the helper, whose successful healing consumes actor tension.
Defeat examples use that revision's `_battle.cgi:28–142,1390–1428,1726–1759`
and `_skill.cgi:10588–10626`: reset precedes the final attack and only lethal
damage calls defeat. For a job change before current-job delegation, see
`_skill.cgi:6593–6610` and `_data.cgi:1260–1264` at the same revision.
Entry-specific behavior remains owned by
[Battle's invocation contract](../design/battle.md#random-and-delegated-job-closure), not inferred from synthetic
fixtures. Detailed audits and experiment logs belong in Issues/PRs, not a growing
inventory of test names here.

#### Fixtures, oracles and diagnosis

Build positive fixtures from explicit valid facts: unique IDs and consistent
roster/team membership, raw equipment and definitions, base/current resources,
requester facts and the real owner context required by reachable paths. Include
supported missing-context/no-effect classes deliberately. Label a negative case
with the precise missing fact or unsupported branch; otherwise an earlier fixture
error can make a purported admission regression pass for the wrong reason.
Keep legitimate production candidate/RNG pools intact, including unsupported
descendants that should cause rejection. Limit a generated test's inputs, not
the production resolver. Constrain only what the contract actually excludes.

Use independent snapshots of the mutable graph, not `before := context` when its
pointers/slices/maps alias live data. Observe affected shared referents as well as
the root, counters, arguments and external hook call counts. A failing-on-draw RNG
is useful for admission; an explicit draw tape with exhaustion checks protects
contractual draw order/count during execution. JSON snapshots omit runtime hooks
and unexported facts, so supplement them with explicit assertions. Owner projection
runs on isolated validation state; an external `Apply` spy should remain unused.

Keep exact draw order assertions at source-defined RNG/command boundaries.
Broader properties can use deterministic seeds or tapes without freezing every
incidental draw; report seed, entry/route, initial state/context facts, operation
sequence and observed error/state. Minimize a failure by deleting operations,
actors and gear and shrinking numbers while preserving validity and the failure.
For a fuzz failure, retain the minimized `testdata/fuzz/<FuzzName>/...` input and
its replay command; promote a readable regression when useful. A seed alone is
insufficient if catalog order or generator logic changes.

#### Execution and cost

Run the smallest named regression during iteration, then affected packages. The
normal behavior-change gate remains one completed `make check` on final artifacts
(section 2); avoid parallel or repeated full checks. Wider seed/sequence sweeps
are opt-in investigation when a specific invariant or risk warrants them, with a
recorded seed set, maximum generated actors/operations and run budget. Deterministic
cases and fuzz seed corpora execute under ordinary `go test`/CI; active fuzzing is
a separate bounded run, not implied by `make check`.

```sh
go test -count=1 ./internal/core/battle -run '^TestAreaDeathFinalStrikeAccumulatesDamage$'
go test -count=1 ./internal/core/battle ./internal/core/skill
```

When a fuzz target exists, use these templates for a bounded run and a stored
failure replay. For seeded table/property cases use the failing subtest and its
recorded input.

```sh
go test ./<package> -run '^$' -fuzz '^<FuzzName>$' -fuzztime=30s
go test ./<package> -run '^<FuzzName>/<corpus-hash>$'
```

Assess both generated test count and admission's explored states/allocations:
one generated case can branch over many target/reset/catalog continuations.
Measure small, typical and largest supported rosters/catalogs with realistic
branching and depth, including admitted and rejected paths. Record those sizes
and branch counts where observable; a tiny synthetic catalog gives no evidence
about production scale. Use `testing.B` and the existing [benchmark procedure](#7-continuous-performance-verification--benchmarking)
for latency/allocations, never sub-second functional assertions. The existing
engine simulation benchmarks do not measure every admission path.

Explicit tables, controlled RNG, bounded loops and JSON continuation tests already
provide the necessary seams. No universal fixture DSL, second battle engine, new
dependency, wholesale test migration or CI tier is needed to apply this policy.
Add a local property/fuzz/benchmark only when its changed behavior demonstrates a
gap; separate shared infrastructure work into a bounded Issue with the measured
cost/duplication, missing property, target boundary and verification criterion.
Sampling leaves untested combinations: report protected contracts and residual
gaps, not test counts/coverage percentages as proof of complete battle support.


### 5.4 Execution Budgets and Stop Detection

Repository full-suite test entrypoints (`make check`, `make test`, `make test-integration`,
`make test-docker`, CI) enforce an explicit default package-wide deadline of 30
minutes (`-timeout=30m`), replacing Go's implicit 10-minute timeout. This aggregate
budget is a package-level dead-man limit for complete package execution across heavy
setup fixtures and numerous subtests (such as candidate generation in Guild tests);
it does not relax individual test timeouts. Callers can override the package budget
via `TEST_TIMEOUT=<duration>` or `GOFLAGS="-timeout=<duration>"`. Unrelated `GOFLAGS`
(such as `GOFLAGS=-p=2`) remain effective.

A DB integration test's context covers fixture preparation and business operations;
normally allow 60 seconds so package concurrency and `-race` have headroom.
Character/guild creation consumes that budget even when a fixture helper uses its
own context. This budget is a cancellation safety net, not a latency assertion.

Keep separate bounds for intentional lock waits and isolated stuck-worker checks;
the workflow rule's 2–5 second dead-man guidance applies to those checks, not the
whole DB scenario. Do not relax SQL lock-wait settings or suppress deadline,
deadlock or cancellation errors. Wait for barriers/state observations rather than
fixed sleeps, make channel waits cancellation-aware, and join all workers before
assertions and fixture cleanup. Existing concurrency helpers join workers; their
callbacks must honor cancellation themselves.

## 6. Mechanical AST Static Analysis Linters

In-tree Go tests under [internal/architecture](../../internal/architecture/),
[internal/database](../../internal/database/), Core and HTTP enforce architectural
boundaries, transaction/lock usage, authentication wrappers, mutation encapsulation,
error propagation and unused-definition checks. `make arch-lint`, `make lock-lint`
and `make check` run the relevant suites. Their source owns the exact scope and
exceptions; passing AST checks do not certify runtime locks or legacy parity.

Legitimate best-effort/compensatory persistence calls use
`//lint:ignore error-swallow <reason>` immediately before or on the statement.
New checks use standard Go AST packages, synthetic positive/negative cases, and
actionable file/line diagnostics. Avoid duplicating the checks' implementation in prose.

## 7. Continuous Performance Verification & Benchmarking

Functional test timeout constraints are owned by the [workflow rule](../../.agents/rules/01-development-workflow.md#unit--integration-test-time-budget-policy-vs-benchmarking).
Measure performance separately with `testing.B`: `make bench` runs
[scripts/benchmark.sh](../../scripts/benchmark.sh), writes `.benchmarks/current.txt`
and compares an existing baseline. `./scripts/benchmark.sh --record` records
`.benchmarks/baseline.txt`; these local results are ignored by Git.
`go run ./scripts/benchcompare .benchmarks/baseline.txt .benchmarks/current.txt`
compares latency/allocations; the default +30% regression threshold exits with code 2.

## 8. Continuous Integration and Delivery

The [CI workflow](../../.github/workflows/ci.yml) runs formatting, OpenAPI sync
validation, vet and tests with atomic coverage in the Compose development
container using the default 30-minute package timeout while honoring `TEST_TIMEOUT`
and `GOFLAGS="-timeout=..."` overrides, then builds the production image. It does not
currently run the race detector or exercise the built production image. MariaDB initializes from migrations
on first volume creation; the GitHub-hosted runner is disposable per job. Tests
inside a job still follow the database ownership/isolation rules above.

CI runs on pushes to main and PR opening/synchronization/reopening against main.
After retargeting a PR, use a supported trigger and confirm a passing run for the
final head; an absent check is not a passing check. Before merge, verify native
closing-Issue linkage (`gh pr view <number> --json closingIssuesReferences`);
`Closes #...` links natively when the base is the default branch. Merge completion
and related-ticket synchronization follow the [workflow rule](../../.agents/rules/01-development-workflow.md#2-issue-and-pr-workflow).

Investigate a CI failure against the pre-change baseline in isolated checkouts
under matching conditions. Record revisions, commands and reproducible seed/input;
preserve behavioral assertions when fixing an unstable fixture. Container startup
and headless gameplay coverage remain distinct from a successful image build.

## 9. Related Documents

- [`../../AGENTS.md`](../../AGENTS.md) & [`.agents/rules/01-development-workflow.md`](../../.agents/rules/01-development-workflow.md) — mandatory TDD and DoD rules.
- [`development-environment.md`](development-environment.md) — local services and reset procedures.
- [`openapi.md`](openapi.md) — transport specification maintenance.
