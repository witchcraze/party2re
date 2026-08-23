# Character Resting and Recovery

## Purpose

This document describes the design and behavioral rules for character resting and recovery facilities.

## Rules

### 1. Inn Recovery

After combat and adventure activities, a character's current HP and MP can be depleted.
The Town Inn provides full restoration:

- **Recovery effect**: Current HP is restored to Max HP; current MP is restored to Max MP.
- **Cost**: A fee in gold is required, scaling by character level (defaulting to 5 gold per level, with a minimum base fee of 5 gold).
- **Invariants**:
  - The character must have sufficient currency (`Money >= Fee`).
  - If currency is insufficient, the resting attempt is rejected without modifying character HP, MP, or money.
  - Upon successful rest, the fee is deducted and stats are updated atomically.

## Related Documents

- [`game-overview.md`](game-overview.md)
- [`../architecture/feature-modules.md`](../architecture/feature-modules.md)
