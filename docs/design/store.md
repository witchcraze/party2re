# Player Store Design

## Overview

Player stores (`store.cgi`) are boutiques built in the four towns, listing Depot items for gold or for barter, with signboard, wallpaper and furniture customization. Prices, limits and transport are owned by code and [OpenAPI](../api/openapi.json).

- Buyers register the acquired item in their collection after a purchase or barter (`party2/lib/store.cgi:175–176,276–277`).
- Store deliveries (purchase, both sides of a barter, withdrawal) go to the Depot past capacity if needed (`store.cgi:386–394,468–476,544–555`; [delivery capacity](depot.md#delivery-capacity-by-caller)).
- Open: the ten-store town limit is not enforced across concurrent builders (#1114).

## Customization charges (#1146)

**Legacy behavior.** The signboard (`store.cgi:289–313`, `kanban`) shows a 5,000 G fee and rejects a smaller balance, and furniture (`store.cgi:626–661`, `interior`) shows a 1,000 G price and rejects a smaller balance. **Pitfall:** neither routine nor the common dispatcher (`system.cgi:639–655`) deducts the gold, so the check is only a balance threshold. Wallpaper (`store.cgi:593–620`) does deduct its price.

**Decision (#1146, 2026-10-19):** the user chose 「原典の観測動作に復元する（看板5,000G・家具1,000Gの残高検査のみ行い、実行時にゴールドを控除しない）」. Renaming the signboard requires at least 5,000 G and placing furniture at least 1,000 G, but neither deducts gold; wallpaper keeps its deduction. An exact-threshold balance is accepted.
