# Stages and Monsters Design

## Purpose

This document describes the stage hierarchy, monster encounters, and clean-room content definitions for adventure progression in Party2.

## Structure and Catalogs

### 1. Stages (`stages.json`)
- **Stage ID (`id`)**: Stable identifier (`stage-00` through `stage-27`), in 1:1 parity with legacy Party2 stage scripts (`0.cgi` to `27.cgi`).
- **Name (`name`)**: The Japanese name of the stage location.
- **Minimum Level (`min_level`)**: Current Go metadata enforced against Character.Level. These thresholds are not established legacy admission rules; see the difference below.
- **Monster IDs (`monster_ids`)**: List of monster references encountered in the stage.
- **Boss IDs (`boss_ids`)**: Designated Floor 10 boss encounters.
- **Treasure Pools (`treasure_weapons`, `treasure_armors`, `treasure_items`)**: Authentic drop tables from legacy `@treasures`.
- **Dungeon Crawl Structure**: Authentic 10-floor dungeon crawl loop (`vs_monster.cgi`). Floors 1–9 spawn random stage enemies, Floor 10 features the stage boss battle, and upon victory, Floor 11 serves as the Treasure Room (`add_treasure`). Adventures resolve immediately upon initiation without artificial waiting timers.
- **Seasonal Stage Variants**: Stage 26 (`四季のダンジョン` / `神秘の森`) supports 4 seasonal variants (`spring`, `summer`, `autumn`, `winter`) selected randomly upon crawl execution, configuring season-specific monster rosters, bosses, and treasure drop pools.

### Standard Adventure Stages (00–27)
0. **プニプニ平原** (`stage-00`): introductory meadow.
1. **キノコの森** (`stage-01`): forest filled with fungal and small beast creatures.
2. **幽霊城** (`stage-02`): haunted castle with undead encounters.
3. **海辺の洞窟** (`stage-03`): coastal caverns.
4. **地獄の砂浜** (`stage-04`): hostile coastal shores.
5. **魔術師の塔** (`stage-05`): arcane spire.
6. **荒野の獣道** (`stage-06`): wild beasts in rocky wilderness.
7. **マグマ山** (`stage-07`): volcanic domain.
8. **妖精の森** (`stage-08`): mystical fey woods.
9. **スライムランド** (`stage-09`): slime habitat.
10. **死霊の沼地** (`stage-10`): cursed marshlands.
11. **ドラゴンの谷** (`stage-11`): valley of drakes and dragons.
12. **暗黒魔城** (`stage-12`): fortress of darkness.
13. **死の大地** (`stage-13`): desolated wasteland.
14. **魔界** (`stage-14`): netherworld realm.
15. **鏡の世界** (`stage-15`): shadow realm with doppelganger and shadow enemies.
16. **マダムガーデン** (`stage-16`): garden estate.
17. **幻の秘境** (`stage-17`): hidden sanctuary (3x treasure chest multiplier).
18. **闇のランプ** (`stage-18`): shadowy lamp cavern.
19. **封印の地** (`stage-19`): sealed ground.
20. **天空城** (`stage-20`): floating sky citadel (3x treasure chest multiplier).
21. **カオスフィールド** (`stage-21`): chaotic rift (3x treasure chest multiplier).
22. **ワイルドアピアリー** (`stage-22`): wild apiary nesting grounds.
23. **プニプニ雪原** (`stage-23`): frozen snowfields.
24. **白亜の宮殿** (`stage-24`): chalk palace.
25. **氷の彫刻館** (`stage-25`): glacial museum of ice sculptures.
26. **神秘の森 / 四季のダンジョン** (`stage-26`): seasonal dungeon with Spring, Summer, Autumn, and Winter variants. Once-daily challenge lock (`CategoryDungeonOnce`).
27. **ハロウィンタウン** (`stage-27`): festival town of tricks and treats.

### Legacy admission and current differences

Legacy `lib/quest.cgi:474–548` validates normal party-stage access with
`stage <= job_lv + 2`, subject to `exclude_job_lv_stage`; optional `need_join`
adds user-selected conditions. The selection UI has extra stage gates (22 at
JobLevel 6, 24 at 50), winter-only 23/25 (25 at JobLevel 10), once-per-sleep
26, and Halloween-only 27 (`get_limited_stages`). UI availability and the
server creation guard are separate paths and must both be reconstructed.

Current `StageCatalog.CanAccessStage` additionally requires Character.Level
to meet `min_level`, uses a different `RequiredJobLevel` mapping, and does not
evaluate seasonal availability. These are known implementation differences,
not new canonical game rules. The catalog names above identify stages; they
do not establish fabricated Level 3–90 entrance requirements.

### 2. Monsters (`monsters.json`)
Each monster definition specifies:
- **ID (`id`)**: Stable unique identifier (`monster-001` .. `monster-322`).
- **Name (`name`)**: Generic Japanese fantasy creature name (clean-room compliant; external IP and proprietary names replaced).
- **Combat Stats**: `hp`, `mp`, `attack`, `defense`, `agility`.
- **Rewards**: `exp_reward`, `gold_reward`.
- **Drop Items (`drop_item_ids`)**: Item IDs awarded upon defeat.

### Clean-Room Name Replacements
To maintain strict clean-room isolation from third-party franchises:
- Dragon Quest-specific names (e.g. `ドラキー`, `スライムベス`, `キラーマシン`, `ホイミスライム`, `メガザルロック`, `マドハンド`, `ナスビーラ`, `プチヒーロー`, `メタルスライム`, `はぐれメタル`, `メタルキング`) are replaced with generic terms (`ナイトバット`, `レッドスライム`, `自動戦闘機械`, `ヒーラースライム`, `守護の岩石`, `泥の魔手`, `魔界ナス`, `ちび勇者`, `銀滴スライム`, `流銀スライム`, `王様流銀スライム`).
- Final Fantasy-specific names (e.g. `サボテンダー`, `片翼の天使`) are replaced with generic terms (`トゲサボテン`, `隻翼の堕天使`).
- Chrono Trigger-specific names (e.g. `ラボス`) are replaced with generic terms (`深淵の巨魁`).
