---
name: Domain Modeling Principles
description: Rules for modeling characters, items, jobs, battle, scheduled actions, events and core invariants.
---

# Domain Modeling Principles

## 1. Model separation
- `Player` (account) is separate from `Character`. Character owns its invariants but MUST NOT absorb feature systems (buying, job change, guild management).
- Items: `ItemDefinition` (what), `ItemInstance` (owned instance), `Inventory` (ownership), `Equipment` (equipped state).
- `JobDefinition` is separate from `CharacterJob` (mastery/history). Model job requirements and skills as rules/data, not special cases.
- Battle is an independent component (participants, state, actions, effects, results) with no feature-specific assumptions; features supply context.
- Time-based actions use a reusable `ScheduledAction` (actor, type, start, execution time, state). The scheduler MUST NOT contain feature rules.

## 2. Domain events and hooks
- Use events or explicit hooks only for meaningful cross-module decoupling; same-context operations sharing transactional invariants use direct calls.
- Producers emit through `internal/core/event.Dispatcher` (`tc.EmitEvent`) and define hook/event types at their own boundary; registrations are wired in `cmd/party2/wire.go`. Semantics: [`cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md).
- Sync handlers that mutate records MUST follow the lock order in `05-database-and-caching.md`.

## 3. Core invariants
Feature modules MUST NOT mutate core fields directly. Use:
- experience/level: `progression.ApplyExperience*`; MUST NOT add a reincarnation/reset loop absent from the legacy specification;
- money/medals: `char.AddMoney`, `DeductMoney`, `AddSmallMedals`, `DeductSmallMedals`, or `economy.TransactionRunner`;
- job state: `CharacterJob.ChangeTo`, `CharacterJob.Master`;
- inventory: `Inventory.Add`, `Consume`, `Update`;
- equipment: `Equipment.Equip`, `Unequip`.

Enforced by `internal/core/core_lint_test.go`. Passing it is not proof of legacy parity.
