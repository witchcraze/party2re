# Character Progression & Level Growth Design

## Purpose

This document defines the language-agnostic progression formulas, experience thresholds, level-up stat growth mechanics, and reincarnation (rebirth) rules for characters.

## Initial Character State

A newly created character starts with:
- **Level**: 1
- **Experience**: 0
- **Starting Gold**: 200
- **Base Stats Range**:
  - `MaxHP`: 30–32 (HP is initialized to MaxHP)
  - `MaxMP`: 6–8 (MP is initialized to MaxMP)
  - `Attack`: 6–8
  - `Defense`: 6–8
  - `Agility`: 6–8

## Experience & Level Thresholds

Experience is cumulative. To advance from a given `level` to `level + 1`, a character's total accumulated experience must reach the threshold:

$$\text{RequiredExp}(\text{level}) = \text{level}^2 \times 10$$

### Key Rules
- Progression begins at Level 1 and caps at Level 99 (`MaxLevel = 99`).
- Multiple levels can be gained simultaneously if the awarded experience exceeds multiple cumulative thresholds.
- When maximum level (99) is reached, no further level advancement occurs.

## Job-Based Stat Growth

When a character levels up, stat increases are determined by the character's assigned `JobDefinition`.

For each level gained:
- $\Delta\text{HP} = \text{random}(0, \text{HPGrowth}) + 1$ (HP always gains a guaranteed minimum of +1)
- $\Delta\text{MP} = \text{random}(0, \text{MPGrowth})$
- $\Delta\text{Attack} = \text{random}(0, \text{AttackGrowth})$
- $\Delta\text{Defense} = \text{random}(0, \text{DefenseGrowth})$
- $\Delta\text{Agility} = \text{random}(0, \text{AgilityGrowth})$

Where $\text{random}(0, N)$ returns an integer in the inclusive range $[0, N]$.

### Current HP/MP Preservation
Level advancement increases maximum HP and maximum MP (`MaxHP`, `MaxMP`). It deliberately does **not** restore current HP and MP (`HP`, `MP`). Recovery must be achieved through healing items or resting at an Inn.

## Character Rebirth (Reincarnation)

Rebirth allows a max-level character to reincarnate, trading level back to 1 for permanent stat bonuses.

### Requirements & Transitions
- **Eligibility**: Character must be Level 99 (`Level >= 99`).
- **State Changes upon Rebirth**:
  1. `RebirthCount` increases by 1 (`RebirthCount++`).
  2. `Level` is reset to 1.
  3. `Experience` is reset to 0.
  4. Base stats are reset and recalculated with permanent rebirth bonuses:
     $$\text{Bonus} = \text{RebirthCount} \times 5$$
     $$\text{MaxHP} = 30 + (\text{Bonus} \times 2)$$
     $$\text{MaxMP} = 10 + \text{Bonus}$$
     $$\text{Attack} = 10 + \text{Bonus}$$
     $$\text{Defense} = 10 + \text{Bonus}$$
     $$\text{Agility} = 10 + \text{Bonus}$$
  5. Current `HP` and `MP` are initialized to full maximum values.
