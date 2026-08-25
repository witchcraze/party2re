# Custom Skill Loadout & Slot Management Design

## Overview

The Custom Skill Feature Module (`internal/custom_skill`) manages cross-job skill customization and loadout slot configuration (`custom_skill.cgi`). As characters progress, change classes, and master jobs (or synthesize elemental gemstone powers), they can configure an active loadout of equipped skills with tactical execution priorities for use in all combat modes (PvP, GvG, Boss, Dungeon, Adventure, Challenge).

---

## Architectural Policy & Boundaries

- **Mastery & Cross-Job Validation**:
  - Characters can equip skills associated with their currently active Job.
  - Characters can equip skills from any previously **Mastered Jobs** (Lv 99 mastery).
  - Universal / Gemstone skills (empty `RequiredJobID`) can be equipped regardless of current class.
- **Slot Capacity & Prioritization**:
  - Default capacity is 4 slots (expandable in progression).
  - Each equipped slot defines an execution priority (1 to 10) and MP cost.
  - Duplicate equips of the same skill across multiple slots are rejected.
- **Combat Integration**:
  - When constructing a combat `battle.Participant`, the equipped loadout provides the active tactical action list to the Core Battle engine.

---

## Skill Catalog Definition

| Skill ID | Name | Required Job | Req Level | MP Cost | Power | Kind | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `gem_strike` | 気合斬り | Universal | Lv 1 | 5 MP | 25 | Damage | Concentrated physical strike |
| `gem_cure` | 小癒しの光 | Universal | Lv 1 | 8 MP | 30 | Healing | Light healing incantation |
| `gem_barrier` | 水晶の守り | Universal | Lv 10 | 12 MP | 20 | Buff | Protective mana barrier |
| `slash` | 一閃 | 剣士 (`job-02`) | Lv 5 | 6 MP | 35 | Damage | Sharp blade slash |
| `power_strike` | 渾身撃 | 戦士 (`job-01`) | Lv 5 | 8 MP | 45 | Damage | Heavy armor-shattering strike |
| `shield_bash` | シールドバッシュ | 騎士 (`job-03`) | Lv 5 | 7 MP | 30 | Damage | Staggering shield blow |
| `fireball` | 火炎球 | 魔法使い (`job-06`) | Lv 5 | 10 MP | 50 | Damage | Primary elemental fireball |
| `heal` | ヒール | 僧侶 (`job-05`) | Lv 3 | 8 MP | 45 | Healing | Standard restoration spell |
| `shadow_strike` | 急所突き | 盗賊 (`job-09`) | Lv 5 | 10 MP | 55 | Damage | Precision vital organ strike |
| `greater_heal` | ハイヒール | 白魔道士 (`job-16`) | Lv 15 | 20 MP | 100 | Healing | Advanced restorative light |
| `dark_flame` | 冥界の炎 | 闇魔道士 (`job-19`) | Lv 15 | 22 MP | 95 | Damage | Cursed dark firestorm |
| `berserk_rush` | 狂乱乱舞 | バーサーカー (`job-21`) | Lv 15 | 15 MP | 110 | Damage | Wild multi-hit flurry |
| `dragon_breath` | 竜の咆哮 | 竜騎士 (`job-23`) | Lv 20 | 25 MP | 130 | Damage | Draconic shockwave roar |
| `meteor` | 大隕石召喚 | 賢者 (`job-33`) | Lv 30 | 40 MP | 200 | Damage | Cataclysmic meteor impact |

---

## State Machine & Transitions

```text
[Character] --> [Get Available Skills] (Current Job + Mastered Jobs + Universal)
      |
      v
[Equip Skill (Slot Index, Skill ID, Priority)]
      |
      +---> Validates:
      |       1. Slot Index <= Max Slots
      |       2. Skill ID in Catalog
      |       3. Required Level <= Character Level
      |       4. Job is current OR in MasteredJobs list
      |       5. Skill not already equipped in another slot
      |
      +---> Success: Upsert into character_custom_skills
```

---

## Persistence Schema

- `character_custom_skills`: Primary key `character_id`, `max_slots`, `equipped_skills_json`, and `updated_at`.
