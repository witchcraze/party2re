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
names, legacy image paths, or copied legacy assets. Dynamic rules are modeled
as explicit behavior with tests rather than encoded as unevaluated expressions
in JSON.
