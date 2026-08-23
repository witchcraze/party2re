# Stages and Monsters Design

## Purpose

This document describes the stage hierarchy, monster encounters, and clean-room content definitions for adventure progression in Party2.

## Structure and Catalogs

### 1. Stages (`stages.json`)
- **Stage ID (`id`)**: Stable identifier (`stage-01` through `stage-28`).
- **Name (`name`)**: The Japanese name of the stage location.
- **Minimum Level (`min_level`)**: The recommended character/job level requirement to safely enter the stage.
- **Monster IDs (`monster_ids`)**: List of monster references encountered in the stage.
- **Duration (`duration_seconds`)**: Standard scheduled duration for completing an adventure in the stage (default: 3600s / 1 hour).

### Standard Adventure Stages (01–28)
1. **プニプニ平原** (`stage-01`): Level 1+ introductory meadow.
2. **キノコの森** (`stage-02`): Level 3+ forest filled with fungal and small beast creatures.
3. **幽霊城** (`stage-03`): Level 6+ haunted castle with undead encounters.
4. **海辺の洞窟** (`stage-04`): Level 9+ coastal caverns.
5. **地獄の砂浜** (`stage-05`): Level 12+ hostile coastal shores.
6. **魔術師の塔** (`stage-06`): Level 15+ arcane spire.
7. **荒野の獣道** (`stage-07`): Level 18+ wild beasts in rocky wilderness.
8. **マグマ山** (`stage-08`): Level 21+ volcanic domain.
9. **妖精の森** (`stage-09`): Level 24+ mystical fey woods.
10. **スライムランド** (`stage-10`): Level 27+ slime habitat.
11. **死霊の沼地** (`stage-11`): Level 30+ cursed marshlands.
12. **ドラゴンの谷** (`stage-12`): Level 33+ valley of drakes and dragons.
13. **暗黒魔城** (`stage-13`): Level 36+ fortress of darkness.
14. **死の大地** (`stage-14`): Level 39+ desolated wasteland.
15. **魔界** (`stage-15`): Level 42+ netherworld realm.
16. **鏡の世界** (`stage-16`): Level 30+ shadow realm with doppelganger and shadow enemies.
17. **マダムガーデン** (`stage-17`): Level 35+ garden estate.
18. **幻の秘境** (`stage-18`): Level 40+ hidden sanctuary.
19. **闇のランプ** (`stage-19`): Level 45+ shadowy lamp cavern.
20. **封印の地** (`stage-20`): Level 50+ sealed ground.
21. **天空城** (`stage-21`): Level 55+ floating sky citadel.
22. **カオスフィールド** (`stage-22`): Level 60+ chaotic rift.
23. **ワイルドアピアリー** (`stage-23`): Level 65+ wild apiary nesting grounds.
24. **プニプニ雪原** (`stage-24`): Level 70+ frozen snowfields.
25. **白亜の宮殿** (`stage-25`): Level 75+ chalk palace.
26. **氷の彫刻館** (`stage-26`): Level 80+ glacial museum of ice sculptures.
27. **神秘の森** (`stage-27`): Level 85+ ancient mystical forest.
28. **ハロウィンタウン** (`stage-28`): Level 90+ festival town of tricks and treats.

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
