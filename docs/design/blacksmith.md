# Blacksmith Item Enhancement & Refinement Design

## Overview

The Blacksmith (鍛冶屋) enables characters to upgrade equipment (weapons, armors, shields, accessories) by increasing their enhancement level (`+1` to `+10`) in exchange for gold and upgrade materials.

## Domain Model

### Enhancement Level
- Base level: `+0`
- Maximum level: `+10` (`MaxEnhancementLevel`)

### Enhancement Cost Calculation
- **Gold Cost**: `basePrice * (currentLevel + 1) / 2` (minimum 50 Gold)
- **Material Cost**: `1 + currentLevel / 3` (e.g. `item-084` 魔石のカケラ)

### Success Rate Schedule
- Level 0 -> +1: 100% (1.00)
- Level 1 -> +2: 95% (0.95)
- Level 2 -> +3: 90% (0.90)
- Level 3 -> +4: 80% (0.80)
- Level 4 -> +5: 70% (0.70)
- Level 5 -> +6: 60% (0.60)
- Level 6 -> +7: 50% (0.50)
- Level 7 -> +8: 40% (0.40)
- Level 8 -> +9: 30% (0.30)
- Level 9 -> +10: 20% (0.20)

### Stats Bonus Calculation
- **Weapon Attack Bonus**: `baseAttack * currentLevel / 10 + currentLevel * 2`
- **Armor Defense Bonus**: `baseDefense * currentLevel / 10 + currentLevel * 2`

## Operations & Invariants

- **Eligibility**: Only items with equipment slots (`Slot != SlotNone`) can be enhanced.
- **Max Level**: Items at `+10` cannot be enhanced (`ErrMaxEnhancementReached`).
- **Success**: Increases item enhancement level by 1, consumes required gold and materials.
- **Failure**: Consumes required gold and materials without increasing enhancement level (equipment is preserved and not destroyed).
- **Atomicity**: Character money deduction, material consumption, and item enhancement update occur atomically in a single database transaction (`*sql.Tx`).
