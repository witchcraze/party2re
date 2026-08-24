# Lottery & Raffle Design

## Overview

The Lottery and Raffle Feature Module (`internal/lottery`) provides two distinct prize drawing systems:
1. **Raffle (福引 / Fukubiki)**: Instant drawing mini-game using raffle tickets (coupons) to win tiered item and gold rewards.
2. **Numbered Lottery (宝くじ / Takarakuji)**: Periodic numbered ticket drawing with matching digit tiers and jackpot prize pools.

---

## Raffle (福引 / Fukubiki)

### Ticket Acquisition
- Tickets are acquired via gold purchase (100 Gold per ticket) or awarded from activities.

### Drawing Modes & Tiers

#### Standard Raffle (`RaffleStandard`)
- **Cost**: 3 Raffle Tickets
- **Random Range**: 0 to 999 (out of 1000)

| Roll Range | Prize Tier | Reward Gold / Item | Description |
| :---: | :--- | :---: | :--- |
| $< 1$ (0.1%) | **Grand Prize (特賞)** | 5,000 Gold / Special Reward | Grand Golden Slime Jackpot |
| $< 4$ (0.3%) | **1st Prize (1等)** | 2,500 Gold | Red Slime |
| $< 8$ (0.4%) | **2nd Prize (2等)** | 1,000 Gold | Purple Slime |
| $< 14$ (0.6%) | **3rd Prize (3等)** | 500 Gold | Yellow Slime |
| $< 45$ (3.1%) | **4th Prize (4等)** | 200 Gold | Pink Slime |
| $< 55$ (1.0%) | **5th Prize (5等)** | 100 Gold | Blue Slime |
| $< 75$ (2.0%) | **6th Prize (6等)** | 50 Gold | Green Slime |
| $\ge 75$ (92.5%) | **Miss (ハズレ)** | 0 Gold | White Slime |

#### Special Orb Raffle (`RaffleSpecial`)
- **Cost**: 300 Raffle Tickets
- **Random Range**: 0 to 99 (out of 100)

| Roll Range | Prize Tier | Reward Gold | Description |
| :---: | :--- | :---: | :--- |
| $< 3$ (3%) | **Grand Prize (特賞)** | 100,000 Gold | Legendary Gold Orb |
| $< 15$ (12%) | **1st Prize (1等)** | 20,000 Gold | Silver Orb |
| $< 30$ (15%) | **2nd Prize (2等)** | 10,000 Gold | Red Orb |
| $< 40$ (10%) | **3rd Prize (3等)** | 5,000 Gold | Blue Orb |
| $< 50$ (10%) | **4th Prize (4等)** | 3,000 Gold | Green Orb |
| $< 60$ (10%) | **5th Prize (5等)** | 2,000 Gold | Yellow Orb |
| $< 70$ (10%) | **6th Prize (6等)** | 1,000 Gold | Purple Orb |
| $\ge 70$ (30%) | **Miss (ハズレ)** | 0 Gold | White Orb |

---

## Numbered Lottery (宝くじ / Takarakuji)

### Ticket Specifications
- **Ticket Price**: 300 Gold per ticket.
- **Ticket Number**: 4-digit decimal string (`0000` to `9999`).
- Choice of player-chosen number or random quick-pick.

### Prize Evaluation & Matching Rules
Compared against round winning number $W_0 W_1 W_2 W_3$:

| Match Condition | Prize Tier | Payout Gold | Multiplier |
| :--- | :---: | :---: | :---: |
| Exact 4 digits match ($T_0 T_1 T_2 T_3 = W_0 W_1 W_2 W_3$) | **1st Prize (1等 / Jackpot)** | **100,000 Gold** | $\approx 333\text{x}$ |
| Last 3 digits match ($T_1 T_2 T_3 = W_1 W_2 W_3$) | **2nd Prize (2等)** | **10,000 Gold** | $\approx 33.3\text{x}$ |
| Last 2 digits match ($T_2 T_3 = W_2 W_3$) | **3rd Prize (3等)** | **1,000 Gold** | $\approx 3.33\text{x}$ |
| Last 1 digit match ($T_3 = W_3$) | **4th Prize (4等)** | **300 Gold** | $1\text{x}$ (Refund) |
| None | **Miss (ハズレ)** | **0 Gold** | $0\text{x}$ |

---

## Persistence & Transactions

- **Atomic Settlement**:
  - Buying raffle tickets or lottery tickets atomically deducts character wallet gold.
  - Claiming lottery winnings credits character wallet gold and marks the ticket as claimed (`claimed = TRUE`) in a single database transaction.
  - Double claims return `ErrTicketAlreadyClaimed`.
