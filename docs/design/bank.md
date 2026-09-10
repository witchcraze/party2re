# Bank System Design

## Overview

The Bank (銀行) system provides secure gold storage managed by the bank teller NPC Taxeed (`@タクシード`, asset: `bank.gif`), matching the original Party2 Perl CGI specification (`party2/lib/bank.cgi`).

Savings are stored per character (`Character.Deposit int64`) rather than at the player account level. The bank provides zero-fee, 24/7 gold deposits and withdrawals with an upper limit of 99,999,999,999,999 G (99兆9999億9999万9999 G). Direct transfers between players are not part of the legacy bank specification and are handled via postal depot mechanics.

## Domain Model

### Character Bank Deposit (`Character.Deposit`)
- **CharacterID**: Target character identifier.
- **Money**: Active wallet gold (clamped to `MaxWallet = 999,999 G` upon withdrawal).
- **Deposit**: Bank deposit balance (`int64`, up to `MaxDeposit = 99,999,999,999,999 G`).

### NPC Receptionist: Taxeed (`@タクシード`)
- **Asset**: `bank.gif`
- **Standard Dialogues**:
  - `"ゴールドをお預かりいたします"`
  - `"24時間いつでも、手数料もございません"`
  - `"99999999999999 Gまでお預かりいたします"`
- **Interaction Messages**:
  - On Deposit: `"{amount} Gお預かりいたしました"`
  - On Withdraw: `"{amount} Gお返しいたします"`
  - On Limit Exceeded: `"これ以上お預かりできません"`

### Operations

- **Deposit (`Deposit`)**:
  - Deposits gold from character wallet (`Character.Money`) into bank savings (`Character.Deposit`).
  - Validates `amount > 0` and character wallet has sufficient funds (`Money >= amount`).
  - Enforces `Deposit + amount <= MaxDeposit` (99,999,999,999,999 G). Rejects with `ErrDepositLimitExceeded` if exceeded.
  - Updates character wallet: `money -= amount`, and bank savings: `deposit += amount`.

- **Withdrawal (`Withdraw`)**:
  - Withdraws gold from bank savings (`Character.Deposit`) into character wallet (`Character.Money`).
  - Validates `amount > 0` and bank savings has sufficient funds (`Deposit >= amount`).
  - Legacy Wallet Clamp (`999,999 G`):
    - When gold is withdrawn, wallet money is incremented by requested amount and deposit is decremented.
    - If `money > MaxWallet (999,999)`:
      - Excess gold `excess = money - MaxWallet` is automatically refunded back to `deposit`.
      - Character wallet `money` is set to `999,999`.
      - Actual gold added to wallet is `requested - excess`.
    - Formula:
      ```
      deposit -= amount
      money += amount
      if money > 999999:
          refund = money - 999999
          deposit += refund
          money = 999999
      ```

## Transaction & Concurrency Invariants

- Deposits and withdrawals operate strictly within the `characters` table using Tier 2 row locking (`SELECT ... FOR UPDATE`).
- Deadlock-free by design: each bank operation targets a single character record, eliminating multi-row lock ordering concerns.
- Character bank deposits are included in wealth rankings (`characters.money + characters.deposit`).
