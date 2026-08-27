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

## HTTP API Endpoints

- `GET /medals/rewards`: Returns the active small medal reward tiers and item definitions.
- `POST /medals/claim`: Exchanges character small medals for a reward item (`{"character_id": "...", "item_id": "..."}`) with session authentication and character ownership verification.
