# Image Guidelines & Required Images Specification

## 1. Asset Type Guidelines

Practical starting dimensions based on inspected legacy Party2 assets. Pixel dimensions serve as reference baselines, not strict reproduction limits:

| Asset Type | Typical Dimensions | Legacy Upper Bound | Transparency | Default Placeholder | Usage / Notes |
|---|---|---|---|---|---|
| **Character** | 20 × 20 px | 40 × 40 px | Supported | `character-default` | Small player, party, and NPC icons |
| **Monster / Enemy** | 40 × 40 px | 140 × 140 px | Supported | `monster-default` | Enemy combat and monster book sprites |
| **Effect** | 30 × 30 px | 50 × 50 px | Supported | `effect-default` | Battle spell, skill, and impact animations |
| **Mark / Indicator** | 13 × 13 px | 15 × 14 px | Supported | `mark-default` | Status indicators and UI badge icons |
| **Background / Tile** | 40 × 40 px | 180 × 180 px | N/A | `background-default` | Map and stage environmental tiles |
| **UI Graphic** | 50 × 16 px | 468 × 60 px | Flexible | `ui-default` | Buttons, headers, and UI panels |

---

## 2. Placeholder Registry & Active Work Queue

Placeholders reside in `assets/images/placeholders/<asset-type>/<name>.<format>` and serve as temporary development assets until production artwork is integrated (#140, #654, #729).

### IMG-001: Player Character — Idle
- **Status**: Placeholder available (`assets/images/placeholders/character/character-default.svg`)
- **Type**: Character (128 × 128 px, SVG)
- **Used by**: Character component / Player presentation
- **Provenance**: Newly authored SVG; no legacy or third-party assets reused.

### IMG-002: Battle — Generic Encounter
- **Status**: Placeholder available (`assets/images/placeholders/battle/battle-default.svg`)
- **Type**: Battle (256 × 128 px, SVG)
- **Used by**: Battle and Adventure presentation
- **Provenance**: Newly authored neutral geometric combatants; no legacy assets reused.

### IMG-003: Adventure — Generic Route
- **Status**: Placeholder available (`assets/images/placeholders/adventure/adventure-default.svg`)
- **Type**: Adventure (256 × 128 px, SVG)
- **Used by**: Adventure presentation
- **Provenance**: Newly authored neutral mountain route; no legacy assets reused.

### IMG-004: Job — Generic Emblem
- **Status**: Placeholder available (`assets/images/placeholders/job/job-default.svg`)
- **Type**: Job (96 × 96 px, SVG)
- **Used by**: Job presentation
- **Provenance**: Newly authored neutral star emblem; no legacy assets reused.

### IMG-005: Battle — Temporary Transformation Appearances
- **Status**: Placeholder available (`assets/images/placeholders/battle/forms.svg` [128×128 sheet, four 64×64 views] and `assets/images/placeholders/battle/monster-default.svg` [64×64])
- **Type**: Character / Monster
- **Used by**: Battle transformation appearance references
- **Replacement Contract**: Saved `Icon` values are appearance identity references (`mechanical-m`, `mechanical-f`, `winged-m`, `winged-f`), not raw URLs. Artwork changes must preserve fragments or supply resolver aliases to avoid invalidating saved runs.
- **Provenance**: Newly authored geometric drawings; no legacy assets reused.

---

## 3. Production Asset Migration (#140, #654, #729)

Public client presentation and the production asset manifest/resolver will replace temporary SVGs with finalized assets.
