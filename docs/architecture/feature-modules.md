# Feature Modules

Feature Modules are the primary unit for adding new game systems.

## Goal

A new feature should be implementable without scattering feature-specific logic throughout the Core.

Examples include:

```text
internal/
  adventure/
  guild/
  casino/
  alchemy/
  auction/
  farming/
  collection/
  ranking/
  events/
```

The initial activity implementation is an Activity feature. It owns the
training rules and activity state while consuming the public Character
contract and persistence boundary. Scheduling makes an activity executable at
a later time, but the scheduling concept does not own the training rules.
When a delayed result is claimed, the feature persistence boundary must expose
one atomic claim-and-apply operation. That operation compare-and-sets the
claimed state and applies the resulting Core character state in the same
transaction, so concurrent requests cannot duplicate rewards.

The list is illustrative, not exhaustive.

## Ownership

A feature owns:

- its domain rules;
- feature-specific state;
- feature-specific persistence;
- its application logic;
- its public interface.

A feature does not own shared concepts merely because it uses them.

## Internal Structure

Feature Modules should follow a standard logical structure, typically distinguishing between:
- Domain logic
- Application services
- Infrastructure / Persistence
- Interfaces / API

**CRITICAL RULE:** Do not create unnecessary layers or empty packages. 

Every Feature Module does not need every layer. If a feature is simple and has no complex domain logic, do not create an empty `domain` package just to satisfy a template. 
The standard structure is a *design checklist*, not a directory tree to be blindly copy-pasted.

During Architecture Review, the primary focus must be on **"What boundary does this Feature have?"**, not on whether it strictly implements every layer of Clean Architecture.

## Dependencies

Allowed:

```text
Feature -> Core
Feature -> Shared Component contract
Feature -> Infrastructure
```

Avoid:

```text
Feature A -> Feature B internal implementation
Feature A -> Feature B database tables
```

If two features need to communicate, first determine whether they should use:

- a public domain contract;
- a synchronous application-level operation;
- a domain event.

Choose the simplest mechanism that expresses the actual relationship.

## Adding a new feature

Before implementation, document:

1. What game problem does the feature solve?
2. What state does it own?
3. Which existing components does it need?
4. Which contracts does it expose?
5. Which events does it publish or consume?
6. Can a second feature of the same category be added without modifying unrelated code?

## Avoid premature frameworks

Do not create a universal plugin framework merely because the project has Feature Modules.

Feature Modules are an architectural concept first. A runtime plugin system is a separate requirement and should only be introduced if a real use case appears.

## Feature review

Every substantial feature should be reviewed for:

- boundary clarity;
- Core contamination;
- coupling to existing features;
- testability;
- data ownership;
- future extensibility;
- unnecessary abstraction.

## Transactional Boundaries & Concurrency Control

State-mutating feature modules (such as `shop`, `blacksmith`, `alchemy`, `bank`, `inn`, `medal`, `guild`, `casino`, `lottery`, `farm`, `auction`) must strictly adhere to the **Unit of Work** pattern and prevent race conditions / lost updates under concurrent load:

1. **Ambient Transaction Propagation (`RunInTx` & `ExecutorFromContext`)**:
   - Every database repository accesses the database via `database.ExecutorFromContext(ctx, r.db)`.
   - When an application service initiates a multi-module operation within `database.RunInTx(ctx, db, func(txCtx context.Context) error { ... })`, the resulting `txCtx` carries the active transaction.
   - Any repository or nested service invoked with `txCtx` automatically participates in the existing transaction without opening new connections or committing prematurely.
2. **Deterministic Lock Acquisition Ordering (Deadlock Prevention)**:
   - When composite operations span multiple domain entities, locks (`SELECT ... FOR UPDATE`) MUST be acquired strictly in the following hierarchical order:
     1. `players`
     2. `characters` (if multiple characters, sorted ascending: `id1 < id2`)
     3. `inventory_items` / `equipment_slots`
     4. `character_jobs` / `character_job_masteries`
     5. `character_depots` / `depot_items`
     6. `bank_accounts` / `bank_transfers`
     7. `guilds` (if multiple guilds, sorted ascending: `id1 < id2`) / `guild_members`
     8. Feature tables (`auction_listings`, `farm_plots`, `casino_accounts`, `gvg_standings`, `character_boss_records`, `challenge_records`, etc.)
3. **Application Orchestrator Pattern**:
   - Cross-module operations (such as purchasing an auction listing involving Buyer character wallet, Seller bank account, and Inventory transfer) should be orchestrated at the application layer inside a single `RunInTx` boundary.
   - No feature repository calls `BeginTx` directly; all repositories delegate to `RunInTx` and `ExecutorFromContext`.
4. **High-Concurrency Stress Testing & Deadlock Verification**:
   - Automated concurrency benchmarks (`internal/database/concurrency_stress_test.go`, `make test-stress`, `scripts/stress_test.sh`) simulate 50–100 concurrent workers hammering MariaDB with high contention.
   - Verifies zero deadlocks (MariaDB Error 1213 / 1205), conservation of money/inventory invariants, deterministic auction buyout/bid resolution, and exact atomicity across mixed multi-domain chaos workflows.
5. **Sub-Resource Ownership & Repository SQL Scoping**:
   - When modifying, claiming, or deleting sub-resources belonging to a player/character (`challenge_sessions`, `lottery_tickets`, `auction_listings`, `dungeon_expeditions`, `letters`, `companion_phrases`), SQL queries MUST include character/player ID ownership scoping (`WHERE id = ? AND character_id = ?`).
   - Domain services and HTTP handlers must enforce ownership verification and return `ErrForbidden` (`403 Forbidden`) on unauthorized access attempts.

## Related documents

- [`overview.md`](overview.md) — overall architecture.
- [`components.md`](components.md) — component responsibilities.
- [`interfaces.md`](interfaces.md) — contracts between components.
- [`../design/game-overview.md`](../design/game-overview.md) — feature/domain context.
- [`../../.agents/rules/`](../../.agents/rules/) — mandatory feature-boundary rules.
