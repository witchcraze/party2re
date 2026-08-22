# Content Data

## Purpose

Game content that changes independently of algorithms should be represented as
data rather than embedded in application code. This keeps feature rules small
and allows content review without changing control flow.

## Initial format and location

The initial Job catalog uses JSON loaded by the Job component from
`internal/core/job/data/jobs.json`. Go's standard `encoding/json` package is
used; no additional data or configuration dependency is required.

Each entry contains:

```json
{
  "id": "job-01",
  "name": "Job 01",
  "hp_growth": 6,
  "mp_growth": 1,
  "attack_growth": 3,
  "defense_growth": 5,
  "agility_growth": 2,
  "min_level": 5
}
```

The loader validates IDs, names, non-negative growth values, minimum levels,
and duplicate IDs before constructing the in-memory catalog. Invalid or
unknown definitions fail explicitly.

## Responsibilities

- JSON: content values and simple declarative requirements.
- Job component: loading, validation, and public definition lookup.
- Progression: applying validated growth values using progression rules.
- Asset manifest and asset documentation: image paths, dimensions, provenance,
  and licenses.

Content data must not contain executable code, legacy source structure, legacy
image paths, or copied legacy assets. For Version 1 job content, reference job
names may be retained as behavioral/content labels after individual review;
names with strong association to another work are replaced with generic
equivalents. This review is performed per name rather than by discarding all
reference terminology: ordinary genre terms may remain when they do not carry
a distinctive association, while names with strong associations are replaced
with neutral Japanese names. Dynamic rules are modeled as explicit behavior
with tests rather than encoded as unevaluated expressions in JSON.
