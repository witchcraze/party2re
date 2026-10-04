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


`core/battle` exposes both two-participant and party/faction combat contracts.
The resolver receives participant snapshots and returns outcomes/logs without
owning persistence or the reason for combat. `internal/battle` binds game models
and applies outcomes within the initiating feature's transaction boundary.
The detailed legacy mechanics are in [battle.md](../design/battle.md).

## Delayed-result claims

Activity owns an atomic claim-and-apply persistence operation: the claimed flag
and character reward state change together. Adventure now resolves immediately
and has no delayed claim endpoint. Scheduling must not impose the initial
training workflow on unrelated legacy combat.

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


**Routes and authentication:**

The complete current transport contract is in [OpenAPI](../api/openapi.json),
generated from `docs/api/base.json` and modular paths. Routes are unversioned.
Session Bearer tokens and Personal Access Tokens authenticate player operations;
administrative endpoints use their explicit admin credential contract.
Public health, registration, login, and discovery routes do not all require a
player session. Consult each operation's security definition rather than an
obsolete blanket exception list.

**Request invariants and security headers enforced at the transport layer:**

- Standard security headers are applied globally across all responses via middleware:
  - `X-Content-Type-Options: nosniff` — prevents MIME-type sniffing
  - `X-Frame-Options: DENY` — protects against clickjacking
  - `Referrer-Policy: strict-origin-when-cross-origin` — restricts referrer header leakage
  - `Content-Security-Policy: default-src 'none'` — disables client script execution on API responses
- CORS policy is enforced globally via configurable allowed origins (`WithAllowedOrigins` / `PARTY2_CORS_ORIGINS`):
  - Requests from configured allowed origins receive `Access-Control-Allow-Origin: <origin>` and `Vary: Origin`.
  - `OPTIONS` preflight requests from allowed origins receive `204 No Content` with `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS`, `Access-Control-Allow-Headers: Content-Type, Authorization, X-Admin-Key`, and `Access-Control-Max-Age: 86400`.
  - Wildcard origin (`*`) is explicitly prohibited and ignored if configured.
  - Requests from unlisted origins receive no `Access-Control-Allow-Origin` headers.
  - *CORS is a browser interoperability policy, not an authorization control.* Preflight success does not grant access; session authentication (`Authorization: Bearer <session-id>`) and administrative credentials (`X-Admin-Key`) are verified independently at the service and transport layers.
- Client IP extraction & rate-limiting identity policy (`WithTrustedProxies` / `PARTY2_TRUSTED_PROXIES`):
  - **Direct Exposure (Safe by Default)**: When `PARTY2_TRUSTED_PROXIES` is empty or unset, the server strictly uses `RemoteAddr` as the client IP identity. Forwarding headers (`X-Forwarded-For`, `X-Real-IP`) sent by direct clients are ignored, preventing rate-limit rotation and spoofing attacks.
  - **Reverse Proxy Deployment**: Forwarding headers are honored only if the immediate peer (`RemoteAddr`) belongs to an explicitly configured trusted proxy CIDR or IP (e.g. `10.0.0.0/8, 127.0.0.1/32`).
  - **Chain Traversal**: Multi-hop `X-Forwarded-For` headers are inspected right-to-left, identifying the rightmost untrusted peer as the authentic client IP. Spoofed headers prepended before reaching the trusted proxy boundary are discarded.
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

**Response contract & presentation decoupling:**

HTTP handlers standardize API responses following pragmatic REST conventions and presentation decoupling:
- **Direct Domain Payloads (Success)**: Successful operations return structured domain resources directly at the JSON root, avoiding unnecessary `{ "data": ... }` envelope nesting (YAGNI). When presentation dialogue or NPC text is required (e.g., shopkeeper reactions), handlers include dedicated presentation attributes (such as `npc_message`) within the response structure. The generic `SuccessResponse[T]` helper in `response.go` remains available for endpoints that explicitly decouple envelope metadata.
- **Error Envelope (`StructuredErrorResponse`)**:
  - `error.code` (string): Standardized machine-readable error code for client/agent branching.
  - `error.message` (string): Safe, user-facing error description. Internal system and database diagnostics are logged to operational storage and masked from API responses.

### Client / Agent Gateway (CQRS Architecture)

To support modern Web UI (Server-Driven UI), autonomous AI Agents, and Chatbot integrations (Line/Discord) without client-side routing sprawl or token bloat, the application plans a unified two-pillar CQRS Gateway. The HTTP boundaries below are not registered yet (#939/#646):

| Pillar | Method & Path | Responsibility | Output |
|---|---|---|---|
| **Query (Observe)** | `GET /characters/{id}/context` | Character snapshot, active timers, and **authoritative whitelist of available actions**. | Lightweight snapshot + `available_actions` (with `required_params`) |
| **Command (Execute)** | `POST /characters/{id}/actions` | Single-entry-point command dispatcher for all state mutations. | `{ success, result, context }` (updates client state in 1 round trip) |

See [`client-agent-api.md`](client-agent-api.md) for the proposed protocol and implementation status, LLM tool integration, and Server-Driven UI lifecycle.

## Application logging contract

Application services that need operational diagnostics receive an injected
logger rather than using global state. The contract accepts an operation name,
structured attributes, and (for errors) an error value. Its implementation
emits JSON and records only the error type, so error messages cannot expose
passwords, sessions, or database credentials. See
[`logging.md`](logging.md) for the safety and correlation rules.
