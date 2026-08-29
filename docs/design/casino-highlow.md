# Casino High & Low Mini-Game Design

## Overview

The High & Low mini-game is a classic card prediction game in the Casino Feature Module (`internal/casino`), where players wager casino coins and predict whether the next dealt card will be higher or lower in rank than the current card.

---

## Domain Rules & Card Mechanics

### Card Deck & Ranks

- **Deck**: Standard 52-card playing deck (4 suits: ♠, ♥, ♦, ♣; 13 ranks: A, 2, 3, 4, 5, 6, 7, 8, 9, 10, J, Q, K).
- **Rank Ordering**:
  - Ace is Low: $\text{Rank} = 1$.
  - Number cards: $\text{Rank} = 2 \dots 10$.
  - Face cards: Jack ($\text{Rank} = 11$), Queen ($\text{Rank} = 12$), King ($\text{Rank} = 13$).
  - Suit has no rank hierarchy in High & Low comparison.

---

## Game Progression & Resolution

### Betting & Guess Options

- **Allowed Wager**: $1 \le \text{Bet} \le 5,000$ Casino Coins.
- **Guess Types**:
  - `HIGH`: Predicts $\text{Rank}_{\text{next}} > \text{Rank}_{\text{current}}$.
  - `LOW`: Predicts $\text{Rank}_{\text{next}} < \text{Rank}_{\text{current}}$.

### Round Resolution Table

| Condition | Outcome | Multiplier | Payout | Net Coins |
| :--- | :---: | :---: | :---: | :---: |
| Guess matches actual relative rank | `WIN` | $2\times$ | $\text{Bet} \times 2$ | $+\text{Bet}$ |
| $\text{Rank}_{\text{next}} = \text{Rank}_{\text{current}}$ | `TIE` / `PUSH` | $1\times$ | $\text{Bet}$ | $0$ (Refund) |
| Guess does not match relative rank | `LOSS` | $0\times$ | $0$ | $-\text{Bet}$ |

---

## Multi-Round Streak Mode

- Players can continue playing consecutive streak rounds:
  - Each consecutive win doubles the accumulated winnings ($\text{Accumulated} = \text{InitialBet} \times 2^{\text{Streak}}$).
  - A `TIE` pushes without resetting or doubling the streak.
  - A `LOSS` forfeits all accumulated coins ($\text{Accumulated} = 0$).
  - Players may cash out at any time between winning rounds.

---

## Persistence & Transactions

- Single-round, multi-round, and cash-out operations verify wagers and adjust the character's casino account (`casino_accounts`) atomically via `DeductBetAndCreditPayout` using conditional updates (`coins >= bet`).
- Prevents concurrency exploits across spammed game rounds.
