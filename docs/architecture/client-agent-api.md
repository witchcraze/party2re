# Client & Agent API — CQRS & Server-Driven UI Architecture

This document describes the planned client boundary, not the current HTTP API contract.

| Boundary | Status | Tracking |
|---|---|---|
| Action catalog and OpenAPI drift checks | Implemented | #944, #946 |
| PlayerContext evaluator and character/scheduling reads | Implemented in-process; legacy parity still requires review | #938, #972 |
| HTTP context query | Planned; not registered in the router | #939 |
| HTTP action dispatcher | Planned; failure/refresh semantics need specification | #646 |
| Individual REST route retirement | Planned after Gateway migration | #947–#950 |

Current clients use the registered routes in [OpenAPI](../api/openapi.json).
The `/api/v1` prefix and payloads below are proposed examples; the current
router uses unversioned paths. Do not remove working REST contracts before
their replacement is implemented and verified against the original Party2.

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

The discovery entry point for any client session.

### Response Specification

```json
{
  "character": {
    "id": "char-123",
    "name": "勇者太郎",
    "job_id": "warrior",
    "hp": 150,
    "max_hp": 200,
    "mp": 30,
    "max_mp": 50,
    "gold": 12000,
    "tired": 20
  },
  "ongoing_action": {
    "action_type": "activity:training_complete",
    "execute_at": "2026-10-02T10:05:00Z",
    "remaining_seconds": 120
  },
  "available_actions": [
    {
      "action": "bank_deposit",
      "label": "銀行に預ける",
      "category": "economy",
      "required_params": ["amount"]
    },
    {
      "action": "shop_weapon",
      "label": "武器屋へ行く",
      "category": "shop",
      "required_params": []
    },
    {
      "action": "home_sleep",
      "label": "自宅で休む",
      "category": "home",
      "required_params": []
    }
  ]
}
```

### Invariants:
- **Compact discovery**: `available_actions` contains top-level actions from the catalog. Individual items or shop goods use parameters or feature queries; do not duplicate the catalog count here.
- **Strict Whitelist**: Actions restricted by HP (`hp <= 0`), Fatigue (`tired >= 100`), Sleep/Restraint (`ongoing_action != null`), or Money are automatically excluded by the Action Evaluator.
- **Timer observation**: Pending/Processing work blocks applicable actions even when its deadline has passed. A deadline is not proof of completion. After waiting, fetch fresh context before choosing an action; use bounded retry delays for overdue work.

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
    "ongoing_action": null,
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
    "ongoing_action": null,
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
    if context.ongoing_action:
        sleep(max(context.ongoing_action.remaining_seconds, retry_delay))
        context = get_character_context(char_id)
        continue
    
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

## 5. Planned architectural constraints

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
