# Client & Agent API — CQRS & Server-Driven UI Architecture

This document describes the implemented HTTP observation boundary and the planned command gateway.

| Boundary | Status | Tracking |
|---|---|---|
| Action catalog and OpenAPI drift checks | Implemented | #944, #946 |
| PlayerContext evaluator and character/scheduling reads | Implemented in-process; legacy parity still requires review | #938, #972 |
| HTTP context query | Implemented; owned character observation with four canonical slots | #939 |
| HTTP action dispatcher | Planned; failure/refresh semantics need specification | #646 |
| Individual REST route retirement | Planned after Gateway migration | #947–#950 |

Current clients use the registered routes in [OpenAPI](../api/openapi.json).
`GET /api/v1/characters/{id}/context` is registered. Existing REST routes remain
unversioned; `POST /actions` is planned. Do not remove working REST contracts
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
| **Query (Observe)** | `GET /api/v1/characters/{id}/context` | **Observation & Next Moves** | Lightweight character snapshot, active timers, and **whitelist of currently executable actions**. |
| **Command (Execute)** | `POST /api/v1/characters/{id}/actions` | **State Transition** | Accepts `{ action, params }`, dispatches to domain services, and returns `{ result, context }`. |

---

## 2. Query Pillar: `GET /api/v1/characters/{id}/context`

The authenticated discovery endpoint verifies character ownership using the
standard HTTP wrapper, calls the existing uncached `playercontext.Service.Query`,
and enriches its facts with profile avatar and job catalog presentation. Its
`PlayerContextResponse` DTO and reusable OpenAPI schema are also the required
context contract for the future #646 command gateway.

The four top-level fields are:

- `character`: ID/name/job ID and name, level, HP/MP maxima and current values,
  wallet gold, fatigue, death and sleeping flags, and icon fields. `icon_url`
  uses the existing profile AvatarURL (URL or data URI), or an empty string.
  The asset ID remains empty until production mappings are specified.
- `scene`: the initial `town` hub, title, background ID/URL, dialogue, and
  optional structured speaker/opponent. The background is a self-authored SVG
  data URI placeholder; no legacy images are reused. Production art resolution
  remains #654/#729, and dynamic facility/combat scenes remain #947–#949.
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

The universal command execution gateway.

### Request Body

```json
{
  "action": "bank_deposit",
  "params": {
    "amount": 5000
  }
}
```

### Success Response (200 OK)

Returns both the domain execution result AND the refreshed client context in a single round-trip:

```json
{
  "success": true,
  "result": {
    "deposited": 5000,
    "balance": 15000
  },
  "context": {
    "character": {
      "id": "char-123",
      "gold": 7000,
      "tired": 20
    },
    "ongoing_actions": [],
    "available_actions": [
      {
        "action": "bank_withdraw",
        "label": "預金を引き出す",
        "category": "economy",
        "required_params": ["amount"]
      }
    ]
  }
}
```

### Domain Error Response (4xx)

When domain preconditions fail (e.g. insufficient gold, exhausted stamina), the gateway returns structured error details alongside the refreshed context so the client knows what actions ARE available to recover:

```json
{
  "success": false,
  "error": {
    "code": "FATIGUE_LIMIT_REACHED",
    "message": "疲労度が100%です。自宅で休んでください。"
  },
  "context": {
    "character": { ... },
    "ongoing_actions": [],
    "available_actions": [
      {
        "action": "home_sleep",
        "label": "自宅で休む",
        "category": "home",
        "required_params": []
      }
    ]
  }
}
```

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
3. Action buttons submit `{ action, params }` to `POST /actions`.
4. The response directly replaces the store's `context`, automatically updating UI buttons, HP/MP bars, and timers without separate reload calls.

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
   #646 must specify how a successful mutation and a subsequent context-read
   failure are reported. A retry must not duplicate the mutation. The examples
   above show the intended normal response, not an implemented failure contract.

---

## 6. Related Issues & Implementation Roadmap

### Phase 1: Gateway Core Construction
- **#944**: `[Specification] PlayerContext: Define Action Catalog & Legacy Precondition Matrix` (Master Action Catalog & Params Schema)
- **#946**: `[Architecture] PlayerContext: Automated Drift-Detection Test between Action Catalog and OpenAPI Specification` (Automated Schema Linting)
- **#938**: `[Feature] PlayerContext: Action Evaluator Engine with ScheduledAction Cooldown Gate` (Availability Filtering)
- **#939**: `[Feature] HTTP/PlayerContext: GET /context handler & Client Context Model` (Query Pillar)
- **#646**: `[Feature] Client/Agent: Unified Action Gateway Dispatcher (POST /actions) & Server-Driven Execution` (Command Pillar)

### Phase 2: Phased Legacy REST Purge & Migration
- **#947**: `[Architecture] API/Migration: Migrate Economy endpoints (Bank, Shop, Depot, Market) to Action Gateway and purge legacy routes`
- **#948**: `[Architecture] API/Migration: Migrate Combat & Adventure endpoints (Adventure, Dungeon, Boss, PvP) to Action Gateway and purge legacy routes`
- **#949**: `[Architecture] API/Migration: Migrate Town, Faith & Social endpoints (Home, Casino, Chapel, Guild) to Action Gateway and purge legacy routes`
- **#950**: `[Architecture] API/Cleanup: Shrink handler.go to <150 lines and purge legacy paths from OpenAPI specification`

### Phase 3: Client Verification & Presentation
- **#650**: `[Architecture] Test: Headless E2E Gameplay Simulation Test Architecture Design & Ticket Decomposition` (E2E Validation via Gateway)
- **#140**: `[Feature] Client Presentation: Web UI Client and Presentation Layer` (Server-Driven UI)
