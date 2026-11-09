# Standard Shops Design

## Purpose

This document records legacy rules, decisions and open gaps for the town shops (weapon, armor, item and accessory) and the secret shop. Catalog tiers, prices, dialogue and transport are owned by code and [OpenAPI](../api/paths/shop.json); other commerce facilities have their own documents ([Store](store.md), [Flea Market](fleamarket.md), [Auction](auction.md), [Black Market](black-market.md), [Accessory Shop](accessory-shop.md)).

## Helper quest exclusion

Items requested by helper quests are not sold (`system.cgi:1424–1438`, filtered by item kind). **Pitfall:** legacy armor batch purchase filters helper kind 1 (`armor.cgi:120`) while single armor purchase uses kind 2 (`armor.cgi:52`); Go filters armor definition IDs consistently, and the difference is unreconciled (#947).

## 5. Purchase Routing and Depot Auto-Transfer

- A single purchase goes to the inventory only when the relevant legacy hand field is empty: main hand for weapons, body for armor, and the held-tool slot for shields, accessories and consumables. Otherwise, and for every quantity above 1, goods go to the Depot.
- Depot delivery ignores capacity and may exceed it (`send_item`, `system.cgi:818–837`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- Batch purchase (`まとめてかう`: `weapon.cgi:116`, `armor.cgi:115`, `item.cgi:137`) exists only in the weapon, armor and item shops and always delivers to the Depot.

## 7. Secret shop

Access requires job level 7 or more (`item.cgi:57–69`); the secret shop sells its eight items at three times the base price (`secret.cgi` sales). Stored uncompleted helper requests exclude their targets, also after expiry, until no other uncompleted request needs them ([Helper lifecycle](rescue-and-helper.md#lifecycle-and-expiration)); a failed helper read aborts listing or purchase before any change (`secret.cgi:44–51`). Inventory delivery registers the item collection (`secret.cgi:44–46`); Depot delivery does not. Puff-puff (`ぱふぱふ`) has no effect beyond its dialogue.

### 7.4. Purchase Transport and Remaining Migration

Purchase uses the Gateway's `secretshop_purchase` with explicit item and quantity ([command contract](../architecture/client-agent-api.md#secretshop-purchase-command-contract)). Puff-puff uses `secretshop_puffpuff`, an NPC act with no inputs: like legacy `pafupafu` (`$to_name = $m`), its actor row and NPC row are private to the actor ([NPC interaction](../architecture/place-log.md#npc-interaction-1940)). No SecretShop REST route remains. **Known difference:** legacy `kau` (`secret.cgi:34–82`) buys one item, while Go accepts 1–99 and sends bulk quantities to the Depot; this is not an approved replacement of the single-purchase rule.

## 10. Selected Shop observation and migration boundary

The four ordinary shops map directly to scene destinations through the shared [navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery). Accessory products stay ordinary item catalog entries. Observation never invokes secret discovery, purchase, sale or synthesis. Go quantity bounds (1–9999) are not legacy: the original bought one item per purchase, and batch input named several items.

### Scoped legacy action/call-path reconciliation

Shop files load through `party.cgi:14–30`; `system.cgi:9–24` registers shared controls and `system.cgi:639–655` dispatches with the action-time guard. Purchase (`かう`), sale (`うる`), batch purchase, secret discovery (`ひみつのみせ`) and accessory synthesis (`ごうせい`, `accessory.cgi:100–101,172–292`) still run through REST; their Gateway migration is open (#947). `get_depot` is an uncalled utility, not an omitted action. Talk and inspect use the shared NPC commands; the static dialogue still omits legacy dynamic recommendations (such as the accessory shop's random item line), so exact dialogue parity is not claimed.

Open gaps (#947):

- Legacy weapon and armor listings show nominal strength and weight; the catalog shows only price and slot.
- Standard purchase does not enforce sales-tier membership, unlike batch and accessory purchase.
- A failed helper read in standard purchase is treated as no exclusion.

### Retained routes

Shop REST routes (catalog, purchase, sale, batch purchase, secret discovery and the accessory buy/sell/synthesize/recipes routes) remain until verified replacements exist (#947).
