# Standard Shops Design

## Purpose

This document records legacy rules, decisions and open gaps for the town shops (weapon, armor, item and accessory) and the secret shop. Catalog tiers, prices, synthesis recipes, dialogue and transport are owned by code and [OpenAPI](../api/base.json).

## Helper quest exclusion

Items requested by helper quests are not sold (`get_helper_item`, filtered by legacy item kind and number). In every shop, including the secret shop, a helper read failure aborts the listing or purchase before any change. **Pitfall (reproduced):** armor batch purchase passes the weapon kind (`armor.cgi:119`) while single armor purchase passes the armor kind (`armor.cgi:52`). A requested armor can still be bought in a batch, and armor-NN leaves the batch listing while weapon-NN is requested. Catalog products therefore report single and batch listing membership separately.

## 5. Purchase Routing and Depot Auto-Transfer

- `かう` buys one unit. It fills the matching [legacy hand field](items-and-equipment.md#legacy-hand-fields) when that field is empty, and otherwise goes to the Depot.
- Depot delivery ignores capacity and may exceed it (`send_item`, `system.cgi:818–837`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- Batch purchase (`まとめてかう`: `weapon.cgi:116`, `armor.cgi:115`, `item.cgi:137`) exists only in the weapon, armor and item shops and always delivers to the Depot. Legacy splits the typed names on `<>` and buys each one, so a repeated name buys another unit; a batch line's quantity stands for that repetition.

## 7. Secret shop

The item shop's `＠ひみつのみせ` (`item.cgi:45–67`) is registered in `%actions` but not `@actions`: only the keeper's `しらべる` reply hints at it, and below job level 7 it returns without a message. Go offers `shop_secretshop` in the item shop only to characters the secret shop admits, and the secret shop is entered only from the item shop. The secret shop sells its eight items at three times the base price (`secret.cgi` sales). Stored uncompleted helper requests exclude their targets, also after expiry, until no other uncompleted request needs them ([Helper lifecycle](rescue-and-helper.md#lifecycle-and-expiration)). Inventory delivery registers the item collection (`secret.cgi:61–62`); Depot delivery does not. Puff-puff (`ぱふぱふ`, `secretshop_puffpuff`) has no effect beyond its dialogue; like legacy `pafupafu` (`$to_name = $m`), its actor and NPC rows are private to the actor ([NPC interaction](../architecture/place-log.md#npc-interaction)).

## 10. Shop scenes and commands

The four ordinary shops map directly to scene destinations through the shared [navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery). Accessory products stay ordinary item catalog entries. Each command names its shop explicitly (`shop_type`), so later navigation cannot redirect it.

### Scoped legacy action/call-path reconciliation

Shop files load through `party.cgi:14–30`; `system.cgi:9–24` registers shared controls and `system.cgi:639–655` dispatches with the action-time guard.

| Legacy action | Gateway | Note |
|---|---|---|
| `かう` (four shops) | `shop_purchase` | The scene lists the catalog with nominal strength/weight in the weapon and armor shops. |
| `うる` | `shop_sell` | Only the item in the shop's hand sells (`sale_offer`): weapon, armor, or the tool hand in the item and accessory shops. |
| `まとめてかう` (weapon, armor, item) | `shop_batch_purchase` | Always delivers to the Depot. |
| `ひみつのみせ` (item shop) | `shop_secretshop` | See [secret shop](#7-secret-shop). |
| `ごうせい` (accessory, `accessory.cgi:100–101,172–292`) | `shop_synthesize` | Legacy fails when continuous `rand(100) > rate` (`accessory.cgi:184`); Go succeeds when `random.Intn(100) < rate`. For the catalog's integer rates both give the displayed percentage, so the different operators are not a probability bug. |
| `はなす` / NPC `しらべる` | `npc_talk` / `npc_inspect` | Talk uses each shop's `@words`, including the lines built from the actor's facts and a random product of the actor's sales tier. |

Each success logs `＠verb>target` and the keeper's reply. Missing `ごうせい` materials are a logged reply that changes nothing (`accessory.cgi:204–206`).

`get_depot` is an uncalled utility, not an omitted action.
