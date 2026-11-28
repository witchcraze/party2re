# Town Black Market, Rare Point Barter & NPC @闇商人 Design

The Black Market (`internal/blackmarket`, legacy `party2/lib/black_market.cgi`, NPC `@闇商人` at `闇市場`) is a barter exchange, not a gold shop: characters sacrifice rare items for Rare/U-Rare points and trade those points for prizes delivered to their Depot. Eligible items, yields and prizes are owned by `internal/blackmarket/data/`.

## Access

Legacy `闇市場` is reachable only through the Oracle's hidden `＠やみいちば` action, which returns without moving the character below `job_lv` 15 (`lib/goods.cgi:51–56`). The scene's parent is therefore the Oracle shop, not the town list, and it is entered only from there: `oracle_blackmarket` enters it, and entry from any other scene is rejected. `CheckEligibility` applies the same threshold to scene entry, the two commands and the retained NPC operations; rejected operations return `ErrAccessDenied` without moving assets.

## Gateway and Observation

Selecting `blackmarket` exposes owned Rare/U-Rare points, the visible prizes and a paged list of eligible sacrifice candidates through character context. Observation never sacrifices, trades, chooses dialogue or creates a Depot. See the [context and recovery contract](../architecture/client-agent-api.md).

### Scoped legacy action reconciliation

| Legacy action (`lib/black_market.cgi`) | Go disposition | Known difference |
| --- | --- | --- |
| `header_html` (97–106) | `rare_points`, `u_rare_points` | Equipped names are not repeated in the scene. |
| `とりひき` without a target (132–152) | `prizes`, `u_prizes`; `u_prizes` is empty while U-Rare points are zero | None for listing visibility. |
| `とりひき` with a target (116–125, 135–144) | `blackmarket_trade` | Explicit prize ID replaces the name. A U-Rare trade at zero points rejects instead of redisplaying the list; neither moves assets. A full Depot still receives the prize ([delivery capacity](depot.md#delivery-capacity-by-caller)). |
| `ささげる` without a target (168–172, 200–250, 310) | `sacrifice_candidates` | Lists eligible instances only, inventory then Depot; equipped instances come first with `equipped`, as legacy lists the hand slots first. Legacy lists every owned item and rejects common ones when offered. The empty-handed message is an empty array. |
| `ささげる` with a target (179–300) | `blackmarket_sacrifice` | Explicit instance ID replaces legacy's Depot-first name match. |
| Shared `はなす` (`lib/system.cgi:240–246`), `shiraberu_npc` (81–83) | `npc_talk`, `npc_inspect` | None: talk logs the word and starts the wait; inspect is private. |

Each success logs `＠ささげる>name` or `＠とりひき>name` and the dealer's reply. Legacy also logs `…ダメだな…` for a common item offered to `ささげる` (194–243); Go never offers common items, so that row has no counterpart.
