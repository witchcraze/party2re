# Accessory Shop and Synthesis

The accessory shop (`lib/accessory.cgi`, NPC @ミラ) sells items, resells the
held item, and synthesizes depot materials. The original CGI defines the
rules below. `internal/shop` is their current Go representation; merged #779
does not certify complete behavioral parity.

## Catalog and prices

Catalog tiers use **job-change count (`job_lv`)**, not character level.
Legacy item numbers map to `item-NNN` in the reconstruction:

| JobLevel | Legacy item numbers |
|---|---|
| Below 50 | 147, 148, 158, 169, 175, 222 |
| 50–99 | 144–148, 158, 169, 175, 218, 219, 220, 222, 223, 225, 226, 228, 229, 230, 248 |
| 100+ | 143–151, 158, 169, 175, 218, 219, 220, 222, 223, 225, 226, 228, 229, 230, 181, 182, 189, 190, 248 |

Retail price is 10 times the base price. Items 150 and 151 apply another
100-fold multiplier (1000 times base). Items currently requested by helper
quests are excluded. Resale is floor(base price × 0.5).

With an empty held-item slot, the purchased item becomes the held item and is
recorded in collection; with an occupied slot, it is sent to depot. The Go
transport also supports quantities through the existing shop application
contract; that does not imply a different legacy equipment-slot layout.

## Synthesis

Legacy `@acces` contains 48 recipes, each with two named depot materials,
displayed success rate, and target selector. Each material is consumed once;
do not infer doubled ingredient quantities from a fabricated recipe list.
The first recipe is **命の宝珠 = 命の木の実 + イエローオーブ**, displayed at 30%.

The detailed legacy recipe source is `lib/accessory.cgi:33–86`; current ID
mapping is in `internal/shop/synthesis_recipes.go`. The former Markdown recipe
table contained different products, ingredients, and probabilities and was
removed. Do not use it to implement or test synthesis.

`check_depot` checks/consumes the materials; failed synthesis also consumes
them. A held synthesis elixir (item 180) guarantees success and is consumed
when used. Successful products are delivered to depot. There is no gold fee.

Legacy fails when continuous `rand(100) > rate` and otherwise succeeds
(`accessory.cgi:184`). Current Go uses integer `random.Intn(100) < rate`.
For the catalog's integer rates, both produce the displayed success percentage;
the different comparison operators alone do not establish a probability bug.

**Elixir eligibility difference:** the legacy check requires item 180 in the
held-item slot (`accessory.cgi:175`). Go scans all inventory items and consumes
the first item-180 it finds, whether equipped or not. Preserve the held-item
condition as the specification when reconciling inventory/equipment modeling.

## Implementation boundary

Current synthesis locks character, inventory (for held elixir), then depot
inside one injected transaction. Catalog, purchase, sell, and synthesis routes
are documented in [shop OpenAPI sources](../api/paths/shop.json).
NPC dialogue/inspect routes are shared shop routes. There is no separate
accessory `/talk` or `/inspect` route, and response envelopes vary by operation.
