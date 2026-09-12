# King Sealing Battles Design (封印戦)

## Overview

The King Sealing Battle Feature Module (`internal/boss`) provides high-tier endgame cooperative sealing battles (`vs_king.cgi`, `stage/king1..10.cgi`, `king99.cgi`) using the shared Multi-Participant Core Battle engine (`internal/core/battle`).

Up to 4 players form a party in the Multiplayer Party System (`internal/party`) under quest mode "封印戦" (quest type 6) to challenge ancient sealed kings and calamitous deities. Characters face devastating enemy skills like **Dejon** (デジョン; dimensional banishment of unconscious members with severe fatigue penalties). Victorious adventurers achieve resealing via **`@ふういん`**, gaining Hero Count (勇者カウント / `$m{hero_c}`), rare treasures, worldwide server news broadcasts, and hosting a celebratory banquet in the town Event Plaza (`internal/eventplaza`).

---

## Architectural Policy & Boundaries

- **Multi-Participant Battle Engine**: Encounters are resolved through `corebattle.PartyBattleResolver` (`corebattle.Engine{}`), handling party vs. multi-enemy boss formations, agility turn-order, MP/CMP skills, and revive/banishment rules.
- **Authentic Stage Catalog**: 11 stages faithfully reproduced from legacy Party2 CGI data (`stage/king1.cgi` through `stage/king10.cgi`, and dynamic clone stage `king99.cgi`).
- **No Fictional Daily Limits**: Legacy Party2 has no daily attempt caps (the previously fabricated 3-entry solo raid limit has been completely removed). Entrance is governed solely by character fatigue (`tired < 100`) and stage-specific `need_join` conditions (e.g. `hp_400_o`).
- **Transactional Consistency**: Post-battle state transitions (HeroCount increment, entry fatigue +20%, dejon fatigue +30%, loot awards, news broadcast, banquet hooks, party disbandment) are committed within a single database transaction.

---

## Authentic King Stages

| Stage ID | Stage Name | Leader / Boss | Speed | Max Members | Participation Gate (`need_join`) | Loot Drops |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **king1** | @全てを無に還す者@ | 破壊神 (HP 150,000) + 6 Stones | 12 | 4 | `hp_400_o` (Max HP ≥ 400) | item-059, item-071, item-104 |
| **king2** | @全てを憎む者@ | 暗黒竜 (HP 140,000) | 12 | 4 | `hp_400_o` (Max HP ≥ 400) | item-059, item-071, item-104 |
| **king3** | @全てを破壊する者@ | 悪魔の書 (HP 100,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071 |
| **king4** | @全てを喰らう者@ | デス・マスター (HP 100,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king5** | @全てを司る者@ | 邪神官 (HP 100,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king6** | @全てを統べる者@ | 破壊神 (HP 120,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king7** | @全てを導く者@ | 竜神 (HP 150,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king8** | @全てを裁く者@ | 審判者 (HP 160,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king9** | @全てを赦す者@ | 救世主 (HP 180,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king10** | @全てを越える者@ | 創世神 (HP 200,000) | 12 | 4 | `hp_300_o` (Max HP ≥ 300) | item-059, item-071, item-072 |
| **king99** | @自分を倒す者@ | @Player Clone (Allies Stats × 50) | 10 | 4 | `hp_400_o` (Max HP ≥ 400) | item-059, item-071, item-104 |

---

## Battle Mechanics

### 1. Entry & Fatigue Cost
- Each participating character incurs **+20% Tired** upon entering the sealing battle (`$m{tired} += 20`).
- Characters with `tired >= 100` or `hp <= 0` cannot enter.

### 2. Dejon (デジョン) Banishment
- Bosses execute the **Dejon** skill (`ActionKindDejon = "dejon"`).
- Target: Any opposing character whose HP is reduced to `0` or below.
- Effect: The unconscious character is cast into another dimension and permanently removed from combat. They cannot be revived for the remainder of the battle.
- Penalty: An additional **+30% Tired** (`$m{tired} += 30`) is applied to banished characters upon battle conclusion.

### 3. Victory & `@ふういん` Resealing
Upon defeating all enemy boss participants:
1. **Hero Count**: All party members receive **+1 Hero Count** (`characters.hero_count` / `$m{hero_c}`).
2. **Treasure Drop**: A random item from the stage's `treasure_item_ids` is awarded.
3. **Exp & Gold**: Distributed to all party members.
4. **Server News**: A worldwide announcement is broadcast: `"勇者○○が○○を封印する"`.
5. **Celebration Banquet**: Triggers a 2-hour victory banquet in Event Plaza (`_win_vs_king.cgi`).
6. **Lobby Disbandment**: The temporary staging party is deleted upon conclusion.
