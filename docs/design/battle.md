# Battle System Design

## Purpose

This document defines the language-agnostic game rules, resolution formulas, and contracts for the Battle component.

Battle is an independent, reusable domain component. It resolves combat between participants and determines outcomes without knowing why the battle was started (e.g., adventure quest, arena, dungeon, or guild battle).

## Domain Model

### Participant
A participant in combat possesses fundamental combat attributes:
- `ID`: Unique participant identifier (e.g., character ID or monster ID).
- `HP`: Current and maximum hit points (must be > 0).
- `Attack`: Offensive power (≥ 0).
- `Defense`: Defensive resilience (≥ 0).

### Request
A battle resolution request consists of:
- `Participants`: An ordered pair of participants `[first, second]`.
- `VictoryReward`: Reward selected when the first participant wins.
- `DefeatReward`: Reward selected when the first participant loses.
- `DrawReward`: Reward selected in case of a draw.

### Reward Model
A reward bundle can include:
- `Experience`: Experience points awarded (≥ 0).
- `Currency`: Gold currency awarded (≥ 0).
- `ItemDefinitionID`: Optional item catalog identifier dropped.
- `ItemQuantity`: Quantity of dropped items (≥ 1 if item ID is present).

## Resolution Algorithm & Formulas

### Turn-Based Combat Loop
1. Battle runs deterministically in alternating turns between `first` and `second`.
2. In each turn:
   - `first` attacks `second`, inflicting damage calculated as:
     $$\text{Damage} = \max(1, \text{Attack}_{\text{first}} - \text{Defense}_{\text{second}})$$
   - If `second`'s HP drops to $\le 0$:
     - Check if simultaneous counter-damage would reduce `first`'s HP to $\le 0$ (simultaneous knockout). If so, outcome is `Draw` with `DrawReward`.
     - Otherwise, `first` is declared the `Winner` with outcome `Win` and `VictoryReward`.
   - `second` attacks `first`, inflicting damage calculated as:
     $$\text{Damage} = \max(1, \text{Attack}_{\text{second}} - \text{Defense}_{\text{first}})$$
   - If `first`'s HP drops to $\le 0$, `second` is declared the `Winner`, and `first` receives `DefeatReward`.
3. The loop terminates when one or both participants' HP is reduced to 0.

### Outcome Types
- `OutcomeWin`: One participant defeated the other.
- `OutcomeDraw`: Both combatants were defeated in the same exchange.

## Boundaries & Invariants

- **Context Isolation**: The Battle engine never queries database persistence or mutates character state directly. It returns a `Result` containing the selected reward.
- **Consumer Ownership**: The caller (e.g., Adventure feature, PvP service) is responsible for applying the returned reward to the character or inventory atomically.
- **Minimum Damage Guarantee**: Every attack inflicts at least 1 damage, guaranteeing termination even against high defense.
