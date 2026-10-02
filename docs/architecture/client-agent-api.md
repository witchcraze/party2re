# Client & Agent API — CQRS & Server-Driven UI Architecture

This document describes the enduring architecture for how clients (Web UI, AI Agents, Line Bot, Discord Bot) interact with the Party2 Re backend.

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
    "action_type": "adventure",
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
      "label": "宿屋で休む",
      "category": "home",
      "required_params": []
    }
  ]
}
```

### Invariants:
- **No Token Bloat**: `available_actions` contains **only top-level executable actions** (approx. 30–40 actions). Individual items or shop goods are manipulated within their respective sub-actions or parameters.
- **Strict Whitelist**: Actions restricted by HP (`hp <= 0`), Fatigue (`tired >= 100`), Sleep/Restraint (`ongoing_action != null`), or Money are automatically excluded by the Action Evaluator.
- **Smart Timer Sleep**: If `ongoing_action` is active, clients can inspect `remaining_seconds` and sleep without polling the server.

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
    "message": "疲労度が100%です。宿屋で休んでください。"
  },
  "context": {
    "character": { ... },
    "ongoing_action": null,
    "available_actions": [
      {
        "action": "home_sleep",
        "label": "宿屋で休む",
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
        sleep(context.ongoing_action.remaining_seconds)
    
    # LLM selects action from context.available_actions
    chosen_action, params = llm.decide(context.available_actions)
    
    resp = execute_character_action(char_id, chosen_action, params)
    context = resp.context  # Context updated automatically
```

**Benefits**:
- Zero OpenAPI prompt bloat: no need to inject 180+ endpoint definitions into LLM system prompts.
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

## 5. Architectural Invariants

1. **Authentication & Ownership**:
   All gateway calls verify character ownership (`char.PlayerID == player.ID`) using `withAuthenticatedCharacter`.
2. **Domain Service Decoupling**:
   Transport handlers and the Action Dispatcher contain no business rules; they decode parameters, invoke application services, and format responses.
3. **Best-Effort Context Refresh**:
   If context calculation encounters an unexpected failure, the mutation result is still committed and returned.

---

## 6. Related Issues & Implementation Roadmap

- **#944**: `[Specification] PlayerContext: Define Action Catalog & Legacy Precondition Matrix` (Master Action Catalog & Params Schema)
- **#938**: `[Feature] PlayerContext: Action Evaluator Engine with ScheduledAction Cooldown Gate` (Availability Filtering)
- **#939**: `[Feature] HTTP/PlayerContext: GET /context handler & Client Context Model` (Query Pillar)
- **#646**: `[Feature] Client/Agent: Unified Action Gateway Dispatcher (POST /actions) & Server-Driven Execution` (Command Pillar)
- **#650**: `[Architecture] Test: Headless E2E Gameplay Simulation Test Architecture Design & Ticket Decomposition` (E2E Validation)
