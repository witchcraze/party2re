# Player versus Player (PvP) Arena Design

## Overview

The Player versus Player (PvP) Arena Feature Module (`internal/pvp`) provides asynchronous competitive combat against opponent character snapshots using the shared Core Battle engine (`internal/core/battle`). Players challenge opponent loadouts to climb the arena rating ladder, earn gold and experience, and view match history and defensive records.

---

## Architectural Policy & Boundaries

- **Core Battle Engine Isolation**: PvP purely consumes `internal/core/battle.Engine`. The battle engine remains language-agnostic and unaware of arena or PvP context.
- **Opponent Snapshot**: Reads the defending character's attributes (stats, level, HP snapshot) to construct the defending `battle.Participant`.
- **Feature Module State**: Arena ratings, win/loss/draw records, match history, and defense logs are owned entirely by `internal/pvp` (`arena_ratings`, `arena_matches`).
- **Transactional Consistency**: Rating adjustments, match history logging, and reward application are executed in a single atomic database transaction.

---

## Domain Rules & Formulas

### 1. Starting Ratings & Bounds
- **Initial Rating**: $R_0 = 1000$
- **Minimum Rating Floor**: $R_{\min} = 0$
- **K-Factor**: $K = 32$

### 2. Standard Elo Rating Calculation

Given Attacker rating $R_A$ and Defender rating $R_D$:

1. **Expected Score for Attacker ($E_A$)**:
   $$E_A = \frac{1}{1 + 10^{(R_D - R_A) / 400}}$$

2. **Actual Score ($S_A$)**:
   $$S_A = \begin{cases} 1.0 & \text{if Attacker Wins} \\ 0.0 & \text{if Attacker Loses} \\ 0.5 & \text{if Draw} \end{cases}$$

3. **Rating Delta ($\Delta R_A$)**:
   $$\Delta R_A = \text{round}(K \times (S_A - E_A))$$
   - A win always awards at least $+1$ rating: $\Delta R_A \ge 1$.
   - A loss always deducts at least $-1$ rating: $\Delta R_A \le -1$.
   - The defender delta is $\Delta R_D = -\Delta R_A$.

4. **Updated Ratings**:
   $$R'_A = \max(0, R_A + \Delta R_A)$$
   $$R'_D = \max(0, R_D + \Delta R_D)$$

### 3. Rewards Formula

When an attacker initiates a challenge, rewards are awarded upon match resolution:

- **Victory**:
  $$\text{Reward EXP} = 50 + (\text{Defender Level} \times 10)$$
  $$\text{Reward Gold} = 100 + (\text{Defender Level} \times 20)$$
- **Defeat**:
  $$\text{Reward EXP} = 10, \quad \text{Reward Gold} = 0$$
- **Draw**:
  $$\text{Reward EXP} = 25, \quad \text{Reward Gold} = 50$$

Experience gains advance character progression and trigger level-ups via `internal/core/progression`.

---

## Matchmaking & Query Rules

1. **Self-Exclusion**: Characters cannot challenge themselves ($c.id \neq \text{attacker.id}$).
2. **Account Win-Trading Prevention**: Characters owned by the same player account are excluded from opponent candidate searches.
3. **Proximity Ordering**: Candidates are sorted primarily by rating proximity ($|R_{\text{candidate}} - R_{\text{attacker}}|$) in ascending order, then by level descending.
4. **Defense Logs**: Defender characters record all matches where they were challenged, enabling defensive match tracking without active player participation.
