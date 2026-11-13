# Town Black Market, Rare Point Barter & NPC @闇商人 Design

The Black Market (`internal/blackmarket`, legacy `party2/lib/black_market.cgi`, NPC `@闇商人` at `闇市場`) is a barter exchange, not a gold shop: characters sacrifice rare items for Rare/U-Rare points and trade those points for prizes delivered to their Depot. Eligible items, yields and prizes are owned by `internal/blackmarket/data/`.

## Access

Legacy `闇市場` is reachable only through the Oracle's hidden `＠やみいちば` action, which returns without moving the character below `job_lv` 15 (`lib/goods.cgi:51–56`). The scene's parent is therefore the Oracle shop, not the town list; `oracle_blackmarket` enters it. `CheckEligibility` applies the same threshold to scene entry, the two commands and the retained NPC operations; rejected operations return `ErrAccessDenied` without moving assets.

## Gateway and Retained Routes

Selecting `blackmarket` exposes owned Rare/U-Rare points, the visible prizes and a paged list of eligible sacrifice candidates through character context. Observation never sacrifices, trades, chooses dialogue or creates a Depot. A required read failure fails the whole observation, and a qualification loss shows `selection_unavailable` with a safe back. See the [context and recovery contract](../architecture/client-agent-api.md).

| Former or retained REST operation | Supported replacement / disposition |
| --- | --- |
| GET `/characters/{id}/blackmarket`, GET `.../blackmarket/points` (retired) | POST `scene_enter` with `destination:blackmarket`, then GET `/api/v1/characters/{id}/context` |
| POST `.../blackmarket/sacrifice` (retired) | POST `/api/v1/characters/{id}/actions`, `action:blackmarket_sacrifice`, explicit `params.item_instance_id` |
| POST `.../blackmarket/trade` (retired) | Same Gateway, `action:blackmarket_trade`, explicit `params.prize_id`; the result keeps the delivered Depot instance and remaining points |
| POST `.../blackmarket/talk`, POST `.../blackmarket/inspect` (retired) | Same Gateway, `action:npc_talk` / `action:npc_inspect` in the Black Market scene; both require the scene's qualification |

### Scoped legacy action reconciliation

| Legacy action (`lib/black_market.cgi`) | Go disposition | Known difference |
| --- | --- | --- |
| `header_html` (97–106) | `rare_points`, `u_rare_points` | Equipped names are not repeated in the scene. |
| `とりひき` without a target (132–152) | `prizes`, `u_prizes`; `u_prizes` is empty while U-Rare points are zero | None for listing visibility. |
| `とりひき` with a target (116–125, 135–144) | `blackmarket_trade` | Explicit prize ID replaces the name. A U-Rare trade at zero points rejects instead of redisplaying the list; neither moves assets. A full Depot still receives the prize ([delivery capacity](depot.md#delivery-capacity-by-caller)). |
| `ささげる` without a target (168–172, 200–250, 310) | `sacrifice_candidates` | Lists eligible instances only, inventory then Depot; equipped instances come first with `equipped`, as legacy lists the hand slots first. Legacy lists every owned item and rejects common ones when offered. The empty-handed message is an empty array. |
| `ささげる` with a target (179–300) | `blackmarket_sacrifice` | Explicit instance ID replaces legacy's Depot-first name match. |
| Shared `はなす` (`lib/system.cgi:240–246`), `shiraberu_npc` (81–83) | `npc_talk`, `npc_inspect` | None: talk logs the word and starts the wait; inspect is private. |

Legacy successful actions also apply the shared action wait and write `npc_com` to the facility log, while list views set `$act_time = 0`. The Gateway now applies the [action wait](../architecture/place-log.md#action-wait); writing these commands' log rows remains [#1941](https://github.com/witchcraze/party2re/issues/1941), and presence remains open.
