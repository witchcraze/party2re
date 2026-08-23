# Character Item Depot & Storage Design

## Overview

The Item Depot (預かり所 / 倉庫) provides characters with persistent storage for item instances and gold outside of their active inventory.

## Domain Model

### `Depot`
- **`CharacterID`** (`string`): Unique identifier of the character owning the depot.
- **`Gold`** (`int`): Stored gold balance (non-negative integer).
- **`Capacity`** (`int`): Maximum distinct item instance slots (default: 50).
- **`Items`** (`[]item.Instance`): Stored item instances (each with unique instance ID, definition ID, and quantity).

## Operations & Invariants

### 1. Gold Storage
- **Deposit Gold**: Transits gold from character wallet to depot balance.
  - Invariant: `Character.Money >= amount` and `amount > 0`.
- **Withdraw Gold**: Transits gold from depot balance to character wallet.
  - Invariant: `Depot.Gold >= amount` and `amount > 0`.

### 2. Item Storage
- **Deposit Item**: Moves an item instance from character inventory to depot storage.
  - Invariant: Item exists in character inventory.
  - Invariant: `len(Depot.Items) < Depot.Capacity` (unless stacking with existing identical definition).
- **Withdraw Item**: Moves an item instance from depot storage to character inventory.
  - Invariant: Item exists in depot storage.
  - Invariant: Character inventory has available space (`len(Inventory.Items) < Inventory.Capacity`).

## Atomicity and Concurrency

All depot operations (`DepositGold`, `WithdrawGold`, `DepositItem`, `WithdrawItem`) execute atomically inside a single database transaction (`*sql.Tx`) across character stats, inventory items, character depot metadata, and depot item records.
