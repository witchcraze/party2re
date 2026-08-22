# Components

This document defines the responsibilities and boundaries of the initial components.

These are conceptual boundaries. The initial implementation uses Go.

## Component definition

Each component should be describable by:

- Responsibility
- Inputs
- Outputs
- State
- Dependencies
- Public contract
- Persistence requirements
- Initial implementation

The implementation language is intentionally not part of the component's identity.

## Core

### Player

**Responsibility:** account-level identity and authentication-related state.

Does not own game-specific character state.

### Character

**Responsibility:** the player's in-game character and its fundamental state.

Owns invariants about its own state but should not become a God object containing every game system.

### Progression

**Responsibility:** level, experience, stats, and other fundamental character progression.

Progression consumes job growth values through the public Job definition
contract. It does not contain a built-in catalog of job-specific data.

### Job

**Responsibility:** job definitions, job availability rules, and character job
history.

Job definitions are content data, not executable progression logic. The initial
catalog is stored outside Go source as validated data under
`internal/core/job/data/`. The Job component loads and validates that data at
startup or construction time, then exposes definitions through a small lookup
contract. Consumers do not access the data file or catalog map directly.

The data file may contain stable IDs, display names, growth values, and simple
requirements. Dynamic requirements and dynamic growth formulas require explicit
rules and tests; they must not be approximated by silently replacing them with
fixed values.

Visual asset paths, provenance, and licenses are maintained separately from
domain definitions in the asset documentation and asset manifest. Job data
must not embed legacy image paths or legacy asset bytes.

### Item

**Responsibility:** item definitions and concrete item instances.

Keep definitions separate from instances.

### Inventory

**Responsibility:** ownership and storage of item instances.

Does not contain unrelated item behavior.

### Equipment

**Responsibility:** which eligible item instances are equipped and where.

### Currency

**Responsibility:** generic ownership and movement of game currencies.

Avoid hard-coding every future currency into Core.

### Game Time / Scheduling

**Responsibility:** represent and process delayed actions without owning feature-specific rules.

A scheduled action should identify what is scheduled and when it becomes executable; the relevant feature determines its result.

### Domain Events

**Responsibility:** publish meaningful domain-level facts to decouple optional consumers.

Examples:

- BattleFinished
- QuestCompleted
- CharacterLeveledUp
- ItemObtained
- JobChanged
- GuildJoined

Events are not required for every operation.

## Shared components

### Battle

**Responsibility:** execute and represent battles between participants.

Conceptual model:

```text
Battle
  - Participants
  - State
  - Actions
  - Effects
  - Result
```

Battle must not know whether it was initiated by a quest, guild, arena, or another feature.

### Adventure / Quest

**Responsibility:** define and execute adventure-oriented game flows, including destinations, encounters, durations, requirements, and rewards.

Adventure may use Battle but should not own Battle's implementation.

## Feature modules

Examples:

- Guild
- Casino
- Alchemy
- Auction
- Farming
- Collection
- Ranking
- Events

Each feature owns its feature-specific rules and state.

A feature may consume public contracts from Core or shared components. It should not access another feature's private implementation or persistence model.

## Component review criteria

For every new component ask:

1. What does this component own?
2. What does it deliberately not own?
3. What are its public inputs and outputs?
4. Which dependencies are necessary?
5. Could another implementation replace it without changing consumers?
6. Would adding another component of the same kind require changes here?
7. Is the component boundary justified by an actual responsibility rather than speculative abstraction?

## Related documents

- [`overview.md`](overview.md) — overall architecture.
- [`feature-modules.md`](feature-modules.md) — feature boundaries.
- [`interfaces.md`](interfaces.md) — public component contracts.
- [`../design/game-overview.md`](../design/game-overview.md) — domain context.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory architectural rules.
