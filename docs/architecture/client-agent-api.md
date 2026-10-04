# Client & Agent API — CQRS & Server-Driven UI Architecture

This document defines the HTTP observation and command contracts, service boundaries and client recovery behavior. See [STATUS](../../STATUS.md) for major current capabilities/gaps and [ROADMAP](../../ROADMAP.md) for remaining milestones; detailed progress and dependencies are tracked in GitHub.

Current clients use the registered routes in [OpenAPI](../api/openapi.json).
`GET /api/v1/characters/{id}/context` and `POST /api/v1/characters/{id}/actions`
are registered. Existing REST routes remain unversioned. Do not remove working REST contracts
before their replacement is implemented and verified against the original Party2.

---

## 1. Core Architecture: CQRS & Action Gateway

In game domain engineering and agentic computing, gameplay interactions are fundamentally **state transition commands** rather than CRUD resource manipulations.

Instead of scattering game logic across hundreds of disconnected REST URLs, the client boundary is organized into two authoritative pillars:

```text
               +-------------------------------------------+
               | Client (Web UI / AI Agent / Line / Discord)|
               +--------------------+----------------------+
                                    |
          Query (Observe)           |         Command (Execute)
          GET /characters/{id}/context        POST /characters/{id}/actions
                                    |
               +--------------------v----------------------+
               |          Client / Agent Gateway           |
               |                                           |
               |  - Action Evaluator (Availability Engine) |
               |  - Action Dispatcher (Command Execution)  |
               |  - Action Catalog (Parameter Validation)  |
               +--------------------+----------------------+
                                    |
               +--------------------v----------------------+
               |       Domain Services & Persistence       |
               |  (Bank, Adventure, Shop, Home, Combat)    |
               +-------------------------------------------+
```

| Pillar | Endpoint | Responsibility | Payload |
|---|---|---|---|
| **Query (Observe)** | `GET /api/v1/characters/{id}/context` | **Observation & Next Moves** | Lightweight character snapshot, active timers, and eligible action entry candidates. Exact parameters remain service-validated. |
| **Command (Execute)** | `POST /api/v1/characters/{id}/actions` | **State Transition** | Accepts `{ action, params }`, dispatches to domain services, and returns `{ result, context }`. |

---

## 2. Query Pillar: `GET /api/v1/characters/{id}/context`

The authenticated discovery endpoint verifies character ownership using the
standard HTTP wrapper, calls the existing uncached `playercontext.Service.Query`,
and enriches its facts with profile avatar and job catalog presentation. Its
`PlayerContextResponse` DTO and reusable OpenAPI schema are also the required
context contract for the command gateway.

The four top-level fields are:

- `character`: ID/name/job ID and name, level, HP/MP maxima and current values,
  wallet gold, fatigue, death and sleeping flags, and icon fields. `icon_url`
  uses the existing profile AvatarURL (URL or data URI), or an empty string.
  The asset ID remains empty until production mappings are specified.
- `scene`: the initial `town` hub, title, background ID/URL, dialogue, and
  optional structured speaker/opponent. The background is a self-authored SVG
  data URI placeholder; no legacy images are reused. Production art resolution
  and dynamic facility/combat scenes are remaining work linked from STATUS.
- `ongoing_actions`: every unfinished scheduled action, plus an observed
  sleep/wake recovery timer when present. Empty observations return `[]`.
  Entries contain `id`, `action_type`, `label`, `execute_at`, rounded-up
  nonnegative `remaining_seconds`, and `is_ready`. Scheduled deadlines are
  ordered ascending, with ID breaking ties. Sleep deadlines are estimates
  derived from the remaining lock duration, not persisted queue records.
- `available_actions`: eligible catalog entries in catalog order, with
  `action`, `label`, `category`, `style`, and `required_params` (always an array).
  Adventure controls use `primary`; other controls use `secondary`.

### Invariants

- **Entry eligibility**: catalog gating is action-specific. HP=0 excludes
  combat entries, but permits legacy noncombat town actions. Free prayer,
  crystal-funded sealing and coin games do not require wallet gold.
  Exact prices, amounts, alternative currencies, items and game state still
  require validation by the execution service.
- **Unfinished work**: Pending/Processing entries remain visible and block
  applicable actions after ExecuteAt. `is_ready` denotes deadline arrival,
  never successful settlement. Refresh context with bounded retry delays for
  overdue work; do not infer completed mutations from a countdown.
- **Sleep recovery**: active sleep exposes rescue; expired sleep with recovery
  pending exposes wake and rescue. The sleep observation remains until explicit
  wake recovery clears it, with `is_ready: true` and zero remaining seconds.
- **Failure boundary**: session and ownership errors return 401/403 (missing
  characters return 404). Query errors and propagated profile-service errors return 500 without partial
  context. An unconfigured query service returns 501 after authentication.
- **Read consistency**: authorization, query facts and profile reads are not a
  cross-store transaction. No observation/result cache is introduced; command
  execution must revalidate current state. Character facts come from the query
  snapshot, not the separate profile view's character projection.

---

## 3. Command Pillar: `POST /api/v1/characters/{id}/actions`

The command outcome/recovery contract was approved in [#646](https://github.com/witchcraze/party2re/issues/646) on 2026-10-04. Service adapters preserve existing game rules and persistence while the HTTP boundary owns parameter decoding and response composition. Current coverage and route retirement are tracked separately from this contract.

### Request Body

```json
{
  "action": "bank_deposit",
  "params": {
    "amount": 5000
  }
}
```

### Input and execution boundary

The authenticated path character is the actor. Reject actor identity fields such
as `character_id` or `player_id` inside `params`; operation-specific target IDs
such as `target_home_id` remain valid. Use the standard ownership wrapper and
propagate the request context to every service/read call.

`params` is a JSON object when present. It may be omitted for commands with no
required inputs. Missing/null required values, incorrect types, unknown fields,
and actor overrides return `400 INVALID_ACTION_PARAMS` without executing a
command. Preserve strict JSON decoding, the 64 KiB limit and existing transport
errors for authentication, ownership, content type and malformed envelopes.
Unknown actions return `404 ACTION_NOT_FOUND`; known but unconnected commands or
unconfigured services return `501 ACTION_NOT_IMPLEMENTED`. These failures perform
no command and expose no context.

Re-read current entry eligibility before execution; reject an ineligible entry
with `409 ACTION_UNAVAILABLE`. Scheduled/timer read errors stop execution with
`500 ACTION_PREFLIGHT_FAILED`, without execution or context. An earlier
`available_actions` list is never execution authorization.
Preserve the shared Sleep/CanWake guard for ordinary commands and explicit
Wake/Rescue exceptions. Domain services still validate exact amounts, currencies,
items and state. Adapters call services directly and reuse HTTP result composition;
they do not invoke REST handlers through internal HTTP requests or copy game rules.

### Command outcome and context refresh

`result` preserves the existing operation's structured result and transport-owned
presentation. A non-null `context` uses exactly the shared `PlayerContextResponse`
defined in section 2, including all four slots and non-null arrays. Refresh is an uncached
read after execution, not part of the mutation transaction. Recheck ownership
before returning context; failed reads, enrichment or ownership checks never
return partial observations.

The HTTP-owned registration seam `withActionCommand` binds a catalog ID to a
typed parameter struct, direct service invocation and explicit 4xx error mapper.
It is configured once during Handler construction, rejects unknown/duplicate
registrations, and treats a nil execution function as an unconfigured dependency.
Parameter decoding completes before state reads and execution. Only the mapped
4xx errors are known rejections; all other execution errors use `EXECUTION_FAILED`.
Both GET and command refresh composition reject ownership changes observed while
reading profile enrichment.

### Stage adventure command contract

`adventure_start` accepts only `{ "stage_id": "stage-00" }`. A supplied non-null
string is required; an empty string retains `StartStage`'s existing starter-stage
default. The adapter calls the existing Adventure service once with the owned
actor and request context. `result` reuses the `POST /adventures` fields: `id`,
`character_id`, `stage_id`, `started_at`, `floors_cleared`, `is_cleared`,
`party_size`, `resolved`, and `experience_reward`. The crawl resolves immediately;
this connection adds no scheduled expedition or new game rule.

| Service rejection | HTTP | Stable code |
|---|---|---|
| Stage not found | 422 | `ADVENTURE_STAGE_NOT_FOUND` |
| Level requirement | 403 | `ADVENTURE_LEVEL_REQUIRED` |
| Job level requirement | 403 | `ADVENTURE_JOB_LEVEL_REQUIRED` |
| Unconscious character | 422 | `ADVENTURE_UNCONSCIOUS` |
| Exhausted character | 422 | `ADVENTURE_EXHAUSTED` |
| Once-daily entry already used | 422 | `ADVENTURE_DAILY_LIMIT` |
| Character no longer exists | 404 | `CHARACTER_NOT_FOUND` |

Catalog exclusions (including HP=0, fatigue, sleep, pending wake recovery and
unfinished Pending/Processing work) return `409 ACTION_UNAVAILABLE` before
service execution. A ready deadline does not clear unfinished work. Service
validation remains authoritative for exact stage requirements and state changes
after preflight; unexpected service/store errors use `500 EXECUTION_FAILED`.
Both known outcomes use the refresh/recovery contract below.

### Emergency rescue command contract

`rescue_request` accepts only `{ "reason": "stuck activity" }`, with a required
non-null string. Actor identity comes from the owned path. Empty or whitespace-only
reasons reach existing service validation and return `422 RESCUE_INVALID_REASON`;
invalid actor IDs map to `400 RESCUE_INVALID_CHARACTER_ID`, and a missing service
character maps to `404 CHARACTER_NOT_FOUND`. Unexpected service/store errors
remain `500 EXECUTION_FAILED` with unknown outcome and no context.

The adapter calls `EmergencyRescue` once with the request context and current UTC
time. Rescue remains an explicit entry/sleep recovery exception during sleep,
cooldown and unfinished work. Its structured `RescueRecord` result includes
`id`, `character_id`, `reason`, `penalty_seconds` and `created_at`, including the
service's idle/no-op result. Cleanup and penalties remain service-owned; rescue
does not guarantee removal of sleep. Known success/rejection survives refresh
failure under the shared contract. Existing REST rescue routes remain available.

### Outcome responses

| Condition | HTTP response | Client behavior |
|---|---|---|
| Command succeeds, refresh succeeds | 200, `success: true`, `result`, `context` | Replace observed context. |
| Command succeeds, refresh fails | 200, `success: true`, original `result`, `context: null`, `context_error` | Re-fetch GET context only; do not resend the command. |
| Known domain/precondition rejection, refresh succeeds | Mapped 4xx, `success: false`, structured `error`, `context` | Show the rejection and recovery candidates. |
| Known domain/precondition rejection, refresh fails | Preserve original 4xx and `error`; `context: null`, `context_error` | Re-fetch GET context only. |
| Unexpected execution error | 500, `success: false`, `error.code: EXECUTION_FAILED`; no context | Treat the mutation outcome as unknown; do not automatically resend. |

`error` and `context_error` use the existing `{ code, message }` error detail
shape. Refresh errors use `CONTEXT_REFRESH_FAILED` with a safe public message;
internal storage details are not exposed. Successful refresh omits
`context_error`. For example, a completed bank deposit with a failed refresh is:

```json
{
  "success": true,
  "result": {
    "character_id": "char-123",
    "money": 7000,
    "deposit": 15000,
    "amount": 5000,
    "message": "5000 Gお預かりいたしました"
  },
  "context": null,
  "context_error": {
    "code": "CONTEXT_REFRESH_FAILED",
    "message": "操作は完了しました。状態を再取得してください。"
  }
}
```

This contract prevents a refresh failure from disguising a known successful
mutation as a retryable command failure. It does **not** provide idempotency or
exactly-once execution. A lost response, timeout or unexpected execution error can
leave the outcome unknown; GET context is an observation, not a command receipt.
Do not automatically replay POST. Safe command replay would require a separate
idempotency-key/result-storage design, outside this scope.

---

## 4. Client Integration Models

### 4.1 AI Agent (LLM Tool Calling)

AI Agents interact with Party2 Re using **only two LLM Tools**:

1. `get_character_context(character_id)`
2. `execute_character_action(character_id, action, params)`

```python
# Minimal Agent Loop
context = get_character_context(char_id)

while True:
    # Select from available_actions even while timers exist: wake/rescue
    # can be eligible. A ready timer does not certify settled work.
    # Poll with a bounded retry delay when no suitable action is chosen.
    # LLM selects action from context.available_actions
    chosen_action, params = llm.decide(context.available_actions)
    
    resp = execute_character_action(char_id, chosen_action, params)
    context = resp.context or get_character_context(char_id)
```

**Benefits**:
- Compact tool definitions: clients need not inject the entire REST specification into the model prompt.
- Zero URL hallucination: the LLM only selects from the authoritative `available_actions` list.

### 4.2 Web UI (Server-Driven UI)

1. Initial load calls `GET /context` to populate the global state store (Pinia / Redux).
2. Navigation buttons and facility menus are rendered dynamically from `available_actions`.
3. Action buttons submit `{ action, params }` to `POST /actions` once.
4. A non-null response context replaces the store's observation. If refresh
   failed, preserve the command result/error and re-fetch GET context without
   replaying POST. A transport failure does not authorize automatic replay.

### 4.3 Line / Discord Bots

Chatbot frameworks handle state transitions with zero routing boilerplate:
- When a user clicks an interactive button (e.g. "預金する"), the bot sends `action="bank_deposit"` and `params={"amount": ...}` to `POST /actions`.
- The bot replies with `result` and renders quick-reply buttons directly from the returned `context.available_actions`.
- If an error occurs, the bot displays the error message alongside the recovery buttons.

---

## 5. Architectural constraints

1. **Authentication & Ownership**:
   All gateway calls verify character ownership (`char.PlayerID == player.ID`) using `withAuthenticatedCharacter`.
2. **Domain Service Decoupling**:
   Transport handlers and the Action Dispatcher contain no business rules; they decode parameters, invoke application services, and format responses.
3. **Context refresh failure**:
   Preserve the command outcome and report refresh failure separately under
   section 3. Clients re-read context; the dispatcher never retries a mutation
   to repair a failed observation.

---

## 6. Related documents

- [STATUS](../../STATUS.md): major current capabilities, command coverage and gaps.
- [ROADMAP](../../ROADMAP.md): remaining milestones and migration direction.
- [OpenAPI](../api/openapi.json): registered routes and transport schemas.
- [Components](components.md#playercontext): observation/command responsibility boundaries.
- [Action preconditions](../design/action-preconditions.md): game-entry eligibility and behavioral evidence.
