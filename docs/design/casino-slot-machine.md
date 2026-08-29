# Casino Slot Machine Design

## Overview

The Slot Machine (スロットマシン) is a single-player casino wagering game. Players wager casino coins to spin 3 reels containing 5 distinct symbols. Symbol combinations along the payline award multipliers and coin payouts.

## Paytable & Odds Multipliers

The machine uses 3 reels ($R_1, R_2, R_3$) with 5 symbols:
- **Seven (`SymbolSeven` / ７)**
- **Star (`SymbolStar` / ★)**
- **Dagger (`SymbolDagger` / †)**
- **Note (`SymbolNote` / ♪)**
- **Cherry (`SymbolCherry` / ∞)**

### Payout Multipliers

| Combination | Multiplier | Payout on Bet $B$ | Description |
| :--- | :---: | :---: | :--- |
| ７ ７ ７ | **100x** | $100 \times B$ | 777 Jackpot |
| ★ ★ ★ | **70x** | $70 \times B$ | Super Win |
| † † † | **50x** | $50 \times B$ | Big Win |
| ♪ ♪ ♪ | **20x** | $20 \times B$ | Standard Win |
| ∞ ∞ ∞ | **10x** | $10 \times B$ | Triple Cherry |
| ∞ ∞ [Any] | **3x** | $3 \times B$ | Double Cherry (First 2 reels) |
| Any other | **0x** | $0$ | Miss (Loss of wagered bet $B$) |

## Betting Rates

Standard bet options (coins per spin):
- **$1 Slot**: 1 coin
- **$10 Slot**: 10 coins
- **$50 Slot**: 50 coins
- **$100 Slot**: 100 coins
- **$200 Slot**: 200 coins (high roller)

## Mechanics & Concurrency

- **Atomic Settlement**:
  - Each spin verifies and deducts the wagered bet (`Coins >= Bet`) and credits payouts atomically via `DeductBetAndCreditPayout` in a single database transaction.
  - Concurrent spin requests are strictly serialized through database row locks/conditional updates, preventing any free spins or payout exploits with insufficient balances.
  - No negative coin balances are permitted.
