# Bank System Design

## Overview

The Bank (銀行) system provides safe long-term gold storage and player-to-player remittances. Active wallet gold held by characters is subject to loss or spending; depositing into the Bank ensures safety and enables currency transfers between player accounts.

## Domain Model

### Bank Account (`Account`)
- **PlayerID**: Associated player account ID.
- **Balance**: Total gold balance (`int64`).
- **UpdatedAt**: Timestamp of last modification.

### Operations
- **Deposit (`Deposit`)**:
  - Moves gold from active character wallet (`Character.Money`) to the player's bank account.
  - Rejects with `ErrInsufficientFunds` if character money is less than requested amount.
- **Withdrawal (`Withdraw`)**:
  - Moves gold from player's bank account to character wallet.
  - Rejects with `ErrInsufficientBalance` if bank balance is less than requested amount (no overdrafts permitted).
- **Direct Transfer (`Transfer`)**:
  - Moves gold directly between two player bank accounts (`from_player_id` -> `to_player_id`).
  - Rejects self-transfers (`ErrSelfTransfer`).
  - Creates an immutable audit record in `bank_transfers`.

## Transaction & Concurrency Invariants

- All multi-table mutations (character wallet update + bank balance update, or player-to-player transfer) are executed inside an atomic MariaDB transaction (`*sql.Tx`).
- Row locking (`FOR UPDATE`) is used on both character and bank account records to prevent race conditions during concurrent deposits, withdrawals, and remittances.
