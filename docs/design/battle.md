# Battle System Design

## Purpose

This document defines the language-agnostic game rules, resolution formulas, and contracts for the Battle component (`internal/core/battle`).

Battle is an independent, reusable domain component. It resolves combat between participants and determines outcomes without knowing why the battle was started (e.g., adventure quest, arena, dungeon, boss, or guild battle).

## Domain Model

### Participant
A participant in combat possesses combat attributes:
- `ID`: Unique participant identifier (character ID or monster ID).
- `Name`: Display name for combat turn logs.
- `HP` & `MaxHP`: Current and maximum hit points.
- `MP` & `MaxMP`: Magic points consumed by class/job skills.
- `CMP` & `MaxCMP`: Custom magic points consumed by custom incantation skills.
- `Attack`: Offensive power (≥ 0).
- `Defense`: Defensive resilience (≥ 0).
- `Agility`: Speed attribute determining turn resolution order (≥ 0).
- `JobSkill`: Optional class skill (`ActionSkill`).
- `CustomSkill`: Optional custom blended skill (`ActionCustomSkill`).
- `ActionItems`: Slice of active combat items (`ActionItem`) for `@どうぐ` commands.
- `RevivalTriggers`: Slice of active revival conditions/items (`pharaoh`, `undying`, `touki_shield`, `dokuro_amulet`, `cursed_revive`).
- `Defeated`: Boolean flag indicating if participant is knocked out.
- `TeamID`: Optional faction/team identifier (e.g., team color hex code `#FF3333` in PvP, or Guild ID in GvG). Participants sharing the same `TeamID` are treated as allies; participants with different `TeamID`s are hostile opponents.

### Action Models
- **Job Skill (`ActionSkill`)**: Class skill requiring MP. Includes `Name`, `CostMP`, `Power`, `Element`, `TargetScope` (`TargetScopeSingleEnemy`, `TargetScopeAllEnemies`, `TargetScopeAllAllies`, `TargetScopeSelf`), and optional `Heal` boolean.
- **Custom Skill (`ActionCustomSkill`)**: Blended gem skill requiring CMP. Includes `Name`, `Incantation` (quote announced in logs), `CostCMP`, and a slice of `GemEffect` entries (damage, heal, create field, create anti-field).
- **Combat Item (`ActionItem`)**: Active item command requiring `UsageCategory == UsageCategoryCombatOnly` (`1`). Non-combat items (categories `0`, `2`, `3`, `4`) are strictly prohibited and return `ErrCannotUseInCombat`. Includes `ID`, `Name`, `UsageCategory`, `Kind` (`heal`, `buff`, `status`, `attack`), `Power`, `TargetScope`, and optional `Element`, `BuffStat`, or `Status`.
- **Gem Effect (`GemEffect`)**: Type (`attack`, `heal`, `field`, `anti_field`), `Power`, `Element`, and `TargetScope`.

### Field State (`FieldState`)
- `Element`: Active field element (`fire`, `water`, `wind`, `earth`, `light`, `dark`).
- `RemainingTurn`: Turns remaining for the active elemental field.
- `AntiFieldTurn`: Turns remaining for anti-field suppression (negates elemental affinity multipliers).

### Battle Request & Reward Model
- `Participants`: For 1v1 combat, ordered pair `[first, second]`.
- `Allies` & `Enemies`: For standard 2-side party combat, ally group (up to 4) and enemy group (1..N).
- `Teams`: For multi-faction battles (PvP Colosseum with 2..8 players / up to 9 colors, GvG with multiple guilds), `map[string][]Participant` keyed by team/faction ID.
- `VictoryReward`, `DefeatReward`, `DrawReward`: Rewards containing `Experience`, `Currency`, `ItemDefinitionID`, `ItemQuantity`, and `SmallMedals`.

## Resolution Algorithms & Formulas

### 1. Multi-Turn Party Battle Loop (`ResolvePartyBattle`)
The current Go batch resolver runs the following automatic algorithm. It is not
equivalent to the original player's command-by-command normal-adventure combat
(`_battle.cgi`, `_skill.cgi`); that missing interaction is tracked by
[#1323](https://github.com/witchcraze/party2re/issues/1323) and specified in
[Adventure](adventure.md#original-interaction-contract-and-reconstruction-target).
Reconstruction should reuse combat effects without treating the batch action
selection, agility round order or automatic termination as the interaction rule.

1. **Round Initialization (up to 100 turns)**:
   - Collect all living combatants across all teams.
   - If $\le 1$ distinct team has living participants -> combat terminates early.
   - Sort active combatants in descending order of `Agility`. In case of tied agility, ally order is prioritized, followed by stable input order.

2. **Turn Execution (per participant)**:
   - Skip if participant has been defeated earlier in the round or banished.
   - **Status Ailment Incapacitation Check (`execute_status.go`)**:
     - `dofuu` (動封): 100% turn skip, clears status immediately.
     - `paralyze` (麻痺): 33% cure chance (`rand(3) < 1`, cures and acts), 67% skips turn.
     - `sleep` (眠り): 33% cure chance (`rand(3) < 1`, cures and acts), 67% skips turn.
     - `kinju` (禁呪): 25% chance to trigger 10% MaxHP self-damage, clear status, and skip turn. 75% chance acts normally. Immobilizes evasion.
     - `sabaku` (鎖縛): 25% chance to skip turn (with 50% chance to self-cure on skip). 75% chance acts normally. Blocks stat buffs and tension gain. Immobilizes evasion.
     - If turn skipped by incapacitation, skip further action and post-action poison evaluation.
   - **Confusion Check & Target Redirection**:
     - `confusion` (混乱): 20% cure chance (`rand(5) < 1`). If uncured, logs confusion message; single-target attacks, skills, and items redirect to a random participant selected from all living participants (allies and enemies).
   - **Faction / Target Segregation**:
     - Friendly party (`allyParty`): living participants sharing the actor's `TeamID`.
     - Hostile party (`opponents`): living participants belonging to any opposing `TeamID`. In multi-team battles (e.g., 3 teams), all non-actor teams are mutually hostile opponents.
     - Early exit: if `opponents` is empty, the actor's team has won and the round terminates immediately.
   - **Action Selection Priority**:
     1. If `CustomSkill` is configured and participant has sufficient CMP:
        - Deduct CMP.
        - Log incantation quote: `"<Name> calls: '<Incantation>'!"`.
        - Execute each `GemEffect` sequentially (damage, heal, create field, or create anti-field). Single-target effects redirect if confused.
     2. Else if `JobSkill` is configured and participant has sufficient MP:
        - Deduct MP.
        - Apply job skill effects according to `TargetScope` and `Element`. Single-target effects redirect if confused. Buffs are blocked if target has `sabaku`.
     3. Else if participant has available `ActionItems`:
        - Use the first available `ActionItem` (validated with `UsageCategoryCombatOnly`).
        - Execute item effect (Heal, Buff, Status, or Attack) and consume item from combatant's active items. Single-target effects redirect if confused. Buffs are blocked if target has `sabaku`.
        - Log item action: `"<Actor> は <Item> をつかった！ ..."`.
     4. Else if `Defending`:
        - Execute defend stance.
     5. Else execute **Normal Attack**:
        - Target: Lowest HP living opponent (or random participant if confused).
        - **Critical Strike Check**: Roll `isExceedAg(actor, target)`. Triggers critical strike dealing direct damage ($0.75 \times \text{Attack}$) completely bypassing defense mitigation, and logs `TurnLog.IsCritical = true` with message `"<Actor> の会心の一撃！！ <Target> に <Dmg> のダメージ！"`.
        - **Hit & Evasion Check**: If not a critical strike, attack misses if `rand(100) >= 95` (5% base miss rate) OR if target evades via `isExceedAg(target, actor)`, UNLESS target is immobilized (`kinju`, `sabaku`, `paralyze`, `sleep`, `dofuu`) or attacker holds 必中の剣 (`weapon-63`). On miss, logs `TurnLog.DamageDealt = 0` with message `"<Actor> の <Action>！ ミス！<Target> は攻撃をかわした！"`.
        - **Canonical Damage Calculation**: If hit, calculate damage using the canonical DQ formula:
          $$\text{Base} = \begin{cases} \text{Attack} \times 0.75 & \text{if critical strike} \\ \text{Attack} \times 0.5 - \text{Defense} \times 0.3 & \text{otherwise} \end{cases}$$
          $$\text{Damage} = \max\left(1 \text{ or } 2, \text{int}(\text{Base} \times (0.9 + \text{Float64}() \times 0.3))\right)$$
        - Apply Field elemental multiplier if action or attacker has an elemental affinity.
   - **Post-Action Poison Lifecycle (`execute_status.go`)**:
     - Evaluated immediately after actor acts:
       - `virulent_poison` (劇毒): deals 10% MaxHP damage (capped at 950–1049 if >999) to all other living party members afflicted with `劇毒` on every ally action (`party2/lib/_battle.cgi:792-807`), followed by 10% MaxHP damage to the actor if afflicted (`_battle.cgi:810`). Natural cure cannot occur.
       - `deadly_poison` (猛毒): deals 10% MaxHP damage (capped at 950–1049 if >999) to the actor at the end of their own turn (`_battle.cgi:810`). Natural cure cannot occur.
       - `poison` (毒): deals 10% MaxHP damage (capped at 950–1049 if >999) to the actor at the end of their own turn (`_battle.cgi:840`). If alive, rolls 20% natural cure chance (`rand(5) < 1`); clears status on success.
   - **Defeat & Revival Check (`defeat.go`)**:
     - When any participant's HP drops to $\le 0$:
     - The batch resolver uses `CheckRevival`'s historical ability-based approximation.
       Its fixed undying recovery, quarter-HP item193 and trigger consumption are
       known differences from normal-adventure rules. The explicit engine uses
       [the ordered revival contract below](#resources-revival-and-roster-changes);
       changing other combat modes is outside that reconstruction boundary.
     - If revived: HP set to revival amount, log revival message, participant remains active.
     - If not revived: Mark `Defeated = true`, HP clamped to 0.

3. **Round Conclusion**:
   - Decrement `RemainingTurn` and `AntiFieldTurn` on active `FieldState`.
   - Check termination conditions. The current Go batch resolver returns `OutcomeDraw` after 100 iterations
     (`party_battle.go:maxTurns`); this safeguard is not the legacy explicit-action
     termination rule described below.

### Explicit-action resumption requirements

Normal Adventure needs a feature-independent explicit actor/action/target step,
with complete resumable effect state. This is the #1325 reconstruction contract;
it does not change the existing batch contract for other producers. Adventure
owns storage, admission, room progress, clock eligibility and reward settlement;
core battle owns combat validation/effects and structured resulting facts.
Reuse the participant adapter and effect implementations where their behavior
is verified, without exporting a request that runs another whole agility round.

| Concern | Legacy evidence and required continuation state |
| --- | --- |
| Action eligibility | `_battle.cgi:453–468,640–644,1174–1178`: per-actor previous-action time and room speed, accepted at the exact interval boundary. No all-player barrier or automatic action when due |
| Action/target validation | `_skill.cgi:set_action` (`22–79,85–283,286–309`) derives current/previous-job and custom choices from actor resources and situation; `_battle.cgi:561–575,640–675,761–789` dispatches the choice and charges resources. Skill-specific target helpers remain authoritative; do not use the batch lowest-HP selection as a user target |
| NPC response | `_battle.cgi:1169–1171`, `_npc_action.cgi:89–155`: inspect living enemies in persisted order, skip actors below their interval, process at most one eligible NPC and preserve resulting order/timestamps. Suppression and additional effects belong to the submitted operation, not extra whole-party rounds |
| Field lifetime | `_battle.cgi:636–638,913–914,1534–1557,1661–1694`: preserve temporary/default-field distinction and anti-field counters. Temporary-field decrement depends on field existence, enemies, suppression and anti-field; anti-field decrement has its own predicate. These are operation effects, not seconds elapsed or GET effects |
| Actor status/lifetime | `_battle.cgi:678–759,792–896,1065–1104`: preserve incapacitation, poison, temporary effects and their action-specific counters. State survives requests; `vs_monster.cgi:74–76` resets applicable effects on explicit advance through `reset_status_all` (`_battle.cgi:1390–1428`), without resetting HP/MP or restoring consumed items |
| Additional actions | `_battle.cgi:565,574–575,780–785,1084–1088` permits a specific second submitted action under an active effect, whitelist and MP predicate (strict `>` there). `ActionCounter.pm` and `TurnEndProcessor.pm` coordinate within-operation effects. Finish applicable queued end processing before committing; do not leave opaque callbacks in persisted state |

Current `battleContext` (`execute_action.go`) owns HP/MP/CMP, defense, statuses,
attack/defense/agility buffs, mutable abilities/revival availability, active and
consumed item instances, banishment, teams, ordered participants, field and
already-counted crystal drops. All behaviorally relevant facts must survive a
serialize/reload continuation. The initial `Participant` or `RemainingHP` alone
cannot reconstruct this state. Persist encounter identities and randomized
outcomes already produced; retries read committed results rather than rerolling.
Do not require the in-process RNG object or a Go closure to be serializable.

Explicit operations use `core/battle.LifetimeState` for ordered mutable
participants, resources, item-instance/consumption facts, banishment and crystal
facts, temporary/extended effects, tension/hit, CMP recharge, room skill bans and
per-side use history. Its named `LifetimeFields` separates temporary/default and
anti-field state from the batch elemental `FieldState`. JSON round trips preserve
these facts; timestamps, encounter authorization and receipts remain consumer
responsibilities. Neither an RNG nor deferred executable work belongs in saved
state.

The lifetime operations implement the predicates in the matrix below. They do
not certify the formulas or production availability of individual catalog
entries. Consumers must retain whole-entry admission and resolve the certified
normal-action, equipment, modifier, defeat/revival and reward effects in source
order. `StatusDecision` reports whether the normal-action block is reached and
whether defeat processing is required. A paralysis/sleep cure still skips that
block. Player other-ally virulent poison precedes actor poison; ordinary NPCs
omit that party pass. Suppression does not bypass normal-action poison.

Call `CompleteOperation` only with resolved operation facts: unaccepted operations
are inert; accepted incapacitated actions can still reach field/player tails.
Player field ticks precede the tail and do not require a normal action or living
actor; anti-field ticks even without enemies. The player counter/end-processing
tail additionally requires remaining enemies, no suppression/NPC comment and no
preceding level-up branch. NPC response has its own purification/end-processing
tail and never ticks the player's fields/CMP/bans/incense again. Damage-call
counts must be recorded at the original helper points, including misses and
single-use defense, rather than inferred from HP deltas. Additional-action
parsing captures effect eligibility before the first callback; both entries'
whitelist membership and remaining MP strictly greater than the raw second cost
govern execution. Its lifetime decrement independently uses whitelisted second
input. CMP checks the saved threshold before incrementing; only a reached
threshold needs a new resolved recharge amount/threshold.

Normal-action temporary expiry preserves the player mirror-sharing/item163
exceptions and consumes the original random draw even for empty temporary
state. Reflection/cover redirects persist; only one-hit defense is consumed at
its damage-helper point. Reset restores tension1/hit95 and cleared status/effects,
then uses `ApplyEquipmentReset` to evaluate current equipment for the separate
requester, actor and target. `LifetimeReset` remains its baseline/fact operation;
a cached final encounter-start profile cannot replace equipment callbacks.
HP/MP/CMP, consumed items, bans and history survive the baseline reset; explicit
accessory HP/MP callbacks retain their own guards. Explicit advance performs the
first living item188 holder's ally revival
pass, resets members with current equipment, reapplies living holders' revival
temporary effect, and clears temporary/anti fields while retaining the default.
Equipment formulas and automatic revival stay with their separate native owners.

Operation-local `EndProcessing` captures the first qualifying darkness debit
(floor of half the damage, capped500). It filters that debit against immediate
risk invocation facts once before flushing, skips fallen actors and leaves living
actors at least1HP. No queued risk can retroactively change this filter: the
original gems101/102 have no valid reachable queued execution and remain wholly
unsupported, including their enclosing closures. No callback is serialized.
Untraced callbacks remain explicit production blockers.
The old batch turn ceiling/order remains scoped to that resolver; an interactive
encounter cannot inherit a fictional 30/100-operation timeout from it.

### Normal-adventure action and target matrix

Evidence is the original checkout at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`; paths below are under `party2/lib/`.
This is family-level reconciliation, not certification of every skill callback.
The user selected preservation of original behavior in #1327 on 2026-10-12.
Native action names must follow the migration naming rule; legacy names below
identify evidence only. Adventure owns [discovery and operation admission](adventure.md#normal-run-operation-discovery).

| Action/effect family | Dispatch and target/resource evidence | State/lifetime and native owner/test gate |
| --- | --- | --- |
| Normal attack | `_skill.cgi:106–113,863–884` → `kougeki` → `_damage` → `_check_enemy`. Missing/wrong-side target chooses a random enemy; seal 10 repeats the attack; equipment can add a strike | HP, MP for relevant seals, tension/hit, death/revival/drop facts; current `execute_action.go` is reusable only after explicit-target and effect verification. #1329 tests omitted/wrong-side targets and repeated hits |
| Defend and tension | `_skill.cgi:115–122,942–978`: self, zero listed MP; tension blocked by binding, follows staged values and level/equipment conditions | Defend uses temporary state; tension is consumed by applicable helpers/reset, not a permanent attack buff. #1332 owns state/lifetime; test next action, intervening enemy hit and floor reset |
| Current/previous-job skills | `set_action:35–104,260–283` dynamically calls `skill_<job>` then its callback. SP, extended-skill/equipment, curse and MP gates; previous-job SP is independent | #1188 owns production supply. Damage/heal/stat/status kinds alone are not complete callback coverage; #1333 traces indirect effects and registers unsupported implementations before cutover |
| Equipment extensions, books and weapon-seal commands | `_skill.cgi:124–145,219–224`, `@extend_skill_item:321`, `@booK_skill_item:479`; equipped-item/seal predicates add callbacks to the previous-job admission loop | Callback can change temporary effects, resources, roster or inventory. Book chaining (`6874–6895`) takes two targets, checks internal MP cost, stops on actor death and returns the book to storage. #1333 owns callback coverage; #1197 owns consumption identity |
| Custom gem skill | `set_action:288–309`: unique custom name, at least one gem, sufficient CMP, not cursed. Missing target defaults to actor before gem callbacks; each gem uses its own target helpers | Ordered gem effects stop after actor death; `_battle.cgi:787–789` charges CMP. #1333 traces `_data.cgi:@gems:2087` callbacks, #1332 owns recharge/field state, #1329 tests multi-gem order and restart |
| Combat tool | `set_action:150–151` → `dougu:758–764` → `_data.cgi` item callback; combat usage category 1, missing target defaults to actor, callback-specific targeting | Successful callback clears held item; an error does not consume it. #1197 owns durable instances/consumption, #1333 owns missing effect coverage. Do not assume every tool is generic heal/buff/damage |
| Between-encounter recovery, transformation and calling | `set_action:166–217,226–258`: no enemies, job/SP/curse/equipment gates; survival recovery costs 10 MP, forward transformation 10 MP, reverse 0; calling takes job callbacks | HP, transformed current/previous job, helper membership and randomized stats survive requests. #1333 owns effect/catalog certification; #1329 consumes certified choices. Generic reset is not permission to undo transformation |
| Revival and fallen self action | `_battle.cgi:654`, `_skill.cgi:4910–4930`: discovery still requires its job/extended-skill gates and living designated helper; fallen self gains 50% MaxHP and immobilization. Living actor can revive fallen ally to 1 HP only with MP strictly >40, then spends 40 | Helper HP halves and a three-operation skill ban is set. #1333 owns effect, #1332 ban lifetime; #1329 tests fallen admission, no helper, exact 40 MP, and death-mark follow-up. A blanket living-actor guard is incorrect |
| Additional submission and random invocation | `_battle.cgi:565,574–575,780–785,1084–1088`; `_skill.cgi:_yubiwofuru:9773–9787` dynamically selects a non-extended job callback | Second action is not another player turn/NPC round. Random callback temporarily preserves MP when its internal effect lowered it. #1332 owns second-submission lifetime, #1333 transitive random-effect coverage, #1329 step integration |
| Ordinary enemies and stage bosses | `_npc_action.cgi:89–155,158–495,600–633`: ordered eligible NPC, current/previous-job and custom callbacks, resources and full boss attributes. Random selection among available NPC entries (`313`), not the player's batch priority | Preserve NPC-specific cures/tails, curse payload and special abilities. Curse release (`116–135`) occurs before NPC choice, clears payload, and rebuilds living opponents. #1333 owns ordinary boss catalog coverage; #1329 tests response admission/order |

Player listed MP admission uses the item-249 modifier (integer half cost for
magician skills, double otherwise); execution charges after the callback unless
it reports `$mes` (`set_action:68–79,263–283`, `_battle.cgi:761–778`). Calling
with item 165 overrides the charged cost to zero. Callback-internal costs remain
separate. NPC catalog cost handling (`_npc_action.cgi:177–224,313–322`) is its
own path. #1188/#1333 must reconcile the relevant admission/charge differences;
no-effect callback return alone is not a resource-validation failure.

Target rules are per helper, not one global living-target filter:

| Target path | Original rule and reconstruction contract |
| --- | --- |
| Ally helper `_check_party:9615–9648` | Omitted/unknown legacy name or wrong-side reference chooses a random ally; a fallen selection becomes self except for revival. Temporary effects may redirect target/value. Revival callbacks separately require a fallen target |
| Enemy helper `_check_enemy:9654–9715` | No enemies reports failure; omitted/unknown legacy name or same-side reference chooses a random enemy. Outside confusion, fallen selected target returns without effect. Temporary defense/cover/reflection and special phantom fields may redirect |
| Confusion | Helper-specific selection uses `@members`, not Go's living-opponents/lowest-HP filter; actor inclusion, repeated draws and fallen handling differ between helpers. Preserve those branches; #1333 must certify special-field variants before advertising them |
| Group/self/direct callback | Group helpers iterate preserved party/enemy order; e.g. `_damages:8807–8823` reduces successive power and stops on actor death. Self actions ignore enemy selection. Direct callback assignments and two-target books must be traced individually; helper rules cannot be assumed to apply |
| Native ID boundary | Preserve original wrong-side fallback **within the current encounter**. Omission is an intentional input, never filled from UI navigation. A native explicit ID outside that encounter or from an obsolete encounter is rejected before resource/effect mutation; it is an invalid reference, not a legacy unresolved text name. An encounter member becoming fallen uses the action's fallen-target rule, not a blanket stale-ID rejection. #1329/#1330 test these distinctions |

**NPC order decision:** preserve the original head rotation, including skipped
heads. For enemy order `[A,B,C]`, when A is not interval-ready and B acts, the
result is `[B,C,A]`, not `[A,C,B]`. Only B's action timestamp changes. With no
eligible living NPC, no rotation occurs. Selection is `_npc_action.cgi:90–92`;
rotation is `149–153`. Persist semantic order independently of SQL lock order.

**Additional-submission rule:** the active extended effect permits parsing two
submitted actions. Both must belong to the seven-entry whitelist at
`_battle.cgi:780–781`; after first-action cost, MP must be **strictly greater**
than the second entry's raw listed cost. Execute/charge the second once, without
applying the first-action cost modifier again. The effect counter at
`1084–1088` checks a nonempty whitelisted second submission while enemies remain;
it does not check whether that submission executed. Test exact-cost rejection
and counter decrement separately. Generic arbitrary extra turns are unsupported.

### Effect lifetime and reload coverage gate

| State / trigger | Required original behavior and evidence | Native responsibility / boundary test |
| --- | --- | --- |
| Status and poison | Player incapacitation branches (`_battle.cgi:686–728`) consume an accepted operation but bypass the normal-action block; paralysis/sleep cure branches do not execute the chosen action in that operation. Item 204 bypasses relevant checks. Player confusion cure is 1/5, ordinary NPC is 1/3 (`_npc_action.cgi:295–310`). Poison, party virulent poison and death checks use their actual post-action block (`792–857`), not every elapsed interval | #1332/#1333 reconcile existing `execute_status.go` gaps; #1329 tests accepted incapacitation vs rejected operation, player/NPC distinction and serialize/reload before poison/death |
| Temporary effects | On normal player action, defense/reflection clears, other temporary effects may clear with 1/3 probability; mirror-sharing and item 163 exceptions apply (`748–755`). Target helpers apply cover/reflection; single-use defense clears on hit (`_skill.cgi:8838–8843`) | #1332 owns typed effect facts/reset; reload before/after expiry, reflected hit and defense consumption. Current defending/abilities maps do not establish these lifetimes |
| Extended effects | With enemies and the enclosing tail eligible (`_battle.cgi:938–1172`), counters differ: frenzy/awakening on operation; magic sword/crown/war-god/flame on nonzero damage-helper invocation count (which can include misses/blocked hits); bow on qualifying bow action; additional submission on whitelist input. Incense ticks each holder on an ally tail with one party heal (`1055–1128`). NPC purification has its own decrement (`_npc_action.cgi:466–468`) | #1332 owns counters and operation-local action facts. Round-trip immediately before/after every decrement/zero transition; test zero damage and blocked second execution |
| Suppression / skill history | `_skill.cgi:2518–2531` suppresses on the first qualifying cosmic-field use for a color; `6257–6273` rotates a head and suppresses; `6843–6853` suppresses after successful book retrieval. `_battle.cgi:913–914,938,1169` independently gate field ticks, tail and NPC response | #1332 owns history, #1333 callback predicates; test first/repeated use, failed retrieval, and suppressed operation. Do not equate suppression with disabling all poison/extra processing |
| CMP and skill bans | Seal-field entry changes recharge need/progress (`636–638`); eligible tail checks threshold, recharges, resets and chooses next threshold (`1039–1053`). Skill ban file blocks named submissions (`667–675`); eligible tail decrements finite bans, retaining sentinel 999 (`1137–1149`) | #1332 persists recharge thresholds/progress and bans/history. Reload at threshold−1/threshold, ban expiry and permanent ban; a GET/retry never ticks |
| Temporary/default/anti fields | `get_field_info:1568–1610` prefers temporary over default. Temporary decrements when present before/after operation, enemies remain, anti absent and response not suppressed (`913`). Anti decrements under its own presence/suppression predicate (`914`); creation deletes temporary field (`1661–1668`). Default is not a temporary countdown; expiry can reveal it | #1332 extends current elemental `FieldState`; reload at creation, last tick, anti expiry and default reveal. Special burn/ghost/library hooks use `check_field_exercise:1715–1720`; #1333 certifies catalog-specific effects |
| Reset / death / advance | `reset_status:1390–1408` clears status/tmp/extended/tension/hit, recalculates stats and reapplies equipment effects. `reset_status_all:1411–1428` can revive fallen allies via item 188 before reset. `vs_monster.cgi:74–76` resets and deletes temporary/anti fields on explicit advance | #1332 tests reset reapplication and advance revival; HP/MP, consumed items and earned assets are not generically restored. #1197/#1198 remain asset owners |
| Summons, transformed jobs, ordinary boss payloads | `_add_party:9793–9828` creates ordered allies with rolled stats; `add_boss:600–633` initializes full supplied attributes and gem cost. NPC curse and specter/absorber hooks (`116–135,471–491`) change state/roster/resources | #1333 certifies behavior and registers missing implementations; #1329 round-trips identities, order, helper ownership and consumed payloads without rerolling |
| End-processing and rewards | ActionCounter records damage/heal/status/attribute facts, not merely elapsed turns. TurnEndProcessor queues work flushed at player `1164` and NPC `139`; death-mark follow-up runs at player `1155–1161` | Finish effects before commit; no persisted closures. #1329 saves resulting facts/receipts; #1197/#1198/#1225 own consumption/earnings corrections |

### Dynamic effects and catalog admission

An action is available only when its **whole reachable effect** has an evidenced
native implementation. Admission covers callback-specific targets, resource
changes, resets, death/revival, roster mutations and durable side effects, not
just its first damage/heal instruction. A gem's shop/synthesis availability or
learning definition is not certification of its battle effect, and shared helper
presence does not certify concrete entries. Unknown effect kinds must not become
ordinary attacks, and an empty effect list must not spend CMP as a successful
custom skill. This gate applies equally to player, helper and ordinary enemy
actions, including random or delegated invocation. It does not authorize
permanent omission of original choices: the missing behavior remains a cutover
prerequisite. Preserve original order, repeated draws, duplicate slots and
accepted no-effect outcomes once the entry is supported.

Native admission is enforced across these boundaries:
- **Definitions and Hydration**: `skill.BattleDefinitions` (`internal/core/skill/battle_definitions.go`)
  explicitly maps catalog IDs to validated native action payloads; learning or commerce
  definitions cannot create combat entries by matching kind strings. Participant adaptation
  (`internal/battle`, `internal/core/battle`) omits job entries lacking validated native
  definitions, and hydrates occupied custom slots in original order (including duplicates)
  only when every slot passes. An empty custom action or any missing/unsupported slot omits
  the whole custom choice without partial execution.
- **Atomic Preflight Validation**: `battle.ValidateSkillAction` and `battle.ValidateCustomSkillAction`
  (`internal/core/battle/admission.go`) return `ErrUnsupportedEffect` for unsupported effect kinds
  or incomplete payloads. The battle engine validates the complete action before executing
  any effect and before mutating HP, MP, CMP, items, RNG state, or combat context. Consumers
  must derive discovery and submission from the same server-owned definitions and propagate rejection;
  client-submitted payloads are never trusted. A supported operation with no eligible target
  remains an accepted no-effect action and is charged according to its existing resource contract.
- **Random and Delegated Invocations**: Indirect callbacks use the separate invocation boundary
  described [below](#random-and-delegated-job-closure) rather than batch scalar payloads.
  Their entire reachable closure must be certified; substituting a supported-only draw pool
  is prohibited.

Weapon, weather and stance callbacks compose the shared effect owners
(`internal/core/battle/weapon_weather_stance.go`). Job24's attacks preserve
physical AT×1.2/AT×1.5, direct untyped AT×0.4 and direct physical230; its
single-ally AT buff adds baseAT×1.0 through ordinary stat-up caps. Weapon
enchantment sets seven damage-helper tails, using the existing outer-command
category conversion and lifetime rules (`_skill.cgi:2272–2289`). Job27's living
party breath reflection has no caster seal check. Weapon break resolves its
first physical target, clears that target's temporary/tension, restores baseAT,
then invokes a fresh physical stat-down at20%; reflection/fallback can make the
second recipient differ. Its extended recovery increments the party-heal count
once before any child, then heals each ordered ally at untyped300 and cures
using that loop ally's current status, even when a helper redirects to another
member (`2408–2445`). Full/fallen allies retain ordinary helper behavior.
Job28's self accuracy assignment sets95; deflection and physical immunity are
raw assignments without seal/living guards. Physical status uses80% single or
65% area; physical instant death uses25% with the shared death immunity and
defeat closure (`2455–2486`). Tables retain wallet28.3 for its separate asset
owner; its missing binding rejects an enclosing indirect choice. Operation
facts survive reload and ports are rebuilt; completed callbacks are not replayed.

Time and mixed-magic callbacks compose the shared target, stat, damage, healing
and fully admitted defeat operations (`internal/core/battle/time_mixed_magic.go`).
Job29's repeated magic draws two or three hits at base50, or four through six at
base99; each hit resolves a fresh target and stops after caster death. Its HP
reduction uses the ordinary stat-down immunity and 20%MaxHP floor, rather than
directly halving HP (`_skill.cgi:2492–2515`).

Job30's extended callback branches on the submitted participant before helper
targeting: an unknown participant triggers magic hits using current AT, DF and
AG in that order, rereading each stat and stopping after caster death. An ally
has a 50% chance to receive physical mitigation before party healing at base250;
an enemy receives base350×(tension+5)/6 through ordinary magic damage, including
its subsequent tension multiplier (`_skill.cgi:2584–2603`).

Job31's roulette first checks the magic seal, then draws from all members,
substitutes the caster for a fallen selection, and invokes raw defeat only when
both current HP and current defense are ≤999. It bypasses helper targeting and
ordinary instant-death immunity. Self-destruction attempts current-HP magic
damage before unconditionally invoking caster defeat, even after a failed cast.
The wound attack uses MaxHP−currentHP+5, and party healing captures current HP
before the first heal. The party guard's living-member magic-mitigation tail
still runs after a sealed defense buff (`_skill.cgi:2612–2651,2670`). Ordered tables
retain separately owned field/arithmetic positions; missing bindings reject the
enclosing indirect invocation without narrowing its original pool. Operation
ports are rebuilt after reload; saved results are never replayed as callbacks.

Specification evidence and per-entry provenance use `party2/lib/` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`. Numeric item, gem, and job references are
provenance identifiers, not authorization to reuse distinctive display names.
Finite source ledgers and specification evidence are recorded in
[#1333](https://github.com/witchcraze/party2re/issues/1333), with non-job and ordinary
NPC entry mapping in [#1410](https://github.com/witchcraze/party2re/issues/1410) and
shared player-job identities in [#1409](https://github.com/witchcraze/party2re/issues/1409).
Unimplemented original branches retain their concrete implementation gates; completed
catalog audits do not certify production support. Production consumers remain
[#1329](https://github.com/witchcraze/party2re/issues/1329) and
[#1330](https://github.com/witchcraze/party2re/issues/1330).

| Dispatch | Enduring behavior / effect boundary | Evidence |
| --- | --- | --- |
| Equipped extensions | Ten entries require the designated held item. Effects include helper-stat combination, agility burst/reset, reflection sharing with party healing, interception, summoning, incense, job-dependent projectile/poison, healing with regeneration, extended combat state, and full revival with curse. Their costs are respectively 30,30,12,5,30,5,2,37,10,0 MP. Admission uses the previous-job loop even though these are equipment entries | `_skill.cgi:124–145,260–283,321–477` |
| Books | 27 callbacks: healing, revival, reflection, proportional defense/agility/attack buffs, cures, damage/status, copying another participant and full party healing. Selected heal/buff/damage base values receive ×1.5 in the library field; this is not a universal book multiplier. Full single revival and full party healing seal the caster's magic outside that field | `_skill.cgi:479–736` |
| Weapon seals | Active seals 7/8/9 require previous-job SP ≥90 and respectively 40/27/40 MP, with all-enemy base damage 180 breath /160 magic /180 magic. These categories are not evidence of elemental fire/water/light. Seal 10 invokes ordinary attack twice; passive seals 11/12 follow their attack hooks | `_data.cgi:2209–2230`; `_skill.cgi:106–112,219–224,260–283` |
| Combat tools | Only usage-category 1 supplies the tool command, including outside an encounter. Dispatch defaults a missing argument to self, then the particular callback interprets it as participant, job, destination or floor. Clear the held tool only when the callback returns without an error; accepted no-effect results can consume it. MP restoration, revival, temporary effects, summoning, random skills, reset, copying, capture and travel are distinct effects | `_skill.cgi:150–151,758–764`; `_data.cgi:619–2083` |
| Custom gems | Run gem slots in order and stop when the actor dies. A slot can change HP/MP/CMP, tension/hit, status/temporary/extended state, roster or appearance, invoke another callback, or enqueue end processing. CMP cost belongs to the custom action; do not substitute each gem's catalog cost as an ordinary MP charge | `_skill.cgi:288–309`; `_data.cgi:2087–2198`; `_battle.cgi:787–789` |
| Random skills | The general random helper draws a job from 1 through the last indexed player job, then redraws extended entries until a non-extended callback is selected; an empty job yields no effect. Tool 84 instead draws **any** current-job entry, including extended entries. Both restore MP only when the callback lowered it, preserving MP increases and other side effects. Neither applies ordinary SP/admission/MP charging for the selected callback | `_skill.cgi:9773–9787`; `_data.cgi:1255–1267,2171` |
| Delegated gems | Gem 86 tries the current job's last extended callback before the previous job. Finding an extended callback returns “handled” even when SP/MP prevents execution, so it does not fall back in that case; job 73 has a separate 1-MP stat increase. Gems 87–92 prefer a calling previous job, otherwise current job, and invoke its named command/first non-command non-extended callback without normal SP or listed MP charging | `_data.cgi:2174–2180`; `_skill.cgi:10841–10918` |
| Two-target book use | Allow the same encounter participant twice and require MP ≥floor(book cost×1.5). Death after the first callback skips the second, internal charge and return. After a normally returning second callback, charge and return even if the actor died during it. Ordinary one-target use does not return the book. Parsing and asset context follow the contract below | `_skill.cgi:6868–6897,10538–10557`; native ID authorization remains mandatory |

Active weapon seals are player-only choices: discovery and submission require
an equipped weapon, its matching seal, enemies, no cursed ability, previous-job
SP ≥90 and affordable MP. Current-job SP does not substitute for previous-job
SP. All three original names are absent from the item-249 magician-name list,
so item249 **doubles** their listed MP costs, including the two magic callbacks
(`_battle.cgi:1804–1825`). Ordinary MP is debited after the callback, including
accepted no-effect outcomes and caster death, using the same adjusted cost as
discovery (`_battle.cgi:761–778`). NPC selection appends job callbacks, without
the player seal additions (`_npc_action.cgi:156–218`).

Active weapon seals bind `seal-7-breath`, `seal-8-magic`, and `seal-9-magic` to the
shared ordered area-damage pipeline (`internal/core/skill/weapon_seal.go`,
`internal/core/battle/weapon_seal.go`):
- **Admission and Cost Debit**: `WeaponSealAction` and invocation execution share
  whole-effect admission against enemy targets, weapon requirement, and previous-job SP.
  The returned adjusted MP cost is debited by the calling consumer after the effect.
- **Lifetime Actor Context**: Seal identity, equipped weapon, and previous-job SP are
  preserved on `LifetimeActor` (`internal/battle/lifetime_builder.go`). Operation-local
  damage/reset/defeat context and callback closures must be rebuilt after encounter reload.
- **Magic Absorption**: Native seal callbacks apply magic-absorption MP recovery at
  target resolution before HP damage or defeat; applied recovery facts must not be
  charged or re-applied.
- **Rejection Integrity**: Missing reachable descendants reject before any mutation
  to state, counters, RNG, or costs. Accepted one-hit defense or immune outcomes are
  valid resolutions and do not require unreachable tail context.
- **Consumer Gates**: Production supply, SQL receipts, retry charging, and published
  choices retain their unresolved integration gates in
  [#1329](https://github.com/witchcraze/party2re/issues/1329) and
  [#1330](https://github.com/witchcraze/party2re/issues/1330).

#### Combat books, stored tools and purchases

The requesting player, temporary combat actor, durable held-tool projection and
combat held tool are separate identities/state. NPC dispatch changes the actor
and sides, but does not replace the requesting player's wallet or user ID
(`party.cgi:8–31`; `_npc_action.cgi:89–145`). Jobs7/8 read that wallet,
job7/81 credit it, job28 debits it and job78 purchases debit it even when the
actor is an NPC. A commanded or random child does not acquire its actor's own
wallet. Preserve this ownership explicitly; reject the whole invoking entry
before mutation when the native context cannot express it. Never infer the
payer from an actor's name. Detailed source reconciliation belongs to
[#1341](https://github.com/witchcraze/party2re/issues/1341).

| Operation | Source-backed asset, effect and lifetime contract |
| --- | --- |
| Book exchange, job86 positions0/3 | Require a held book; position0 also permits an empty durable tool in the library field. Search requesting-player depot order for the first matching stored book. Replace that one row with the durable held tool if nonzero, then set both held projections to the selected book. Other duplicate rows retain order. No match reports selection failure, resets action interval to0 and changes no held/depot state. Position3 alone, on success, suppresses NPC response, applies15%MaxHP risk and writes the skill ban, in that order (`6802–6816,6843–6855,10489–10536`) |
| Book return, job86 position2 | Held combat item must be a book. Attempt15%MaxMP magic restoration first; then send the **durable** held tool to the **actor-named** depot and clear both held projections. Return is not conditional on MP recovery succeeding (`6829–6840`) |
| Double invocation, job86 position5 | First matching book callback, falling back to position0 if no item ID matches. Gate on floor(listed book MP×1.5) once before the first invocation; no ordinary cost per child. Check actor death only between children. After a normally returning second child, subtract the internal cost without a local zero clamp, send durable held item to actor-named depot, then clear both projections. A child may alter MP/items; return uses their current values, not a captured original item (`6868–6899`) |
| Stored-tool callback, job50 extended | NPC invokes magic heal500 and returns. Player uses outer submitted command/item/remaining text, trims target whitespace and leading encoded separator, and selects first eligible matching tool in requesting-player depot. Usage-category1 invokes its actual callback with that target; retain the row on selection error, otherwise remove it. Other categories swap the row with the durable held tool, even item0, and equip selected tool in both projections. Record its definition as used **before** child invocation; all copies of that definition are excluded for the actor's color for the room-directory lifetime. Child error retains that history in source; no-match sets interval0. Do not replace this with ordinary held-tool consumption (`3612–3626,10433–10487,10949–10973`) |
| Enhanced held-tool use, job78 position0 | Separate table of **22**, not23, IDs:1,2,3,4,5,6,9,11,12,13,14,15,18,19,20,41,42,78,80,81,84,87. Category2 items18–20 are included. A callback returning1 clears combat held tool and, only for non-NPC color, durable held tool; unsupported ID returns0 without consumption. Item6's early invalid/living-target return also skips consumption (`5988–6013,10196–10282`) |
| Empty-hand purchase, job78 position0 | Pool positions are1×5,2,9,11,14×2,15×2,41,80×2,81,84×2. Remove exact helper-quest tool IDs before the weighted draw; insufficient money still follows a draw. Nonzero selected ID and wallet≥floor(base price×2.5) equip combat held tool and debit requester wallet; update durable held tool only for non-NPC. Empty pool produces no purchase. No capacity check or depot delivery is involved (`10284–10305`; `system.cgi:1424–1442`) |
| Scavenging, job78 position5 | Damage and agility increase precede empty-hand1/5 draw. Its distinct pool is1×5,2×2,3,4,11×2,12,15×2,41,42,80×2,81×2,84×2. Draw first, then helper-list **substring regex** exclusion converts a matching result to0 without redraw. Update combat held tool; durable projection only for non-NPC. This exclusion differs from purchase (`6176–6196`) |

Enhanced callbacks use neutral categories, not ordinary tool formulas: single
HP120/300/750; party HP400; party HP999 plus MP100; full single revival with
temporary revival state; each fallen ally independently1/2 half-MaxHP revival;
single MP120/300; MP999 then reset; strong defense; direct damage equal to
target AT; AT/DF/AG+100%; all-enemy direct400; area death40; add enemy once plus
independent1/2 second; all-party defense including fallen; enemy temporary-state
clear; base25%MP reduction then actor recovery of the returned amount using
attack category; participant reset. These correspond to the
22 IDs in order above, and retain each helper's guards, draw order and tails.
Item84 here is MP theft, **not** ordinary item84's random job invocation.

Argument binding is part of these effects. Enhanced dispatch passes only the
numeric item ID. Reading that ID into a lexical does not consume the argument;
the first `shift` therefore reads the ID, not the submitted participant. Item84
checks that first value then its second `shift` is absent, so MP reduction uses
helper fallback rather than the checked target. Item87 uses the numeric value
if it names a participant, otherwise draws from all ordered members, including
fallen. Do not silently pass the submitted target or consume an extra argument
to repair these paths. Stored ordinary tools instead receive job50's submitted
target, including job/destination/floor selectors and random/travel descendants.

Double-target source splits encoded separators, keeps only the first two
pieces and scans members with an OR predicate; it is not a general validated
pair parser. Equal names succeed on one matching member. Distinct names normally
require both to occur; duplicate member rows can satisfy the scan twice while
one name is absent. Failure sets a selection message and implicitly returns
that scalar, rather than an explicit empty pair. Native submission still
requires encounter-scoped IDs; malformed/foreign/duplicate-roster contexts
that cannot represent the evidenced behavior remain rejected before any child,
cost or asset change. Normal typed pairs preserve same-target-twice behavior;
this does not approve exposing arbitrary names (`10538–10557`).

`send_item` appends without checking capacity when the actor-named depot file
exists; absent file silently skips delivery, but the caller still clears held
items (`system.cgi:818–837`). Exchange instead rewrites the requesting player's
existing depot and needs no extra slot for a nonempty swap. The native depot's
capacity-checked delivery policies do not implement capacity-unchecked return.
[#1139](https://github.com/witchcraze/party2re/issues/1139) owns full/over-capacity
decisions; no strict rejection, loss or new pending queue is approved here.
Missing/mismatched recipient or held projections remain unsupported contexts,
not permission to destroy a player's book or transfer an unrelated item.

Native command receipt/state, wallet, held/depot instances and room history
must commit together through injected transaction ports. Preserve instance
identity via #1197 and existing storage owners; never debit or consume twice on
retry, reroll a committed purchase or discard child travel/roster effects.
Source file writes are not atomic across history/depot/user files; this is
behavioral evidence, not an instruction to reproduce partial writes on faults.
Native owners are #1370 book transactions, #1371 stored-tool execution/history
and #1372 enhanced tools/purchase; #1359 retains other wallet/scavenging effects.
The shared registry remains a whole-entry gate until these transactions,
argument contexts and every reachable child are implemented and tested.

#### Random and delegated job closure

The draw is over **job tables and ordered callback positions**, not distinct
names, learned skills or native definitions. At the pinned revision the player
table ends at job87 (`_data.cgi:12–301`). The 86 concrete tables in jobs1..87
contain 599 entries: 548 ordinary and 51 extended. Job73 supplies a generated
view of another table, rather than a further independent callback catalog.
Commented entries at `_skill.cgi:3241,3689` are excluded. The per-callback source
ledger belongs to [#1344](https://github.com/witchcraze/party2re/issues/1344);
the source-to-native implementation and registration ownership is reconciled in
[#1409](https://github.com/witchcraze/party2re/issues/1409). These counts establish
the evidence boundary, not native certification.

General random invocation first draws each job1..87 with equal probability,
then draws positions in that job's table. Extended positions are redrawn within
the same table; repeated names remain separate positions. This gives callbacks
in differently sized tables different probabilities. No concrete table or
job73 proxy is empty or all-extended at this revision. The helper nevertheless
has an accepted empty-table no-effect branch; an all-extended table would never
finish selection. A future such table needs an explicit decision before
admission, not a silent retry limit or supported-only pool. Tool84 has no empty
table guard and no extended-entry exclusion (`_data.cgi:1260–1264`).

| Indirect edge | Required context, target and resource behavior | Evidence / owner |
| --- | --- | --- |
| Job73 table proxy | Use `(AT+DF+AG)%73`, replacing zero with job8. Retain the actual actor's jobs, stats, held items and role when constructing/executing the selected table; selecting a table does not change the actor's job | `_skill.cgi:5583–5588`; #1357 |
| Job82 generated entry | Select table `(AT+DF+AG+member count)%87`, replacing zero,73,82 with8; select its position by the same sum modulo that table's size. Extended entries are included. The generated callback checks its child's listed MP internally, even under random invocation; it does not itself subtract that cost | `_skill.cgi:6537–6550`; #1357 |
| Recursive random | Job55 and56 callbacks and a branch of calling-job80's special command reach the general random helper. Job79's NPC callback draws again if alive after the first draw. Coverage is a transitive graph, including cycles, not a fixed one-level list | `_skill.cgi:3860,3886,6257–6262,6381–6419`; #1357, #1339, #1360 |
| Extended gem86 | Inspect only the current table's last position. A last extended entry returns handled even when its SP/MP check prevents execution; only unhandled falls back to previous job. On execution subtract listed MP **after** the callback, clamped at zero. Job73 always reports handled: at MP>0 increase AT by party count when AT<999, then subtract1 MP; MP0 blocks fallback too. No ordinary equipment gate is applied | `_skill.cgi:10841–10877`; #1357 |
| Calling gems87–92 | Prefer a calling previous job, otherwise current job, even if the preferred callback has no usable helper. Named commands choose the first matching table entry. Gem92 chooses the first ordinary entry excluding the five command names. The selected child has no ordinary SP or listed-MP charge; its internal gates and side effects still apply | `_skill.cgi:738–753,10879–10918`; `_data.cgi:2175–2180`; #1339, #1357 |
| Arithmetic and discovered-command delegation | Job72 selects a divisor-dependent effect from the discovered action map and submitted text; job31 has fixed-divisor and HP digit-reversal callbacks. Preserve both sides, effect-specific fallen predicates and the [arithmetic contract](#arithmetic-selectors-and-digit-reversal). A direct random child without a divisor fails to cast | `_skill.cgi:2653–2713,5450–5581`; arithmetic effect owner, invoking closure #1357 |
| Book/tool descendants | Job50's extended callback can invoke a depot tool; job78 can use an enhanced tool or buy one; job86 can exchange/return a book or invoke it twice. These reach the finite non-job tables and their descendants, including random invocation and travel. An NPC branch is not interchangeable with a player asset operation | `_skill.cgi:3612–3626,5988–6013,6802–6899,10196–10305,10433–10557`; #1341, #1197 |
| Transformed tables | Copying job pairs and job82's form change affect subsequent table construction. Temporary job300 has four ordinary entries although it is outside the initial general-random job draw. Tool84 can select its current-job table; do not extend the general pool to300 or infer job88 reachability merely from a declared routine | `_skill.cgi:2944–2964,6593–6610,7465–7532`; #1340 |

Job82's generated table and position are correlated because both use the same
sum. For a table of size `N` selected by the original residue `z` modulo87,
position `p` is reachable only when `(p-z) mod gcd(87,N) = 0`. Job8 also accepts
the remapped residues0,73,82. Table87 is never generated by this selector,
although general random invocation can draw it. This restriction does not remove
any position from its original table or from other invoking paths. Capture the
selected child's identity and listed MP when constructing the generated entry;
do not independently redraw its table or position. Evidence:
`_skill.cgi:6537–6550` at the pinned revision.

Ordinary job73 discovery adds a separate item158 command for the current or
previous job73 with nonzero corresponding SP. Its listed MP is1; when AT<999,
the body adds party size without clamping the result to999. This is distinct
from the proxy table and from gem86's always-handled job73 branch, which has no
ordinary item/SP gate. Evidence: `_skill.cgi:35–56,86–104,10841–10877`.

Bare Perl callback invocation forwards the caller's remaining arguments.
Consequently random invocation does not universally discard its target:
tool84/85 receive the tool argument (default self), gem83 receives the resolved
custom target, and nested callbacks may already have consumed an argument.
Gem86 and calling helpers pass an explicit target. Current/previous discovery
instead assigns a name-keyed action map in order; a later admitted same-name
entry overwrites that map value. Random positional selection and delegated
first-match selection must not be replaced with this last-write lookup.
Evidence: `_skill.cgi:35–104,260–309,758–764,9773–9787,10887–10918`;
`_data.cgi:1260–1264,2171,2174–2180`.

Calling jobs are exactly59–69,71,80,87. Summon and dismiss address their named
helper; attack commands temporarily change the acting participant, while many
special commands use helper stats with the summoner still acting. A missing
named helper can fall back to the first matching **appearance** in party order;
`_search_job` excludes self and mixed forms and does not search job IDs. A
present but fallen named helper does not fall back. Summon branches, merge,
duplication, growth and cross-actor restoration use the
[shared roster operations](#summoned-roster-and-helper-commands). Legacy appearance
paths identify branch evidence only; native assets must be newly authored.
Evidence: `_skill.cgi:4029–5447,6307–6467,6904–7038,9793–9828,9935–9942,10627–10650`.

The random wrapper restores only the invoking actor's MP decrease after a
normally returning callback. It preserves increases, other actors' MP changes,
actor death, HP, statuses, field state, ordered roster, held items, money and
room transitions. Internal MP checks, including job82's generated entry and
job86's two-target book use, are not waived by absence of ordinary child costs.
Job callbacks that read or change wallet/held tools are owned by #1359;
job78/79 encounter replacement, SP reduction and enemy ordering by #1360.
These operations require the same receipt/transaction boundary as their outer
command. Refresh/retry must not reroll or replay a nested mutation.

Explicit invocation discovery uses `skill.BattleDefinitions.InvocationAction`;
execution uses the same `LifetimeState.ValidateInvocations` gate before any
slot runs. The registry identifies outer entries and their declared transitive
edges by native identity. Tables retain ordered positions, duplicate names and
all source-context variants; table construction chooses against the actual
actor at each invocation and captures generated children before execution.
Admission traverses recursive random, generated, book/tool and helper edges,
including cycles. Later slots and nested current-job calls must also cover
contexts reachable after earlier effects, including copied jobs and job300.
Pure callback admission must certify those contexts, not only the initial
actor. A missing entry/variant or unresolved helper rejects the whole operation
before costs or draws. An unusable gem86 last entry and job73's MP0 special case
remain handled no-effect without demanding skipped callback context.

A callback may supply a pure `Project` transition when its complete resulting
state and caller context are deterministic. Admission applies it to a private
copy, revalidates each later slot against those facts, and stops after projected
actor death. No execution hook or caller RNG runs during projection. An unknown
transition retains conservative `AfterEffects` validation; it must not be
approximated by a sampled outcome. Delegated calling selection uses the projected
current/previous jobs and preserves its explicit target boundary.

Naked nested calls share their remaining arguments; each custom slot starts
with its own copy of the outer resolved target. Explicit gem86/calling helpers
pass only that target. Later slots stop after actor death, while accepted roster
and resource changes survive. The invoking consumer resolves tool/custom
arguments and owns outer admission/costs and durable transactions. It must
preflight before those mutations, inject frozen command randomness and commit
the resulting facts with the receipt; core stores neither callbacks nor RNG
objects and does not replay committed commands. The default registry certifies
no production invocation entries; synthetic complete catalogs test this
mechanism without enabling the original catalog or the SQL consumer.

The [shared modifier contract](#shared-modifiers-and-callback-outcomes) separates
callback categories, target resolution and source-context gates. Job81's song
category always passes `is_bad_state`; the apparent rejection at6525 is
unreachable. #1344's helper edges do not certify caller formulas or context.

Required executor/consumer vectors (#1357, #1329, #1330) include an unlearned
high-cost child, extended exclusion versus tool inclusion, unusable-but-handled
current gem86 entry, old calling-job preference, MP decrease restoration versus
increase retention, actor death before later gems, helper/held-item/room changes,
and a hidden unsupported descendant after a supported custom slot. The last
case rejects the **whole** outer action without spending MP/CMP/items or
changing any state. Compare uninterrupted and serialized/reloaded results,
including draws, ordering and asset receipts. Neither this source audit nor
an executor tested against a synthetic catalog certifies production entries.

#### Protective magic callbacks

Evidence: `_skill.cgi:2722–2862` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`.
`RegisterProtectiveMagic` contributes complete ordered job32/33/34 tables and
binds19 intrinsic bodies. Revival32.4/33.6 retain their `RegisterRevivals`
bindings; a missing binding rejects the entire indirect pool.

Job32's single direct magic strike chooses power100 with25% probability,
otherwise30, before its target helper. Its party physical mitigation and magic
reflection, and job33's party magic/breath mitigation, check the caster's magic
seal and replace only living party members' temporaries. Job34's magic immunity
uses the same seal check on self. Fixed damage, party healing, self meditation300
and job33's100% base-AT buff compose the existing effect owners. Healing retains
party order, fallen-target fallback, tension, modifiers and caps.

Job34's cover resolves a physical ally helper before assigning the caster's
covering role and the returned nonself target's covered role. Its group strike
first resolves a symbolic magic enemy helper. Only a resolved result debits15MP
from each nonself party member with at least15MP, including fallen members, and
adds85 per member to base100. Helper resource effects precede those debits; the
numeric direct magic hit resolves its target again. A zero symbolic payload
does not cancel cooperation, while an unresolved helper consumes no ally MP.

Job33's extension captures the caster's status for an untyped cure helper, then
grants five additional-submission uses regardless of the cure's returned effect.
Admission validates complete descendants before costs or committed RNG and
projects only exact deterministic transitions for subsequent callbacks. Unknown
prior transitions retain the whole-action gate. Consumers save resulting facts,
operation counters and frozen input, then rebuild ports and registrations without
replaying committed callbacks; production Adventure adoption remains separate.

#### Counter and reset callbacks

Evidence: `_skill.cgi:2871–3013` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`.
`RegisterCounterReset` contributes complete ordered job35/36/37 tables and binds
18 intrinsic bodies. The all-member equipment reset35.1 retains its
`RegisterStatuses` binding and preserves each member's tension; copy36.4 retains
`RegisterTransformations`. Missing bindings reject the complete indirect pool.

Job35 grants unguarded self deflection, magic instant-death20%, direct area
breath180, untyped self healing300, or magic immunity after a caster magic-seal
check. Its final callback sets the additional-effect flag, deals direct area
magic150, then independently attempts area magic paralysis20%. Damage stops on
caster death; status iteration continues. Each helper retains its own seal
check, target resolution, counters, incidental resources and immunity rules.

Job36's four self counters independently replace the temporary with dance,
breath, magic or physical reflection, without a caster seal or living guard.
Job37 uses area magic seal70% and dance seal70% in their respective categories,
magic-seal-guarded self absorption, and magic mitigation for every living ally
without an outer seal check. Its reflection resolves a single ally through the
magic helper, including fallen-target fallback and temporary redirection.
Physical seal80% and self healing300 are untyped. Final break resolves a physical
enemy target, clears that returned target's temporary, sets tension1 and restores
base AT/DF; it adds no proportional stat-down tail and keeps the baselines intact.

Intrinsic callbacks leave listed SP/MP charges to their invocation owner;
random/tool invocation retains MP restoration and ordinary admission exceptions.
Admission checks the complete descendant sequence before mutation or committed
RNG. Consumers persist lifetime facts, operation counters and frozen input, then
rebuild operation ports after reload without replaying committed callbacks.
Production Adventure command adoption remains with its consumer owners.

#### Hybrid and breath callbacks

Evidence: `_skill.cgi:3074–3125,3222–3238` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`.
`RegisterHybridBreath` contributes ordered job39/40/41/44 tables and binds22
intrinsic bodies through the existing invocation boundary. Job39's half revival
retains its separate `RegisterRevivals` binding; an unbound position still rejects
the enclosing random/tool action. Listed SP/MP remain ordinary admission metadata.

Fixed direct area magic25/60 and breath15/55/115/180/195 retain their distinct
powers, categories and ordered area modifiers. Job39's extension uses AG×1.8
physical direct damage. Job40's MP wave captures current MP×2 before the area
helper and writes MP1 on normal return, including a sealed or empty area. Indirect
random/tool invocation restores only decreases, so MP0 becomes1.

Job41's extension resolves an untyped target first, then uses current AT×1.8
when that returned target has no temporary, otherwise AT×2. Its physical direct
helper resolves the target again; cover/reflection and absent-target fallback
retain their ordinary semantics. Job44 uses AT×1.4 physical damage, a50% base-DF
untyped self buff, DF×2 physical damage with7% of positive returned damage as
immediate recoil, direct magic170, magic healing150, and AG×1.5 direct **magic**.
Job39's healing30, party base-DF buff25%, enemy current-DF reduction25% and
confusion50%, and job41's paralysis35% reuse their existing effect owners.

Discovery validates complete reachable descendants without applying hooks or
consuming arguments/RNG. Unknown preceding callback transitions retain the
conservative whole-action gate. Consumers save resulting lifetime facts and
operation counters with frozen input, then rebuild ports/registrations on reload
without replaying committed effects; Adventure command wiring remains separate.

#### High-power and vulnerability callbacks

Evidence: `_skill.cgi:3418–3568,3693–3718,11001–11018` and
`_battle.cgi:1390–1409` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`.
`RegisterHighPower` contributes complete ordered job47/48/49/52 tables through
the existing invocation boundary. Listed costs belong to ordinary admission;
indirect invocation retains its own cost and argument rules.

Job47 retains returned-target guards for recoil and the 10% current-AT loss.
Its wound strike uses missing target HP plus10, capped at400, then resolves the
physical direct hit again. HP above MaxHP can produce zero or negative power;
nonzero damage retains the shared positive roll floor. Four strikes choose
targets afresh and stop on caster death. The extension adds floor(current HP×25%)
to AT, caps the resulting AT at999 (over-level: max(999,min(1998,base AT×3))),
then grants regeneration and sets hit95.

Job48's magic waves deal direct damage before their separate poison/sleep
helpers; status iteration continues after caster death. The single flare reduces
hit by8% only on a returned, living target. Life collapse sets HP1 on a strict
one-in-three draw when MaxHP and base DF are both ≤999. Its extension resets
every ordered member through equipment reapplication, restores saved tension,
then revives any still-fallen member to HP1; finally the caster gains magic
absorption. Accessory callbacks run before revival and can restore statuses or
temporary effects during reset. Package target-cursor changes remain operation
facts for subsequent owners.

Job49 cures the resolved ally's current status, raises defense by60% of base DF
with the existing stat-up modifiers/cap, or performs its physical strike. The
defense callback always grants physical mitigation after its buff helper.
Job52's armor break clears the returned target's temporary and tension, restores
base DF, then resolves the 10% defense reduction independently. Its focus reuses
the double-tension/vulnerability operation; the direct area200 callback writes
vulnerability after the hit even when the caster died.

Admission preserves consumed defenses, category cures and item195 activation
before later helpers. Missing reset facts reject before the first member.
Deterministic weapon extension and equipment resets without accessory/variable
weapon callbacks can project exact state into later slots, including newly
revived targets. Other preceding outcomes retain the existing `AfterEffects`
gate; admission never samples reset RNG or executes external hooks. Save the
resulting facts and frozen next input, rebuild ports after reload, and never
replay a committed callback. Adventure production integration remains with its
consumer owners above.

#### Possession, control and climax callbacks

Evidence: `_skill.cgi:3723–3754,3943–4024,5293–5345` and
`_battle.cgi:587–594` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`.
`RegisterControlClimax` binds the seventeen intrinsic bodies and contributes
complete ordered job53/58/70 tables. The field53.6 and revival70.3 entries retain
their own registrations; a missing body or descendant rejects an indirect
closure rather than removing an entry from its original pool. Job58.3 requires
the MP owner's pure validation and execution through `ControlMPDrain`; no
resource operation is certified by table presence alone.

Possession temporarily changes the effect actor for one ordinary attack,
retaining the source side arrays, requester facts and outer command. Restore
the actor even when a descendant fails. Trance clears status before its ordered
AT/AG buffs, then writes magic absorption and confusion. Arrow consumes status
before current temporary, falling back to the pre-action temporary snapshot;
transfer the selected seal/temporary before resolving its magic hit. Consumers
must save that snapshot with frozen operation input and reconstruct hooks after
reload. Unguarded writes through an absent target remain unsupported source
contexts; guarded no-effect results remain accepted.

Climax captures the maximum AT/DF/AG, strips the selected target's temporary
around its magic hit and restores it only while that target lives. Positive
damage and a living caster permit the follow-up: compare the captured maximum
against current AT first, then DF, otherwise use the AG branch. The AT branch
deals 30% of returned damage to enemies; DF heals the party using 30% caster
MaxHP; AG raises self AG by 30% and can immobilize the original target. Helper
redirection, counters and incidental resource effects retain their existing
owners. Admission includes earlier status changes, consumed defenses and
item195 activation before later helpers. Unprojected preceding callbacks retain
the shared `AfterEffects` gate. A follow-up is omitted from admission only when
every positive-hit path proves caster death; possible survival, automatic revival,
retargeting or a pre-hit reset retains its descendant requirements.
Full service/receipt integration remains with
the normal-adventure consumer; saved committed results are never replayed.

#### Arithmetic selectors and digit reversal

Evidence: `_skill.cgi:2653–2713,5450–5581` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`. These callbacks check the acting
participant's magic seal, then operate directly on the ordered member list,
including both sides. They bypass ordinary target redirection, immunity,
damage/healing modifiers, tension and helper counters. Ordinary SP/item
eligibility and the post-callback listed-MP charge belong to the invoking owner;
random invocation retains its separate cost/restoration rules.

| Callback | Selection and direct result |
| --- | --- |
| Job31 gravity / death | Living members with nonzero MP divisible by4 /5 and current HP≤999. Gravity sets `max(1, floor(HP/4))`; death sets HP0 and invokes the shared defeat owner. Base defense is irrelevant. |
| Job31 digit reversal | Use an existing supplied participant on either side directly, including fallen participants. Only an absent participant invokes the untyped ally fallback and its normal confusion/temporary context. Reverse two or three decimal HP digits, including10/100→1, without a MaxHP clamp; one/four digits are unchanged. Negative two/three-character HP strings remain unsupported rather than inventing numeric treatment of the sign. |
| Job72 divisor wrappers | Magic-state check, then parse the second `＠×…` command from submitted text with its ASCII/full-width space terminator. Resolve against the complete discovered name map; its last matching entry wins. Set divisor3/4/5 and invoke that captured callback, including its own magic-state check. No matching command is an accepted failed cast. |
| Job72 five effect entries | Nonzero MP divisible by the operation's divisor selects all ordered members. Damage/heal/reflection/death require living HP; revival requires HP≤0. Damage independently draws `floor(260×(0.9+0.3u))` per member; base defense>999 consumes an additional draw and replaces damage with1..2. Healing draws `floor(500×(0.9+0.3u))` and caps at MaxHP. Reflection assigns the magic-counter temporary; revival directly sets MaxHP without reset/cure; death additionally requires current HP≤999. Lethal damage and death invoke defeat immediately, before the next member's eligibility check. |

The divisor belongs to the current operation and is forwarded through nested
callbacks; it is not a turn-spanning saved effect. A direct random effect entry
without a divisor still checks the magic state, then returns a failed cast.
Positional random and first-match delegation pass their captured callback ID;
they never repeat the wrapper's last-match name lookup. Member order is captured
at callback entry, while HP/MP eligibility is reread after each defeat hook.
Arithmetic HP writes do not enqueue darkness recoil or increment damage/risk
counters; already-recorded end processing retains its normal owner and ordering.

`skill.BattleDefinitions.RegisterArithmetic` binds these eleven bodies to the
shared `ValidateInvocations`/`ApplyInvocations` boundary, reusing
`ArithmeticAction` and the intrinsic arithmetic executor. Its ordered job31/72
table contributions preserve every source position, including job31's five
unbound bodies; table presence does not certify those missing callbacks or a
complete random pool. Batch scalar definitions remain separate, and the
registry admits nothing by default. A discovered unsupported or unregistered
last-match child rejects the whole selector, even when an earlier duplicate is
supported. Reachable lethal effects require an already admitted shared
defeat/drop/reset closure; this owner does not certify that catalog. Accepted
nonlethal/no-effect paths need no defeat hook.

`CallbackContext.ArithmeticDivisor` carries the divisor through the current
nested invocation frame; start a fresh frame for each operation. Rebuild
registrations with frozen submitted text and ordered discovery facts after
reload, without persisting callbacks or replaying committed operations. Exact
gravity, supplied-target reversal, reflection and revival projections carry
changed eligibility into later admission without hooks or RNG. Random amounts,
magic-seal cures, fallback target selection and reachable defeat hooks remain
unknown transitions. After unknown earlier effects, admission requires defeat
support for potentially lethal descendants and rejects reversal whose signed HP
or fallback context cannot be certified. The operation consumer still owns
ordinary SP/MP/item admission, receipts and production integration.

#### Shared modifiers and callback outcomes

Behavioral evidence: `_skill.cgi:8807–9770,9948–10194,10384–10430,10559–10624,
11035–11104`; `_battle.cgi:28–142,748–1178,1390–1428,1715–1759,1855–1873`;
revision and detailed reconciliation in
[#1343](https://github.com/witchcraze/party2re/issues/1343).
This contract does not certify native production formulas. Callback guards,
coefficients and tails remain part of each entry's complete support boundary.

Callback category (physical, magic, breath, dance, song or untyped) is independent
of an elemental name and of direct/normal-attack flags. Category seals check only
magic/dance/physical, each with a fresh 1/4 cure attempt. Enemy-side category
conversion precedes that check: item166, eligible noncustom magic-sword commands,
then seal12 with its 3 MP charge. The magic-sword multiplier and replacement of
trailing arguments with direct1 apply to every category on that whitelist; only
its physical-to-magic conversion is category-dependent (`10171–10184`). Thus
item166's untyped result still receives that multiplier. An elemental multiplier
cannot replace these rules.

Resolve the target before applying its temporary effect once. Cover selects the
first same-side covering member, without a living filter. Defense/reduction/double
can modify numeric healing or buff coefficients as well as damage; symbolic
payloads bypass numeric multipliers. Matching reflection redirects to the actor
(item163 doubles nonzero numeric payloads), nullification sets zero, and deflection/phantom
use their own ordered draws. Magic absorption adds capped MP but does **not**
cancel the incoming effect. Ordinary ally resolution redirects fallen to actor
except revival; confusion has different member-selection predicates. Preserve
source-context gates instead of replacing them with living-only or lowest-HP rules.

Explicit spell operations use `LifetimeState.ResolvePartyTarget` and
`ResolveEnemyTarget`, with ordered side rosters and caller context independent of
element. `CallbackPayload` preserves numeric fractions and symbolic payloads;
`TargetResolution.Resolved` means the target helper returned a continuation, not
that damage/healing/status was applied. Ordinary fallen enemy or sealed-category
helper rejection returns an unresolved result; a resolved zero payload remains
zero for the consuming helper's early return, without a new minimum damage floor.
Category cures and item195 activation remain observable even on later helper
rejection. The two enemy-confusion replacements use the full saved member order,
including fallen/self, and retain the special boss marker's continuous >1 gate.

`ResolveTemporary` exposes optional target/payload replacements. Spell helpers
retain incoming values when a replacement is absent. Optional tool HP/MP hooks
instead use tuple assignment and can observe an absent payload after cover or an
absent pair after an unknown temporary; disabling the hook skips it altogether
(`10559–10584`). Tools do not acquire spell category checks or fallback rules.
MP debit and absorption requests are returned explicitly for the resource owner
to apply in source order: seal12 subtracts3 with a zero floor before subsequent
helper gates, absorption adds to the temporary's owner with a MaxMP clamp before
the incoming effect continues, even if a later phantom-field redirect changes
the final target. Symbolic magic restores30..49 MP; den reduces this to
`floor(recovery * 0.025) + 1`, and an actor's Phantom skips the target temporary
(`_skill.cgi:9542–9555,9695–9715`). These shared operations do not themselves write
MP or certify a resource callback. Phantom-field identities must be supplied by
the certified roster owner, including its matching-copy existence predicate and
ordered candidate list; display names never substitute for encounter identities.

`ApplyTemporaryGem` implements the finite source producers34–53
(`_data.cgi:2122–2141`): unconditional self defense34/37, category-checked
self/party effects, targeted symbolic effects and paired cover52. Party effects
skip fallen; applying revival temporary51 still uses ordinary fallen-ally
fallback, unlike an actual revival payload. Source41/42/45 omit the returned
target guard and can mutate an empty record after category rejection; their sealed
variants remain wholly unsupported. Regeneration50, automatic revival51 and
absorption53 produce state but do not certify their later HP/MP/lifetime consumers.

Explicit HP recovery uses `ApplyHealing`/`ApplyPartyHealing`; proportional
AT/DF/AG changes use `ApplyStatUp`/`ApplyStatDown`/`ApplyPartyStat` (down also
supports HP and hit). `Participant.BaseStats` retains equipment-free AT/DF/AG
from the adapter, independently of current stats. Up uses base stats and a
2.5× cap (self164, or self183 AG: 3×); down uses current stats with the source
immunities, 20% base floors and hit50. Returned cap/binding tuples are explicitly
unapplied. `EffectApplication` separates the requested amount from actual delta,
and carries target-resource facts and incidental healing even on no main change.
`EffectContext` counters/activation flags belong to one submitted operation and
restart for the next operation; persistent resources, tension, temporary effects
and base stats remain in `LifetimeState` across reloads.

Healing preserves target resolution, ordered146/256/148 modifiers, the independent
special-boss marker exception, shared modifiers, rage removal, random variation,
tension consumption, cap, single-only256 splash and sanctuary overflow in that
order. A full target still consumes tension; a modifier-zero return does not.
`ApplyToolHealing` has its separate living precheck and optional untyped tuple
hook, then randomness and cap; no tension/shared modifier/overflow pipeline.
An optional cover hook may return only a target (zero power); a registered hook
returning no target addresses an unrepresented empty record and remains rejected.
Unknown temporaries have no registered hook and preserve tool input values.
`ModifyEffect` owns only the shared nondamage modifier segment, including186's
tool heal before144/239 and the bound/poison recovery modifiers. MP owners reuse
that segment without adopting HP writes; MP drain/exclusions remain separate.

Explicit damage consumes each producer's base power. Direct and normal-attack
flags are independent of callback category and element. Resolve targets and
temporary redirects first, return on zero, count the invocation, redirect a
fallen continuation to the actor, consume one-hit defense, then test physical
hit/evasion. The accuracy comparison is strict (`hit < draw×100`); the hit and
agility draws precede the binding/forbidden-status and weapon63 exceptions.
Critical power is current AT×0.75 at the normal-attack producer, not an extra
multiplier in the helper, and direct alone does not skip hit checks.

Job77/83 callbacks compose these same target, damage and instant-death owners
(`_skill.cgi:5929–5983,6620–6667`). Job77's two ordinary attacks temporarily
use floor(AT×0.75), restoring saved AT even after defeat/reset; the ordinary
helper retains item147's followup but does not repeat the outer seal10 command.
Its other bodies use target tension `(tension+1)/2`, 2–3 area attacks at AG×1.5,
and two untyped attacks with one captured target. Its final callback freezes
whether a temporary effect existed when the table was built: that variant
clears the current temporary then performs six direct AT×0.55 hits; the other
performs three ordinary AT hits. Preserve the selected variant's concrete ID
in frozen input rather than recomputing it from later temporary state.

Job83's ordinary attack temporarily uses AG×5 and hit+5, restoring both saved
values after reset/defeat; the misplaced source hit-cap variable does not cap
actor accuracy at99. Party accuracy adds20, capped95, including fallen allies.
The remaining callbacks use physical instant death with chance19, three fresh
target-argument shifts at AT×0.85, self rage, and one direct hit at
max(AT×0.85, AG×1.5). Repeated skill hits stop on caster death; job77's two
ordinary helper calls have no intervening living guard. Complete ordered
job77/83 tables, including both final-job77 variants, share invocation admission;
unimplemented random tables and reachable missing descendants remain whole-action
gates. Listed MP/SP costs belong to ordinary dispatch, while indirect dispatch
retains its existing cost rules. Runtime closures and operation counters are
rebuilt after reload; committed effects are never replayed.

Job74's duel (`_skill.cgi:5594–5720`) temporarily uses floor(AT×1.1) for up
to two alternating ordinary attacks per combatant. Replies swap actor and the
ordered side arrays; restore those arrays and the saved AT, retaining incidental
effects and activation facts. Its unresolved-target fallback indexes all members
by enemy count, including when the enemy array is empty. Compare HP losses after
death stops: restore actor HP on a win, target HP on a loss, or both on a tie.
Actor death takes precedence when both die. These direct HP writes do
not undo defeat/reset, queued effects or rewards. A lethal win always grants an
untyped base-AT×0.2 increase and five-turn crown; a damage win grants them with
probability1/4.

The other job74 bodies provoke each other living member with probability1/2,
set self physical reduction directly, deal ordered direct untyped area power150,
or choose self revival protection with probability1/3 versus equal chances of
deadly poison/immobilization with tension reset to1. Its flurry captures the
target only when crowned and starts with six hits, otherwise three fresh random
targets. Power is current AT×0.9, with the coefficient multiplied by
`1 + rand(0.1)` after a returned target. The first three successful hit positions
may each extend the loop with probability1/2 while the actor is uncrowned; each
returned target independently attempts recoil with probability1/3 at current
AT×0.075×the increased coefficient. Misses and one-hit defense return no target
and skip these tails. Losing the crown during an initially six-hit loop can
therefore extend it to nine hits. Admission preserves later defense consumption, confusion
activation and possible recoil cures before costs or committed RNG. The complete
ordered table uses the same ordinary/indirect cost and reload boundary above.

Damage modifiers follow `_skill.cgi:9948–10129`, including strict magic143
and area144 thresholds, independent direct/normal predicates, and item239's
second/third/fourth invocation bonuses only outside area damage. Purification
resets after modifiers while preserving target status/tension. The defense term
uses resolved current DF **plus separately evaluated armor DF**, then attacker
181/187 and target162 rules. Tension precedes mitigation;149 instead modifies
base power and retains tension. Direct bypasses mitigation except physical
against162 or baseDF>999. Random variation is applied once; only a result below1
draws the1-or-2 fallback. Damage activates185/195 before defeat, then surviving
physical162 defense increase takes precedence over interception. Interception
uses an untyped-package, direct, nonlethal tool hit, consumes one-hit defense,
and decrements even when blocked; it neither counts another damage invocation
nor inherits the lexical physical category. Darkness records the first eligible
hit before seal11's reset, which preserves target tension. Evaluated gear terms
survive reload and are not redrawn or inferred from current buffs.
The explicit actor adapter supplies equipped weapon/armor/accessory identities;
bag contents do not activate these modifiers.

`ApplyDamage`/`ApplyAreaDamage` and finite `ApplyDamageGem` producers cover
sources1–18,26,72–73 (`_data.cgi:2089–2106,2114,2160–2161`), including each
multi-hit draw, successful-damage recoil3, drain26's original capped power for
healing, and72's unconditional actor defeat call. Normal attack preserves
seal10 and147 additional-hit predicates. `ApplyExtension201` requires represented
item150/item159 cursor provenance; its write uses the cursor **after** damage,
including a nested reset's cursor change, then restores actor agility. Its
intrinsic agility guard requires no skipped cursor/damage context. These are
formula certifications with explicit prerequisites, not batch DTO or outer
catalog certification. Fully admitted reset/defeat operations remain supplied
by their existing owners; absence rejects reachable work before draws. Cosmic
and antlion damage descendants remain gated for the named-field owner. Admission
follows ordered area targets or each repeated selection up to the producer's
maximum hit count. Only defenses reachable in earlier invocations are projected
as consumed; fully blocked attacks need no skipped damage-tail context. Confusion,
reflection, category cures and later195 activation can make a target reachable
again and must still reject unsupported tails before any draws or writes. All custom,
random and delegated closures still require whole-entry admission through
`skill.BattleDefinitions`; helper availability never enables a slot by itself.

`DamageGemEntries` retains existing native IDs, generic callback names, source
positions, target/category roles and CMP contributions for these21 bodies.
`BattleDefinitions.RegisterDamageGems` binds them to the existing invocation
catalog; `DamageGemAction` and execution share `ValidateDamageGem` before costs
or committed draws. CMP metadata is not an MP debit. Single-target bodies consume
their target argument;6–9 randomize each hit without consuming it, and area bodies
ignore it. Custom missing targets default to actor before confusion/fallback,
while naked callbacks retain their supplied argument context.

Each body applies target-helper MP conversion/absorption immediately, including
drain26's initial symbolic target check, its subsequent damage check and healing
target hooks. Returned MP facts are already applied and must not be replayed.
Zero/blocked HP damage may still leave these MP changes. Drain admission includes
targets made reachable by a successful magic-seal cure followed by item195
confusion, before selecting or mutating anything. Its requested base is computed
from the first resolved target's current HP, capped300; only a nonempty returned
damage target enables healing. Reflection, nullification and one-hit defense retain
their own returned-target semantics. Sacrifice72 invokes the same shared defeat
closure after damage even when damage has no effect; automatic revival may prevent
death, with no added MP0/1 write.

Repeated producer calls retain operation counters and use current battle facts;
reload restores those facts and frozen input rather than replaying damage or
absorption. Arbitrary prior callback transitions still require the enclosing
#1436 custom-composition projection and remain gated by `AfterEffects`; registration
does not certify a complete mixed-slot action, random pool or production consumer.
SQL/service/Gateway continuation remains #1328/#1329/#1330. Named cosmic/antlion,
missing reset/defeat and malformed101/102 gates remain unchanged.

Concrete native bodies in `ApplyRecoveryStatGem` are source19–23 (single,
self-untyped and party healing),28–33 (self-physical or targeted-magic AT/DF/AG
doubling), and93–96 (party enemy AT/DF/AG/hit reduction), evidenced at
`_data.cgi:2107–2111,2116–2121,2181–2184`. Source54 restores party hit95,
including fallen records, without a category check. Source67 invokes self tension
twice in order;68 resolves an enemy with physical-category symbolic input before
writing tension1;69 writes tension1 to every member, including fallen opponents.
Source74 resolves an untyped ally and invokes tension without adding a helper-return
guard; tension retains its absent-target actor fallback. These five bodies are
evidenced at `_data.cgi:2142,2155–2157,2162`; staged tension, requester-level
thresholds and ordered145/161 hooks follow `_skill.cgi:942–990`.
Stat-up preserves201's untyped/direct
damage call with half the resulting AG before testing205's AT/tension/double
tail. Admission checks only targets reachable through the ally helper's fallback,
confusion/activation and one-pass temporary redirects; unrelated equipment cannot
block the operation. Fallen resolved targets, the 2.5×/self3× cap, binding and
zero-power/rounded-zero paths return without201/205 tails and require neither tail's execution
context. Admission checks these guards without consuming draws or changing state;
execution still preserves target hooks, no-effect tuples and186 healing before239
zeroing. Ordered party/distribution admission also accounts for earlier reachable205
tails changing tension or replacing a temporary with double. Later cover lookup
includes the next covering member or the uncovered target when preceding coverers
can lose their covering state; a remaining stable coverer still blocks that target.
Cap, binding and zero-power guards prevent inactive205 tails from expanding these
candidates. A later rounded-zero projection alone is insufficient after a205 tail.
Reachable201 tails require the damage owner's fully admitted executor
before any draws or writes;205 requires the original requester level. The damage
executor updates the same operation's damage count after its
target/error/zero gates (`_skill.cgi:8826–8831`), including misses; the stat helper
does not count the call again. Subsequent239 zeroing and256 splash conditions read
that shared count.

`RecoveryStatGemEntries` retains existing native gem IDs, generic callback names,
source IDs, target/category roles and individual CMP contributions. Explicit
`BattleDefinitions.RegisterRecoveryStatGems` binds these complete bodies to the
existing invocation catalog; the batch scalar registry remains separate.
`RecoveryStatAction` and `ApplyRecoveryStatGems` share whole-slot admission before
outer costs or committed draws. Both boundaries reject more than three positions,
matching `_skill.cgi:302–307` and `_npc_action.cgi:220–222`; empty positions do not
permit a fourth position. Player synthesis separately enforces weighted gem capacity
(`custom_skill.cgi:144`, `internal/customskill/synthesis.go`); NPC template loading
does not apply that player synthesis gate (`_npc_action.cgi:623–627`).
Repeated slots keep the original submitted target;
custom commands normalize an absent target to the actor before the helpers,
while naked callbacks retain helper fallback. A complete custom body can be bound
with `RecoveryStatInvocation`; its enclosing admission projects reachable205
temporary changes, tension stages, category cures/confusion and stat-cap reopening.
Alternative target paths within a slot (such as fallen fallback and cover redirects
converging on the same ally) are projected from that slot's starting bounds without
accumulating multiple tension applications onto the same candidate in one slot.
Category cures remain conditional: a seal can block every matching helper in the
sequence. Admission retains both possible tension consumption and retention, so an
initial tension8 cap cannot hide a lower stage that later needs requester context.
Gem68 retains fallen-enemy and wrong-side fallback no-effect branches, preserving
the unreset tension ceiling for subsequent slots unless all reachable selection paths
are guaranteed to resolve to an alive self-reflecting actor or confusion deterministically
targets self without a boss field.
An accepted stat-cap outcome still retains tension (`_skill.cgi:9318–9338`).
Possible reductions reopen later stat/equipment-tail admission, while the unchanged
input cap remains a possible no-effect branch when proving tension consumption.
An immune or redirected reduction must not turn that possible change into a guarantee.
An arbitrary preceding callback, or a reachable201 damage tail followed by another
slot, needs that owner's transition projection and remains rejected here.

Complete gem bodies apply helper MP debit/absorption before their effect tails.
Returned `EffectApplication` facts describe completed writes, including requested
amount versus actual clamped change; consumers must not apply those resource facts
again. Operation counters and callback closures are rebuilt per command. Save
resulting `LifetimeState`, frozen input and facts with the consumer's receipt;
loading or retrying a committed receipt never executes the callbacks again.
Registration does not supply outer custom eligibility, charging, NPC selection,
SQL/assets/receipts or mixed callback closure certification.

`ApplyDistributedAttack` supplies calling80's first strict maximum (including
fallen), cursor-owned999/1998 cap, living nonholder increases and independent
AT/DF/AG/heal tails. Its caller must supply the roster owner's named/fallback
helper and explicit missing/item150/item159 cursor provenance. Summoning and
complete calling80 composition remain the roster owner's responsibility.

Shared target/producer operations certify no additional production catalog entry
by themselves. Full slots, complete damage/recovery/status formulas and indirect
closures still need their callback-specific guards and tails through
`skill.BattleDefinitions` and whole-action admission. Unsupported
contexts must be rejected before costs, state or RNG, including an absent target
that boss-field confusion can retain and reachable deflection into an unrepresented
empty side (preserving operations where deflect is on an unreachable participant or
after single-evaluation cover/reflection redirects). The existing batch resolver retains its separate contract; these operations
do not certify its lowest-HP/living-only targeting as normal-adventure parity.

Distinguish helper rejection, accepted no-effect, actual application and failure.
Legacy returned tuples and invocation counters do not establish applied success:
stat-up at cap/bound target returns a tuple, failed status can return a tuple,
and damage counts before one-hit defense/miss. Zero modifier returns can suppress
tension use and all later damage tails. Area damage's initial modifier evaluation,
roster order, actor-death break and conditional item144 decay threshold (<=300)
must remain distinct from independent repeated single hits.

Instant death uses currentHP>999 or baseDF>999 immunity and probability>=draw;
status uses NPC **MaxHP**>999 versus player **currentHP**>999, baseDF>999,
and probability>draw. Den's bypass applies only to the player threshold branch.
Item161 tension>=5 and weapon68+armor53 supply separate gates. Cure requires a
living resolved target and matching status; deadly-poison cure also clears
ordinary/virulent poison. Ability cure runs before incapacitation: matching
specific ability 1/2, then general ability 1/5 only if status remains. A hook's
existence alone does not import its map17-only producers into ordinary stages.

`ApplyStatus`/`ApplyAreaStatus` modify probability through the shared nondamage
segment, while retaining the symbolic effect through target redirection. A zero
modified death probability still succeeds at draw0. Status checks the equipment
pair before the tension shield; death checks the shield before drawing. The
plural loops visit every ordered enemy without area damage's actor-death break.
Death admission projects each possible helper recovery before the existing
revival/defeat closure. Ordered area continuations retain that recovery even
when the death roll fails; later targets are checked after earlier resource and
revival changes. Unreachable final strikes and reward hooks cannot reject a
guaranteed revival, while any reachable final defeat retains their validation.
`ApplyCure`/`ApplyPartyCure` retain matching-state and living-target predicates;
gem62 resolves once to read current status and again inside the cure helper.
`CheckIncapacitation` runs ability cure before its status-specific checks.

`StatusEntries` and `BattleDefinitions.RegisterStatuses` bind gems55–62,70–71,
75–81 and job35.1 (`_data.cgi:2143–2150,2158–2159,2163–2169`;
`_skill.cgi:2879–2886`). These are concrete intrinsic bodies with independent
category, target and CMP metadata. They do not assemble job35's table or admit
mixed custom transitions; unprojected `AfterEffects`, incomplete random tables
and unavailable descendants remain whole-entry gates. Ordinary learning,
MP/CMP charges and the production consumer retain their existing owners.

Reset clears transient state, rebuilds equipped stats, **reapplies type3 accessory,
weapon revision and seal effects**, then floors agility. It does not generically
restore HP/MP, but an accessory can explicitly do so. Revival precedence, item
consumption and post-reset death attacks must use the same defeat/reset path.
Dispel therefore need not leave empty state or unequipped stats. #1332 owns
reset/lifetime state, #1338 revival, #1337 MP and #1339 roster operations.

`ApplyEquipmentReset` recognizes the source's117 category3 identities explicitly,
including empty callbacks; item110 remains rejected in living and fallen
contexts. Appraisal callbacks251–255 still consume their discarded-result draw.
Raw table weapon attack and weights survive in `BattleGearStats`; seal3's
requester weapon coefficient and seal6's one table-load random weight are shared
`WeaponSealTable` facts, captured by the adapter and retained for the request's
continuation. Missing raw weapon or seal facts reject before reset. Weapon69/70
use requester level capped99, while71 uses raw50/75 thresholds and the reset
target's actual158/240–242 accessory or the **current actor's** awakening. A
target's awakening has already been cleared by its own reset. Accessory159 also
updates the operation cursor. Job35.1 resets every ordered member, including
fallen members and both sides, restoring each member's saved tension afterward.
Its preflight checks later members before any earlier reset or draw.

`ApplyDefeat` composes `ApplyAutomaticRevival`, this same equipment reset, the
post-reset212 tool strike and same-side212 inheritance, then the NPC/self reward
guard. Inheritance includes fallen holders: party stats cap999, enemy stats do
not. The source death-mark temporary survives final defeat. Reward callbacks
remain owned outside core and run only after those hooks; fortress is delegated
to the field owner only after rewards, using the battle-load field snapshot and
current anti-field. Callback failure stops later hooks. Admission uses post-reset
attack bounds and the tool's defense, armor,162 and damage variation to require
target defeat only when a212 strike can be lethal. Guaranteed nonlethal hits
retain their HP loss for subsequent area invocations; one-hit defense is consumed
once. Alternative recipients and surviving/defeated outcomes keep separate
continuations, including equipment reset, revival/consumption and inheritance.
Validation uses private state and finite reset boundaries without consuming the
caller's RNG or changing its operation context. Owners whose reward/field hooks
change combat scalars supply a pure continuation projection; ordinary rewards
are combat-neutral when no projection is supplied. A reached fortress hook must
supply this projection before a later area invocation, otherwise the entire
entry is rejected. Validation never executes external settlement callbacks.
A reachable recursive212 closure
without a finite supported continuation remains rejected. A212 helper path that returns
no target also remains gated: its source tool continuation addresses an
unrepresented empty record, rather than a supported no-effect outcome.
Item193 consumption and260's
second cursed defeat reuse automatic revival; no separate asset settlement is
introduced. Evidence: `_battle.cgi:28–142,282–303,1390–1428,1726–1759` and
`_data.cgi` category3 callbacks, reconciled in #1343/#1367.

Named-field reads and eligible field tails have different predicates. The latter
check before **or** after field name, response eligibility, no anti-field and no
suppression; helper modifiers often read the name without an anti check. Burn
is nonlethal and clears transient states; grave restores/recreates the snapshotted
helper, with existing-helper MP restored on player tail but not NPC tail. Library
has selected book modifiers and a player-only random-member MP tail; fallen picks
have no effect. Cosmic distinguishes repeated single hits from area count;
antlion draws immobilization and defense reduction independently. Sanctuary,
seal, den, phantom and fortress require certified producers as well as consumers.
Snow has no evidenced shared numeric modifier; stage23's declared default is EX-only.

Immediate recoil and deferred darkness debit are different operations. `_risk`
counts, rounds value+1, and item160 can turn it into recovery/cure/AT; `__risk`
only debits a living actor nonlethally. Darkness queues the first hit's half-damage
capped at 500; a prior immediate recoil removes that queued entry before execution.
Gems101/102 register bare code references, but every reachable queue lookup or
execution dereferences hash fields and fails on that entry. No successful recoil
execution exists through these consumers at the reference revision. Registration
alone succeeds; failure can occur later, after other effects. Native admission
rejects the entire invoking entry before effects/costs instead of reproducing
partial CGI failure. Repairing the representation requires an explicit gameplay
decision; these gems remain unavailable.

Finite random composites retain branch-specific writes: `_parupunte` probabilities
1/2 temporary state, 1/4 abnormal state, 1/8 full HP, then 1/24 each agility 0,
HP 1 and no effect;
states are shared draws, and fullHP/agility branches can include fallen members.
`_rainbow` selects seven outcomes uniformly; revival tmp includes HP0 but excludes
negative HP, revival restores fallen, and other branches compose the ordinary
heal/stat/cure/MP helpers in source order. They are not random damage payloads.

The [callback context contract](#callback-context-and-failure-admission) resolves
the named binding gates. #1362–#1366 own target, damage, recovery/stat, status/reset
and field/composite implementations respectively. The detailed #1343 modifier
and #1367 binding ledgers supply evidence; unsupported closure rejects the entire
outer command through the existing admission boundary.

#### Callback context and failure admission

Reference revision remains `ff97fc549932a146cc945d7edf71b44c868a840e`.
[#1367](https://github.com/witchcraze/party2re/issues/1367) owns detailed dispatch,
scope evidence and boundary vectors. The decision is to preserve evidenced
bindings and reject broken or unrepresented contexts; no gameplay repair is
approved. This specification does not register native production effects.

Keep the requesting player's data, current effect actor, submitted target,
resolved target, callback category and operation-entry field snapshot distinct.
An NPC/helper can temporarily become actor without changing the requesting
player. A caller's lexical variable is not inherited by a separately declared
callback. A loop-local variable does not become an ambient value after the loop.
Native state should express only the evidenced context, not emulate CGI globals.

| Gate and evidence under `party2/lib/` | Preserved rule and bounded native owner |
| --- | --- |
| Extension201, `_skill.cgi:343–358` | Damage uses the submitted target. Afterwards agility is written through the then-current package target cursor, independently of the returned damage target, followed by actor agility reset. Item159 reset can change that cursor during damage. Missing cursor addresses an empty-key record in Perl, not self. #1363 owns composition with #1365 reset; reject any invocation whose cursor provenance/state is unrepresented, including indirect closures |
| Job78 ordering, `_skill.cgi:6145–6174` | The initializer reads the package cursor before declaring its local returned target; submitted target is ignored. Misfire draws first (1/6), immobilizes actor and sets tension1 without damage. Otherwise breath50 direct uses the cursor and ordinary resolution. Returned ally/fallen/breath-reflector suppresses reorder; otherwise move that enemy to the end. #1360 composes #1362/#1363; missing cursor follows missing-target resolution, not an invented submitted target |
| Calling80 distributed AT, `_skill.cgi:6436–6465` | Maximum search includes fallen and keeps the first strict maximum. The cap999/1998 reads the package cursor's over-level flag, not the loop variable, summoner, helper or maximum holder. Raise only living nonholders below cap, then independently draw four 1/4 tails in AT/DF/AG/heal order. #1339 composes #1364; missing cursor selects cap999. Unrepresented cursor remains a whole-entry gate |
| Enemy confusion, `_skill.cgi:9654–9715` | After item195 activation, category conversion, empty-enemy and category checks, draw a member unless current target is actor. Then repeat that test against the replacement. In the special boss field only, a self target draws range0..3 and is kept when draw>1; otherwise draw another member. Both replacements use all ordered members, including fallen/self. Confusion bypasses the ordinary fallen-target rejection; later temporary/phantom resolution still runs. #1362 preserves both blocks and their short-circuit draw order |
| Item110 fallen reset, `_data.cgi:1366–1384`; `_battle.cgi:1390–1408` | Living target returns. For fallen target, party is overwritten with enemy then immediately restored; area instant death therefore sees the original sides. Only after it returns does persistent item removal run, followed by swapping the sides. Actor and submitted target are not swapped; members are not rebuilt. Nested resets share scratch rosters and can recur before consumption. #1365/#1339 retain whole-entry rejection for this broken callback; #1197 owns any eventual approved consumption contract |
| Tool damage, `_skill.cgi:10588–10624` | Item162's direct override reads package category, not the lexical category in a calling damage helper. Checked battle producers leave it absent or set a numeric saved AT (job77 at5931), never physical. Thus physical interception does not itself enable this override; item162's larger defense term and baseDF>999 rule still apply. #1363 owns tool/interception, #1365 item212, #1366 cosmic descendants; reject unknown category provenance rather than assign physical to all tools |
| Fortress defeat, `_battle.cgi:21,28–303,1568–1610,1715–1723` | The field name is a file-lexical snapshot taken when battle code loads, before member/action processing. A new or removed field does not replace that snapshot in the same request; anti-field is checked at the hook. Revival, NPC-actor and self-defeat returns skip it; reward-loop skips alone do not. Heal living members of fallen participant's side with tool20%MaxHP only if the hook is reached. #1366 composes #1365; map15 producers are separate from ordinary stage support |
| Weapon revisions, `_battle.cgi:1726–1759`; `system.cgi:1705–1708` | Weapons69/70 add floor(original player level×1.5/2), where original level caps at99. Weapon71 uses that player's raw level thresholds50/75 even when resetting an NPC/helper/other target; its item158-equivalent check uses current actor awakening or queried target's actual158/240/241/242. #1365 preserves these independent contexts; missing required player/actor context rejects admission |
| Deferred gems, `_data.cgi:2194–2195`; `TurnEndProcessor.pm:28–47`; `_battle.cgi:1855–1873` | Registration succeeds without recoil. Darkness lookup, immediate-risk prefilter lookup, or execution fails when it reaches a bare code entry. Actor death does not make hash dereference safe. #1363 retains native rejection for101/102; #1332/#1366 own valid typed darkness work and ordering. Neither skipping nor wrapping the malformed entry is approved |

The fortress producer is a persisted default-field write in
`party2/map/15/__1.cgi:28` and `_2.cgi:30`, reached through dungeon map dispatch
(`lib/vs_dungeon.cgi:23–24,74–101`). A later request can load that default into
the snapshot. Its existence does not certify an ordinary numeric-stage producer.
Likewise the package target cursor has concrete setters in item150/tool dispatch
and item159/reset (`_data.cgi:1619,1682`), but is not a durable participant target
carried automatically between CGI requests. Do not infer its value from a caller's
local variable or serialize arbitrary global state to make these entries pass.

For valid darkness entries, immediate recoil count is checked **before** queued
execution and deletes darkness work then; it cannot be incremented retroactively
by queued work to change that prefilter. A malformed gem before a damage slot
can fail in darkness lookup; after a valid darkness entry it can fail in prefilter
lookup or execution. Earlier valid queued work can execute before execution reaches
the malformed entry; later work cannot. These are source failures, not successful
20%MaxHP recoil or accepted no-effect. Discovery and submission must reject a custom
or delegated closure containing such an entry before any slot runs, even when
another slot might kill the actor before it is reached. Closing a specification
gate does not close #1329/#1330's native effect prerequisites.

#### Resources, revival and roster changes

HP recovery and MP recovery are separate operations. `_mp_h:9150–9187`
requires a living resolved ally, applies effect modifiers, random variation and
tension, clears rage, enforces the maximum and special-item exclusions, and has
a sanctuary overflow penalty. MP drain uses `_st_down:9252–9290`, with its own
immunity/floor/cap rules; it is not damage against HP. Immediate values become
committed state; no reload may draw their random result again.

Revival requires its own callback predicate. Tool 6 restores full MaxHP;
book 173 succeeds with probability 1/2 at floor(MaxHP×0.5); book 196 restores
full MaxHP and may seal its caster. Gems 63/64 restore half/full MaxHP, gem 65
draws independently for each fallen ally, and gem 66 restores every other ally
before invoking the actor's defeat handling and setting actor MP to zero.
Extension item 261 revives fallen, not-yet-cursed allies to full HP and adds
curse. These are not ordinary healing of living allies. The helper-dependent
fallen-self action retains the separate rules in the action-family matrix:
living helper required, fallen self at half MaxHP, otherwise fallen ally at
1 HP with actor MP **strictly greater than 40**, helper HP halved, immobilization
and a three-operation ban (`_skill.cgi:4910–4930`). Death-triggered revival and
reset/reward ordering remain part of `_battle.cgi:defeat:28–142`, not a blanket
post-battle “keep 1 HP” correction.

`RevivalEntries` identifies19 concrete job producers and the tool/book/gem/extension
subset. `ValidateRevival`/`ApplyRevival` implement their bodies;
`ValidateRevivalAction`/`RevivalAction` add ordinary bans and admit a fallen actor
only for66.6 with a living designated helper. Discovery still owns current/previous-job,
extended-SP, equipment and cost admission. Indirect callbacks retain the source's
absence of these ordinary action guards. `RegisterRevivals` binds the same bodies to `BattleDefinitions` and the
existing indirect invocation gate, without advertising an incomplete job table.
`ApplyRoster` delegates66.6 to this owner; calling-table assembly remains #1480/#1481.
Direct revival preserves recipient status, temporary effects and tension except
for the callback's explicit writes. It has no healing modifiers, counters or
universal minimum1HP/MaxHP clamp.

| Concrete job positions | Enduring callback rule | Evidence in `party2/lib/_skill.cgi` |
| --- | --- | --- |
| 3.6,14.4,17.4,76.3 | FullHP for every other ally, including living allies; invoke actor defeat, then MP1. No caster seal or living check in the body | 1169–1181,1734–1747,1897–1909,5842–5855 |
| 7.8 | Untyped area instant-death60, then actor defeat and MP1, even if the area call already defeated actor | 1348–1353 |
| 5.7,39.5 | Magic revival target helper; fallen recipient has independent1/2 chance at floor50%MaxHP | 1245–1253,3081–3089 |
| 16.6;17.6,18.6,33.6,43.6 | Magic revival target helper; fallen recipient gets floor25%MaxHP, or fullHP respectively | 1848–1853,1914–1919,1956–1961,2805–2810,3209–3214 |
| 18.4,70.3;51.5 | Iterate fallen allies directly; independent1/2 halfHP, or1/5 fullHP respectively. No caster seal check | 1941–1951,5306–5316,3657–3667 |
| 32.4 | One magic seal/cure check, then floor15%MaxHP for every fallen ally | 2757–2765 |
| 50.6 | Magic target helper uses a nonrevival symbol, so ordinary fallen fallback applies; a resolved nonself recipient receives fullHP/fullMP before actor defeat/MP1 | 3598–3610 |
| 75.5 | Magic revival target helper; fallen target requires MP≥1, receives its pre-debit MP as HP without MaxHP clamp, then MP halves/floors | 5791–5797 |
| 66.6 | Named helper presence takes precedence over appearance fallback, including a dead named helper. A living helper is required for either branch; successful revival immobilizes recipient, halves helper HP (possibly to0 without defeat), and sets the shared three-operation ban | 4911–4931 |

Position identities follow #1409's source ledger. Non-job evidence is
`_data.cgi:628–639,2151–2154` and `_skill.cgi:461–475,522–535,687–698`.
Book196 seals its caster only after successful full revival outside `library`.
Party loops and sacrifices retain their absence of caster seal/living checks;
single magic callbacks use the existing symbolic revival target helper, including
confusion, reflection and cover. Custom missing targets default to the actor in
their enclosing callback context.

`ValidateAutomaticRevival`/`ApplyAutomaticRevival` own the **prefix** of shared
defeat (`_battle.cgi:28–87`), before final-defeat/reward handling:

1. Temporary revival clears that temporary, restores floor(MaxHP×(0.1+rand0.1)),
   resets and returns, even when the rounded HP is0.
2. Pharaoh requires MP strictly greater than25%MaxMP; restore fullHP, debit
   floor50%currentMP, reset and return. The ability remains available.
3. Every other member with weapon37+armor38 receives floor50%fallen currentAT,
   capped999, with no living/team filter. This precedes later item revivals.
4. Item161 at tension≥8, or item193, restores floor50%MaxHP and resets. Item193
   is checked/removed **after** reset, recorded in `ConsumedItems`, and immobilizes
   its holder. Item161 remains equipped; reset can lower its tension eligibility.
5. Noncursed item260 restores floor30%MaxHP, resets, then adds300 AT/DF/AG
   without a999 cap and adds curse. Already cursed holders skip this branch.

`Prevented` means the enclosing defeat returns before rewards, independently of
positive HP. Otherwise it continues its existing final reset, final strike,
inheritance and reward logic. Sacrifices call this same defeat closure with the
actor's current HP intact and overwrite MP0/1 **after** it returns. The area-death
sacrifice still invokes that closure again if the area operation already defeated
the actor. No revival primitive awards rewards or repeats settlement.

Reset hooks must already certify equipment in the separate requester/actor/target
contexts, including possible HP0 resets; missing hooks, item110's broken fallen
reset, unknown context and missing instant-death owners retain whole-entry gates.
#1365 owns those nonrevival descendants and composes this prefix without a reverse
dependency. Arbitrary preceding callback transitions remain rejected until the
enclosing owner can project them; #1436 owns mixed gem-slot composition. Saved
facts include consumed equipment, curse, resources, helper HP and bans; reload
rebuilds only closures and never replays a revival roll. SQL continuation remains
#1328/#1329. The batch `CheckRevival` contract is unchanged.

##### Summoned roster and helper commands

`LifetimeState.Actors` preserves member order. Summoned incarnations receive a
monotonic run-local ID; `RosterIdentity` separately identifies the named helper
or enemy species/suffix. Tools72/73/74 compete for one dummy identity,216 uses a
separate guardian identity, and extension244 uses a separate snow identity.
Presence blocks another summon even at HP0. Saving via `PruneRoster` removes dead
NPCs/helpers and retains fallen real players, then rebuilds side arrays in member
order. Side reconstruction retains the operation actor's team before pruning,
including when that actor is a dead NPC being removed. A recalled helper receives
a new incarnation ID. Dismissal first sets HP0
and NPC color; the current request's arrays stay in their original order until
the save boundary. A same-request revival can therefore retain that participant
on the NPC side after reload. No roster operation submits an additional NPC turn
or finishes an action tail. Evidence: `_battle.cgi:453–493,518–529`.

`AddHelper` supplies the shared primitive for calling, tools and field recreation.
Each of HP/MP/AT/DF/AG independently rolls floor(base × (0.9 + draw × 0.3)), sets
its corresponding maximum/base, then separately floors ×1.2 for requester
item165. Initial hit is95 and tension1; current/previous job payloads survive,
and defeat EXP/gold are floor(requester level/2) and floor(requester level/4).
Grave-field composition restores an existing helper's MP on the player tail,
not the NPC tail; recreation calls this summon primitive and rolls fresh stats
instead of restoring a byte-identical old helper (#1366).
The requester remains distinct from a temporary effect actor. Helpers use
generic native identities and newly authored placeholder art. Evidence:
`_skill.cgi:9793–9828`; `_data.cgi:1140–1174,1914–1924`.

`RegisterCallingJobs` contributes complete `RosterEntries` tables and binds the
certified roster bodies, retaining other owners' positions and registrations.
Register revival and other descendant owners separately with their operation
contexts; table presence alone never admits their effects. Rebuild these closures
after loading saved facts. Ordinary learning/costs and normal-adventure consumer
wiring remain with their callers.
`RosterEntries` keeps the complete calling-table order and extended markers;
`ValidateRoster`/`ApplyRoster` own its finite producers. Normal attack switches
actor only for the existing attack helper, restores it even after an error or
roster growth, and retains operation counters. Special attacks use the helper's
source-selected stats with the summoner's modifiers. Merge caps the summoner by
its own over-level maxima/base stats and the helper at999, without changing their
maxima; an NPC/helper is dismissed and a real substitute loses its old appearance.
Duplication63 rolls a new helper from current stats, preserves source base stats,
clamps copied current HP/MP, copies temporary defense, and heals using the number
of helpers present **before** creation. Small-helper69 spawning checks each
identity independently. Growth87 rolls new maxima while retaining abnormal state,
tension, jobs and extended effects; its trigger and form checks remain independent
of ordinary command MP/SP. Evidence: `_skill.cgi:4103–4123,4519–4585,5241–5285,6904–7035,10627–10650,11001–11015`.

First non-command callbacks remain distinct:61 directly revives fallen party
members at42% MaxHP and adds300 AT/AG;62 composes area damage then party healing;
65 draws3–4 attacks using helper AG;66 adds420 power per fallen party member;
68 heals the party;69 resolves its magic target before taking5 MP from every
other party member with at least5 MP, including fallen members;80 composes the
existing cursor-provenance distribution and ordered tails. 67 independently draws
after each150-power breath hit to assign double temporary state to the returned
target; absent-tuple writes and unrepresented redirect transitions remain rejected.
87 performs one or conditionally two ordered120-power breath passes, skips status
when the returned target differs, writes poison directly above the999 MaxHP/baseDF
threshold, otherwise chooses deadly poison/paralysis and invokes its separately
certified75% status helper, then checks growth. Its status port must admit both
choices before any hit. Status/death, MP drain, revival, grave-field and general-random descendants
remain whole-entry gates under #1365/#1337/#1338/#1366/#1357. No scalar damage
substitution certifies those branches. Item110 remains rejected. The registry's
`RosterAction` and `RosterInvocation` use the same admission; an arbitrary earlier
indirect callback without a represented transition projection still rejects the
enclosing continuation before any slot executes or costs are paid. Roster
projections cover exact dismissal, merge, party revival, non-growing defense and
already-present or missing/fallen helper no-effects. Random creation, growth,
damage and recovery do not claim an exact transition. A revived helper's newly
reachable descendants must be admitted before the revival executes.

Enemy summoning consumes a complete current-stage payload pool and separate
weighted appearance positions, falling back to position0 for an invalid sampled
position. It draws an I–Z identity suffix and tries only that suffix and its
predecessor (I wraps to Z). Member count>=10 prevents all draws; gem82 also requires
at most one enemy. Hard mode floors ×1.5 for current resources/stats, job SP and
rewards before party scaling of resources/stats by 1 + (party count−2)×0.05.
Supplied maxima, hit/tension, status, extended effects, jobs, rewards and custom
costs are retained; absent maxima take the scaled current values. Native stage
payload certification remains #1342; ordinary `Monster` stat DTOs do not supply
missing job/custom effects. Evidence: `_skill.cgi:9834–9931`;
`_data.cgi:1197,2170`. These are descendants of existing job/custom/tool commands,
not new transport actions; #1329/#1330 own service/Gateway integration.

`NPCJobEntries` retains job95's two separate SP0/MP50 positions under one
generic summon name, and job99's SP0/MP0 escape position. `RegisterNPCJobs`
binds these complete tables through the same `RosterInvocation` admission as
direct execution. Job95 uses tool78's unguarded stage summon, including when
two or more enemies exist; it does not use gem82's extra enemy-count guard.
Job99 writes only the acting participant's HP0: no reset, defeat, reward,
automatic revival or immediate roster removal. Evidence:
`_skill.cgi:7183–7189,7409–7412`; `_battle.cgi:518–529`.

NPC selection retains duplicate eligible names as weights and resolves each
through the last eligible name binding after current-job, previous-job and
custom entries are appended. The selected cost is charged once after the
callback, including accepted no-effect summons. The selector and action tails
remain the NPC consumer's responsibility; these bindings do not initialize or
execute a spawned NPC's next action. Persist the frozen stage pool/appearance
weights and resulting ordered roster, then rebuild operation-local registrations
after reload. Committed retries replay the consumer's receipt rather than call
the effects again. Evidence: `_npc_action.cgi:177–224,313–322`.

#### Temporary transformation and job memory

Transformation separates run job identity, participant copying, previous-job
memory and cosmetic appearance. Explicit state retains current/previous job/SP
pairs independently of original character jobs and the actor's own recorded
job history. Copying does not inherit the target's identity, team, equipment,
abilities, CMP, hit, tension, status, history or original jobs.

Form switching changes the current matching job first, otherwise the previous
matching job (82↔300, forward10MP, reverse0). Between encounters it requires a
started run, no enemies and no cursed ability; forward discovery requires111SP
in either matching position. If both forms exist, the reverse name-keyed entry
is registered last and wins discovery. Forward changes appearance only for a
non-NPC actor, using the requesting player's gender; reverse leaves appearance
unchanged. In combat the same form callbacks apply. Temporary job300 supplies
four ordinary entries (cost30/48/99/0): two independent uniform draws among
AG/DF/AT for untyped self50% stat-up; resolve an untyped enemy target, temporarily
clear its temporary effect for physical/direct current-AT damage, then restore
the saved temporary even after defeat/reset; a2–3-hit direct breath callback;
and reverse form. The breath callback resolves the initial target before rolling
hit count, chooses subsequent targets from the invocation's enemy roster, uses
power500 for MaxHP>999 otherwise200, restores saved tension before each hit,
and stops further hits when the actor falls. Its tail still sets tension1,
immobilizes the actor and changes current300 first, otherwise previous300, to82.
Do not add300 to the general-random initial pool1..87.

Job36 and book211 check the magic category seal, then use an existing submitted
participant or a uniform fallback over **all members**, including self/fallen.
Eligibility is living HP and MaxHP/baseDF≤999, not current defense. They copy
current/maxHP/MP, current/baseAT/DF/AG, both job/SP pairs and appearance, with
copied MP floored at50 even above MaxMP. Book211 first requires current/previous
job IDs to match the original character's IDs; on success it also immobilizes
the actor and writes the room's named `copy` ban with sentinel999. Job36 adds
neither book-only effect. The named ban is enforced by outer command admission;
it does not become an invented guard inside a random child. Tool214 chooses a
uniform **other** member before the same living/size checks; no other member is
accepted no-effect. It has no magic seal check, MP floor, immobilization or ban.

Job79 rebuilds its appearance-dependent entry from the actor's current icon and
the requesting player's gender. The forward variant invokes all-enemy direct
breath150, then changes appearance only if it equals the requesting player's
original icon (this comparison applies to NPC actors too). The reverse variant
invokes untyped selfAG+150%, then restores the requesting player's icon even
when the stat helper normally returns no-effect. It does not change jobs.

Tool168 reads the actor's own frozen job history, restoring only previous job/SP
for a recorded job different from both active jobs. The adapter includes only
jobs backed by departure/mastery evidence or an explicit persisted job record;
SP preserved by migration alone never creates history, even after copying away
from the original current job. Recorded0SP remains eligible; missing/unknown/current
choices are accepted no-effect. A recorded job whose historical SP is unknown
is unsupported before any draw or mutation, rather than guessed at zero.
Gem100 changes only appearance, sampling108 positions uniformly; a missing
native appearance uses the monster placeholder. Store the selected reference
once, without rerolling on reload. Native form and fallback images are
[temporary, replaceable development artwork](../assets/required-images.md#img-005-battle--temporary-transformation-appearances).
Saved icon references retain identity independently of future rendering URLs;
an asset resolver/art replacement must preserve these keys or supply compatible
aliases, including gender/form fragments. Display URLs must not be written back
over saved references used by appearance predicates.

`LifetimeState.ApplyTransformation` and `ValidateTransformation` share explicit
payload admission; `TransformationEntries` describes only this owner's subset,
not a complete certified job catalog. `BattleDefinitions.RegisterTransformations`
binds the seven source positions (eight bodies including both job79 variants)
to that same gate and executor. Its ordered job36/79/82/300 table contributions
retain unbound positions; job79 selects its variant from current actor appearance
and frozen requester gender, while a captured entry keeps its selected body.
Table presence never certifies missing bodies or descendants. Ordinary
SP/cost/ban/asset ownership and complete random/delegated/custom closure remain
with their invoking owners. Initial random/generated discovery validates each
alternative against the known input; unknown earlier effects still require
conservative admission. Exact copy/form projections carry changed jobs, stats,
maxima and appearance into later admission without running effect hooks or RNG.
Other unknown transitions retain the conservative gate for state-dependent
descendants; copy and form bodies themselves need no such external transition.
Damage-dependent callbacks require the damage owner's completely admitted
executor; absent damage/area executors, reachable stat equipment tails and
unrepresented outer empty-target contexts reject before any RNG/cost/state
change. No generic batch `ActionSkill`/`GemEffect` certifies those paths. Save
resulting facts and the continuation cursor, rebuilding registrations from
frozen input after reload without replaying committed callbacks.
`BuildLifetimeActor` reuses the equipment-aware adapter,
loads job history read-only and preserves fallen entryHP; reload uses saved run
state, not the original builder. Reset/advance retains copied maxima/base stats,
jobs, icon, history and original identity. Exit writes only eligible current
HP/MP clamped to the permanent maxima, not temporary jobs/maxima/stats/icon or
training; encounter/exit persistence remains the Adventure consumer's boundary.
(`_skill.cgi:173–217,700–724,2944–2964,6240–6292,6593–6610,7465–7532`;
`_data.cgi:1716–1748,1888–1912,2188–2193`; `_battle.cgi:1184–1218,1390–1428`).

#### Ordinary NPC and stage-boss state

Ordinary bosses retain current/previous job and their independent SP, equipment,
HP/MP/CMP and supplied maxima, hit/tension, temporary/extended state, abilities,
custom skill and ordered gem slots. Preserve multiple bosses and branch-selected
rosters, not just the last monster in a stage catalog. `add_boss:600–633` rolls
HP/MP/attack/defense/agility/CMP with
floor(value×(0.9+rand(0.3)+(party size−2)×0.1)); explicitly supplied maxima win.
Custom CMP cost is floor(sum(slot costs)×rate), with rate 1/1.1/1.2 for one/two/three
occupied slots, counting duplicates. Freeze roster selection and rolls once.

The ordinary stage sources are `stage/0.cgi` through `stage/27.cgi`; stage 15
copies the ordered party into bosses and constructs ten ordinary monster
templates with current jobs drawn from player indices 0..87, previous job36
and previous SP999 (`stage/15.cgi:10–61`). Freeze both generated tables and
their context. Stage 25 randomly selects one of three ordinary boss rosters;
stage 26 selects its ordinary season from room markers in spring, summer,
autumn, winter priority, defaulting to spring (`stage/26.cgi:16–28,623–1205`).
This selection does not read the current calendar season. Read the
ordinary `@bosses` branch separately from `@ex_bosses`/`@exx_bosses` and `king*.cgi`.
Stage23's ordinary boss uses job21/SP10/MP30 (`stage/23.cgi:15–31`);
job406 and the declared snow default belong to its EX branch, so neither is
an ordinary NPC callback prerequisite through that stage.
Stage 24's **ordinary floor 10** invokes `createExBossField` despite its name
(`vs_monster.cgi:143–151`): party base maxima are capped at HP/MP 999 and stats
255, doubled for over-level participants, then reset. The default named field
in `add_ex_boss:722` belongs to the later EX branch; do not apply it to ordinary
boss initialization merely because `stage/24.cgi` declares `$ex_field`.

NPC action selection starts with the mode's SP0/MP0 normal attack
(`vs_monster.cgi:11–14`), independently of job0's five SP-gated table positions.
It builds current/previous-job lists with independent gates and custom gems
(`_npc_action.cgi:158–322`), then randomly selects an admitted position.
Repeated names retain their selection weight; the later admitted same-name
callback supplies the name-keyed execution body. NPC-only tables need their
own complete bodies even when they share player effect helpers. Selection does
not reuse the player's preferred-action policy or equipment extension list.
NPC confusion cure is 1/3 versus the player's 1/5; paralysis and
sleep cures skip action on that operation. The special boss-field and named
field extra cure draws are separate branches. NPC poison/regeneration/rage run
inside the normal-action block (`324–361`); item/field/ability tails outside it
(`364–495`) can still run after an incapacitated action. Burn is nonlethal,
capped at 200, and clears temporary/status/extended state; a grave-field helper
can be restored or recreated. These tails cannot be represented by one generic
“end turn” hook with the same eligibility for every actor.

The shared NPC curse hook releases its stored count of 400-base all-enemy
attacks **before** action selection, clears the payload and rebuilds living
opponents (`116–135`); damage can reduce that count (`_skill.cgi:8896–8902`).
Specter absorption is a distinct tail (`471–491`). Neither hook's existence
proves ordinary-stage reachability: the numeric stage files do not directly
initialize these payloads, and the producer `skill_505` is outside the ordinary
player-job draw. Keep raid producers outside this scope; any future catalog
entry reaching a shared hook must certify its producer and consumer together.

Production adoption still requires #1188 supply, #1332 resumable lifetimes,
#1337 MP resources, #1338 revival, #1339 summons, #1340 transformation and #1342
ordinary NPC payloads. Book/purchase effects require the
[combat asset contract](#combat-books-stored-tools-and-purchases), including
#1139's independent capacity decision, and the [shared modifier follow-ups](#shared-modifiers-and-callback-outcomes)
from #1343, including #1367's context and failure-admission contract. The transitive job
reconciliation in #1344 also requires #1357 invocation execution, #1358
arithmetic selectors, #1359 wallet/held-tool effects and #1360 job-driven room
transitions; each remains subject to complete per-entry certification.
Travel's dispatch/tail/next-operation rules are owned by
[Adventure](adventure.md#combat-travel-and-encounter-replacement), including
tool 150's retained old-enemy response/reappearance; source certification is
not native travel support. #1346 implements the shared support/admission gate.
#1329 must compare uninterrupted with serialized/reloaded continuation at each
supported boundary. #1330 cannot advertise a direct or indirect effect until
those gates are met.

### 2. Elemental Field Multipliers (`field.go`)
- **Matching Element**: $+30\%$ bonus damage (multiplier $1.30$).
- **Opposing Element**: $-20\%$ damage penalty (multiplier $0.80$).
  - Opposing pairs: Fire $\leftrightarrow$ Water, Wind $\leftrightarrow$ Earth, Light $\leftrightarrow$ Dark.
- **Anti-Field Suppression**: If `AntiFieldTurn > 0`, all elemental bonuses/penalties are neutralized (multiplier $1.00$).

### 3. Canonical Damage & Combat Mechanics (`CalculateDamage`, `execute_action.go`)

- **DQ Physical Damage Formula**:
  $$\text{Base} = \begin{cases} \text{Attack} \times 0.75 & \text{if Direct / Critical} \\ \text{Attack} \times 0.5 - \text{Defense} \times 0.3 & \text{otherwise} \end{cases}$$
  $$\text{Variance} = 0.90 + \text{Float64}() \times 0.30 \quad [0.90, 1.20)$$
  $$\text{Damage} = \text{int}(\text{Base} \times \text{Variance})$$
  $$\text{Minimum Damage Guarantee} = \begin{cases} \text{Intn}(2) + 1 \in \{1, 2\} & \text{if } \text{Damage} < 1 \\ \text{Damage} & \text{otherwise} \end{cases}$$

- **Agility Exceed Check (`_is_exceed_ag`)**:
  - Compares effective agility between actor $m$ and opponent $y$:
    - Standard: $\frac{1}{3}$ chance (`rand(3) == 0`) AND $\text{rand}(m.\text{Agility}) \ge \text{rand}(y.\text{Agility} \times 3)$.
    - Item 147 (伝国の懐刀): $\frac{1}{2}$ chance (`rand(2) == 0`) AND $\text{rand}(m.\text{Agility}) \ge \text{rand}(y.\text{Agility})$.
  - Used symmetrically for:
    1. Critical Strike roll: `isExceedAg(actor, target)`
    2. Evasion roll: `isExceedAg(target, actor)`

- **Multi-Target Damage Decay (`execute_skill.go`, `execute_item.go`)**:
  - When an attack targets all enemies (`TargetScopeAllEnemies`):
    - Damage decays cumulatively by 15% (`decayMult *= 0.85`) across successive targets in line order.
    - If the attacker possesses Diamond Ring (`item-144`), damage decay is completely negated (`decayMult = 1.0`).

### 4. 1v1 Combat Resolution (`Resolve`)
Deterministic 1v1 duel resolution utilizing the canonical damage formula:
- Alternating attacks between `first` and `second`.
- Damage computed via `CalculateDamage(attacker.Attack, defender.Defense, rng, false)`.
- Terminates upon knockout or simultaneous knockout (`OutcomeDraw`).

### Legacy Defeat Synergies

- A cursed revival from `item-260` revives the participant at 30% MaxHP and
  grants +300 Attack, +300 Defense, and +300 Agility, then adds curse
  (`party2/lib/_battle.cgi:75–87`). These are all three stats, not Attack alone.
- Weapon37+armor38 death transfer follows the ordered
  [normal-adventure revival contract](#resources-revival-and-roster-changes),
  including fallen holders and transfers before item-triggered revival.

## Battle Adapter & State Application (`internal/battle`)

The application-layer bridge (`internal/battle`) standardizes combat participant binding and post-battle state application across all feature modules (`adventure`, `boss`, `pvp`, `gvg`, `dungeon`, `challenge`, `party`), eliminating duplicate ad-hoc logic and ensuring deadlock-free transactional persistence.

### 1. Participant & Request Construction
- **Domain Mapping (`BuildParticipantFromData`)**: Constructs `corebattle.Participant` directly from `Character`, `Inventory`, `Equipment`, `Job`, and `CustomSkill`.
- **Passive Abilities Extraction**: Automatically detects and binds pre-death revival abilities:
  - `touki_shield`: 闘気の盾 (`item-161`, UsageCategory 3)
  - `dokuro_amulet`: ドクロのお守り (`item-193`, UsageCategory 3)
  - `cursed_revive`: 転生の呪魂符 (`item-260`, UsageCategory 3)
  - `pharaoh`: Pharaoh job class or ability
- **Active Combat Items (`@どうぐ`)**: Extracts consumable items with `UsageCategoryCombatOnly` (`1`) from inventory and equipped tools (e.g. 祈りの指輪 `item-012` in `SlotAccessory`). Each item includes an `InstanceID` so concrete item instances can be tracked and consumed during battle.
- **Stat Orb Binding (`ExtractStatOrbOptions`)**: Automatically scans inventory for `item-152` through `item-156` (Life, Magic, Power, Defense, Agility Orbs) and `item-157` (Skill Orb), binding them into `progression.ApplyExperienceOptions`.
- **Equipment Stat Calculations (`CalculateEquipmentStats`)**: Computes attack, defense, and agility modifications across weapons (71 weapons), armor (55 armors), and accessories (12 accessories):
  - **Excalibur & Ex Amulet / Awakening Scaling (`_battle.cgi:1736-1744`)**:
    - When wielding Excalibur (`weapon-71`), base weapon attack is normally `int(mat * (0.75 + rand(0.5)))`.
    - If Ex Amulet (`item-158`) or Awakening gems (`item-240` 覚醒の紅玉, `item-241` 覚醒の蒼玉, `item-242` 覚醒の翠玉) are equipped in the accessory slot, weapon attack is boosted to `int(mat * (1.5 + rand(0.5)))`.
    - Level scaling factor (`ite_158`): Lv < 50 applies 0.5x, Lv < 75 applies 0.7x, and Lv >= 75 applies 1.0x.
  - **Dragon God Sword Revisions (`_battle.cgi:1728-1734`)**:
    - Dragon God Sword (`weapon-69`): Attack bonus gains `+ int(min(char.Level, 99) * 1.5)`.
    - Dragon God King Sword (`weapon-70`): Attack bonus gains `+ int(min(char.Level, 99) * 2.0)`.
  - **Awakening Gems Accessory Stats (`_data.cgi:1997-1999`)**:
    - `item-240`: +30 Attack, -20 Defense, -20 Agility.
    - `item-241`: -20 Attack, +30 Defense, -20 Agility.
    - `item-242`: -20 Attack, -20 Defense, +30 Agility.

### 2. Atomic Post-Battle State Application (`ApplyPostBattleResult`)
- **Row-Lock Ordering**: Adheres strictly to the global pessimistic lock hierarchy:
  1. **Rank 2 (Character)**: Multiple participating characters are locked in ascending lexicographical order (`sort.Strings`).
  2. **Rank 3 (Inventory & Equipment)**: Inventories are locked in ascending character order; equipment slots are loaded.
  3. **Rank 5 (Depot)**: When item drops exceed inventory capacity, depots are locked in ascending character order.
  4. **Rank 8 (Monster Book & Collection Completion)**: Upon victory, defeated PvE monsters are recorded into `character_monster_book` and milestone Hall of Fame induction (`comp_mon`) is finalized within the reward settlement transaction boundary before commit.
- **Resource Mutation**: Updates current HP, MP, and fatigue (`Tired`). Fallen members survive with HP = 1. Transient combat status effects (`RemainingStatus`) are preserved and propagated in the response.
- **Consumed Item Persistence**: Items used during combat (`ConsumedItems`) are decremented from character inventory. If an equipped item was consumed or broken (e.g. 祈りの指輪), its equipment slot is automatically unequipped (`equip.Unequip`) simultaneously.
- **Reward Distribution**: Adds gold, calculates experience growth via `progression.ApplyExperienceWithJobFull` (strictly propagating errors to abort the transaction if character progression calculation fails), awards monster crystal drops (`character.Crystal` clamped to 999,999), and routes item drops to designated recipients (`RecipientDrops`, `RecipientCharacterID`, or primary character default) preventing drop duplication in multi-character parties. If inventory is full, overflow drops are routed to character depot (`DepotDeliveries`). If the depot is at capacity (`ErrDepotFull`) or unconfigured, undeposited items are strictly tracked in `LostDrops` without fabricating phantom deliveries.
- **Milestone Counters & Monster Book Settlement**:
  - `MonsterKills` (`kill_m` / 撃退数): Upon victory, surviving characters (`RemainingHP > 0`) evaluate each defeated non-player enemy with `IsStrongEnemy(enemyStrong, playerStrong)` (`_battle.cgi:230-235`). Increments `char.MonsterKills` for each defeated strong enemy.
  - `MaoCount` (`mao_c` / 魔王カウント): Upon completing the Stage EX demon king unseal event (`UnsealDemonKing = true` or clearing 封印の地 / `stage-20`), increments `char.MaoCount` for all participating non-NPC characters (`vs_monster.cgi:218`).
  - `MonsterDefeatRecorder` (Monster Book & Legend Induction): Defeated PvE monsters are recorded into `character_monster_book`. When the defeat triggers the 100% completion milestone, mandatory Hall of Fame induction (`RecordLegend`) is executed within the same ambient transaction. Any persistence failure or induction error rolls back all rewards (HP/MP, EXP, Gold, item drops, depot transfers) atomically, preventing desynchronized partial commits. Optional monster taming is resolved outside the transaction without altering the settlement outcome.
- **Single vs Multi-Character Transactions**: Single-character battles route through `economy.TransactionRunner` (`ExecuteTransaction`). Multi-character party battles route through `TransactionProvider` (`RunInTx`). Ambient transactions propagate to all repositories (`ExecutorFromContext`).

## Boundaries & Invariants

- **Context Isolation**: The Battle engine never queries database persistence or mutates external character state directly. It returns an immutable `Result` containing turn logs and rewards.
- **Consumer Ownership**: Feature modules (`adventure`, `dungeon`, `boss`, `pvp`, `gvg`, `challenge`, `party`) utilize `internal/battle` to map character/monster models to `Participant` and handle post-battle reward persistence atomically via `economy.TransactionRunner` or `TransactionProvider`.
- **Damage floor scope**: The physical base formula has a 1-or-2 damage fallback. Misses, evasion, immunity, and subsequent modifiers can yield zero; this is not a minimum for every attack or skill.
- **Round Ceiling**: The current Go batch resolver has a 100-iteration safety ceiling; normal-adventure explicit commands retain the separate legacy interaction contract.
- **File Size Constraint**: All battle module source files are kept $\le 500$ lines.
