# Alchemy Crafting & Recipe Synthesis Design

## Overview

The Alchemy system (錬金術 / 錬金堂, NPC @トロデ) provides overnight crafting and item synthesis from 114 data-driven recipes.
Unlike modern instant-crafting or fee-charging MMO systems, authentic Party2 synthesis adheres to the following principles:
- **Zero Gold Fee**: Trode performs synthesis entirely free of charge (`lib/alchemy.cgi`).
- **Depot-Linked Materials**: Ingredients are consumed directly from the player's Depot (`character_depots`, `depot_items`), rather than active inventory.
- **Overnight Maturation & Home Rest**: Synthesis requires time to mature until the next morning (`timer.NextMidnightJST`) or completes instantly when resting at home (`internal/home/sleep.go` `Wake`).
- **Depot-Direct Delivery**: Completed synthesized items are deposited directly into the player's Depot upon claim.
- **Recipe Compendium**: Players discover and learn recipes (via recipe books or base materials), tracking learned and crafted statuses across all 114 recipes.
- **Title Award**: Achieving 100% compendium crafted completion awards the `comp_alc` title (`$title .= "comp_alc,"`).

---

## Domain Model

### Recipe Model
A recipe consists of:
- `ID`: Unique recipe identifier (e.g. `recipe-001`)
- `Name`: Human-readable synthesis name (e.g. `上やくそう`)
- `ResultItemDefinitionID`: Target item definition ID in the Item Catalog
- `ResultQuantity`: Output quantity produced per craft (typically 1)
- `Ingredients`: List of required ingredient definition IDs and quantities

### Recipe Catalog
- Loaded from data-driven definitions (`internal/alchemy/data/recipes.json`).
- 114 canonical recipes matching legacy `party2/lib/_alchemy_recipe.cgi` (including `魔獣の皮` + `幸せの種` -> `福袋`, and `祈りの指輪` + `金塊` -> `金の指輪`).
- All ingredient item definition IDs and result item definition IDs map 1:1 with entries in the global Item Catalog (`internal/core/item`).
- Zero gold fee across all recipes.

### Synthesis State
- `CharacterID`: Character identifier
- `RecipeID`: ID of the recipe currently being crafted (empty if none)
- `State`: `none`, `ongoing`, or `completed`
- `StartedAt`: Timestamp when synthesis commenced
- `MaturesAt`: Next midnight JST timestamp (`timer.NextMidnightJST`)
- `TotalCrafts`: Total number of successful synthesis claims
- `CompAlc`: Boolean indicating whether the 100% compendium title has been awarded

### Recipe Compendium
- `character_alchemy_recipes` tracks each discovered/learned recipe for a character.
- `IsCrafted`: Flag set to `true` once the character has successfully claimed a synthesis of that recipe.
- **Observation projection** (legacy `alchemy.cgi` `＠れしぴ`/`＠れんきん`, `home.cgi` recipe-book use): only learned combinations are published. Unlearned combinations are absent from compendium entries; `total_recipes`, `learned_count`, `crafted_count` and `completion_percentage` still count the whole catalog. Until a recipe has been claimed once, its product is unknown: the entry and the synthesis-start response carry only `recipe_id` and `ingredients`, with the recipe `name` (which names the product), result item and quantity omitted. Claiming publishes them. Hiding is done by omitting data in the projection, never by substituting display text.
- Learning recipes: Players learn unlearned recipes matching allowed base materials (`LearnRecipe`).

---

## Operations & Invariants

### 1. Synthesize (`POST /characters/{id}/alchemy/synthesize`)
1. **Lock Hierarchy**: Rank 2 `characters` -> Rank 5 `character_depots` -> Rank 8 `character_alchemy`.
2. **Preconditions**:
   - No ongoing synthesis (`state == none` or existing synthesis claimed).
   - Recipe must be learned / discovered in `character_alchemy_recipes`.
   - Depot must contain all required ingredients with sufficient quantities (`depot.Quantity(ing.DefinitionID) >= ing.Quantity`).
3. **Execution**:
   - Consume required ingredients directly from Depot storage (`depot.Consume`).
   - Save updated Depot.
   - Record synthesis state with `state = ongoing`, `started_at = now`, `matures_at = NextMidnightJST(now)`.

### 2. Status & Maturity (`GET /characters/{id}/alchemy`)
- Returns the current synthesis status, active recipe, maturity timestamp, and the Recipe Compendium observation projection (learned combinations only).
- If `now >= matures_at`, the state is reported as `completed`.

### 3. Home Rest Acceleration (`internal/home/sleep.go`)
- When a character rests and wakes up at home (`Wake`), `AlchemyCompleter.CompleteOngoingSynthesis` is invoked.
- Any `ongoing` synthesis state is transitioned to `completed` immediately (`matures_at = now`).

### 4. Claim (`POST /characters/{id}/alchemy/claim`)
1. **Lock Hierarchy**: Rank 2 `characters` -> Rank 5 `character_depots` -> Rank 8 `character_alchemy`.
2. **Preconditions**:
   - Synthesis must be in `completed` state (`matures_at <= now` or accelerated by home rest).
3. **Execution & Ambient Transaction Boundary**:
   - Runs within a single ambient database transaction (`RunInTx`):
     - Create item instance (`coreitem.NewInstance(recipe.ResultItemDefinitionID, recipe.ResultQuantity)`).
     - Add item instance directly into player's Depot (`depot.AddItem`).
     - Save updated Depot (`depots.Save`).
     - Mark recipe as crafted in `character_alchemy_recipes`.
     - Increment `total_crafts`.
     - If total unique crafted recipes equals 114 (100%) and `comp_alc` not yet awarded:
       - Set `comp_alc = true` in `character_alchemy`.
       - Record Hall of Fame induction via `LegendInductor.RecordLegend(txCtx, "comp_alc", characterID)`.
     - Reset synthesis state to `state = none`.
     - Invoke `SynthesisHook(txCtx, characterID, recipeID)` (e.g. Commemorative Medal progress tracking).
4. **Failure Propagation & Rollback Guarantees**:
   - If any step—including `LegendInductor` or `SynthesisHook`—returns an error, the error is propagated out of `Claim` and the entire transaction rolls back.
   - Rollback ensures depot item addition, crafted marks, `comp_alc` title flag, synthesis state clearing, and achievement progress never commit partially.
   - The synthesis remains in `completed` state, allowing safe, idempotent retry without lost progress or duplicate items.

### 5. Learn Recipe & Recipe Scroll Books (`internal/home/recipe_usage.go`, `internal/alchemy/compendium.go`)
- **Discovery Pool Membership**:
  - `基本錬金レシピ` (Item 127, 500G): 34 base items represented by stable definition IDs (`basicRecipePool`), covering exactly 52 learnable recipe combinations.
  - `応用錬金レシピ` (Item 128, 2000G): 30 base items represented by stable definition IDs (`advancedRecipePool`, 29 base items matching 33 recipe combinations; legacy `怒りのタトゥー` defines no synthesis recipe where it acts as base). Covers exactly 33 learnable recipe combinations.
  - `神の錬金レシピ` (Item 129, 7000G): No base filter (`allowedBases = nil`), covers all 114 recipes across the entire catalog (including the 29 God-only recipes marked `#` in legacy CGI that are omitted from basic and advanced scroll pools).
- **Two-Stage Random Selection Contract (`Service.LearnRecipe`)**:
  - Parity with legacy `party2/lib/home.cgi:524-554`:
    1. **Stage 1 (Base Selection)**: Group all unlearned candidate recipes matching the allowed base pool by their base item (`r.Ingredients[0].DefinitionID`). Select an eligible base uniformly at random from all bases that have at least one unlearned recipe.
    2. **Stage 2 (Material Selection)**: Select an unlearned recipe uniformly at random from the candidate combinations for that selected base.
  - **Parity Guarantee**: Ensures equal discovery probability across eligible base items regardless of the number of combinations each base has (e.g. `金の指輪` with 7 combinations has equal base-level odds as single-combination bases).
  - **Learned Exclusion**: Once all recipes for a base item are discovered, that base is eliminated from Stage 1 selection.
  - **Pool Exhaustion**: When all recipes matching the scroll's pool are discovered, returns `ErrNoRecipesToLearn`. In Home item consumption, 1 scroll item is consumed and the message `"この錬金レシピからこれ以上習得できる錬金方法はないようだ…"` is returned.
- **Clean-Room Display Guarantee**:
  - Recipe learning notifications via `alchemyRecipeLearnerAdapter` in `cmd/party2/wire.go` resolve ingredient definition IDs to their clean-room item definition names (`def.Name`). Renamed assets (e.g. `ヒノキの棒`, `神薙の剣`, `霊樹の葉`, `不死殺しの剣`, `不死者の鎧`) never leak legacy proprietary or distinctive names into product messages.
- **No Direct HTTP Discovery**: Recipe discovery is exclusively driven by recipe scroll item consumption via Home item usage; no direct HTTP endpoint exists for recipe learning.

---

## Persistence Schema

### `character_alchemy`
SQL definitions are owned by [migrations](../../migrations/).

### `character_alchemy_recipes`
SQL definitions are owned by [migrations](../../migrations/).
