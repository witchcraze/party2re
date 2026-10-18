# Documentation Audit & Unresolved Implementation Gaps

During reconstruction, **the original Party2 defines correct gameplay**. Current Go behavior, passing tests, and closed issues are implementation evidence; they cannot override a conflicting legacy rule.

## Unresolved Implementation Gaps

The following implementation differences against legacy Party2 have been identified and require focused reconciliation:

| Finding | Legacy / Intended Behavior | Current Implementation & Next Review |
|---|---|---|
| Adventure admission | `lib/quest.cgi:524–528` uses `stage <= job_lv + 2` with exclusions; selection UI/`get_limited_stages:1213–1220` add special/seasonal gates | `stage_catalog.go` enforces extra Character.Level thresholds, a different job-level mapping, and no seasonal predicate; reconcile creation, discovery, and joining paths |
| Accessory elixir eligibility | `lib/accessory.cgi:175` requires held item 180 | `internal/shop/synthesis.go` scans all inventory items; reconcile eligibility with the held-item rule. The integer-rate probability comparisons themselves give the same success percentage |
| Demon King unsealing stage | `lib/vs_monster.cgi:105` invokes `make_vs_king` on stage 19 | `internal/adventure/unseal.go` also accepts stage 20; its stage-20 test does not prove legacy correctness |
| Dungeon events and limits | `lib/vs_dungeon.cgi:73–105` dispatches map-owned events; `map/1/1.cgi` has keys/doors; selected maps own `$max_round` | `internal/dungeon/expedition.go` dispatches generic tiles and automatic turn-limit wipeout; reconcile maps, rewards/exit, trap bases, and actor-specific scouting |
| Initial Training slice | Legacy equivalent/provenance not established by this audit | `internal/activity` has a one-hour/10-EXP slice; do not promote those constants into canonical reconstruction requirements without review |
| Async events in ambient transactions | Optional effects should follow the actual outer commit when that guarantee is required | `economy/runner.go` publishes after inner `RunInTx` returns; `database/tx.go` reuses an ambient transaction, so no outer-commit guarantee exists |
| Cluster readiness | Multi-key scripts need common hash-slot placement for Cluster | Current Party lobby scripts use untagged keys; standalone execution does not prove Cluster compatibility |
