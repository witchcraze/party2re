# Standard Shops Design

## Purpose

This document records legacy rules, decisions and open gaps for the town shops (weapon, armor, item and accessory) and the secret shop. Catalog tiers, prices, dialogue and transport are owned by code and [OpenAPI](../api/base.json); other commerce facilities have their own documents ([Store](store.md), [Flea Market](fleamarket.md), [Auction](auction.md), [Black Market](black-market.md), [Accessory Shop](accessory-shop.md)).

## Helper quest exclusion

Items requested by helper quests are not sold (`get_helper_item`, filtered by legacy item kind and number). In every shop, including the secret shop, a helper read failure aborts the listing or purchase before any change. **Pitfall (reproduced):** armor batch purchase passes the weapon kind (`armor.cgi:119`) while single armor purchase passes the armor kind (`armor.cgi:52`). A requested armor can still be bought in a batch, and armor-NN leaves the batch listing while weapon-NN is requested. Catalog products therefore report single and batch listing membership separately.

## 5. Purchase Routing and Depot Auto-Transfer

- A single purchase goes to the inventory only when the relevant legacy hand field is empty: main hand for weapons, body for armor, and the held-tool slot for shields, accessories and consumables. Otherwise, and for every quantity above 1, goods go to the Depot.
- Depot delivery ignores capacity and may exceed it (`send_item`, `system.cgi:818–837`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- Batch purchase (`まとめてかう`: `weapon.cgi:116`, `armor.cgi:115`, `item.cgi:137`) exists only in the weapon, armor and item shops and always delivers to the Depot.

## 7. Secret shop

Access requires job level 7 or more (`item.cgi:57–69`); the secret shop sells its eight items at three times the base price (`secret.cgi` sales). Stored uncompleted helper requests exclude their targets, also after expiry, until no other uncompleted request needs them ([Helper lifecycle](rescue-and-helper.md#lifecycle-and-expiration)). Inventory delivery registers the item collection (`secret.cgi:44–46`); Depot delivery does not. Puff-puff (`ぱふぱふ`) has no effect beyond its dialogue.

### 7.4. Purchase Transport and Remaining Migration

Purchase uses the Gateway's `secretshop_purchase` with explicit item and quantity ([command contract](../architecture/client-agent-api.md#secretshop-purchase-command-contract)). Puff-puff uses `secretshop_puffpuff`, an NPC act with no inputs: like legacy `pafupafu` (`$to_name = $m`), its actor row and NPC row are private to the actor ([NPC interaction](../architecture/place-log.md#npc-interaction)). No SecretShop REST route remains. **Known difference:** legacy `kau` (`secret.cgi:34–82`) buys one item, while Go accepts 1–99 and sends bulk quantities to the Depot; this is not an approved replacement of the single-purchase rule.

## 10. Shop scenes and commands

The four ordinary shops map directly to scene destinations through the shared [navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery). Accessory products stay ordinary item catalog entries. Each command names its shop explicitly (`shop_type`), so later navigation cannot redirect it. Go quantity bounds (1–9999) are not legacy: the original bought one item per purchase, and batch input named several items.

### Scoped legacy action/call-path reconciliation

Shop files load through `party.cgi:14–30`; `system.cgi:9–24` registers shared controls and `system.cgi:639–655` dispatches with the action-time guard.

| Legacy action | Gateway | Note |
|---|---|---|
| `かう` (four shops) | `shop_purchase` | The scene lists the catalog with nominal strength/weight in the weapon and armor shops. |
| `うる` | `shop_sell` | Only the item in the shop's hand sells (`sale_offer`): weapon, armor, or the tool hand in the item and accessory shops. |
| `まとめてかう` (weapon, armor, item) | `shop_batch_purchase` | Always delivers to the Depot. |
| `ひみつのみせ` (item shop) | `scene_enter` `secretshop` | Qualified navigation (job level 7). Legacy enters only from the item shop; Go enters from town. |
| `ごうせい` (accessory, `accessory.cgi:100–101,172–292`) | `shop_synthesize` | The accessory scene lists the recipes; see [Accessory Shop](accessory-shop.md#synthesis). |
| `はなす` / NPC `しらべる` | `npc_talk` / `npc_inspect` | Static dialogue omits legacy dynamic recommendations (such as the accessory shop's random item line); exact dialogue parity is not claimed. |

`get_depot` is an uncalled utility, not an omitted action. No ordinary Shop REST route remains.
