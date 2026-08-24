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

Game behavior is implemented independently of any specific UI. Major game
operations are routed through the application service layer rather than being
implemented directly in transport handlers.

### HTTP JSON API (`internal/api/http`)

The initial transport layer is an HTTP JSON API using only the Go standard
library `net/http`. The `Handler` struct is constructed with injected
application service interfaces and exposes a `ServeMux` via `Router()`.

**Endpoints:**

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check — always public |
| `POST` | `/players` | Player registration |
| `POST` | `/sessions` | Login — returns a session token |
| `DELETE` | `/sessions` | Logout — revokes the current session |
| `POST` | `/characters` | Create a character (authenticated) |
| `GET` | `/characters/{id}` | Get character state (authenticated) |
| `POST` | `/adventures` | Start a stage adventure (authenticated) |
| `POST` | `/adventures/{id}/claim` | Claim an adventure result (authenticated) |
| `POST` | `/shop/purchase` | Purchase items (authenticated) |
| `POST` | `/shop/sell` | Sell items (authenticated) |

**Authentication:**

All endpoints except `GET /health`, `POST /players`, and `POST /sessions`
require `Authorization: Bearer <session-id>`. The handler validates the session
via `PlayerService.Authenticate` before delegating to the target service.

**Request invariants and security headers enforced at the transport layer:**

- Standard security headers are applied globally across all responses via middleware:
  - `X-Content-Type-Options: nosniff` — prevents MIME-type sniffing
  - `X-Frame-Options: DENY` — protects against clickjacking
  - `Referrer-Policy: strict-origin-when-cross-origin` — restricts referrer header leakage
  - `Content-Security-Policy: default-src 'none'` — disables client script execution on API responses
- `Content-Type: application/json` is required on all endpoints that consume a
  request body. Requests with a missing or incorrect content type receive
  `415 Unsupported Media Type`.
- Request bodies are limited to 64 KiB via `http.MaxBytesReader`. Bodies
  exceeding this limit receive `400 Bad Request`.
- Unknown JSON fields are rejected (`DisallowUnknownFields`).

**Character ownership verification:**

All endpoints that operate on a character (`GET /characters/{id}`, `POST /adventures`, `POST /shop/*`)
verify that the authenticated player owns the targeted character (`char.PlayerID == player.ID`).
Cross-player requests are rejected with `403 Forbidden`.

**Handler contract:**

Handlers must contain no domain business logic. All game rules remain inside
the application services. The handler's responsibility is limited to:

1. extracting and validating the session;
2. decoding and size-limiting the request body;
3. delegating to the appropriate service;
4. mapping service errors to HTTP status codes;
5. encoding the service result as JSON.

A future implementation could replace the HTTP layer with a gRPC, WebSocket,
or in-process transport without changing the application service layer.

## Application logging contract

Application services that need operational diagnostics receive an injected
logger rather than using global state. The contract accepts an operation name,
structured attributes, and (for errors) an error value. Its implementation
emits JSON and records only the error type, so error messages cannot expose
passwords, sessions, or database credentials. See
[`logging.md`](logging.md) for the safety and correlation rules.
