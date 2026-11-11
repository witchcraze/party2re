# Player Store Design

## Overview

Player stores (`store.cgi`) are boutiques built in the four towns, listing Depot items for gold or for barter, with signboard, wallpaper and furniture customization. Prices, limits and transport are owned by code and [OpenAPI](../api/openapi.json).

- Store deliveries (purchase, both sides of a barter, withdrawal) go to the Depot past capacity if needed (`store.cgi:386–394,468–476,544–555`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- Open: the ten-store town limit is not enforced across concurrent builders (#1114).
- Open: legacy `kau` pays the owner through `send_money` as a pending remittance (`<buyer>から店の支払い`) and `kokan_other` sends both parties a Depot delivery notice; Go credits the wallet at once and sends no notice (#2056).

## Town scenes and commands

**Decision (2026-11-11):** the user asked that the client experience follow the original. As in `_town.cgi`, stores are found, built and entered inside the town where they stand: each of `town1`..`town4` is a scene under the town hub, and selecting a store (`scene_select`, `target_kind:store`) is legacy `はいる`. A store from another town, or an expired one, is unavailable. The selected store offers the owner or visitor action set of `store.cgi:33–64`.

| Legacy action | Replacement |
| --- | --- |
| `_town.cgi` `みせ` | `store_build` (town scene) |
| `_town.cgi` `はいる` | `scene_select` on a town scene; `いどう` back is `scene_back` |
| `みる` | store scene observation (listings in `list_sort` order) |
| `しゅっぴん`, owner `こうかん`, `もどす` | `store_list_gold`, `store_list_barter`, `store_withdraw` |
| `かう`, visitor `こうかん` | `store_buy`, `store_trade` |
| `かんばん`, `かべがみ`, `いんてりあ`, `なづける`, `かたづけ` | `store_rename`, `store_wallpaper`, `store_interior_add`, `store_interior_rename`, `store_interior_clean` |

Commands name explicit Depot instances, listings and interiors instead of legacy item names or listing numbers. Legacy `なづける` renames every interior with the given name, because its stop flag `$f_end` is never set (`store.cgi:696`); Go renames the one selected interior.

## Legacy findings

- **Construction** (`_town.cgi:89–127`): `＠みせ` takes only a house style. The name is the owner's registered name, otherwise `@<name>の店@`; rejection order is funds, town capacity, already-owned store. Expiry removes only the town entry, so a rebuilt store keeps its name, wallpaper, furniture and listings, and neither the owner nor visitors can act in it until it stands again.
- **Signboard name** (`store.cgi:298`): `is_build_store` compares against every town member name, so the owner's current name is also rejected. Signboard names forbid ASCII `@`, `/` and `\`; interior names allow them (`store.cgi:672–684`).
- **Barter wish** (`store.cgi:208`): an empty wish passes validation and creates a listing that can never be traded; Go rejects it.
- **Collection** (`store.cgi:175–176,276–277`): listing calls `add_collection`, which registers the lister's equipped items, not the listed or received item; purchase and barter register nothing. The listing-time rule is open (#2057).

## Customization charges (#1146)

**Legacy behavior.** The signboard (`store.cgi:289–313`, `kanban`) shows a 5,000 G fee and rejects a smaller balance, and furniture (`store.cgi:626–661`, `interior`) shows a 1,000 G price and rejects a smaller balance. **Pitfall:** neither routine nor the common dispatcher (`system.cgi:639–655`) deducts the gold, so the check is only a balance threshold. Wallpaper (`store.cgi:593–620`) does deduct its price.

**Decision (#1146, 2026-10-19):** the user chose 「原典の観測動作に復元する（看板5,000G・家具1,000Gの残高検査のみ行い、実行時にゴールドを控除しない）」. Renaming the signboard requires at least 5,000 G and placing furniture at least 1,000 G, but neither deducts gold; wallpaper keeps its deduction. An exact-threshold balance is accepted.
