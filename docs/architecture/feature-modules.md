# Feature Modules

Feature Modules are the primary unit for adding new game systems.

## Goal

A new feature should be implementable without scattering feature-specific logic throughout the Core.

Examples include:

```text
features/
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

The list is illustrative, not exhaustive.

## Ownership

A feature owns:

- its domain rules;
- feature-specific state;
- feature-specific persistence;
- its application logic;
- its public interface.

A feature does not own shared concepts merely because it uses them.

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

The key question is:

> Does this feature make the next similar feature easier or harder to implement?

## Related documents

- [`overview.md`](overview.md) — overall architecture.
- [`components.md`](components.md) — component responsibilities.
- [`interfaces.md`](interfaces.md) — contracts between components.
- [`../design/game-overview.md`](../design/game-overview.md) — feature/domain context.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory feature-boundary rules.
