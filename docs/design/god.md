# Endgame Wishes & Limit Breaks Design (天界・裏天界)

## 1. Overview

The God system (`internal/god`, legacy `party2/lib/god.cgi` and `party2/lib/u_god.cgi`) provides endgame characters with celestial audiences in Heaven (天界) and Underworld (裏天界). Players can choose permanent character enhancements, resource grants, home companions/customizations, item rewards sent to depot, and limit breaks exceeding standard system ceilings.

Upon successfully granting a wish, the character is automatically returned to their home location (`NextLocation: "home"`, reflecting legacy `$m{lib} = 'home'`), except for joke wishes (`wish_lover`) which return dialogue without state mutation or teleportation.

---

## 2. Realms & NPC Dialogues

1. **Heaven (天界)**:
   - **NPC**: `@神`
   - **Legacy Location**: Accessible via Pegasus Reins (`天馬のたづな`, Item No. 59).
   - **Theme**: Attribute growth, currency/resource rewards, home/guild customizations, depot item gifts, and level cap limit breaks.
2. **Underworld (裏天界 / 天界?)**:
   - **NPC**: `@神?`
   - **Legacy Location**: Accessible via Black Pegasus Reins (`黒い天馬のたづな`, Item No. 262).
   - **Theme**: System capacity limit breaks (depot, monster box, job memory, flea market listings, shop listings).

---

## 3. Wishes Catalog & Rules

### 3.1 Heaven Wishes (天界の願い事 - 全19種)

The Heaven catalog comprises 18 standard wishes plus 1 conditional level-cap wish determined by character state:

| Wish ID | Name (和名) | Description (説明) | Condition (出現/実行条件) | Effect (効果) |
| :--- | :--- | :--- | :--- | :--- |
| `wish_stats` | 強くなりたい | 全ステータス 40 アップ | `!OverLevel` | $\text{MaxHP} + 40, \text{HP} + 40, \text{MaxMP} + 40, \text{MP} + 40, \text{Attack} + 40, \text{Defense} + 40, \text{Agility} + 40$ |
| `wish_sp` | スキルを覚えたい | Sp 50 アップ | None | $\text{SP} + 50$ (via `char.AddSP(50)`) |
| `wish_money` | お金がほしい | 10 万G | None | $\text{Money} + 100,000\text{ G}$ (via `char.AddMoney(100000)`) |
| `wish_casino_coins` | カジノコインがほしい | 5 万枚 | None | Casino Coins $+ 50,000$ (via `casino.AdjustCoins`) |
| `wish_small_medals` | 小さなメダルがほしい | 20 枚 | None | $\text{SmallMedals} + 20$ (via `char.AddSmallMedals(20)`) |
| `wish_lottery_tickets` | 福引券がほしい | 1000 枚 | None | Lottery Tickets $+ 1,000$ (via `lottery.AddRaffleTickets`) |
| `wish_guild_rank` | ギルドランクをあげたい | 1000 ポイント | Must belong to a guild | Guild EXP $+ 1,000$ (recalculates guild level) |
| `wish_guild_gorgeous` | ギルドをゴージャスにしたい | ギルドが… | Must belong to a guild | Guild `bgimg` set to `"god.gif"` |
| `wish_refresh` / `wish_full_recovery` | 元気いっぱいになりたい | 疲労度 -150 % (HP・MP完全回復) | None | $\text{Tired} = \text{Tired} - 150, \text{HP} = \text{MaxHP}, \text{MP} = \text{MaxMP}$ |
| `wish_all_orbs` | 新しい冒険場所に行きたい | 全オーブ | None | Adds all 6 standard orbs (`byrpgs` / `ValidOrbRunes`) |
| `wish_celestial_dragon` | 天竜人になりたい | 転職 (空竜の民) | `JobID != "job-70" && OldJobID != "job-70"` | Job changed to `"job-70"` (Job reset & base stats applied) |
| `wish_god_of_new_world` | 新世界の神になりたい | 自分の家が… | None | Avatar set to `"chr/052.gif"`, home `bgimg` set to `"god.gif"` |
| `wish_ortega` | オルテガを生き返らして | 自分の家に… | None | Adds NPC companion "オルテガ" (`chr/029.gif`) to `home_members` |
| `wish_cat` | 猫を飼いたい | 自分の家に… | None | Adds NPC companion "白猫" (`chr/030.gif`) or "黒猫" (`chr/031.gif`) (50% random) |
| `wish_secret_maid` | メイドを雇いたい | お世話係 | Maid not already in `home_members` | Adds NPC companion "メイド" (`chr/026.gif`) to `home_members` |
| `wish_erotic_book` | エッチな本がほしい | アイテム (預かり所へ送致) | Depot available | Delivers `item-058` to player Depot (`character_depots`) |
| `wish_alchemy_recipe` | 錬金レシピがほしい | アイテム (預かり所へ送致) | Depot available | Delivers `item-128` or `item-129` (50% random) to player Depot |
| `wish_lover` | 素敵な恋人がほしい | 恋人が…？ | None | Rejection joke message ("それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…"); no mutation, no teleport |
| `wish_limit_break_level` | もっと強くなりたい | Lv上限を上げる | $\text{Level} \ge 99 \land \lnot\text{OverLevel}$ | $\text{OverLevel} = \text{true}$ (raises max level from 99 to 150) |
| `wish_restore_level_limit` | もとの強さに戻りたい | Lv上限を元に戻す | $\text{OverLevel} = \text{true}$ | $\text{OverLevel} = \text{false}$ (restores level cap to 99) |

### 3.2 Underworld Wishes (裏天界の願い事)

Each Underworld limit break can be upgraded up to 5 tiers (`0/5` to `5/5`):

| Wish ID | Name (和名) | Condition | Effect per Tier |
| :--- | :--- | :--- | :--- |
| `wish_expand_depot` | もっとアイテムを預けたい | $\text{OverDepot} < 5$ | $\text{OverDepot} + 1$, Depot Capacity $+50$ (Base 50 $\to$ Max 300) |
| `wish_expand_monster` | もっとモンスターを預けたい | $\text{OverMonster} < 5$ | $\text{OverMonster} + 1$, Monster box capacity $+50$ |
| `wish_expand_job_memory` | もっと職業を覚えたい | $\text{OverFuture} < 5$ | $\text{OverFuture} + 1$, Future job memory slot $+1$ |
| `wish_expand_flea_market` | もっとフリーマーケットで出品したい | $\text{OverFlea} < 5$ | $\text{OverFlea} + 1$, Max active listings $+1$ (Base 5 $\to$ Max 10) |
| `wish_expand_shop_store` | もっとお店で出品したい | $\text{OverStore} < 5$ | $\text{OverStore} + 1$, Player shop listings $+1$ |

### 3.3 Fatigue Recovery (疲労度回復) Parity

In legacy Party2 (`party2/lib/god.cgi:57`):
```perl
['元気いっぱいになりたい', '疲労度 -150 %', sub { $m{tired} -= 150; }]
```
- In Party2Re, `wish_refresh` (`wish_full_recovery`) deducts 150 from `Character.Tired` (`char.ReduceTired(150)`), fully matching the legacy behavior where fatigue can drop below 0 to provide a buffer against subsequent combat fatigue.
- In addition to fatigue reduction, HP and MP are fully restored to their respective maximum values (`MaxHP` and `MaxMP`).

---

## 4. Level Cap Limit Break Formula

For a character with $\text{OverLevel} = \text{true}$:
- **Level Ceiling**:
  $$\text{MaxLevel} = \begin{cases} 150 & \text{if } \text{OverLevel} = \text{true} \\ 99 & \text{otherwise} \end{cases}$$
- **Required Experience for Advancement**:
  $$\text{ExpThreshold}(L) = L^2 \times 10 \quad (1 \le L < \text{MaxLevel})$$

---

## 5. Security & Idempotency

- All wish grants are executed within transactional Unit of Work boundaries (`RunInTx`) with strict pessimistic lock ordering:
  1. Rank 2: `characters` (`FindByIDForUpdate`)
  2. Rank 5: `character_depots` (`FindByCharacterIDForUpdate`)
- Character ownership is strictly validated against the authenticated session player (`PlayerID`), preventing IDOR.
- Post-wish routing redirects players safely to their home (`NextLocation: "home"`), ending celestial session state.
