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

The key question is:

> Does this feature make the next similar feature easier or harder to implement?

## Related documents

- [`overview.md`](overview.md) — overall architecture.
- [`components.md`](components.md) — component responsibilities.
- [`interfaces.md`](interfaces.md) — contracts between components.
- [`../design/game-overview.md`](../design/game-overview.md) — feature/domain context.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory feature-boundary rules.
