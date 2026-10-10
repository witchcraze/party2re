# Client & Agent API — CQRS & Server-Driven UI Architecture

This document defines the HTTP observation and command contracts, service boundaries and client recovery behavior. See [STATUS](../../STATUS.md) for major current capabilities/gaps and [ROADMAP](../../ROADMAP.md) for remaining milestones; detailed progress and dependencies are tracked in GitHub.

Current clients use the registered routes in [OpenAPI](../api/openapi.json).
`GET /api/v1/characters/{id}/context` and `POST /api/v1/characters/{id}/actions`
are registered. Existing REST routes remain unversioned. Do not remove working REST contracts
before their replacement is implemented and verified against the original Party2.

---

## 1. Core Architecture: CQRS & Action Gateway

In game domain engineering and agentic computing, gameplay interactions are fundamentally **state transition commands** rather than CRUD resource manipulations.

Instead of scattering game logic across hundreds of disconnected REST URLs, the client boundary is organized into two authoritative pillars:

```text
               +-------------------------------------------+
               | Client (Web UI / AI Agent / Line / Discord)|
               +--------------------+----------------------+
                                    |
          Query (Observe)           |         Command (Execute)
          GET /characters/{id}/context        POST /characters/{id}/actions
                                    |
               +--------------------v----------------------+
               |          Client / Agent Gateway           |
               |                                           |
               |  - Action Evaluator (Availability Engine) |
               |  - Action Dispatcher (Command Execution)  |
               |  - Action Catalog (Parameter Validation)  |
               +--------------------+----------------------+
                                    |
               +--------------------v----------------------+
               |       Domain Services & Persistence       |
               |  (Bank, Adventure, Shop, Home, Combat)    |
               +-------------------------------------------+
```

| Pillar | Endpoint | Responsibility | Payload |
|---|---|---|---|
| **Query (Observe)** | `GET /api/v1/characters/{id}/context` | **Observation & Next Moves** | Lightweight character snapshot, active timers, and eligible action entry candidates. Exact parameters remain service-validated. |
| **Command (Execute)** | `POST /api/v1/characters/{id}/actions` | **State Transition** | Accepts `{ action, params }`, dispatches to domain services, and returns `{ result, context }`. |

---

## 2. Query Pillar: `GET /api/v1/characters/{id}/context`

The authenticated discovery endpoint verifies character ownership using the
standard HTTP wrapper, calls the existing uncached `playercontext.Service.Query`,
and enriches its facts with profile avatar and job catalog presentation. Its
`PlayerContextResponse` DTO and reusable OpenAPI schema are also the required
context contract for the command gateway.

The four top-level fields are:

- `character`: ID/name/job ID and name, level, HP/MP maxima and current values,
  wallet gold, fatigue, death and sleeping flags, and icon fields. `icon_url`
  uses the existing profile AvatarURL (URL or data URI), or an empty string.
  The asset ID remains empty until production mappings are specified.
- `scene`: registered `kind`, selected `location_id`, optional `subject`, typed
  `data`, connection/eligibility `support`, and HTTP-owned presentation fields.
  Town uses a self-authored SVG placeholder; Home uses typed public/owned details
  and mailbox pages. Other initial facility/subject scenes contain controls only.
  Actual activities use a typed owned-facts projection; conflicting activities
  use an explicit recovery scene. Production art remains separate work.
- `ongoing_actions`: every unfinished scheduled action, plus an observed
  sleep/wake recovery timer when present. Empty observations return `[]`.
  Entries contain `id`, `action_type`, `label`, `execute_at`, rounded-up
  nonnegative `remaining_seconds`, and `is_ready`. Scheduled deadlines are
  ordered ascending, with ID breaking ties. Sleep deadlines are estimates
  derived from the remaining lock duration, not persisted queue records.
- `available_actions`: connected, scene-appropriate eligible commands in catalog
  order, with `action`, `label`, `category`, `style`, `required_params` (always an
  array), strict `params_schema` and optional explicit `params_template`.
  Adventure controls use `primary`; other controls use `secondary`.

### Invariants

- **Entry eligibility**: catalog gating is action-specific. HP=0 excludes
  combat entries, but permits legacy noncombat town actions. Free prayer,
  crystal-funded sealing and coin games do not require wallet gold.
  Exact prices, amounts, alternative currencies, items and game state still
  require validation by the execution service.
- **Unfinished work**: Pending/Processing entries remain visible and block
  applicable actions after ExecuteAt. `is_ready` denotes deadline arrival,
  never successful settlement. Refresh context with bounded retry delays for
  overdue work; do not infer completed mutations from a countdown.
- **Sleep recovery**: active sleep exposes rescue; expired sleep with recovery
  pending exposes wake and rescue. The sleep observation remains until explicit
  wake recovery clears it, with `is_ready: true` and zero remaining seconds.
- **Failure boundary**: session and ownership errors return 401/403 (missing
  characters return 404). Query errors and propagated profile-service errors return 500 without partial
  context. An unconfigured query service or selected-scene adapter returns 501
  after authentication. Query rechecks the owned viewer before selection or
  facility reads; enrichment checks profile ownership again.
- **Read consistency**: authorization, query facts and profile reads are not a
  cross-store transaction. No observation/result cache is introduced; command
  execution must revalidate current state. Character facts come from the query
  snapshot, not the separate profile view's character projection.

### Approved navigation and progressive observation contract

The direction approved in [#1047](https://github.com/witchcraze/party2re/issues/1047)
on 2026-10-06 replaces the initial globally broad town menu with choices derived
from the current facility, previously selected subject and actual activity.
This section specifies the replacement. Ordinary selection is implemented;
facility composition and progressive action discovery remain separate work.
Selection storage does not authorize route deletion.

Clients retain two operations. GET observes the current interaction without a
view selector. POST changes location/selection or executes a feature operation.
For example, enter the weapon shop, select a product, then enter only quantity
on the purchase form. The server supplies the selected product ID in a command
template; the submitted mutation still explicitly identifies that product.

Ordinary facility and subject selection belongs to `playercontext`, behind a
small repository contract, using the existing Valkey deployment. Store one
bounded character-scoped record with registered scene/subject identifiers and
the selected collection page. Use a seven-day TTL renewed on successful
navigation/selection writes, without a background refresher. GET does not renew
or create this record. Missing/expired selection defaults to the town hub;
selection loss does not terminate an activity. No new SQL navigation table,
general form storage, navigation history stack or global game-state revision
is required. Concurrent navigation uses the last successful write; clients of
one character share its selection. See the [storage boundary](valkey-keyspace.md#approved-navigation-storage-boundary).

Sleep, room participation and active runs remain authoritative in their existing
feature services/stores. Their facts determine active scenes and permitted
continuations; selection cannot join, spectate, leave, escape, recover vitality
or settle rewards. Composition supplies public feature read adapters; the
navigation owner never reads another feature's private persistence directly.
`scene.location_id` identifies the observed interaction, not an invented Core
Character location or a location inferred from catalog categories.

The legacy persisted facility dispatch and home target are evidence for this
interaction model (`party2/party.cgi:14–30`, `lib/system.cgi:9–24,267–318,418–435`).
Valkey selection is deliberately ephemeral: the original persisted location
does not establish a promise that lost navigation must be restored. Legacy
presence/log updates on movement (`system.cgi:964–985`) need their own facility
reconciliation; merely changing the saved selector does not implement them.

#### Selection commands and typed discovery

The shared navigation commands are `scene_enter`, `scene_select`, `scene_page`
and `scene_back`. Destinations, subject kinds and paging inputs are closed,
typed contracts registered by the relevant adapter. Back means the registered
parent scene, not browser history. A facility/subject change clears subordinate
selection and paging. Navigation commands have no price, fatigue, scheduled
cooldown or implicit feature mutation. Recovery and active-session restrictions
still apply: browsing cannot bypass sleep or abandon a run.

The registry accepts `town`, `bank`, `depot`, `home`, `secretshop` and the four ordinary shops
(`shop_weapon`, `shop_armor`, `shop_item`, `shop_accessory`), with town as each
facility's parent. Shop subjects use `target_kind:item` and IDs validated by
the owned actor's existing Shop catalog, including its eligibility filters.
Home registers `target_kind:home` with a character ID validated by the public
Home service. Entering `home` observes the actor; selecting another ID observes
that target without changing the actor/viewer. `home_inbox` and `home_outbox` have
`home` as their parent and always read the owned actor, without a target input.
Public Home detail never offers owner mailbox destinations. Other subject kinds
remain adapter-owned.
SecretShop registers `target_kind:item` with its public status catalog IDs.
Its existing JobLevel ≥7 qualification is wired at composition through the pure
`SceneDefinition.CanEnter` character predicate. Town and the `scene_enter` input
schema omit disallowed destinations without reading their catalogs.
Enter/select/page recheck qualification against the owned query snapshot and
return `403 SCENE_ACCESS_DENIED` before a navigation
write. A saved disallowed selection becomes `selection_unavailable` without a
facility/subject read; back and entry to another permitted destination remain
available. Feature execution still rechecks qualification independently.
`scene_back` clears a selected subject to its facility list; otherwise it selects
the registered parent. Enter/select reset subject/page descendants.

Page input requires `destination` and exactly one of `offset` (0–1,000,000) or
`cursor`, with optional `limit` (default 20, maximum 100). It applies only to a
registered pageable list without a selected subject and must match the saved
destination. Only Home inbox/outbox accept cursors: an empty string starts their
existing keyset reader, while later pages reuse its timestamp/ID tokens. Tokens
are URL-safe base64 strings bounded to 512 characters; existing service cursor
decoding and ordering are retained. Offset pages retain totals; cursor pages
return the service next token without inventing a total or a cross-page snapshot.
Mixed/unknown fields and null values are rejected. Town, Depot, ordinary Shop and
SecretShop lists remain offset-only. Back never accepts a caller-supplied parent.

GET and command refresh share the HTTP selected-scene composer, replacing interim
`scene.navigation` metadata with `town`, `facility`, `subject`, or
`selection_unavailable` variants. Home facility/subject data contains a whitelisted `view`, parent and own-only
mailbox destinations; its mailbox scenes contain bounded letters and next-page
inputs. The four ordinary Shops expose one ID-ordered, offset-paged `items`
collection, or one selected `product`, with exact GetCatalog prices/slot,
explicit select/purchase inputs, NPC inspect facts and service quantity bounds.
Home, Shop and Bank report `support.observation:details`. Bank facility data
contains `parent` and the owned `GetState` facts: `character_id`, wallet `money`,
int64 `deposit` and `max_deposit`, `npc_name` and the complete `dialogues` array.
The reader passes the incoming context and owned actor to the existing Bank
service. It does not invoke random NPC talk or change selection or savings.
Required Bank read failures fail the whole observation; missing configuration
returns 501. Actual activity overrides saved Bank selection before this reader
runs. Deposit/withdraw refresh uses this same projection, with known outcomes
preserved if the read fails and GET-only recovery after the source recovers.
Bank talk/inspect REST routes still require their independent action-effect
reconciliation and replacement/retirement verification under #947. The old Bank
state GET is retired: enter Bank with `scene_enter`, then GET character context.
GET never changes selection; actual activity can override the Bank scene.

Depot facility data reports `support.observation:details` with typed
`DepotSceneData`: `parent:town`, owned `character_id`, dynamic `capacity`,
purchased expansion count `ex_depot`, nullable `next_expansion_cost` in gold,
total occupied slots `item_count`,
non-null offset-paged `items` and existing page/next inputs. Rows whitelist
instance `id`, `definition_id`, `quantity` and `enhancement_level`; occupied
slots count instances, not their summed quantities. The adapter preserves
`GetDepot` order (currently instance ID ascending in persistence) so a future
feature-owned sort repair is not overridden by transport. No cross-page snapshot
is promised. Required character/depot read errors fail the whole observation;
missing depots yield an empty in-memory projection with dynamic capacity and
no save. Missing configuration returns 501. Actual activities/conflicts skip
ordinary Depot reads. Successful/rejected navigation survives failed enrichment
and recovers through GET without another selection write. GET never creates,
sorts or saves storage, moves items, records collection or schedules work.
The quote uses the public expansion price reader and is null at the purchased
expansion cap. It grants no reservation: execution charges the current tier
inside the existing service transaction. Clients explicitly enter Depot and
read GET context for status, and submit `depot_expand` to the Gateway to purchase;
the former Depot status GET, expansion POST and single/batch sale POST routes
return 404. Sales use `depot_sell` and `depot_sell_batch` through the Gateway.
Five inventory/transfer REST operations remain: deposit/withdraw/send/sort still
need command adapters and verified retirement;
[Depot design](../design/depot.md#gateway-observation-and-retained-operations)
records those operations and the remaining order/equipment/withdrawal gaps.

SecretShop facility/subject data reports `support.observation:details` with a
typed `SecretShopCatalogSceneData` or `SecretShopProductSceneData`. Both contain
`parent`, `title`, `npc_name`, `is_eligible:true` and quantity bounds 1..99. The
catalog uses ID-ordered offset pages with non-null `items`; product facts come
from `GetShopStatus` after required HelperQuest filtering. Each product supplies
explicit select/purchase IDs. Selected products template `secretshop_purchase`'s
`item_id`, while `quantity` remains required and editable. A removed/filtered
product is unavailable; required source errors fail the whole observation.
GET and refresh invoke no Talk/Inspect/PuffPuff action or purchase. Actual activity
overrides selection before SecretShop reads. Known purchase/navigation outcomes
survive a failed scene refresh, with GET-only recovery after the source recovers.

Town's primary collection is `data.destinations`, ordered by destination ID,
with explicit `enter_params`, observation support and offset page/next inputs.
Pages contain at most 100 rows, identifiers at most 128 characters, and
presentation fields use per-variant OpenAPI bounds. Rows contain no images;
the actor's existing avatar bounds are unchanged. `scene.support.actions`
separates `connected` from `entry_eligible`; execution still revalidates.

The composer uses registered facility command IDs, subjects and parents rather
than catalog categories. Offered actions copy their embedded OpenAPI command
schema, with selected constants and explicit templates. Home controls template
the observed home's `target_home_id`, while the authenticated actor remains the
sleeping character. Owner-private fields are never inferred from a target ID. Without a configured navigation store,
connected entry discovery remains usable. Dispatcher entry evaluation is
separate: a full explicit command never binds its target to the displayed scene.
Actual sleep, unfinished scheduling, Party/PvP/GvG/Casino membership and
Dungeon/Challenge buffers override ordinary selection. Activity observations
contain only kind, ID, owned role, phase, round/floor, an existing optional party
relation and eligible continuation IDs. Room secrets, cards, provisional rewards
and private member data are excluded. These observations use production
public-service readers; historical SQL records never determine activity.

`scene.kind:activity` carries `data.activities`; `activity_conflict` retains all
contradictory exclusive facts and only verified leave/recovery candidates.
An explicitly linked live Party roster and one Dungeon/Challenge run are one
activity, with the run selecting the scene. Multiple runs still conflict; no
missing relationship is inferred. Terminal run buffers remain observable until
their feature owner removes them; observing them never replays settlement.
Buffers indexed only by the controlling actor do not authorize other
participants to execute that actor's commands or invent a participant index.

Continuation eligibility comes from actual membership, role and phase, instead
of entry's town/cooldown/HP/fatigue/currency gates. Exact readiness, team setup,
prices and parameters remain service checks. Casino participants that have
already acted cannot act again in Indian/Highlow; Doppel retains its existing
service behavior. Spectators receive leave only. `support.actions.mode` marks
continuation/recovery controls with `entry_eligible:false`; only eligible controls
are listed, and `connected` still distinguishes implemented execution.
Unconnected continuation IDs have strict Gateway schemas and remain 501 until
their owning mutation migrations connect them. No REST route is retired here.

A disappeared subject stays explicit in `selection_unavailable`, with safe
back/reselect choices and no saved fallback. Required storage, subject-read and
adapter enrichment errors fail the whole observation. Navigation remains
blocked during any actual activity, pending recovery and unfinished work, and
writes no feature state.

#### Scoped legacy navigation reconciliation

The inspected sources are `party2/party.cgi:14–30` and
`lib/system.cgi:9–24,267–318,418–435,964–985` (paths relative to the original
Party2 distribution). The full shared `set_action` list and scoped call paths
reconcile as follows; facility-specific gameplay/actions retain their owners.

| Legacy action / dispatch | Replacement or deferred owner |
| --- | --- |
| `いどう` → `idou`, registered `@places` → saved `$m{lib}` | Gateway `scene_enter` → `playercontext.Service.Enter`; the initial closed registry covers the destinations above. Remaining facilities belong to #947–#949. |
| `まち` → `machi`, registered `@towns` → saved `$m{lib}` | Default town selection through `scene_enter` / `scene_back`; estate/town-specific projections remain #949 work. |
| `ほーむ` → `homu`, `$m{home}` target/fallback | Own `home` through `scene_enter` and public Home targets through `scene_select` → Home.GetHomeView. Query authenticates the actor; unavailable targets remain explicit. An unavailable target is explicitly retained instead of adopting the legacy fallback. |
| Reload dispatch: sleep before `$m{lib}`, default `park` | Shared GET/refresh derives owned activity/recovery before ordinary selection. Expired sleep requires explicit Wake; conflicting activities remain explicit. The selector never ends an activity. |
| `ぎるど` → `girudo` | Guild observation/management remains #949; no membership change through navigation. |
| `ささやき` → `sasayaki`, `はなす` → `hanasu`, `しらべる` → `shiraberu` | Facility/social dialogue and presence reconciliation remains #947–#949; existing REST readers/operations are retained. |
| `ろぐあうと` → `roguauto` (`system.cgi:944–961`) | Existing `DELETE /sessions` → Player.Logout supersedes the old index redirect; no navigation side effect. |
| `すくしょ` → `sukusho` (`system.cgi:165–196`); `br` separator | Stored photo acquisition/Contest reconciliation remains #949; client rendering belongs to #140. Separator superseded by structured actions. |
| Movement → `leave_member`, reload/log/presence | Deliberately excluded from selector writes; facility reconciliation belongs to #947–#949. No room join/leave, cooldown, recovery or settlement is introduced here. |

The full scoped Home action/call-path reconciliation and retained REST ownership
are maintained in [Home design](../design/home.md#observation-visibility-and-navigation).
This includes public/own dispatch, private mailbox paging, explicit Wake and
render-time notice acknowledgment departures; it is not a formula parity claim.
The [Shop reconciliation](../design/shops.md#scoped-legacy-actioncall-path-reconciliation)
maps every registered Shop action and its read/mutation branches, retained routes
and deferred differences. Shop observations do not connect purchase, sale, batch,
secret discovery, dialogue or synthesis mutations.

Subject selection and paging are the approved typed Gateway controls, not claims
of new gameplay or formula parity. No legacy source or assets are reused.

```json
{"action":"scene_enter","params":{"destination":"shop_weapon"}}
```

```json
{"action":"scene_select","params":{"target_kind":"item","target_id":"weapon-01"}}
```

Retain the four top-level observation slots and non-null collection arrays.
Extend `scene` with a registered kind, selected subject and one typed payload;
list, detail, room and run variants have their own schemas. Include only the
selected facility's information, alongside the shared actor/recovery facts.
Presentation fields remain HTTP-owned; domain services supply structured facts.

For example, the selected-product response has the following `scene`
fragment, alongside the existing character and the two non-null action arrays.
The selected ID is a catalog ID, not an inventory instance or authorization:

```json
{
  "location_id": "shop_weapon",
  "kind": "subject",
  "subject": {"target_kind": "item", "target_id": "weapon-01"},
  "data": {
    "product": {
      "id": "weapon-01",
      "purchase_params": {"item_definition_id": "weapon-01"}
    },
    "quantity": {"minimum": 1, "maximum": 9999}
  }
}
```

The fragment omits other required fields; [OpenAPI](../api/openapi.json) owns the
complete ShopCatalogSceneData/ShopProductSceneData schemas. Catalog membership
means current sale eligibility, without inventing a wallet/capacity guarantee.
GET and refresh never call purchase or random NPC talk. Common presentation
fields retain their existing contract. A required catalog/helper/inspect/price
read failure fails the whole observation; a disappeared product is explicit and
does not rewrite selection.

`available_actions` narrows to applicable facility operations, selection controls,
continuations and recovery choices. Static catalog membership alone does not
advertise execution support. Disclose adapter support separately from entry
eligibility, and do not offer an unconnected command as executable. Each offered
action supplies its strict `params_schema` and, where useful, `params_template`;
`required_params` still lists the complete command's required fields. Previously
selected values appear as constants/templates, so a client asks only for remaining
inputs without guessing IDs or parameter types. The following purchase-action
schema illustrates the planned mutation adapter contract; Shop purchases remain
unconnected and are not offered in `available_actions` by observation:

```json
{
  "action": "shop_purchase",
  "required_params": ["item_definition_id", "quantity"],
  "params_template": {"item_definition_id": "weapon-01"},
  "params_schema": {
    "type": "object",
    "additionalProperties": false,
    "required": ["item_definition_id", "quantity"],
    "properties": {
      "item_definition_id": {"type": "string", "const": "weapon-01"},
      "quantity": {"type": "integer", "minimum": 1}
    }
  }
}
```

The mutation sends the full typed params. Never fill an omitted mutation target
from mutable navigation, substitute a newer selection, or treat a template as
authorization. The service rechecks price, funds, inventory, membership and state.
Concurrent navigation cannot redirect a submitted operation to a different
target. Observation/preflight and execution remain separate reads/transactions;
no new cross-store atomicity or command replay guarantee is introduced.

#### Actor, target, paging and read failures

The owned path character is always the actor and viewer. Other home/store/room/
guild/listing IDs are targets. A selected public home never supplies the actor
for sleep or grants its private letters, notices, inventories or controls.
Viewer/visitor IDs come from authentication, not caller-supplied identity fields.
Public DTOs whitelist fields instead of serializing raw Character/HomeView/room
state. Casino lobby summaries exclude hidden cards/marks/actions; member,
spectator and nonparticipant detail follows verified game-specific visibility.
Selecting a room cannot bypass password or spectator admission.

Casino provides `ListRooms` lobby summaries and `GetRoomView` for owned admitted
participants/spectators. Its detail adapter verifies the session's player and
character ownership; the `character_id` query selects an owned character rather
than supplying a trusted viewer identity. Anonymous detail is 401 and nonmember
detail is 403. Projections whitelist fields and follow the
[game-specific visibility contract](../design/casino.md#observation-and-visibility).
The lobby adapter presents offset pages within the existing public window of
at most 100 rooms, ordered by ID, with explicit selection and next-page params.
Selected nonmember rooms expose lobby summaries only. Actual admitted activity
adds masked room/member facts, the actor's coins and role/phase game choices;
conflicts preserve these facts while suppressing start/kick/play. Required read
errors or changed admission/phase/turn eligibility fail the whole observation
and use the shared GET-only refresh recovery contract. GET does not renew
navigation or room lifetime; lobby reads retain Casino-owned idle expiry with
propagated errors. No operational Casino route is retired or mutation connected.

Select one primary pageable collection per scene. Reuse limit 20 by default and
maximum 100, with keyset cursors where the owning service supports them and
offset paging otherwise. Expose the mode and next-page inputs. Reject malformed,
mixed or cross-scene/target/filter paging; include an ID tie-breaker in ordering.
No cross-page snapshot consistency is promised. Lists/details use bounded typed
fields and existing media bounds; never repeat large inline image blobs per row,
silently truncate records, or load every facility into one observation. The
existing actor avatar can contain a data URI from a 2 MiB upload, so a smaller
blanket byte cap must not break it. Per-variant field/payload bounds are part of
the implementation's schema and acceptance tests, not an excuse to remove data.

A disappeared/expired saved subject produces a typed `selection_unavailable`
scene with safe back/reselect choices, without stale private data or persisting
a guessed fallback. Invalid selection inputs return 400; unknown targets return
404; access failures use the feature's mapped 403/404 without secrets. Storage/
required enrichment errors return 500 without partial observation; unconfigured
adapters return 501. Do not disguise failed reads as an expired subject/default.

GET and post-command refresh use the same selection and feature-read composition.
Known command success/rejection survives any failed refresh under section 3;
recover with GET only. Navigation itself follows the same outcome contract.
After join/start/leave/escape, actual feature state determines the active scene;
a later selector-write failure cannot erase a confirmed game operation. A GET
does not advance pages, navigate, join/spectate/leave, Wake, acknowledge notices,
harvest/claim, advance combat or draw/settle a game. Narrow existing owner-defined
lifecycle effects (room expiry/index pruning, projection population, lottery
current-round provisioning) must be documented and tested individually, with
errors propagated; they are not gameplay settlement.

#### Entry and continuation

Entry is evaluated against current facility and actual activity. Ordinary town
entries remain available; selected facilities narrow applicable operations,
while navigation requires no active work. Active-session commands use the owning membership/role/run
and turn state; a generic unfinished-work or town-entry gate must not block
legal continuation, escape/leave or recovery. Pending/Processing scheduled work
remains visible after its deadline. Sleep and pending Wake retain their explicit
Wake/Rescue rules. Contradictory exclusive activity facts produce an authorized
recovery/conflict scene, suppress new entries and expose verified recovery/leave
choices; do not select an arbitrary winner or discard unfinished sessions.

Legacy dispatch evidence is `quest.cgi:189–199,1047–1075`,
`vs_dungeon.cgi:33–48`, `vs_challenge.cgi:24–30`, and `_casino.cgi:10–32`.
These anchors establish distinct entry/continuation menus, not complete combat
formula parity. Each facility migration must reconcile its full actions/call
paths and existing Go gaps before retirement.

The inspected activity-dispatch actions reconcile as follows (paths relative
to the original `party2` distribution). This maps observation/continuation
authority; it makes no combat, wager or settlement formula parity claim.

| Legacy dispatch / actions | Current observation or retained execution / owner |
| --- | --- |
| `party.cgi:14–30`: sleep → saved module → park, then member read/action dispatch | Shared owned Query derives sleep/work/room/run facts before ordinary selection. Explicit Wake/Rescue is the approved transport/recovery contract; GET never performs legacy automatic recovery. |
| `lib/quest.cgi:189–199,1047–1075`: `つくる`, `さんか`, `けんがく`; `パーティー`, `とうぎじょう`, `ギルドバトル`, `ダンジョン`, `チャレンジ` selectors | Existing Party.CreateParty/JoinParty, PvP/GvG.CreateRoom/JoinRoom, Dungeon.StartPartyExpedition and Challenge.StartPartySession remain behind registered REST routes. Observation reads existing membership/run services. Full entry, spectator and category selection migrations belong to #948; selection alone grants no admission. |
| Party preparation / departure | Public Party.GetActiveParty supplies verified membership. Candidate `party_ready`/`party_start`/`party_leave` correspond to SetReady/StartPartyAdventure/LeaveParty and retained `/parties/{id}` ready/start/leave routes; mutation connection remains #948. |
| `lib/vs_player.cgi:32–41`, `vs_guild.cgi:31–38`: `かいし`, pre-round `しらべる` and PvP `ぱーてぃー` | GetCharacterRoom verifies actor membership; leader start/advance and phase-legal leave/team candidates correspond to existing PvP/GvG services and retained character room routes. Detail/party inspection and full mutation migration remain #948. |
| `lib/_casino.cgi:10–32,100–111`: `にげる`, `すくしょ`, `かいし`, `さそう`, leader `きっく`; delegated game actions | Casino.GetCharacterRoomView reuses owned admitted GetRoomView; context composes masked detail and role/phase choices. Leave/start/kick/turn candidates refer to retained service operations; mutation connection remains #949. Invitation/screenshot have no Casino service replacements and remain explicit #949/#140 reconciliation. |
| `lib/casino_indian.cgi:10–16`: `つづける`, `しょうぶ`, `おりる`; `casino_highlow.cgi:10–27`: `つづける`, `ハイ`, conditional `ロウ`, `おりる`; `casino_doppel.cgi:10–15`: generated marks | Existing PlayRoomAction delegates to each game service. Context supplies service-owned game choices and visibility; legacy/current menu differences are recorded in the [scoped Casino mapping](../design/casino.md#scoped-legacy-dispatch-reconciliation). Gateway mutation connection remains #949; existing REST action route stays available. |
| `lib/vs_dungeon.cgi:33–48`: leader initial `すすむ`, `にし`, `きた`, `みなみ`, `ひがし`, `ちず`, treasure `しらべる` | Owned GetActiveExpedition supplies phase/floor; Move and Escape remain existing REST operations and unconnected continuation candidates. Current StartPartyExpedition/Move bundle initialization/tile resolution; separate initial advance/treasure observation is not a verified replacement. Map, treasure and per-turn battle/action reconciliation remain #948. |
| `lib/vs_challenge.cgi:20–46`: treasure `しらべる`, phase-dependent `すすむ` | Owned GetActiveSession supplies phase/round; AdvanceRound remains the retained character challenge advance operation and unconnected candidate. Treasure/battle/party detail reconciliation remains #948. |
| Inherited `lib/_skill.cgi:22–29,78–79,282–317`: spectator `ささやき`/`にげる`/`すくしょ`, generated job/custom skills and `すとっく`, then `add_battle_action` | Shared member/spectator dispatch is inspected only to establish activity authority. No generated skill menu or per-turn battle command is replaced here; skill/stock/combat and battle spectator reconciliation remain #948, screenshot/social presentation #949/#140. Existing feature and battle operations stay registered. |
| `lib/sleep.cgi:18–42`: empty action menu, elapsed automatic vitality recovery | Existing timer facts preserve sleep and elapsed recovery pending; only explicit connected Wake/Rescue commands can recover. GET changes no vitality, timer or feature record. |

#### Route retirement and deliberate exceptions

Gameplay operations migrate through commands; required references migrate through
the observation/selection contract. Each removed route needs its tested
replacement, exact input/output/visibility reconciliation, independent command
schema validation and removal of obsolete discovery links. Resolving #1047 alone
does not satisfy these implementation gates. Retain unmatched/shared operations
until their own bounded migration is complete.

System/bootstrap, account/session/API-token and character lifecycle, admin
maintenance/batch operations, and player-scoped notifications remain outside the
owned-character gameplay Gateway. Admin-only news publication, contest settlement
and ranking refresh remain admin operations even where their existing paths lack
an `/admin` prefix. Public news, rankings/legends, replay sharing, and approved
sanitized public home/store/guild projections may retain anonymous routes; normal
interactive clients still obtain gameplay information through context. Other
public facility GETs are migration candidates, not automatic permanent exceptions.
The complete checked route inventory and implementation ownership live in #1047
and its linked migration issues, rather than a duplicate permanent rollout log.

---

## 3. Command Pillar: `POST /api/v1/characters/{id}/actions`

The command outcome/recovery contract was approved in [#646](https://github.com/witchcraze/party2re/issues/646) on 2026-10-04. Service adapters preserve existing game rules and persistence while the HTTP boundary owns parameter decoding and response composition. Current coverage and route retirement are tracked separately from this contract.

### Request Body

```json
{
  "action": "bank_deposit",
  "params": {
    "amount": 5000
  }
}
```

### Input and execution boundary

The authenticated path character is the actor. Reject actor identity fields such
as `character_id` or `player_id` inside `params`; operation-specific target IDs
such as `target_home_id` remain valid. Use the standard ownership wrapper and
propagate the request context to every service/read call.

`params` is a JSON object when present. It may be omitted for commands with no
required inputs. Missing/null required values, incorrect types, unknown fields,
and actor overrides return `400 INVALID_ACTION_PARAMS` without executing a
command. Preserve strict JSON decoding, the 64 KiB limit and existing transport
errors for authentication, ownership, content type and malformed envelopes.
Unknown actions return `404 ACTION_NOT_FOUND`; known but unconnected commands or
unconfigured services return `501 ACTION_NOT_IMPLEMENTED`. These failures perform
no command and expose no context.

Catalog entries migrated to `executeCharacterAction` validate against their own
`CharacterActionRequest.allOf` condition in OpenAPI, selected by
`if.properties.action.const` with `action` required. Each ActionID must have
exactly one condition whose `then.properties.params` resolves to a strict object
schema without actor identity inputs. Its required fields must match the catalog;
`then` requires the params envelope exactly when those fields are nonempty.
Home sleep permits optional `target_home_id`; Home wake and Depot expansion
permit only omitted params or an empty object.
Bank amounts span signed int64, and command strings permit empty values for
service-owned validation/defaults. The catalog drift check rejects missing,
duplicate, malformed or unresolved contracts even for no-input commands.
Unmigrated entries retain their REST-operation checks. Removing a replaced REST
operation therefore does not remove its migrated command's validation; route
retirement still requires the tracked replacement verification.

Re-read current entry eligibility before execution; reject an ineligible entry
with `409 ACTION_UNAVAILABLE`. Scheduled/timer read errors stop execution with
`500 ACTION_PREFLIGHT_FAILED`, without execution or context. An earlier
`available_actions` list is never execution authorization.
Preserve the shared Sleep/CanWake guard for ordinary commands and explicit
Wake/Rescue exceptions. During an active sleep duration, an explicit Wake request
reaches the service and returns its recognized early-wake rejection without
restoring vitality. GET still offers Wake only when recovery is ready; an awake
character's Wake request remains entry-ineligible. Duration expiry alone leaves
ordinary commands blocked until explicit Wake clears pending recovery.
Domain services still validate exact amounts, currencies,
items and state. Adapters call services directly and reuse HTTP result composition;
they do not invoke REST handlers through internal HTTP requests or copy game rules.

`depot_expand` is explicit purchase intent for one expansion of the owned actor's
storage. It accepts no price, count, target or actor fields and calls the existing
`Expand` service once with the request context. Inspect the selected Depot quote
before requesting it. Affordability and the purchased expansion cap remain service
checks, separate from catalog entry eligibility; a zero wallet or a null quote
does not authorize bypassing them. Known failures use `DEPOT_INSUFFICIENT_FUNDS`,
`DEPOT_MAX_EXPANDED`, `DEPOT_INVALID_CHARACTER_ID` or `CHARACTER_NOT_FOUND` with
their explicit 4xx statuses. Unknown execution errors retain the common sanitized
500 and no replay guarantee. Successful results reuse the existing Depot response
(character, capacity, expansion count, occupied slots and item ID/definition/quantity);
enhancement facts remain in the context projection. Shared guards and known
outcome/GET-only refresh recovery apply unchanged.

`depot_sell` requires one non-null string `item_id`; `depot_sell_batch` requires
`item_ids`, an array of non-null strings. These are owned instance IDs from Depot
observation. Unknown fields and actor/price/quantity overrides fail typed decoding;
empty strings/arrays, duplicate IDs and absent targets remain service validations.
Discovery exposes the strict schemas with empty templates: callers explicitly
choose targets, including for a batch; observing a page never selects or sells it.
The adapters call `SellItem` / `SellItems` once and return the existing
`{depot,gold_earned}` result. Catalog reads, exact base half-price, stack removal,
safe arithmetic and wallet saturation remain the [Depot service contract](../design/depot.md#3-depot-item-sales-うる--まとめてうる).

Known sale errors map to `DEPOT_INVALID_CHARACTER_ID`,
`DEPOT_INVALID_ITEM_INSTANCE_ID`, `DEPOT_EMPTY_ITEM_LIST`, `DEPOT_INVALID_AMOUNT`
or `DEPOT_INVALID_QUANTITY` (400); missing owned targets/storage or character use
`DEPOT_ITEM_NOT_FOUND`, `DEPOT_NOT_FOUND` or `CHARACTER_NOT_FOUND` (404).
Provider/catalog/persistence/cancellation/overflow failures keep the shared
sanitized `EXECUTION_FAILED` 500 without context or a replay guarantee. Known
success/rejection survives observation refresh failure and recovers via GET.
Sale REST operations remain registered until their own verified retirement.

### Command outcome and context refresh

`result` preserves the existing operation's structured result and transport-owned
presentation. A non-null `context` uses exactly the shared `PlayerContextResponse`
defined in section 2, including all four slots and non-null arrays. Refresh is an uncached
read after execution, not part of the mutation transaction. Recheck ownership
before returning context; failed reads, enrichment or ownership checks never
return partial observations.

The HTTP-owned registration seam `withActionCommand` binds a catalog ID to a
typed parameter struct, direct service invocation and explicit 4xx error mapper.
It is configured once during Handler construction, rejects unknown/duplicate
registrations, and treats a nil execution function as an unconfigured dependency.
Parameter decoding completes before state reads and execution. Only the mapped
4xx errors are known rejections; all other execution errors use `EXECUTION_FAILED`.
Both GET and command refresh composition reject ownership changes observed while
reading profile enrichment.

### Bank command contract

`bank_deposit` and `bank_withdraw` accept only `{ "amount": 5000 }`, with a
required non-null signed 64-bit integer. The owned path supplies the actor; each
command calls the corresponding Bank service once with the request context.
Exact positive-amount, funds, balance and limit validation remains service-owned.
Both results preserve the REST fields `character_id`, `money`, `deposit`,
`amount` and `message`; withdrawal also preserves `actual_withdrawn` and
`refunded`, including excess returned to savings when the wallet reaches its cap.

| Service rejection | HTTP | Stable code |
|---|---|---|
| Invalid actor ID | 400 | `BANK_INVALID_CHARACTER_ID` |
| Nonpositive amount | 400 | `BANK_INVALID_AMOUNT` |
| Insufficient wallet funds | 400 | `BANK_INSUFFICIENT_FUNDS` |
| Insufficient savings balance | 400 | `BANK_INSUFFICIENT_BALANCE` |
| Savings limit exceeded | 400 | `BANK_DEPOSIT_LIMIT_EXCEEDED` |
| Character no longer exists | 404 | `CHARACTER_NOT_FOUND` |

Entry exclusions and sleep/pending wake guards prevent service execution under
the shared boundary. Known results/rejections use the refresh contract below;
unknown service/store failures remain `500 EXECUTION_FAILED`. The old
`POST /characters/{id}/bank/deposit` and `/bank/withdraw` routes are retired;
clients use `POST /api/v1/characters/{id}/actions` with the explicit ActionID and
amount. `bank_deposit`/`bank_withdraw` resolver links use that same Gateway URL.
Only Bank talk/inspect REST operations remain pending their own reconciliation.

### SecretShop purchase command contract

`secretshop_purchase` accepts only explicit parameters such as
`{ "item_id": "secret_item_herbal_root", "quantity": 1 }`. Both fields are
required and non-null: a catalog ID string and a signed 64-bit integer. The
existing service validates current item availability, job qualification, exact
funds, quantities 1..99, price overflow and delivery capacity. The Gateway does
not apply the retained REST route's omitted/nonpositive quantity default.

The owned path supplies the actor. The adapter reads the presentation name before
calling `PurchaseItem` once with the request context, then shares the REST
purchase formatter. `result` retains `character_id`, `item`, `quantity`,
`total_price`, `remaining_gold`, `inventory_instance_id`, `transferred_to_depot`
and `npc_message`; inventory and depot acknowledgements retain their respective
wording. It neither duplicates the service transaction nor reads presentation
inputs after a successful mutation.

| Service rejection | HTTP | Stable code |
|---|---|---|
| Job qualification denied | 403 | `SECRETSHOP_ACCESS_DENIED` |
| Character no longer exists | 404 | `SECRETSHOP_CHARACTER_NOT_FOUND` |
| Item absent from catalog | 404 | `SECRETSHOP_ITEM_NOT_FOUND` |
| Active HelperQuest target | 409 | `SECRETSHOP_ITEM_UNAVAILABLE` |
| Quantity outside 1..99 | 400 | `SECRETSHOP_INVALID_QUANTITY` |
| Insufficient wallet funds | 400 | `SECRETSHOP_INSUFFICIENT_FUNDS` |
| Total price overflow | 400 | `SECRETSHOP_PRICE_OVERFLOW` |
| Depot full | 400 | `SECRETSHOP_DEPOT_FULL` |

Required HelperQuest/store failures and an unconfigured depot are unexpected
execution failures, not known rejections. Shared entry/Sleep/CanWake/work guards
and the outcome/refresh contract apply unchanged. A failed name read stops before
purchase; a failed post-command query/profile read preserves the known result or
rejection with GET-only recovery. Nil SecretShop service remains 501.

The ActionID-specific schema and resolver URL point to the Gateway independently
of REST operations. `GET /characters/{id}/secretshop` and
`POST /characters/{id}/secretshop/purchase` are retired and return 404. Enter
SecretShop with `scene_enter` (`destination:secretshop`), then GET character
context for the catalog/product observations described above. The context's
character slot identifies the actor; scene `info.title` supplies the former
`location_name`, and `info.npc_name`/`info.is_eligible` supply the facility facts.
Products provide explicit purchase templates. Execute `secretshop_purchase`
with both `item_id` and `quantity`; clients migrating the retired REST default
must send `quantity:1` explicitly. Missing quantity is invalid, and nonpositive
quantity is rejected rather than defaulted. Talk, inspect and puff-puff REST
operations remain registered pending their own replacements; see
[SecretShop design](../design/shops.md#74-purchase-transport-and-remaining-migration)
for legacy differences and action reconciliation.

### Stage adventure command contract

`adventure_start` accepts only `{ "stage_id": "stage-00" }`. A supplied non-null
string is required; an empty string retains `StartStage`'s existing starter-stage
default. The adapter calls the existing Adventure service once with the owned
actor and request context. `result` reuses the `POST /adventures` fields: `id`,
`character_id`, `stage_id`, `started_at`, `floors_cleared`, `is_cleared`,
`party_size`, `resolved`, and `experience_reward`. The crawl resolves immediately;
this connection adds no scheduled expedition or new game rule.

| Service rejection | HTTP | Stable code |
|---|---|---|
| Stage not found | 422 | `ADVENTURE_STAGE_NOT_FOUND` |
| Level requirement | 403 | `ADVENTURE_LEVEL_REQUIRED` |
| Job level requirement | 403 | `ADVENTURE_JOB_LEVEL_REQUIRED` |
| Unconscious character | 422 | `ADVENTURE_UNCONSCIOUS` |
| Exhausted character | 422 | `ADVENTURE_EXHAUSTED` |
| Once-daily entry already used | 422 | `ADVENTURE_DAILY_LIMIT` |
| Character no longer exists | 404 | `CHARACTER_NOT_FOUND` |

Catalog exclusions (including HP=0, fatigue, sleep, pending wake recovery and
unfinished Pending/Processing work) return `409 ACTION_UNAVAILABLE` before
service execution. A ready deadline does not clear unfinished work. Service
validation remains authoritative for exact stage requirements and state changes
after preflight; unexpected service/store errors use `500 EXECUTION_FAILED`.
Both known outcomes use the refresh/recovery contract below.

### Emergency rescue command contract

`rescue_request` accepts only `{ "reason": "stuck activity" }`, with a required
non-null string. Actor identity comes from the owned path. Empty or whitespace-only
reasons reach existing service validation and return `422 RESCUE_INVALID_REASON`;
invalid actor IDs map to `400 RESCUE_INVALID_CHARACTER_ID`, and a missing service
character maps to `404 CHARACTER_NOT_FOUND`. Unexpected service/store errors
remain `500 EXECUTION_FAILED` with unknown outcome and no context.

The adapter calls `EmergencyRescue` once with the request context and current UTC
time. Rescue remains an explicit entry/sleep recovery exception during sleep,
cooldown and unfinished work. Its structured `RescueRecord` result includes
`id`, `character_id`, `reason`, `penalty_seconds` and `created_at`, including the
service's idle/no-op result. Cleanup and penalties remain service-owned; rescue
does not guarantee removal of sleep. Known success/rejection survives refresh
failure under the shared contract. Existing REST rescue routes remain available.

### Home sleep and wake command contract

`home_sleep` accepts `{ "target_home_id": "other-char" }` with an optional string;
omitting or passing an empty string targets the actor's own home. Home sleep is free
and accepts dead or fatigued actors. Duration scales with online player count.
`result` reuses the REST fields: `sleeping`, `duration_seconds`, `remaining_seconds`,
`home_character_id` and `message`. Sleep sets `timer.CategorySleep` and the pending
recovery flag `timer.CategoryAsleep`, and persists `pending_wake = 1` in MariaDB `characters` (Issue #1118).

`home_wake` accepts no parameters (`{}` or omitted). It is an explicit exception
to the ordinary sleep action guard:
- While sleep duration has not elapsed (`timer.CategorySleep` active), the request
  returns `409 HOME_STILL_SLEEPING` without altering vitality or state.
- Concurrent Wake executions for the same character are serialized via Valkey `timer.CategoryWaking`
  (`TryLock` with a 30s safety lease). Concurrent attempts while Wake is running return `409 HOME_WAKE_IN_PROGRESS`.
- When sleep duration has elapsed and pending recovery is active (`characters.pending_wake = 1` in SQL or
  `timer.CategoryAsleep` in Valkey), Wake restores full HP, MP and resets fatigue to 0, then executes configured
  mandatory recovery hooks (tavern fullness reset, chapel blessing clear, alchemy synthesis completion, costume reset)
  and timer cleanup (`timer.CategoryDungeonOnce` lock and `dungeon_once` daily quota).
  Upon successful completion of all required steps, `characters.pending_wake` is set to 0 in SQL and `timer.CategoryAsleep`
  is released in Valkey.
- **Hook failure and partial outcome**: If any mandatory recovery hook or timer cleanup
  fails, Wake aborts and returns `500 EXECUTION_FAILED`. `characters.pending_wake` remains 1 and `timer.CategoryAsleep`
  remains locked (pending recovery is not finalized, keeping ordinary actions blocked). Any effects
  applied before the failure (such as restored vitality or earlier hook actions) persist
  without cross-store rollback. Clients observe this honestly via `GET /context` and must
  not automatically replay the action. A subsequent explicit Wake can complete remaining
  hooks and finalize recovery once transient errors resolve.
- If Wake is called when the character is already awake (`characters.pending_wake = 0` and `timer.CategoryAsleep` not active),
  it returns `200` with an "already awake" message and the current character state.

| Service rejection | HTTP | Stable code |
|---|---|---|
| Still sleeping (duration not elapsed) | 409 | `HOME_STILL_SLEEPING` |
| Wake already in progress | 409 | `HOME_WAKE_IN_PROGRESS` |
| Already sleeping | 409 | `HOME_ALREADY_SLEEPING` |
| Not sleeping | 409 | `HOME_NOT_SLEEPING` |
| Character not found | 404 | `CHARACTER_NOT_FOUND` |
| Target house not found | 404 | `HOME_HOUSE_NOT_FOUND` |
| Target house expired | 404 | `HOME_HOUSE_EXPIRED` |

### Outcome responses

| Condition | HTTP response | Client behavior |
|---|---|---|
| Command succeeds, refresh succeeds | 200, `success: true`, `result`, `context` | Replace observed context. |
| Command succeeds, refresh fails | 200, `success: true`, original `result`, `context: null`, `context_error` | Re-fetch GET context only; do not resend the command. |
| Known domain/precondition rejection, refresh succeeds | Mapped 4xx, `success: false`, structured `error`, `context` | Show the rejection and recovery candidates. |
| Known domain/precondition rejection, refresh fails | Preserve original 4xx and `error`; `context: null`, `context_error` | Re-fetch GET context only. |
| Unexpected execution error | 500, `success: false`, `error.code: EXECUTION_FAILED`; no context | Treat the mutation outcome as unknown; do not automatically resend. |

`error` and `context_error` use the existing `{ code, message }` error detail
shape. Refresh errors use `CONTEXT_REFRESH_FAILED` with a safe public message;
internal storage details are not exposed. Successful refresh omits
`context_error`. For example, a completed bank deposit with a failed refresh is:

```json
{
  "success": true,
  "result": {
    "character_id": "char-123",
    "money": 7000,
    "deposit": 15000,
    "amount": 5000,
    "message": "5000 Gお預かりいたしました"
  },
  "context": null,
  "context_error": {
    "code": "CONTEXT_REFRESH_FAILED",
    "message": "操作は完了しました。状態を再取得してください。"
  }
}
```

This contract prevents a refresh failure from disguising a known successful
mutation as a retryable command failure. It does **not** provide idempotency or
exactly-once execution. A lost response, timeout or unexpected execution error can
leave the outcome unknown; GET context is an observation, not a command receipt.
Do not automatically replay POST. Safe command replay would require a separate
idempotency-key/result-storage design, outside this scope.

---

## 4. Client Integration Models

### 4.1 AI Agent (LLM Tool Calling)

AI Agents interact with Party2 Re using **only two LLM Tools**:

1. `get_character_context(character_id)`
2. `execute_character_action(character_id, action, params)`

```python
# Minimal Agent Loop
context = get_character_context(char_id)

while True:
    # Select from available_actions even while timers exist: wake/rescue
    # can be eligible. A ready timer does not certify settled work.
    # Poll with a bounded retry delay when no suitable action is chosen.
    # LLM selects action from context.available_actions
    chosen_action, params = llm.decide(context.available_actions)
    
    resp = execute_character_action(char_id, chosen_action, params)
    context = resp.context or get_character_context(char_id)
```

**Benefits**:
- Compact tool definitions: clients need not inject the entire REST specification into the model prompt.
- Zero URL hallucination: the LLM only selects from the authoritative `available_actions` list.

### 4.2 Web UI (Server-Driven UI)

1. Initial load calls `GET /context` to populate the global state store (Pinia / Redux).
2. Navigation and facility choices come from typed scene data and
   `available_actions`; selected values prefill command templates. The approved
   progressive model above replaces the initial globally broad catalog menu.
3. Action buttons submit `{ action, params }` to `POST /actions` once.
4. A non-null response context replaces the store's observation. If refresh
   failed, preserve the command result/error and re-fetch GET context without
   replaying POST. A transport failure does not authorize automatic replay.

### 4.3 Line / Discord Bots

Chatbot frameworks handle state transitions with zero routing boilerplate:
- When a user clicks an interactive button (e.g. "預金する"), the bot sends `action="bank_deposit"` and `params={"amount": ...}` to `POST /actions`.
- The bot replies with `result` and renders quick-reply buttons directly from the returned `context.available_actions`.
- If an error occurs, the bot displays the error message alongside the recovery buttons.

---

## 5. Architectural constraints

1. **Authentication & Ownership**:
   All gateway calls verify character ownership (`char.PlayerID == player.ID`) using `withAuthenticatedCharacter`.
2. **Domain Service Decoupling**:
   Transport handlers and the Action Dispatcher contain no business rules; they decode parameters, invoke application services, and format responses.
3. **Context refresh failure**:
   Preserve the command outcome and report refresh failure separately under
   section 3. Clients re-read context; the dispatcher never retries a mutation
   to repair a failed observation.

---

## 6. Related documents

- [STATUS](../../STATUS.md): major current capabilities, command coverage and gaps.
- [ROADMAP](../../ROADMAP.md): remaining milestones and migration direction.
- [OpenAPI](../api/openapi.json): registered routes and transport schemas.
- [Components](components.md#playercontext): observation/command responsibility boundaries.
- [Action preconditions](../design/action-preconditions.md): game-entry eligibility and behavioral evidence.
