# Endgame Wishes & Limit Breaks Design (天界・裏天界)

## 1. Overview

The God system (`internal/god`, legacy `god.cgi` and `u_god.cgi`) provides endgame characters with celestial audiences in Heaven (天界) and Underworld (裏天界). Players can choose permanent character enhancements, resource grants, and limit breaks exceeding standard system ceilings.

---

## 2. Realms & NPC Dialogues

1. **Heaven (天界)**:
   - **NPC**: `@神`
   - **Legacy Location**: Accessible via Pegasus Reins (`天馬のたづな`, Item No. 59).
   - **Theme**: Attribute growth, currency/resource rewards, and level cap limit breaks.
2. **Underworld (裏天界 / 天界?)**:
   - **NPC**: `@神?`
   - **Legacy Location**: Accessible via Black Pegasus Reins (`黒い天馬のたづな`, Item No. 262).
   - **Theme**: System capacity limit breaks (depot, monster box, job memory, flea market listings, shop listings).

---

## 3. Wishes Catalog & Rules

### 3.1 Heaven Wishes (天界の願い事)

| Wish ID | Name (和名) | Condition | Effect |
| :--- | :--- | :--- | :--- |
| `wish_stats` | 強くなりたい | `!OverLevel` | $\text{MaxHP} + 40$, $\text{MaxMP} + 40$, $\text{Attack} + 40$, $\text{Defense} + 40$, $\text{Agility} + 40$ |
| `wish_money` | お金がほしい | None | $\text{Money} + 100,000\text{ G}$ |
| `wish_small_medals` | 小さなメダルがほしい | None | $\text{SmallMedals} + 20$ |
| `wish_full_recovery` | 元気いっぱいになりたい | None | $\text{HP} = \text{MaxHP}$, $\text{MP} = \text{MaxMP}$ |
| `wish_limit_break_level` | もっと強くなりたい | $\text{Level} \ge 99 \land \lnot\text{OverLevel}$ | $\text{OverLevel} = \text{true}$ (raises max level from 99 to 150) |
| `wish_restore_level_limit` | もとの強さに戻りたい | $\text{OverLevel} = \text{true}$ | $\text{OverLevel} = \text{false}$ (restores level cap to 99) |
| `wish_lover` | 素敵な恋人がほしい | None | Rejection joke message |
| `wish_secret_maid` | メイドを雇いたい | None | Secret wish granting maid companion |

### 3.2 Underworld Wishes (裏天界の願い事)

Each Underworld limit break can be upgraded up to 5 tiers (`0/5` to `5/5`):

| Wish ID | Name (和名) | Condition | Effect per Tier |
| :--- | :--- | :--- | :--- |
| `wish_expand_depot` | もっとアイテムを預けたい | $\text{OverDepot} < 5$ | $\text{OverDepot} + 1$, Depot Capacity $+10$ (Base 50 $\to$ Max 100) |
| `wish_expand_monster` | もっとモンスターを預けたい | $\text{OverMonster} < 5$ | $\text{OverMonster} + 1$ |
| `wish_expand_job_memory` | もっと職業を覚えたい | $\text{OverFuture} < 5$ | $\text{OverFuture} + 1$ |
| `wish_expand_flea_market` | もっとフリーマーケットで出品したい | $\text{OverFlea} < 5$ | $\text{OverFlea} + 1$, Max active listings $+1$ (Base 5 $\to$ Max 10) |
| `wish_expand_shop_store` | もっとお店で出品したい | $\text{OverStore} < 5$ | $\text{OverStore} + 1$ |

---

## 4. Level Cap Limit Break Formula

For a character with $\text{OverLevel} = \text{true}$:
- **Level Ceiling**:
  $$\text{MaxLevel} = \begin{cases} 150 & \text{if } \text{OverLevel} = \text{true} \\ 99 & \text{otherwise} \end{cases}$$
- **Required Experience for Advancement**:
  $$\text{ExpThreshold}(L) = L^2 \times 10 \quad (1 \le L < \text{MaxLevel})$$

---

## 5. Security & Idempotency

- All wish grants are executed within transactional Unit of Work boundaries (`RunInTx`) with pessimistic locking (`FindByIDForUpdate`).
- Character ownership is strictly validated against the authenticated session player (`PlayerID`), preventing IDOR.
