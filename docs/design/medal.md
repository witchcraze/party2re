# Small Medal Exchange (メダル王)

## Overview
The Small Medal (ちいさなメダル) system allows players to exchange collected "Small Medals" for rare and exclusive items.

## Behavior Policy (Legacy Reconstruction)
As directed by project requirements, this system faithfully recreates the deductive (消費型) behavior of the original Party2 `medal.cgi`.
- Small Medals act as a special currency.
- When a player exchanges medals for an item, the required medal cost is **deducted** from their balance.
- Players can repeatedly purchase the same reward as long as they have sufficient medals.
- The system uses a pessimistic database lock to ensure transaction integrity during the exchange.

## Data Structures
- **Medal Balance**: Stored in `characters.small_medals` (integer).
- **Catalog**: Defined in `internal/medal/medal_rewards.json`.

## Catalog Tiers
| Cost | Item ID |
|------|---------|
| 3    | armor-32 |
| 5    | item-013 |
| 8    | armor-33 |
| 10   | weapon-32 |
| 15   | item-036 |
| 20   | item-035 |
| 25   | item-037 |
| 30   | item-034 |
| 35   | item-089 |
| 40   | weapon-30 |
| 45   | armor-35 |
| 50   | armor-40 |
| 60   | item-109 |
| 77   | item-107 |
| 100  | item-059 |

## Acquisition Channels (入手手段)

Small Medals are earned through various exploration and combat activities:
1. **Dungeon Exploration (`internal/dungeon`)**:
   - **Treasure Chests ('T')**: Each chest opened awards **+1 Small Medal**.
   - **Dungeon Clear**: Completely clearing a dungeon awards bonus Small Medals equal to the dungeon Tier (Tier 1 = +1, Tier 2 = +2, Tier 3 = +3).
   - Medals gathered during exploration are permanently committed to the character upon safe escape or dungeon clear.
2. **World Boss Battles (`internal/boss`)**:
   - **Boss Defeat**: Conquering a King Boss awards **1 to 5 Small Medals** (Tier 1〜10; Primal Creator Deity Tier 99 awards **10 Small Medals**).
   - **First Clear Bonus**: First-time defeat of a boss tier awards an extra **1 to 20 Small Medals**.
4. **Milestone Achievements (`internal/medal`, [`achievements.md`](achievements.md))**:
   - Completing lifetime achievements (adventures completed, monsters slain, gold earned, bosses defeated, etc.) awards bonus Small Medals upon claim.

## HTTP API Endpoints

### Small Medal Exchange
- `GET /medals/rewards`: Returns the active small medal reward tiers and item definitions.
- `POST /medals/claim`: Exchanges character small medals for a reward item (`{"character_id": "...", "item_id": "..."}`) with session authentication and character ownership verification.

### Milestone Achievements & Commemorative Medals
- `GET /characters/{id}/achievements`: Inspects character lifetime milestone achievements and progress.
- `POST /characters/{id}/achievements/{achievement_id}/claim`: Claims completed milestone rewards (commemorative medal and bonus small medals).
- `GET /characters/{id}/medals`: Inspects character collection of earned commemorative medals.
