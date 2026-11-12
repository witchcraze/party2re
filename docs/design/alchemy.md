# Alchemy Design

## Overview

Alchemy (`lib/alchemy.cgi`) synthesizes items overnight from 114 recipes (`_alchemy_recipe.cgi`). Recipes, ingredients and transport are owned by code and [OpenAPI](../api/openapi.json).

- Synthesis is free and consumes its ingredients from the **Depot**, not the inventory.
- A synthesis completes only through sleeping at home (`lib/home.cgi:191`); elapsed time alone never completes it. **Difference:** legacy sets the flag when the character goes to bed; Go sets it at Wake, because a sleeping character cannot reach the facility.
- **Decision:** `comp_alc`, its hall-of-fame induction and completion news are awarded once, on the first recipe-list view (`alchemy_recipes`, legacy `れしぴ`) after every recipe is crafted, not when the last one is delivered.

### 4. Delivery on Entry (`scene_enter` with `destination:alchemy`)

Entering the facility delivers a completed synthesis to the Depot (`lib/alchemy.cgi:17,183–202`); there is no claim action. The product is delivered past capacity, like legacy `send_item` ([delivery capacity](depot.md#delivery-capacity-by-caller)). Delivery, the crafted mark and progress hooks commit together; on failure the synthesis stays completed, so re-entry retries without duplicates.

## Recipe compendium

**Decision:** the compendium publishes only learned combinations (legacy `＠れしぴ`/`＠れんきん` and `home.cgi` recipe books); totals still count the whole catalog. Until a recipe has been crafted once its product stays unknown, so its name, result item and quantity are omitted rather than replaced with placeholder text.

Recipe books (`home.cgi:524–554`) choose in two stages: first a base item uniformly among bases that still have an unlearned recipe, then one of that base's unlearned recipes. **Pitfall:** each base therefore has equal odds regardless of how many combinations it has. The basic book covers 52 combinations over 34 bases; the advanced book's 30 bases cover 33 combinations, because `怒りのタトゥー` has no recipe as a base; the god book covers all 114, including the 29 god-only recipes. An exhausted book is still consumed. Learning messages use the clean-room item names.

## Gateway and Observation

Observation never claims or writes. Known differences from `lib/alchemy.cgi`:

- Automatic `finish` on load (17, 183–202) happens at `scene_enter`; GET never claims, so a saved selection delivers on the next entry.
- `れんきん` (99–144) takes an explicit recipe ID instead of base and material names.
- Writing the commands' facility log rows is open (#1941), and presence is not reproduced (#1943).
