# Content Data

## Purpose

Game content that changes independently of algorithms should be represented as
data rather than embedded in application code. This keeps feature rules small
and allows content review without changing control flow.

## Initial format and location

The initial Job catalog uses JSON loaded from
`internal/core/job/data/jobs.json`. The Item catalog is organized by category
in `internal/core/item/data/`:

- `weapons.json` (main-hand weapons)
- `armors.json` (body armors)
- `shields.json` (off-hand shields)
- `accessories.json` (accessories)
- `consumables.json` (consumable items, quest items, and materials)

Go's standard `encoding/json` package is used; no additional data or
configuration dependency is required.

Job entries contain growth and level requirements. Item entries contain:

```json
{
  "id": "weapon-01",
  "name": "ヒノキの棒",
  "price": 10,
  "slot": "main-hand"
}
```

The loaders validate IDs, names, non-negative growth values, non-negative
prices, valid slots, and duplicate IDs before constructing the in-memory
catalog. Invalid or unknown definitions fail explicitly.

## Responsibilities

- JSON: content values and simple declarative requirements.
- Job and Item components: loading, validation, and public definition lookup.
- Progression: applying validated growth values using progression rules.
- Equipment / Inventory: managing instances and equipping items by slot.
- Asset manifest and asset documentation: image paths, dimensions, provenance,
  and licenses.

Content data must not contain executable code, legacy source structure, legacy
image paths, or copied legacy assets. For Version 1 job and item content, reference
names may be retained as behavioral/content labels after individual review;
names with strong association to another work are replaced with generic
equivalents. This review is performed per name rather than by discarding all
reference terminology: ordinary genre terms may remain when they do not carry
a distinctive association, while names with strong associations are replaced
with neutral Japanese names. Dynamic rules are modeled as explicit behavior
with tests rather than encoded as unevaluated expressions in JSON.

Catalog-wide validation is part of the test contract. Every loaded definition
is checked for required fields, valid ranges, unique identifiers, and
resolvable references. Shared rules use table-driven boundary tests, while
definitions selecting a special rule must have a matching special-rule test.
Coverage is reported by CI for later review, but no percentage threshold is
currently enforced.

The current Job and Item models have no special-behavior identifier field or
registry. If such identifiers are introduced, the loader must reject unknown
values and the catalog tests must require one explicit test for every registered
value.
