# Character Progression & Level Growth Design

## Purpose

This document defines the language-agnostic progression formulas, experience thresholds, level-up stat growth mechanics, skill point (SP) accumulation, and celestial OverLevel rules for characters.

## Initial Character State

A newly created character starts with:
- **Level**: 1
- **Experience**: 0
- **Skill Points (SP)**: 0
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
- Progression begins at Level 1.
- Default level cap is Level 99 (`MaxLevel = 99`).
- If the character has unlocked celestial OverLevel (`OverLevel == true`), the level cap is extended to Level 150 (`OverMaxLevel = 150`).
- Multiple levels can be gained simultaneously if awarded experience exceeds multiple cumulative thresholds.
- When maximum level (99, or 150 with OverLevel) is reached, no further level advancement occurs.

## Skill Points (SP) & Skill Acquisition

In the original Party2 CGI (`party2/lib/_battle.cgi`), characters gain Skill Points upon leveling up and learn skills when their accumulated SP matches a skill's requirement.

### SP Gain on Level-Up
For each level gained:
1. `SP` is incremented by 1 (`SP++`).
2. If the character possesses "スキルの宝珠" (Skill Orb, item ID `157`), there is a 25% chance to gain an additional +1 SP (`roll < 1` out of 4).

### SP-Based Skill Learning
When a character levels up (or gains SP), the system inspects the skill definitions for the character's current job:
- A skill is automatically learned when the character's accumulated SP exactly equals the skill's required SP:
  $$\text{character.SP} == \text{skill.RequiredSP}$$
- Skill learning is governed strictly by SP thresholds, not character level.

## Job-Based Stat Growth

When a character levels up, stat increases are determined by the character's assigned `JobDefinition`.

For each level gained:
- $\Delta\text{HP} = \text{growthValue}(\text{HPGrowth}) + 1$ (HP always gains a guaranteed minimum of +1)
- $\Delta\text{MP} = \text{growthValue}(\text{MPGrowth})$
- $\Delta\text{Attack} = \text{growthValue}(\text{AttackGrowth})$
- $\Delta\text{Defense} = \text{growthValue}(\text{DefenseGrowth})$
- $\Delta\text{Agility} = \text{growthValue}(\text{AgilityGrowth})$

### Stat Growth Formula & Cap Re-roll
The growth value is generated from a uniform random range $[0, \text{MaxGrowth}]$. If the resulting increase exceeds 9, the legacy CGI formula re-rolls the value within $[1, 9]$:

```text
v = random(0, max)
if v > 9:
    v = random(1, 9)
```

### Current HP/MP Preservation
Level advancement increases maximum HP and maximum MP (`MaxHP`, `MaxMP`). It deliberately does **not** restore current HP and MP (`HP`, `MP`). Recovery must be achieved through healing items or resting at an Inn.

## Celestial OverLevel (天界限界突破)

OverLevel allows characters that have reached the celestial realm to progress beyond the standard Lv99 cap:
- **Maximum Level**: Extends from Lv99 to Lv150.
- **Job Change Reset**: Changing jobs resets the `OverLevel` flag to `false`, requiring re-ascension.
- **Fictional Rebirth Eliminated**: The original Party2 CGI contains no reincarnation/rebirth loop. The clicker-style `RebirthCount` and rebirth stat multipliers are completely eliminated in favor of celestial OverLevel.
