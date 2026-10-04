# Documentation inventory and reconciliation (#979)

Reviewed on 2026-10-04. During reconstruction, **the original Party2 defines
correct gameplay**. Current Go behavior, passing tests, and closed issues are
implementation evidence; they cannot override a conflicting legacy rule.

## Scope and disposition

The initial inventory covered 9 rule modules, 17 guidance JSON files, and 135
files under `docs` (including modular/generated API documents). All files were
included in the path/JSON/reference inventory. Semantic review concentrated on
authority, contradictory statements, obsolete mechanics, transaction/storage
boundaries, current routes, and planned work. This is a documentation audit,
not an exhaustive execution/formula audit of every legacy CGI.

| Area | Disposition and source of authority |
|---|---|
| `.agents/rules` | Preserve mandatory constraints; clarify legacy authority, real package/table names, scheduling lifetimes, and test-factory boundaries |
| `.arch/modules` | Preserve complex transaction navigation; remove redundant single-row Bank index, nonexistent schemas, incorrect table names, and false lock/transaction annotations |
| `.arch/shared_tables` | Preserve partial reverse-call indices; explicitly require source search beyond indexed callers |
| `docs/architecture` | Reconcile current composition/storage; mark Gateway planned and shared world-boss HP withdrawn; retain a short historical note because existing documentation checks reference it |
| `docs/design` | Preserve legacy rules; correct false recipe/equipment/domain summaries; explicitly record Go differences below |
| `docs/development` | Preserve actual Makefile/CI procedures; move Phase 0–2 design rationale into architecture |
| `docs/migration` | Preserve completion history and selected CGI navigation; closed tickets do not certify complete parity |
| `docs/assets` | Consolidate duplicate placeholder list into guidelines/work queue; four SVGs exist, final assets and client integration remain pending |
| `docs/api` | Preserve modular sources and both generated bundles; current REST routes remain until replacement is implemented |
| Root indexes | Align STATUS/ROADMAP and document ownership; distinguish the parity target from a completed certification |

Removed files: `.arch/modules/bank.json`, `docs/assets/PLACEHOLDERS.md`.
Moved file: `docs/development/design-rationale.md` →
`docs/architecture/design-rationale.md`. Useful bank navigation remains in
the characters index and bank design; placeholder provenance remains in
the asset queue.

## Confirmed corrections

- Candidate E shared raid HP, contributor/MVP/last-hit rules, and its Lua PoC
  were removed by #479/#584 (`674322a`); they were still described as live or
  pending production wiring. Current boss specification is the party sealing
  encounter. Legacy `stage/king1.cgi:4` and `stage/king99.cgi:4` establish six
  ordinary members and four for king99.
- The sealing-stage table had fabricated names, leaders, stats, and gates.
  Names/capacities/gates now follow each `stage/king*.cgi:3–7`; detailed enemy
  stats and weighted treasures are referenced instead of duplicating a false list.
- Gateway catalog/evaluator exists in-process (#944/#946/#938/#972), while HTTP
  query/dispatcher remain #939/#646. Current routes have no `/api/v1` prefix.
  REST removal (#947–#950), client (#140), simulation (#650), and asset pipeline
  (#654/#729) remain planned. Proposed response examples are labeled accordingly.
- Scheduled work retains unfinished payloads without TTL, terminal records for
  24 hours, and a five-minute worker lock. Overdue work remains unfinished;
  execution-time queue scores must not be treated as expiry scores.
- Maintenance is SQL-backed with a Valkey projection; ranking snapshots are
  JSON strings. Party/Casino/PvP/GvG lobbies and Dungeon/Challenge run buffers
  are already wired; removed SQL active-state repositories are history.
- Auction performs direct transfers without bidding. Depot owns item/money
  mailing; Tavern owns meal reservations. Bank savings use `characters.deposit`.
  Paid Inn, guild donation leveling, and a level-reset rebirth loop are absent
  from the reference rules and must not be inferred from old examples.
- `.arch` and AST checks verify selected symbols and structure, not every SQL
  table, runtime lock, legacy behavior, or recovery/performance guarantee.

## Implementation differences requiring follow-up

The documentation now preserves these differences. Production behavior was not
changed by this maintenance task.

| Finding | Legacy / intended behavior | Current evidence and next review |
|---|---|---|
| Chapel availability | `lib/chapel.cgi:35–63` sets a blessing; no gold charge or HP revival | `internal/playercontext/catalog.go` labels prayer as revival and requires positive gold; correct the catalog and its parity tests |
| Blacksmith availability | `lib/blacksmith.cgi:123–126` checks/deducts crystals | The seal service uses crystals, but the catalog requires gold; a zero-gold character with sufficient crystals can be rejected |
| Adventure admission | `lib/quest.cgi:524–528` uses `stage <= job_lv + 2` with exclusions; selection UI/`get_limited_stages:1213–1220` add special/seasonal gates | `stage_catalog.go` enforces extra Character.Level thresholds, a different job-level mapping, and no seasonal predicate; reconcile creation, discovery, and joining paths |
| Accessory elixir eligibility | `lib/accessory.cgi:175` requires held item 180 | `internal/shop/synthesis.go` scans all inventory items; reconcile eligibility with the held-item rule. The integer-rate probability comparisons themselves give the same success percentage |
| Demon King unsealing stage | `lib/vs_monster.cgi:105` invokes `make_vs_king` on stage 19 | `internal/adventure/unseal.go` also accepts stage 20; its stage-20 test does not prove legacy correctness |
| Dungeon events and limits | `lib/vs_dungeon.cgi:73–105` dispatches map-owned events; `map/1/1.cgi` has keys/doors; selected maps own `$max_round` | `internal/dungeon/expedition.go` dispatches generic tiles and automatic turn-limit wipeout; reconcile maps, rewards/exit, trap bases, and actor-specific scouting |
| Initial Training slice | Legacy equivalent/provenance not established by this audit | `internal/activity` has a one-hour/10-EXP slice; do not promote those constants into canonical reconstruction requirements without review |
| Async events in ambient transactions | Optional effects should follow the actual outer commit when that guarantee is required | `economy/runner.go` publishes after inner `RunInTx` returns; `database/tx.go` reuses an ambient transaction, so no outer-commit guarantee exists |
| Cluster readiness | Multi-key scripts need common hash-slot placement for Cluster | Current Party lobby scripts use untagged keys; standalone execution does not prove Cluster compatibility |

The incorrect accessory recipe table was removed after comparison with legacy
`lib/accessory.cgi:33–86` and Go `synthesis_recipes.go`. The former five-slot
equipment description referenced nonexistent constants; current Go categories
are documented separately from the legacy weapon/armor/held-item model.

## Validation and limits

Validation on 2026-10-04:

- The final inventory contains 9 rule modules, 16 guidance JSON files, and
  136 files under `docs`. Local Markdown file targets exist and JSON parses.
- `.arch` source-symbol/transaction checks and database lock-hierarchy checks pass.
- OpenAPI bundles remain synchronized with 294 paths / 317 operations.
- `make check` passes: formatting, vet, API coverage, architecture checks,
  migrations, the full test suite with MariaDB/Valkey, and Docker smoke build.
- Production Go, migrations, and API sources/generated bundles are unchanged.

Preserve correct legacy rules when a current test disagrees. Additional gameplay
corrections need focused implementation work using the original references and
meaningful behavioral tests. Do not interpret this inventory or a successful
repository check as certification of all mechanics or all cross-store recovery.
