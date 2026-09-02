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

## 6. Domain Events
Use domain events (e.g., `BattleFinished`, `LevelUp`) when they provide meaningful decoupling. A publisher should not need to know which optional features (like achievements or rankings) consume an event. Do not turn every operation into an event; use direct calls when immediate results or strong transactional coupling are appropriate.

## 7. Core Domain Invariant & Helper Enforcement (Go AST Linting)
Direct struct field mutations across Core domain entities by feature modules are strictly prohibited to prevent rule bypasses, integer overflows, concurrency races, and duplication bugs:
- **Progression (`Experience`, `Level`)**: Must route through `progression.ApplyExperience` or `progression.Rebirth` (ensuring OverLevel Lv 150 thresholds, cumulative experience calculations, and stat growths are applied).
- **Currency & Economy (`Money`, `SmallMedals`)**: Must route through `char.AddMoney`, `char.DeductMoney`, `char.AddSmallMedals`, or `char.DeductSmallMedals` (ensuring 0-debt invariants, non-negative amounts, and max currency caps).
- **Job & Skill State (`CurrentJobID`, `MasteredJobs`)**: Must route through `CharacterJob.ChangeTo` and `CharacterJob.Master` (ensuring prerequisite level/gender validation and history logging).
- **Inventory & Items (`Inventory.Items`)**: Must route through `Inventory.Add`, `Inventory.Consume`, or `Inventory.Update` (ensuring instance uniqueness and quantity consistency).
- **Equipment & Slots (`Equipment.Slots`)**: Must route through `Equipment.Equip` or `Equipment.Unequip` (ensuring slot compatibility and ownership verification).

These encapsulation boundaries are mechanically enforced across all Go source files outside `internal/core` (and database repository mappings) via Go AST static analysis (`internal/core/core_lint_test.go` and `internal/core/progression/progression_lint_test.go`), running with 0 runtime overhead in 0.1s during `make check`.


