# Interfaces and Component Contracts

## Principle

Component contracts must be understandable independently of the implementation language.

The initial implementation is Go, but Go interfaces are an implementation mechanism, not the architecture itself.

## Contract contents

A meaningful component contract should define:

- operation or capability;
- inputs;
- outputs;
- errors/failure conditions;
- relevant state transitions;
- invariants;
- side effects;
- events emitted, when applicable.

## Internal vs external boundaries

Inside the initial Go application, use ordinary function calls and Go interfaces only where they simplify the design.

Do not create network APIs for every logical component.

A component becomes a candidate for extraction only when a real requirement appears.

If a component is later moved to another language, its language-independent contract should be made explicit as needed.

## Example: Battle

Conceptually:

```text
BattleRequest
  -> Battle component
  -> BattleResult
```

The consumer should depend on the meaning of the request/result rather than the internal Battle implementation.

A future implementation could therefore be:

```text
Go Battle
```

or:

```text
Rust Battle
```

without requiring consumers to understand the implementation language.

The initial in-process contract accepts exactly two participants and returns a
win or draw result with the winner, loser, and turn count. The initial
resolver uses deterministic minimum damage of one and does not know why the
battle was started. More detailed battle rules belong to a later Battle rule
issue and must preserve this consumer-facing boundary.

## Contract rules

- Do not expose private persistence structures as contracts.
- Do not make consumers depend on another component's internal types unnecessarily.
- Prefer stable domain concepts over implementation details.
- Keep contracts as small as the actual interaction requires.
- Do not design a remote protocol until there is a reason to make the boundary remote.

## Events

Domain events are facts that have already occurred.

Examples:

```text
BattleFinished
QuestCompleted
ItemObtained
CharacterLeveledUp
```

The publisher should not need to know which optional consumers exist.

Use events selectively. Immediate operations that require a direct result should remain direct operations where appropriate.

## Related documents

- [`overview.md`](overview.md) — overall architecture.
- [`components.md`](components.md) — component responsibilities.
- [`feature-modules.md`](feature-modules.md) — feature boundaries.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory contract rules.

## Application API boundary

The application should expose game operations through a UI-independent application API or command boundary.

Initially this may be an internal API used by the GUI, tests, CLI tools, and other components. The design should avoid coupling the contract to a specific presentation technology so that appropriate operations can be exposed externally in the future.

A possible future consumer is an AI Agent that plays the game through the same game operations available to a human player. This is an example of a future capability, not a requirement for the initial release.
