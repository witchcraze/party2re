---
name: Domain Modeling Principles
description: Guidelines for modeling game logic, combat, progression, and scheduled actions.
---

# Domain Modeling Principles

## 1. Player vs Character
Separate the account-level `Player` concept from the in-game `Character`.
Character owns character state and invariants, but should not become a God object containing every game system (e.g., the Character struct should not directly implement buying items, changing jobs, or managing guilds).

## 2. Items and Inventory
Separate:
- `ItemDefinition`: what an item is.
- `ItemInstance`: a concrete owned instance.
- `Inventory`: ownership state.
- `Equipment`: equipped state.
This supports future systems such as enhancement, randomized properties, and durability.

## 3. Jobs and Progression
Separate `JobDefinition` from `CharacterJob` (a character's relationship with a job, including mastery/history). Job requirements and available skills should be modeled as rules/data rather than as a growing collection of special cases.

## 4. Battle is an Independent Component
Battle is an independent domain component used by features. At a conceptual level, it contains participants, state, actions, effects, and results. Battle must **not** contain feature-specific assumptions (e.g., "this is a guild battle"). Higher-level features provide the context and invoke the Battle component.

## 5. Scheduled Actions
Party2 contains asynchronous/time-based actions (an action is started and a result becomes available later). Model this as a reusable concept such as `ScheduledAction` (actor, action type, start time, execution time, state). The scheduling mechanism itself must not contain the rules of individual features.

## 6. Domain Events & Side-Effect Hooks
Use domain events or explicit observer hooks (e.g., `VictoryHook`, `SynthesisHook`, `BattleFinished`, `LevelUp`) when they provide meaningful decoupling:
- **Two-Phase Dispatcher (`internal/core/event.Dispatcher`)**: Producers emit domain events via `tc.EmitEvent`. The dispatcher separates in-tx synchronous execution from post-commit asynchronous execution. See [`docs/architecture/cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md) §5 for dispatcher architecture and lifecycle semantics.
- **Modular Monolith Decoupling**: Action producers define explicit hook signatures or domain event types at their module boundaries rather than importing downstream consumer modules. Wire registrations in `cmd/party2/wire.go` at the composition root.
- **Lock Hierarchy & Transaction Safety**: Sync event handlers mutating secondary entities must adhere to the lock order in `.agents/rules/05-database-and-caching.md` (`characters` -> `inventory_items` -> secondary records). Secondary records must always be locked *after* characters, never before.
- **Direct Calls vs Events**: Do not turn internal intra-module operations into events or hooks; use direct method calls when operations belong to the same bounded context and share immediate transactional invariants.

## 7. Core Domain Invariant & Helper Enforcement (Go AST Linting)
Direct struct field mutations across Core domain entities by feature modules are strictly prohibited to prevent rule bypasses, integer overflows, concurrency races, and duplication bugs:
- **Progression (`Experience`, `Level`)**: Must route through `progression.ApplyExperience` or `progression.Rebirth` (ensuring OverLevel Lv 150 thresholds, cumulative experience calculations, and stat growths are applied).
- **Currency & Economy (`Money`, `SmallMedals`)**: Must route through `char.AddMoney`, `char.DeductMoney`, `char.AddSmallMedals`, `char.DeductSmallMedals`, or the universal runtime primitive `economy.TransactionRunner` (`ExecuteTransaction`) (ensuring 0-debt invariants, non-negative amounts, deterministic lock order, and max currency caps).
- **Job & Skill State (`CurrentJobID`, `MasteredJobs`)**: Must route through `CharacterJob.ChangeTo` and `CharacterJob.Master` (ensuring prerequisite level/gender validation and history logging).
- **Inventory & Items (`Inventory.Items`)**: Must route through `Inventory.Add`, `Inventory.Consume`, or `Inventory.Update` (ensuring instance uniqueness and quantity consistency).
- **Equipment & Slots (`Equipment.Slots`)**: Must route through `Equipment.Equip` or `Equipment.Unequip` (ensuring slot compatibility and ownership verification).

These encapsulation boundaries are mechanically enforced across all Go source files outside `internal/core` (and database repository mappings) via Go AST static analysis (`internal/core/core_lint_test.go` and `internal/core/progression/progression_lint_test.go`), running with 0 runtime overhead in 0.1s during `make check`.
