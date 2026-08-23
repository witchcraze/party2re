# Alchemy Crafting & Recipe Synthesis Design

## Overview

The Alchemy system (錬金術) allows characters to combine specific ingredients according to catalog recipes to craft superior items, consumables, weapons, and accessories.

## Domain Model

### Recipe Model
A recipe consists of:
- `ID`: Unique identifier (e.g. `recipe-001`)
- `Name`: Human-readable synthesis name (e.g. `上薬草の合成`)
- `ResultItemDefinitionID`: Target item definition ID in the Item Catalog
- `ResultQuantity`: Output quantity produced per craft (typically 1)
- `Ingredients`: List of required ingredient definition IDs and quantities
- `GoldFee`: Gold currency fee required for crafting

### Recipe Catalog
- Loaded from data-driven definitions (`internal/alchemy/data/recipes.json`).
- All ingredient item definition IDs and result item definition IDs must map 1:1 with entries in the global Item Catalog (`internal/core/item`).

## Operations & Invariants

- **Ingredient Availability**: The character's inventory must contain all required ingredients with sufficient quantities (`inv.Quantity(ingredientID) >= ingredientQuantity`).
- **Funds Availability**: The character must possess sufficient gold (`char.Money >= recipe.GoldFee`).
- **Consumption & Output**: Upon crafting, ingredients are consumed from active inventory, the gold fee is deducted from the character, and the synthesized item instance is added to the character's inventory.
- **Atomicity**: The transaction persists character money, consumed ingredients, and created item instance inside a single atomic database transaction (`*sql.Tx`).
