# Chapel, Prayers & Blessings Design

## Overview

The Chapel and Blessings Feature Module (`internal/chapel`) provides player characters with active town church prayers and divine blessings granting reward and modifier boosts during gameplay.

---

## Domain Rules & Blessing Types

### Blessing Types (祈りの種類)

Characters can register one active blessing at the town chapel:

| Blessing Type | Key | Description | Game Modifier Effect |
| --- | --- | --- | --- |
| **None** | `NONE` | No active prayer | Standard rewards (1.0x) |
| **Gold Wish** (お金がほしい) | `GOLD` | Prayer for riches | 25% chance of 1.5x Gold reward on battle victory |
| **EXP Wish** (強くなりたい) | `EXP` | Prayer for strength | 25% chance of 1.5x EXP reward on battle victory |
| **Treasure Wish** (宝箱がほしい) | `DROP` | Prayer for loot | +10% item drop chance bonus in adventures |
| **Casino Wish** (コインがほしい) | `CASINO` | Prayer for luck | Casino luck / bonus coins |

---

## Modifiers Calculation

For victory reward settlements:
- **EXP Calculation**:
  $$\text{Final EXP} = \begin{cases} \lfloor \text{Base EXP} \times 1.5 \rfloor & \text{if } \text{Blessing} = \text{EXP} \land \text{Random}(0, 1) < 0.25 \\ \text{Base EXP} & \text{otherwise} \end{cases}$$
- **Gold Calculation**:
  $$\text{Final Gold} = \begin{cases} \lfloor \text{Base Gold} \times 1.5 \rfloor & \text{if } \text{Blessing} = \text{GOLD} \land \text{Random}(0, 1) < 0.25 \\ \text{Base Gold} & \text{otherwise} \end{cases}$$
- **Drop Rate Bonus**:
  $$\text{Final Drop Rate} = \text{Base Drop Rate} + 0.10 \quad (\text{if Blessing} = \text{DROP})$$

---

## Donations

- Characters can donate Gold to the Chapel.
- Donations increment `donation_gold_total` and are deducted transactionally from character wallet funds.
