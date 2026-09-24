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
Replicating the original Party2 CGI combat engine (`_battle.cgi`, `_skill.cgi`):

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
     - Evaluated immediately after actor acts (if actor remains alive):
       - `deadly_poison` (猛毒/劇毒): deals 10% MaxHP damage (capped at 950–1049 if >999). Natural cure cannot occur.
       - `poison` (毒): deals 10% MaxHP damage (capped at 950–1049 if >999). If alive, rolls 20% natural cure chance (`rand(5) < 1`); clears status on success.
   - **Defeat & Revival Check (`defeat.go`)**:
     - When any participant's HP drops to $\le 0$:
     - Check `RevivalTriggers`:
       - `pharaoh`: Revives at 100% MaxHP (consumed on use).
       - `undying`: Revives at 50% MaxHP (consumed on use).
       - `touki_shield`: Revives at 30% MaxHP (consumed on use).
       - `dokuro_amulet`: Revives at 25% MaxHP with `dofuu` (動封) applied (consumed on use).
       - `cursed_revive`: Revives at 1 HP (consumed on use).
     - If revived: HP set to revival amount, log revival message, participant remains active.
     - If not revived: Mark `Defeated = true`, HP clamped to 0.

3. **Round Conclusion**:
   - Decrement `RemainingTurn` and `AntiFieldTurn` on active `FieldState`.
   - Check termination conditions. If round 30 ends without a victor, outcome is `OutcomeDraw`.

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
- **Resource Mutation**: Updates current HP, MP, and fatigue (`Tired`). Fallen members survive with HP = 1. Transient combat status effects (`RemainingStatus`) are preserved and propagated in the response.
- **Consumed Item Persistence**: Items used during combat (`ConsumedItems`) are decremented from character inventory. If an equipped item was consumed or broken (e.g. 祈りの指輪), its equipment slot is automatically unequipped (`equip.Unequip`) simultaneously.
- **Reward Distribution**: Adds gold, calculates experience growth via `progression.ApplyExperienceWithJobFull` (strictly propagating errors to abort the transaction if character progression calculation fails), awards monster crystal drops (`character.Crystal` clamped to 999,999), and routes item drops to designated recipients (`RecipientDrops`, `RecipientCharacterID`, or primary character default) preventing drop duplication in multi-character parties. If inventory is full, overflow drops are routed to character depot (`DepotDeliveries`). If the depot is at capacity (`ErrDepotFull`) or unconfigured, undeposited items are strictly tracked in `LostDrops` without fabricating phantom deliveries.
- **Milestone Counters**:
  - `MonsterKills` (`kill_m` / 撃退数): Upon victory, surviving characters (`RemainingHP > 0`) evaluate each defeated non-player enemy with `IsStrongEnemy(enemyStrong, playerStrong)` (`_battle.cgi:230-235`). Increments `char.MonsterKills` for each defeated strong enemy.
  - `MaoCount` (`mao_c` / 魔王カウント): Upon completing the Stage EX demon king unseal event (`UnsealDemonKing = true` or clearing 封印の地 / `stage-20`), increments `char.MaoCount` for all participating non-NPC characters (`vs_monster.cgi:218`).
- **Single vs Multi-Character Transactions**: Single-character battles route through `economy.TransactionRunner` (`ExecuteTransaction`). Multi-character party battles route through `TransactionProvider` (`RunInTx`).

## Boundaries & Invariants

- **Context Isolation**: The Battle engine never queries database persistence or mutates external character state directly. It returns an immutable `Result` containing turn logs and rewards.
- **Consumer Ownership**: Feature modules (`adventure`, `dungeon`, `boss`, `pvp`, `gvg`, `challenge`, `party`) utilize `internal/battle` to map character/monster models to `Participant` and handle post-battle reward persistence atomically via `economy.TransactionRunner` or `TransactionProvider`.
- **Minimum Damage Guarantee**: Every offensive attack or damaging skill inflicts at least 1 damage.
- **Round Ceiling**: Party battles terminate at a maximum of 30 rounds to prevent infinite loops.
- **File Size Constraint**: All battle module source files are kept $\le 500$ lines.

