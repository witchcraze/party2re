# Cross-Domain Application Runtime Primitives

This document establishes the architecture for the **Cross-Domain Application Runtime Primitive Layer** in Party2. It defines universal interfaces and execution patterns for currency, inventory items, pessimistic row locking, and domain events across feature modules.

---

## 1. Problem & Context

Across feature domains (such as Inn, Blacksmith, Casino, Alchemy, Guild, and Flea Market), transactions that modify **wallet currencies** (Gold, Small Medals), **inventory items**, or **feature state** require pessimistic row locking (`SELECT ... FOR UPDATE`), balance verification, and transactional commits.

Historically, feature domains implemented transaction orchestration individually, creating four structural risks:

1. **Lock Inversion & Deadlocks**: Locking `characters`, `inventory_items`, and domain-specific tables in inconsistent orders across different feature domains causes runtime deadlocks under concurrent execution.
2. **Boundary Validation Inconsistencies**: Off-by-one or missing checks on exact balances (e.g. `balance >= cost` vs `balance > cost`) lead to subtle overdrafts or rejected legitimate transactions.
3. **Domain Boilerplate Bloat**: Feature domains must implement repetitive transaction management, row-lock queries, integer overflow checks (`SafeMultiply`), and rollback handling instead of focusing purely on business rules.
4. **Scattered Side-Effect Dispatching**: Observer hooks and domain side effects (achievements, activity feeds, news) were wired via ad-hoc function pointers, increasing constructor complexity.

---

## 2. Core Primitives Architecture

The runtime primitive layer centralizes transaction orchestration, resource validation, deterministic row-locking, and event dispatching within `internal/economy` and `internal/core/event`.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Feature Domain                           │
│           (e.g., Inn, Blacksmith, Casino, Alchemy)              │
└────────────────────────────────┬────────────────────────────────┘
                                 │ ExecuteTransaction(req, fn)
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│               Universal Transaction Runner                      │
│                 (internal/economy.Runner)                       │
│                                                                 │
│  1. RunInTx (Unit of Work Context)                              │
│  2. Deterministic Lock Tier 1: Characters (Rank 2)              │
│  3. Balance Validation & Atomic Deduction (Cost >= Balance)     │
│  4. Deterministic Lock Tier 2: Inventory Items (Rank 3)         │
│  5. Execute Domain Callback: fn(TxContext)                      │
│  6. Apply Resource Grants & Save Modified State                 │
│  7. Phase 1: Dispatch Synchronous In-Tx Events                  │
│  8. Commit Transaction                                          │
│  9. Phase 2: Dispatch Asynchronous Post-Commit Events           │
└────────────────┬───────────────────────────────┬────────────────┘
                 │                               │
                 ▼                               ▼
┌─────────────────────────────────┐ ┌─────────────────────────────┐
│    MariaDB Master (ACID Tx)     │ │    Domain Event Dispatcher  │
│ - characters (FOR UPDATE)       │ │     (internal/core/event)   │
│ - inventory_items (FOR UPDATE)  │ │ - In-Tx Sync Handlers       │
│ - Domain secondary tables       │ │ - Post-Commit Async Workers │
└─────────────────────────────────┘ └─────────────────────────────┘
```

---

## 3. Standard Resource Abstractions

### 3.1 ResourceCost

`ResourceCost` defines the resources required to execute a domain operation. All values must be non-negative.

```go
type ResourceCost struct {
    Gold              int
    SmallMedals       int
    ItemInstanceID    string
    ItemInstanceQty   int
    ItemDefinitionID  string
    ItemDefinitionQty int
}
```

- **Currency Costs**: Deducted directly from `Character` under exclusive row lock.
- **Item Instance Cost**: Deducts `ItemInstanceQty` from a specific inventory item instance ID (e.g. equipment enhancement material).
- **Item Definition Cost**: Deducts `ItemDefinitionQty` across any instances matching `ItemDefinitionID` (e.g. general alchemy catalysts).
- **Invariants**:
  - `Gold >= 0`, `SmallMedals >= 0`, quantities `>= 0`.
  - `char.Money >= cost.Gold` (returns `economy.ErrInsufficientGold` on violation).
  - `char.SmallMedals >= cost.SmallMedals` (returns `economy.ErrInsufficientMedals` on violation).
  - `inv.Quantity(definitionID) >= cost.ItemDefinitionQty` (returns `economy.ErrInsufficientItemQuantity` on violation).

### 3.2 ResourceGrant

`ResourceGrant` defines resources awarded upon successful transaction completion:

```go
type ResourceGrant struct {
    Gold             int
    SmallMedals      int
    ItemDefinitionID string
    ItemQuantity     int
}
```

- Grants are applied atomically before transaction commit.
- Item instances are generated with unique IDs and added to the character's inventory with capacity enforcement (`ErrInventoryFull`).

---

## 4. Universal Transaction Runner (`economy.TransactionRunner`)

### 4.1 Interface Contract

```go
type TransactionRunner interface {
    ExecuteTransaction(ctx context.Context, req TransactionRequest, fn TransactionCallback) (*TransactionResult, error)
}
```

### 4.2 Request and Execution Context

```go
type TransactionRequest struct {
    CharacterID   string
    Cost          ResourceCost
    CostFunc      func(char corecharacter.Character) (ResourceCost, error)
    Grant         ResourceGrant
    LockInventory bool
}

type TxContext struct {
    context.Context
    Character corecharacter.Character
    Inventory coreinventory.Inventory
}

func (tc *TxContext) AddGrant(grant ResourceGrant)
func (tc *TxContext) EmitEvent(evt event.Event)
```

- **Dynamic Cost Resolution (`CostFunc`)**: Supports operations whose fee depends on the locked character state (e.g., Inn fee scaled by `char.Level`, guild donation limits).
- **Selective Inventory Locking (`LockInventory`)**: If neither cost nor grant specifies items and `LockInventory` is false, the inventory row lock is omitted, avoiding unnecessary lock contention.

### 4.3 Deterministic Locking Law

To prevent deadlocks mechanically, `ExecuteTransaction` enforces the global lock hierarchy defined in `.agents/rules/05-database-and-caching.md`:

1. **Rank 2 (`characters`)**: Acquired first via `FindByIDForUpdate(txCtx, characterID)`.
2. **Rank 3 (`inventory_items`)**: Acquired second via `FindByCharacterIDForUpdate(txCtx, characterID)` if items are involved or requested.
3. **Rank 8 (Domain secondary records)**: Acquired inside `fn(tc)` within the domain repository (e.g. `inn_records`, `blacksmith_enhancements`).

At no point may a secondary domain table or inventory table be locked prior to `characters`.

### 4.4 Architectural Boundaries: Single-Character Runner vs. Multi-Aggregate and P2P Transactions

Party2 transaction execution distinguishes two distinct operational scopes to avoid God-object bloat while guaranteeing deterministic concurrency control:

#### 1. Single-Character Currency & Inventory Transactions (`economy.TransactionRunner`)
- **Scope**: Operations modifying a single character's wallet, medals, and/or active inventory items (e.g. equipment enhancement in `blacksmith`, casino chip purchase/payout in `casino`, simple shop purchases in `shop`).
- **Execution**: MUST use `economy.TransactionRunner` (`ExecuteTransaction` or `economy.Run[T]`).
- **Guarantee**: Automatically encapsulates Rank 2 (`characters`) -> Rank 3 (`inventory_items`) locking order, balance validation, item capacity checks, and two-phase domain event dispatching.

#### 2. Multi-Aggregate Transactions
- **Scope**: Operations mutating state across multiple storage systems or secondary domain tables (e.g. shop purchases auto-delivering overflow to `character_depots` [Rank 5] in `secretshop`, black market rare item sacrifices affecting Depot items in `blackmarket`, gem store synthesis mutating `gem_boxes` [Rank 8] and Depot items in `gemstore`).
- **Execution**: Feature services inject a `TransactionProvider` (`RunInTx(ctx, fn) error`, matching `economy.TransactionProvider`).
- **Rationale**: `economy` cannot directly reference `depot`, `gemstore`, `store`, or `fleamarket` without introducing cyclic dependencies or bloating `economy` into a monolithic God object.
- **Guarantee**: Domain callbacks must manually enforce ascending lock ranks:
  $$\text{Rank 2 (characters)} \longrightarrow \text{Rank 3 (inventory\_items)} \longrightarrow \text{Rank 5 (character\_depots)} \longrightarrow \text{Rank 8 (domain records)}$$

#### 3. Peer-to-Peer (P2P) Transactions
- **Scope**: Transfers or direct trades between two distinct characters (e.g. `TransferGold` in `economy`, item/money mailing in `depot.SendMoney` and `depot.SendItem`, customer purchasing in `store.Sales`, P2P trade in `fleamarket.PurchaseListing`, auction bidding in `auction`).
- **Execution**: Uses `id.Sort2(charID1, charID2)` to establish deterministic lexicographical ordering.
- **Guarantee**: Always acquires Rank 2 character locks in ascending order before acquiring Rank 3 inventory or Rank 5 depot locks:
  ```go
  firstID, secondID := id.Sort2(buyerID, sellerID)
  char1, err := s.characterRepo.FindByIDForUpdate(txCtx, firstID)
  char2, err := s.characterRepo.FindByIDForUpdate(txCtx, secondID)
  // Subsequent Rank 3 inventory or Rank 5 depot locks...
  ```
- **Prohibition**: Feature services MUST NEVER import `internal/database` directly to invoke raw `database.RunInTx(ctx, db, ...)` or hold raw `*sql.DB` references. All transaction boundaries must be injected via interfaces.

---

## 5. Domain Event Dispatcher Integration (`internal/core/event`)

Aligning with RFC #355, the runtime layer provides an in-process, two-phase event dispatcher:

```go
type Event interface {
    EventName() string
}

type Handler func(ctx context.Context, evt Event) error

type Dispatcher struct { /* ... */ }
```

### 5.1 Two-Phase Dispatch Semantics

1. **Phase 1: In-Transaction Synchronous Handlers (`SubscribeSync`)**
   - Executes inside the active SQL transaction context.
   - Enforces ACID consistency: if a synchronous handler returns an error, the entire transaction is rolled back.
   - Used for critical invariants (e.g. quest progression directly dependent on the purchase).
2. **Phase 2: Post-Commit Asynchronous Handlers (`SubscribeAsync`)**
   - Executes only *after* the database transaction commits successfully.
   - Operates with **At-most-once** delivery. Handlers run concurrently or in worker goroutines.
   - Used for non-critical side effects (e.g. milestone news announcements, activity feed logging, cache invalidation).

---

## 6. Pilot Domain Migration: Inn (`internal/inn` - Historical)

> [!NOTE]
> The Inn domain was used as the historical pilot for `economy.TransactionRunner` in #411. In Issue #459, the fictional paid Inn was decommissioned in accordance with legacy Party2 clean-room specifications (resting is free at Home via `internal/home`). The implementation pattern below is preserved for architectural reference.

The Inn domain was selected as the pilot migration target:

### Before Migration
- Manual `runInTx` orchestration.
- Explicit `FindByIDForUpdate` on character repository.
- Manual fee calculation and balance check: `if char.Money < fee`.
- Manual deduction: `char.DeductMoney(fee)`.
- Manual HP/MP restoration.
- Manual repository update: `characters.Update(txCtx, char)`.
- 124 lines of repetitive transaction boilerplate.

### After Migration
- Injects `economy.TransactionRunner` (or `*economy.Service`).
- Declares dynamic cost via `CostFunc: func(c) { return ResourceCost{Gold: s.CalculateFee(c.Level)} }`.
- Executes pure domain logic inside callback:
  ```go
  res, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
      tc.Character.Stats.HP = tc.Character.Stats.MaxHP
      tc.Character.Stats.MP = tc.Character.Stats.MaxMP
      return nil
  })
  ```
- 0 SQL row lock boilerplate, 0 manual balance checks, guaranteed deadlock freedom.

---

## 7. Migrated Domain: Blacksmith (`internal/blacksmith`)

The Blacksmith domain migrated in Issue #426, demonstrating cross-resource transactions combining currency (Gold) and inventory items (upgrade materials):

### Before Migration
- Manual `runInTx` orchestration with fallback transaction repositories.
- Explicit `FindByIDForUpdate` and `FindByCharacterIDForUpdate` calls.
- Manual balance verification (`char.Money < goldCost`) and manual inventory item deduction loops.
- Manual dual-table saving (`characters.Update` + `inventories.Save` or `CommitEnhancement`).

### After Migration
- Injects `economy.TransactionRunner` (or `*economy.Service`) via `WithEconomy` / `WithTransactionRunner`.
- Pre-calculates static fee and material requirements from equipment level and price.
- Dispatches atomic transaction via `runner.ExecuteTransaction`:
  ```go
  req := economy.TransactionRequest{
      CharacterID: characterID,
      Cost: economy.ResourceCost{
          Gold:              goldCost,
          ItemDefinitionID:  s.materialID,
          ItemDefinitionQty: materialCost,
      },
      LockInventory: true,
  }
  res, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
      // Re-verify target equipment under lock, roll RNG, and increment EnhancementLevel
      if success {
          lockedItem.EnhancementLevel++
          return tc.Inventory.Update(lockedItem)
      }
      return nil
  })
  ```
- 0 manual row locks, guaranteed Rank 2 (`characters`) -> Rank 3 (`inventory_items`) lock order, atomic fee and catalyst consumption.

## 8. Migrated Domain: Casino (`internal/casino`)

The Casino domain migrated in Issue #427, demonstrating cross-domain currency conversions and wager settlements:

### Before Migration
- Manual `runInTx` orchestration for coin exchanges (`ExchangeGoldToCoins`, `ExchangeCoinsToGold`).
- Potential lock inversion hazard: `ExchangeCoinsToGold` previously updated `casino_accounts` (Rank 8) before `characters` (Rank 2), while `ExchangeGoldToCoins` updated `characters` before `casino_accounts`.
- Foreign key verification deadlock hazard on `casino_accounts (character_id) REFERENCES characters (id)` during payouts using `INSERT ... ON DUPLICATE KEY UPDATE` while holding `casino_accounts` locks.

### After Migration
- Injects `economy.TransactionRunner` (or `*economy.Service`) via `WithEconomy` / `WithTransactionRunner`.
- `ExchangeGoldToCoins` executes transaction with `Cost.Gold: goldCost` and callback incrementing casino coins:
  ```go
  req := economy.TransactionRequest{
      CharacterID: characterID,
      Cost: economy.ResourceCost{Gold: goldCost},
  }
  res, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
      acc, err := s.repo.AdjustCoins(ctx, characterID, coins)
      if err != nil {
          return err
      }
      latestAcc = acc
      return nil
  })
  ```
- `ExchangeCoinsToGold` executes transaction with `Grant.Gold: goldReward` and callback decrementing casino coins.
- Strict `balance >= cost` semantics across Slot, Indian Poker, Doppelganger, and HighLow games.
- Deterministic lock order: `characters` (Rank 2) is ALWAYS locked before `casino_accounts` (Rank 8). In repository layer, `DeductBetAndCreditPayout` avoids `INSERT` during normal play when updating existing accounts, preventing implicit foreign key S-lock deadlocks.
- Verified under 50 concurrent workers executing 1,000 mixed exchange and bet operations with 0 deadlocks.

---

## 9. Migrated Domain: Depot (`internal/depot`)

The Depot domain migrated in Issue #445, eliminating dual-execution fallback logic and standardizing all gold and item storage operations onto `economy.TransactionRunner`:

### Before Migration
- Dual execution paths (`if s.runner != nil` vs fallback manual repository transaction).
- Redundant `sqlDepotTx` abstraction duplicating transaction orchestration.
- Complex nested fallback tests attempting to simulate transactionless environments.

### After Migration
- Injects `economy.TransactionRunner` (or `*economy.Service`) via `WithEconomy` / `WithTransactionRunner`.
- Auto-wires `economy.Service` in `NewService` when runner is not supplied.
- `DepositGold` executes with `Cost.Gold: amount` and callback incrementing depot gold balance.
- `WithdrawGold` executes transaction callback decrementing depot gold balance and dispatches `Grant.Gold: amount`.
- `DepositItem` and `WithdrawItem` execute with `LockInventory: true` ensuring deterministic lock order: `characters` (Rank 2) -> `inventory_items` (Rank 3) -> `character_depots` (Rank 5).
- Code size reduced from 473 lines down to 316 lines with 96.6% unit test coverage.

---

## 10. Feature Domain Transaction Status and Roadmap

Following the establishment of the single-character `TransactionRunner` and multi-aggregate / P2P `TransactionProvider` patterns, domain status is tracked as follows:

| Domain | Scope | Status | Execution Pattern & Primary Benefit |
|---|---|---|---|
| **Inn** (`internal/inn`) | Resting HP/MP recovery, level-scaled fee | Decommissioned (#459) | Fictional domain removed; replaced by free Home sleep (`internal/home`) |
| **Blacksmith** (`internal/blacksmith`) | Equipment enhancement, upgrade materials | Migrated (#426) | `economy.TransactionRunner`: eliminates manual inventory + character dual locking |
| **Casino** (`internal/casino`) | Poker, Slot, Doppelganger, HighLow bet & payout | Migrated (#427) | `economy.TransactionRunner`: unifies coin exchange and wager settlement with strict balance checking |
| **Shop** (`internal/shop`) | Standard purchases and resale | Migrated (#446) | `economy.TransactionRunner`: deterministic Rank 2 -> 3 -> 5 locking for auto-depot delivery |
| **Depot** (`internal/depot`) | Storage, expansions, P2P send | Migrated (#445, #554, #562) | `economy.TransactionRunner` for internal moves; `TransactionProvider` + `id.Sort2` for P2P transfers |
| **SecretShop** (`internal/secretshop`) | Secret item purchases and auto-depot transfer | Migrated (#554) | `economy.TransactionRunner` + `TransactionProvider` fallback for Depot delivery under Rank 2 -> 3 -> 5 |
| **BlackMarket** (`internal/blackmarket`) | Rare item sacrifice, prize trade | Migrated (#554) | `TransactionProvider`: multi-aggregate storage consumption and prize delivery under Rank 2 -> 3 -> 5 |
| **GemStore** (`internal/gemstore`) | Gem Box storage, synthesis, appraisal, P2P send | Migrated (#554) | `TransactionProvider`: P2P transfers (`id.Sort2`) and multi-aggregate synthesis under Rank 2 -> 3 -> 5 -> 8 |
| **Store** (`internal/store`) | Player store sales and customer delivery | Migrated (#555) | `TransactionProvider`: P2P customer purchases (`id.Sort2`) under Rank 2 -> 3 -> 5 -> 8 |
| **FleaMarket** (`internal/fleamarket`) | P2P item listings and purchase escrow | Migrated (#555) | `TransactionProvider`: two-party deterministic locking with `id.Sort2` under Rank 0 -> 2 -> 3 -> 5 |
| **Auction** (`internal/auction`) | P2P live bidding and direct trade | Migrated (#556) | `TransactionProvider`: two-party deterministic locking with `id.Sort2` under Rank 0 -> 2 -> 3 |
| **Alchemy** (`internal/alchemy`) | Multi-ingredient consumption and item synthesis | Planned | Streamlines recipe validation and batch inventory deductions |
| **Guild** (`internal/guild`) | Guild founding fee, Gold donations | Planned | Standardizes donation limits and deterministic locking |


