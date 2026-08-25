# Guild versus Guild (GvG) Combat Design

## Overview

The Guild versus Guild (GvG) Feature Module (`internal/gvg`) provides asynchronous roster skirmishes between guilds using the shared Core Battle engine (`internal/core/battle`). Guild leaders and officers declare matches against rival guilds to climb the GvG rating ladder, earn Guild Points (Victory Points), level up their guild, and accumulate tiered victory medals and championship cups.

---

## Architectural Policy & Boundaries

- **Core Battle Engine Isolation**: All individual member round duels are executed deterministically through `internal/core/battle.Engine`.
- **Roster Snapshots**: GvG matches assemble roster snapshots (up to 5 members per guild, ordered by rank/level) without requiring real-time synchronized presence from the defending guild.
- **Authority Validation**: Only guild Leaders and Officers can initiate GvG challenge declarations.
- **Transactional Consistency**: Match recording, round detail logs, rating adjustments, medal promotions, guild EXP increases, and member reward applications are committed in a single atomic database transaction.

---

## Domain Rules & Formulas

### 1. Guild Ratings & Elo Calculation
- **Initial Rating**: $R_0 = 1000$
- **Minimum Rating Floor**: $R_{\min} = 0$
- **K-Factor**: $K = 32$

Given Challenger Guild rating $R_C$ and Defender Guild rating $R_D$:

1. **Expected Score for Challenger ($E_C$)**:
   $$E_C = \frac{1}{1 + 10^{(R_D - R_C) / 400}}$$

2. **Actual Score ($S_C$)**:
   $$S_C = \begin{cases} 1.0 & \text{if Challenger Score} > \text{Defender Score} \\ 0.0 & \text{if Challenger Score} < \text{Defender Score} \\ 0.5 & \text{if Draw} \end{cases}$$

3. **Rating Delta ($\Delta R_C$)**:
   $$\Delta R_C = \text{round}(K \times (S_C - E_C))$$
   - A decisive win guarantees at least $+1$ rating delta ($\Delta R_C \ge 1$).
   - A decisive loss guarantees at least $-1$ rating delta ($\Delta R_C \le -1$).
   - Defender delta is $\Delta R_D = -\Delta R_C$.

---

### 2. Victory Medals & Cup Promotions

Based on standard reference values, winning GvG matches awards Bronze Medals which promote into higher tiers upon reaching 5 of each:

| Medal Tier | Japanese Term | Promotion Requirement | Base Bronze Equivalence |
| :--- | :--- | :--- | :--- |
| **Bronze Medal** | 銅メダル | Base unit awarded on GvG win | 1 |
| **Silver Medal** | 銀メダル | 5 Bronze Medals | 5 |
| **Gold Medal** | 金メダル | 5 Silver Medals | 25 |
| **Trophy** | トロフィー | 5 Gold Medals | 125 |
| **Championship Cup** | 優勝杯 | 5 Trophies | 625 |
| **Champion Cup** | 王者杯 | 5 Championship Cups | 3,125 |

---

### 3. Guild & Member Rewards

When a GvG match concludes:

- **Guild-level Rewards**:
  - **Winning Guild**: $+100$ Guild EXP, $+10$ Victory Points (GP), $+1$ Bronze Medal.
  - **Losing Guild**: $+20$ Guild EXP, $+1$ Victory Point (GP), $0$ Medals.
  - **Draw Match**: $+50$ Guild EXP each, $+3$ Victory Points (GP) each, $0$ Medals.
  - Guild EXP advances guild level progression up to Max Level 10 (expanding guild capacity).

- **Individual Participant Rewards**:
  - **Round Winner**: $+50$ EXP, $+100$ Gold.
  - **Round Loser**: $+15$ EXP, $+20$ Gold.
  - **Round Draw**: $+25$ EXP, $+50$ Gold.
  - Experience applies to character progression and triggers level-ups.

---

## Matchmaking & Queries

1. **Rival Matchmaking**: `FindOpponentGuilds` returns candidate opponent guilds sorted by rating proximity ($|R_{\text{opponent}} - R_{\text{challenger}}|$) and level, strictly excluding the challenger's own guild.
2. **Standings & Leaderboard**: Guilds are ranked by rating descending, then Victory Points (GP) descending, and wins descending.
3. **Match History & Detail**: Detailed history captures overall match scores and round-by-round duelist identities, turns, and outcomes.
