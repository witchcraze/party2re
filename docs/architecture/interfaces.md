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
win or draw result with the winner, loser, turn count, and selected reward. The
initial resolver uses deterministic minimum damage of one and does not know why
the battle was started. More detailed battle rules must preserve this
consumer-facing boundary.

The three request rewards are interpreted from the first participant's
perspective: `VictoryReward` when it wins, `DefeatReward` when it loses, and
`DrawReward` for a draw. Battle selects and returns the reward; the initiating
feature decides whether and how that reward is applied to a player, character,
inventory, or another recipient.

Skill definitions return a small `Effect` value through the same public
contract. Skill availability checks remain outside Battle and can depend on
the character's job, level, MP, and an inventory ownership callback.

## Delayed-result claims

Activity and Adventure claim persistence exposes a public claim-and-apply
operation. The operation accepts the feature result and the resulting Core
character state, then compare-and-sets the claimed flag and persists the
character state in one transaction. A failed compare-and-set returns an
already-claimed result without applying the character state.

## ScheduledAction contract

The scheduling mechanism (`internal/core/scheduling`, `internal/scheduling`)
exposes two separate contracts.

### Enqueue contract — `scheduling.Service`

Feature modules schedule a future action:

```go
id, err := schedulingService.Schedule(ctx, "training_complete", characterID, params, executeAt)
```

- `actionType` is a stable string constant owned by the feature package.
- `params` is a `map[string]string`; keys and values must fit within the
  documented size limits (`MaxParamKeyLength`, `MaxParamValueLength`).
- The returned `id` can be stored by the feature for status queries.
- The scheduling service must not know what the action does.

### Handler contract — `scheduling.ActionHandler`

Feature modules implement one handler per action type:

```go
type ActionHandler interface {
    Handle(ctx context.Context, action core_scheduling.ScheduledAction) error
}
```

- A non-nil error marks the action `failed` with a 24-hour retention.
- A nil return marks the action `completed` with a 24-hour retention.
- Handlers must be idempotent: the lock prevents most duplicate calls,
  but a handler should be safe to call more than once if the lock expires.
- Handlers must not modify the `ScheduledAction` fields they receive.
- Handlers must not embed game-rule logic that belongs to another feature.

Register at startup:

```go
worker.RegisterHandler("training_complete", trainingHandler)
```

### Validation contract

`ScheduledAction.Validate()` is the trust boundary between Valkey
(external, mutable storage) and the application.

- The repository calls `Validate()` on every action returned from `FetchDue`.
- The Worker calls `Validate()` again before lock acquisition (defense-in-depth).
- Any action that fails `Validate()` is removed from the pending queue and
  never dispatched to a handler. It cannot cause a panic or incorrect game state.
- Adding a new field to `ScheduledAction` requires updating `Validate()` with
  appropriate limits.

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

## Application logging contract

Application services that need operational diagnostics receive an injected
logger rather than using global state. The contract accepts an operation name,
structured attributes, and (for errors) an error value. Its implementation
emits JSON and records only the error type, so error messages cannot expose
passwords, sessions, or database credentials. See
[`logging.md`](logging.md) for the safety and correlation rules.
