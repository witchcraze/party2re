# Achievements and Commemorative Medals

Lifetime gameplay milestones unlock commemorative medals (記念メダル / 勲章) kept permanently in the character's collection. Claims grant nothing else.

## Reward policy (#1168)

The commemorative medal and the spendable Small Medal (ちいさなメダル) currency are separate effects. Claims award only the commemorative medal; they never change `characters.small_medals`.

- **Legacy reference:** the original game has no milestone achievement system. Only two paths credit the Small Medal balance: using the 小さなメダル item (`lib/_data.cgi:759-763`; the item comes from dungeons, contests, referrals and alchemy) and the god's wish (`lib/god.cgi:33`). Medals are spent at `medal.cgi`'s 15 tiers (`lib/medal.cgi:18-36`). No original path grants Small Medals for lifetime counters.
- **Approved extension:** the achievement and commemorative medal collection itself was explicitly requested by #70 and #358. #70 asked for commemorative medals only, not spendable currency.
- **Removed addition:** the catalog previously also credited 68 Small Medals in total across the 18 achievements (1–10 each). No approval for that extra supply exists, so it was removed to keep the original medal economy.
- **Existing data:** claims and commemorative medals already recorded stay valid. Balances credited earlier are not rolled back (the project is pre-release, with no production data).

## Claims

A completed achievement can be claimed once, with `achievement_claim` in the [medal castle](medal.md) scene. The claim and its commemorative medal commit together. Metrics and the catalog live in [`achievement.go`](../../internal/medal/achievement.go) and [`achievements.json`](../../internal/medal/achievements.json).

Transport operations and schemas: [OpenAPI](../api/openapi.json).
