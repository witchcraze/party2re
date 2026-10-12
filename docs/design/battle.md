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
     - Check `RevivalTriggers`:
       - `pharaoh`: Revives at 100% MaxHP (consumed on use).
       - `undying`: Revives at 50% MaxHP (consumed on use).
       - `touki_shield`: Revives at 30% MaxHP (consumed on use).
       - `dokuro_amulet`: Revives at 25% MaxHP with `dofuu` (動封) applied (consumed on use).
       - `cursed_revive`: Item 260 revives at 30% MaxHP, adds 300 Attack (capped at 999), and applies cursed state; see the detailed rule below and legacy `_battle.cgi:75–85`.
     - If revived: HP set to revival amount, log revival message, participant remains active.
     - If not revived: Mark `Defeated = true`, HP clamped to 0.

3. **Round Conclusion**:
   - Decrement `RemainingTurn` and `AntiFieldTurn` on active `FieldState`.
   - Check termination conditions. If round 30 ends without a victor, outcome is `OutcomeDraw`.

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

The legacy has effects not represented by these current maps. The matrices below
define the normal-adventure coverage gate; serializing today's maps does not
restore the whole game. Untraced callbacks remain explicit production blockers.
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
| Extended effects | With enemies and the enclosing tail eligible (`_battle.cgi:938–1172`), counters differ: frenzy/awakening on operation; magic sword/crown/war-god/flame on positive damage count; bow on qualifying bow action; additional submission on whitelist input. Incense ticks each holder on an ally tail with one party heal (`1055–1128`). NPC purification has its own decrement (`_npc_action.cgi:466–468`) | #1332 owns counters and operation-local action facts. Round-trip immediately before/after every decrement/zero transition; test zero damage and blocked second execution |
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
just its first damage/heal instruction. A gem's shop/synthesis availability is
not certification of its battle effect. Unknown effect kinds must not become
ordinary attacks, and an empty effect list must not spend CMP as a successful
custom skill. This gate applies equally to player, helper and ordinary enemy
actions, including random or delegated invocation. It does not authorize
permanent omission of original choices: the missing behavior remains a cutover
prerequisite. Preserve original order, repeated draws and accepted no-effect
outcomes once the entry is supported.

Native admission has two boundaries. `skill.BattleDefinitions` explicitly maps
individual job/gem IDs to complete native payloads; learning or commerce
definitions cannot create these entries by matching an effect-kind string.
The participant adapter omits a job entry without a validated native definition,
and hydrates occupied custom slots in their original order (including duplicates)
only when every slot passes. Missing definitions or an empty custom action omit
the whole custom choice. The zero-value registry admits no catalog entries;
no production legacy job/gem entry is certified by this admission work alone.
Each future entry needs legacy evidence and whole-effect tests, including all
descendants, before registration. Existing direct batch payloads remain the
responsibility of their producers; primitive validation does not certify their
legacy parity.

The core returns `ErrUnsupportedEffect` for unsupported kinds or incomplete
payloads. It preflights every participant's job/custom/item actions before batch
execution and validates the selected complete action again before MP/CMP/item
mutation. A later unsupported slot therefore cannot leave partial effects.
Consumers must derive discovery and submission from the same server-owned
definitions and propagate rejection, rather than trusting submitted payloads.
The explicit Adventure/HTTP consumers remain #1329/#1330. Random/delegated
invocation has no native executor and is rejected in its entirety, even if some
possible descendants have primitive implementations; no supported-only draw
pool is substituted. A supported operation with no eligible effect target is
still accepted and charged according to its existing resource contract.

Evidence below uses `party2/lib/` at revision
`ff97fc549932a146cc945d7edf71b44c868a840e`. Numeric item/gem/job references are
provenance identifiers, not authorization to reuse distinctive display names.
The complete finite entry reconciliation and named unresolved helper branches
are recorded in [#1333](https://github.com/witchcraze/party2re/issues/1333).

| Dispatch | Enduring behavior / effect boundary | Evidence |
| --- | --- | --- |
| Equipped extensions | Ten entries require the designated held item. Effects include helper-stat combination, agility burst/reset, reflection sharing with party healing, interception, summoning, incense, job-dependent projectile/poison, healing with regeneration, extended combat state, and full revival with curse. Their costs are respectively 30,30,12,5,30,5,2,37,10,0 MP. Admission uses the previous-job loop even though these are equipment entries | `_skill.cgi:124–145,260–283,321–477` |
| Books | 27 callbacks: healing, revival, reflection, proportional defense/agility/attack buffs, cures, damage/status, copying another participant and full party healing. Selected heal/buff/damage base values receive ×1.5 in the library field; this is not a universal book multiplier. Full single revival and full party healing seal the caster's magic outside that field | `_skill.cgi:479–736` |
| Weapon seals | Active seals 7/8/9 require previous-job SP ≥90 and respectively 40/27/40 MP, with all-enemy base damage 180 breath /160 magic /180 magic. These categories are not evidence of elemental fire/water/light. Seal 10 invokes ordinary attack twice; passive seals 11/12 follow their attack hooks | `_data.cgi:2209–2230`; `_skill.cgi:106–112,219–224,260–283` |
| Combat tools | Only usage-category 1 supplies the tool command, including outside an encounter. Dispatch defaults a missing argument to self, then the particular callback interprets it as participant, job, destination or floor. Clear the held tool only when the callback returns without an error; accepted no-effect results can consume it. MP restoration, revival, temporary effects, summoning, random skills, reset, copying, capture and travel are distinct effects | `_skill.cgi:150–151,758–764`; `_data.cgi:619–2083` |
| Custom gems | Run gem slots in order and stop when the actor dies. A slot can change HP/MP/CMP, tension/hit, status/temporary/extended state, roster or appearance, invoke another callback, or enqueue end processing. CMP cost belongs to the custom action; do not substitute each gem's catalog cost as an ordinary MP charge | `_skill.cgi:288–309`; `_data.cgi:2087–2198`; `_battle.cgi:787–789` |
| Random skills | The general random helper draws a job from 1 through the last indexed player job, then redraws extended entries until a non-extended callback is selected; an empty job yields no effect. Tool 84 instead draws **any** current-job entry, including extended entries. Both restore MP only when the callback lowered it, preserving MP increases and other side effects. Neither applies ordinary SP/admission/MP charging for the selected callback | `_skill.cgi:9773–9787`; `_data.cgi:1255–1267,2171` |
| Delegated gems | Gem 86 tries the current job's last extended callback before the previous job. Finding an extended callback returns “handled” even when SP/MP prevents execution, so it does not fall back in that case; job 73 has a separate 1-MP stat increase. Gems 87–92 prefer a calling previous job, otherwise current job, and invoke its named command/first non-command non-extended callback without normal SP or listed MP charging | `_data.cgi:2174–2180`; `_skill.cgi:10841–10918` |
| Two-target book use | Validate both encounter participants, allow the same target twice, require MP ≥floor(book cost×1.5), execute first then second. Actor death after the first stops the second **and** skips the later MP charge/book return. If the actor survives, subtract the internal cost and return the book to storage after the second. Ordinary one-target book invocation does not return it | `_skill.cgi:6868–6897,10538–10557`; native ID authorization remains mandatory |

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

Summoned allies append to the ordered roster with the actor's team, hit 95 and
tension 1; their five stat pairs are independently rolled from supplied bases
using floor(base×(0.9+rand(0.3))), then ×1.2/floor when the summoner holds item
165 (`_add_party:9793–9828`). Caller-specific duplicate checks remain distinct.
Dismissal marks the helper dead and changes its team; member persistence removes
dead NPC/helpers but keeps fallen players (`_battle.cgi:453–493,518–529`). This
means a later command can call again after removal. Do not replace helper death
with permanent character deletion or discard a fallen player who can self-revive.
Enemy summoning uses the current stage's appearance pool, hard/party scaling,
full supplied payload and custom gem cost; it refuses once the member count is
at least ten (`_skill.cgi:9834–9931`). An NPC caster appends it to its own side;
a player caster appends it to opponents. Gem 82 adds an at-most-one-enemy gate.

Transformation has three separate meanings. Between-encounter form switching
changes current job first, otherwise previous job (82↔300, forward 10 MP,
reverse zero). Book 211 copies current/maximum HP/MP/stats, job/SP pairs and
appearance from an eligible participant, floors copied MP at 50, immobilizes
the actor and permanently bans repeat use for that battle state. Tool 214
chooses another participant randomly and copies the same data without those
book-only additions. Tool 168 restores the previous job and its earned SP from
the actor's job history. Gem 100 changes appearance only. None is permission to
overwrite the persistent character's underlying job/stat history indiscriminately
(`_skill.cgi:173–217,700–724`; `_data.cgi:1716–1748,1888–1912,2188–2193`).

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
copies the current party into bosses, stage 25 randomly selects one of three
ordinary rosters, and stage 26 selects its seasonal ordinary roster. Read the
ordinary `@bosses` branch separately from `@ex_bosses`/`@exx_bosses` and `king*.cgi`.
Stage 24's **ordinary floor 10** invokes `createExBossField` despite its name
(`vs_monster.cgi:143–151`): party base maxima are capped at HP/MP 999 and stats
255, doubled for over-level participants, then reset. The default named field
in `add_ex_boss:722` belongs to the later EX branch; do not apply it to ordinary
boss initialization merely because `stage/24.cgi` declares `$ex_field`.

NPC action selection (`_npc_action.cgi:158–322`) builds current/previous-job
lists with independent gates and custom gems, then randomly selects an admitted
entry. It does not reuse the player's preferred-action policy or equipment
extension list. NPC confusion cure is 1/3 versus the player's 1/5; paralysis and
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
ordinary NPC payloads. Named specification gates remain #1341 book/purchase
transactions, #1343 shared modifiers/special fields, #1344 transitive job
callbacks. Travel's dispatch/tail/next-operation rules are owned by
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
  grants +300 Attack, +300 Defense, and +300 Agility for the battle.
- When an ally is defeated without revival, each living ally equipped with
  `item-037` and `item-038` gains half of the fallen ally's Attack. The
  resulting Attack is capped at 999.

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
- **Round Ceiling**: Party battles terminate at a maximum of 30 rounds to prevent infinite loops.
- **File Size Constraint**: All battle module source files are kept $\le 500$ lines.

