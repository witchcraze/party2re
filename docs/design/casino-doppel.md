# Casino Doppelganger Design

## Overview

The Doppelganger (ドッペル) mini-game is a wagering game where a player wagers casino coins trying to match their chosen mark/symbol with the mark secretly selected by the Doppelganger.

## Domain Rules & Mechanics

### Marks & Symbols

There are 8 distinct marks:
1. **Star (`MarkStar` / ★)**
2. **Circle (`MarkCircle` / ●)**
3. **Diamond (`MarkDiamond` / ◆)**
4. **Note (`MarkNote` / ♪)**
5. **Square (`MarkSquare` / ■)**
6. **Triangle (`MarkTriangle` / ▲)**
7. **Dagger (`MarkDagger` / †)**
8. **Inverted Triangle (`MarkInvertedTriangle` / ▼)**

### Pool Sizes & Odds Multipliers

The player selects a difficulty tier determining the active symbol pool size and winning odds multiplier:

| Pool Size | Available Symbols | Probability | Multiplier | Payout on Bet $B$ |
| :---: | :--- | :---: | :---: | :--- |
| **4** | ★, ●, ◆, ♪ | $1/4 = 25\%$ | **4x** | $4 \times B$ |
| **6** | ★, ●, ◆, ♪, ■, ▲ | $1/6 \approx 16.7\%$ | **6x** | $6 \times B$ |
| **8** | ★, ●, ◆, ♪, ■, ▲, †, ▼ | $1/8 = 12.5\%$ | **8x** | $8 \times B$ |

### Betting Structure & Limits

- Valid bet amount $B$: $1 \le B \le 5000$ Casino Coins.
- Requires sufficient casino account coins ($Coins \ge B$).

### Resolution & Payout Settlement

- **Doppel Match (`PlayerMark == DoppelMark`)**:
  - Payout: $B \times \text{PoolSize}$ coins.
  - Net Coin Delta: $\Delta = B \times (\text{PoolSize} - 1)$.
- **Mismatch / Miss (`PlayerMark != DoppelMark`)**:
  - Payout: $0$ coins.
  - Net Coin Delta: $\Delta = -B$.

## Persistence & Transactions

- All wagers and payouts are settled atomically in MariaDB (`casino_accounts`) within a database transaction, guaranteeing non-negative balance constraints (`CHECK (coins >= 0)`).
