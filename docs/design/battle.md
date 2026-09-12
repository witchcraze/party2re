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
- `Allies` & `Enemies`: For party combat, ally group (up to 4) and enemy group (1..N).
- `VictoryReward`, `DefeatReward`, `DrawReward`: Rewards containing `Experience`, `Currency`, `ItemDefinitionID`, `ItemQuantity`, and `SmallMedals`.

## Resolution Algorithms & Formulas

### 1. Multi-Turn Party Battle Loop (`ResolvePartyBattle`)
Replicating the original Party2 CGI combat engine (`_battle.cgi`, `_skill.cgi`):

1. **Round Initialization (up to 30 rounds)**:
   - Collect all living combatants from allies and enemies.
   - If all allies are defeated -> outcome is `OutcomeDefeat` (or `OutcomeDraw` if simultaneous wipeout).
   - If all enemies are defeated -> outcome is `OutcomeWin`.
   - Sort active combatants in descending order of `Agility`. In case of tied agility, participant order is preserved.

2. **Turn Execution (per participant)**:
   - Skip if participant has been defeated earlier in the round.
   - **Action Selection Priority**:
     1. If `CustomSkill` is configured and participant has sufficient CMP:
        - Deduct CMP.
        - Log incantation quote: `"<Name> calls: '<Incantation>'!"`.
        - Execute each `GemEffect` sequentially (damage, heal, create field, or create anti-field).
     2. Else if `JobSkill` is configured and participant has sufficient MP:
        - Deduct MP.
        - Apply job skill effects according to `TargetScope` and `Element`.
     3. Else if participant has available `ActionItems`:
        - Use the first available `ActionItem` (validated with `UsageCategoryCombatOnly`).
        - Execute item effect (Heal, Buff, Status, or Attack) and consume item from combatant's active items.
        - Log item action: `"<Actor> は <Item> をつかった！ ..."`.
     4. Else if `Defending`:
        - Execute defend stance.
     5. Else execute **Normal Attack**:
        - Target: First living opponent.
        - Base Damage: $\max(1, \text{Attack}_{\text{attacker}} - \text{Defense}_{\text{defender}})$.
        - Apply Field elemental multiplier if attacker has an elemental affinity.
   - **Defeat & Revival Check (`defeat.go`)**:
     - When any participant's HP drops to $\le 0$:
     - Check `RevivalTriggers`:
       - `pharaoh`: Revives at 100% MaxHP (consumed on use).
       - `undying`: Revives at 50% MaxHP (consumed on use).
       - `touki_shield`: Revives at 30% MaxHP (consumed on use).
       - `dokuro_amulet`: Revives at 25% MaxHP (consumed on use).
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

### 3. Legacy 1v1 Combat Loop (`Resolve`)
Maintained with 100% backward compatibility:
- Alternating attacks between `first` and `second`.
- Damage: $\max(1, \text{Attack} - \text{Defense})$.
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

### 2. Atomic Post-Battle State Application (`ApplyPostBattleResult`)
- **Row-Lock Ordering**: Adheres strictly to the global pessimistic lock hierarchy:
  1. **Rank 2 (Character)**: Multiple participating characters are locked in ascending lexicographical order (`sort.Strings`).
  2. **Rank 3 (Inventory & Equipment)**: Inventories are locked in ascending character order; equipment slots are loaded.
  3. **Rank 5 (Depot)**: When item drops exceed inventory capacity, depots are locked in ascending character order.
- **Resource Mutation**: Updates current HP, MP, and CMP based on `RemainingHP`, `RemainingMP`, and `RemainingCMP`. Fallen members survive with HP = 1.
- **Consumed Item Persistence**: Items used during combat (`ConsumedItems`) are decremented from character inventory. If an equipped item was consumed or broken (e.g. 祈りの指輪), its equipment slot is automatically unequipped (`equip.Unequip`) simultaneously.
- **Reward Distribution**: Adds gold, calculates experience growth via `progression.ApplyExperienceWithJobFull` (accounting for Stat Orbs and Job definitions), and adds item drops to inventory or routes them to Depot upon inventory overflow.
- **Single vs Multi-Character Transactions**: Single-character battles route through `economy.TransactionRunner` (`ExecuteTransaction`). Multi-character party battles route through `TransactionProvider` (`RunInTx`).

## Boundaries & Invariants

- **Context Isolation**: The Battle engine never queries database persistence or mutates external character state directly. It returns an immutable `Result` containing turn logs and rewards.
- **Consumer Ownership**: Feature modules (`adventure`, `dungeon`, `boss`, `pvp`, `gvg`, `challenge`, `party`) utilize `internal/battle` to map character/monster models to `Participant` and handle post-battle reward persistence atomically via `economy.TransactionRunner` or `TransactionProvider`.
- **Minimum Damage Guarantee**: Every offensive attack or damaging skill inflicts at least 1 damage.
- **Round Ceiling**: Party battles terminate at a maximum of 30 rounds to prevent infinite loops.
- **File Size Constraint**: All battle module source files are kept $\le 500$ lines.

