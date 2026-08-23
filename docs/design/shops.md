# Item Shop and Commerce Design

## Purpose

This document describes the behavior and economic rules governing item shop purchases and sales in Party2.

## Core Concepts

Town shops allow characters to purchase equipment and consumable items with gold, and sell unwanted or obsolete items back to merchants.

### Purchase Rules

1. **Catalog Availability**:
   - Items available for purchase are defined by the item catalog.
   - Each item definition specifies a standard purchase price (`Price` in gold).

2. **Transaction Requirements**:
   - A character must possess at least `total_price = item.Price * quantity` in gold (`character.Money >= total_price`).
   - If funds are insufficient, the transaction is rejected without altering character gold or inventory.

3. **Purchase Fulfillment**:
   - Upon successful purchase:
     - `total_price` is deducted from the character's gold.
     - A new `item.Instance` is created and added to the character's inventory.
     - The gold deduction and inventory addition must be committed atomically.

### Sale Rules

1. **Eligible Items**:
   - A character may sell any item instance currently held in their active inventory.
   - The item definition corresponding to the instance must exist in the catalog to determine its base price.

2. **Sell Value Calculation**:
   - The standard resale value of an item is **50% of its base catalog price**:
     $$\text{SellPrice} = \lfloor \text{Price} \times 0.5 \rfloor$$
   - Total payout for selling multiple quantities:
     $$\text{TotalPayout} = \text{SellPrice} \times \text{quantity}$$

3. **Sale Fulfillment**:
   - Upon successful sale:
     - The specified quantity of the item instance is consumed/removed from the character's inventory.
     - `TotalPayout` in gold is added to the character's wallet (`character.Money += TotalPayout`).
     - The inventory removal and currency credit must be committed atomically.

### Transaction Invariants

- Transactions cannot create negative gold balances.
- Selling unowned items or consuming more quantity than owned is rejected.
- Concurrent transactions must not permit double-spending of gold or duplicate selling of item instances.
