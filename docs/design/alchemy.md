# Alchemy Crafting & Recipe Synthesis Design

## Overview

The Alchemy system (錬金術 / 錬金堂, NPC @トロデ) provides overnight crafting and item synthesis from 112 data-driven recipes.
Unlike modern instant-crafting or fee-charging MMO systems, authentic Party2 synthesis adheres to the following principles:
- **Zero Gold Fee**: Trode performs synthesis entirely free of charge (`lib/alchemy.cgi`).
- **Depot-Linked Materials**: Ingredients are consumed directly from the player's Depot (`character_depots`, `depot_items`), rather than active inventory.
- **Overnight Maturation & Home Rest**: Synthesis requires time to mature until the next morning (`timer.NextMidnightJST`) or completes instantly when resting at home (`internal/home/sleep.go` `Wake`).
- **Depot-Direct Delivery**: Completed synthesized items are deposited directly into the player's Depot upon claim.
- **Recipe Compendium**: Players discover and learn recipes (via recipe books or base materials), tracking learned and crafted statuses across all 112 recipes.
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
- 112 canonical recipes matching legacy `party2/lib/_alchemy_recipe.cgi`.
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
- Uncrafted recipes display result item name as `"？？？"` in the compendium.
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
- Returns the current synthesis status, active recipe, maturity timestamp, and the complete Recipe Compendium.
- If `now >= matures_at`, the state is reported as `completed`.

### 3. Home Rest Acceleration (`internal/home/sleep.go`)
- When a character rests and wakes up at home (`Wake`), `AlchemyCompleter.CompleteOngoingSynthesis` is invoked.
- Any `ongoing` synthesis state is transitioned to `completed` immediately (`matures_at = now`).

### 4. Claim (`POST /characters/{id}/alchemy/claim`)
1. **Lock Hierarchy**: Rank 2 `characters` -> Rank 5 `character_depots` -> Rank 8 `character_alchemy`.
2. **Preconditions**:
   - Synthesis must be in `completed` state (`matures_at <= now` or accelerated by home rest).
3. **Execution**:
   - Create item instance (`coreitem.NewInstance(recipe.ResultItemDefinitionID, recipe.ResultQuantity)`).
   - Add item instance directly into player's Depot (`depot.AddItem`).
   - Save updated Depot.
   - Mark recipe as crafted in `character_alchemy_recipes`.
   - Increment `total_crafts`.
   - If total unique crafted recipes equals 112 (100%) and `comp_alc` not yet awarded, set `comp_alc = true` and record title.
   - Reset synthesis state to `state = none`.
   - Trigger optional `SynthesisHook` (e.g. for Commemorative Medal milestone tracking).

### 5. Learn Recipe (`POST /characters/{id}/alchemy/learn`)
- Discovers a new unlearned recipe matching `allowed_bases` filter (or any recipe if empty).
- Inserts entry into `character_alchemy_recipes` with `is_crafted = false`.

---

## Persistence Schema

### `character_alchemy`
```sql
CREATE TABLE IF NOT EXISTS character_alchemy (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    recipe_id VARCHAR(64) NOT NULL DEFAULT '',
    state VARCHAR(16) NOT NULL DEFAULT 'none',
    started_at TIMESTAMP NULL DEFAULT NULL,
    matures_at TIMESTAMP NULL DEFAULT NULL,
    total_crafts INT NOT NULL DEFAULT 0,
    comp_alc BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_alchemy_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);
```

### `character_alchemy_recipes`
```sql
CREATE TABLE IF NOT EXISTS character_alchemy_recipes (
    character_id CHAR(32) NOT NULL,
    recipe_id VARCHAR(64) NOT NULL,
    is_crafted BOOLEAN NOT NULL DEFAULT FALSE,
    discovered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    crafted_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (character_id, recipe_id),
    CONSTRAINT fk_character_alchemy_recipes_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);
```
