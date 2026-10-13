# Required Images

> **Temporary Version 1 document**
>
> This file is a work queue for images identified as necessary during the
> Version 1 reconstruction process.
>
> Images should not block implementation. When an implementation requires an
> image that does not yet exist, record the requirement here and continue
> implementation as far as practical.
>
> After Version 1, this workflow may be replaced by a permanent asset
> management process.

## Status

- `Required`: identified but not yet produced
- `In Progress`: currently being produced
- `Placeholder available`: a temporary file exists; final artwork and client integration are still pending
- `Completed`: final artwork produced, licensed, and integrated
- `Rejected`: no longer required

## Required Images

### IMG-001: Player Character — Idle

- Status: Placeholder available
- Type: Character
- File: `assets/images/placeholders/character/character-default.svg`
- Placeholder: `character-default`
- Used by: Character component
- Purpose: Main player character displayed in the default/idle state
- Description:
  - A single player character standing in an idle pose.
  - Transparent background.
  - The character should remain recognizable at the typical display size.
- Size: 128 × 128 px
- Format: SVG
- Transparency: None
- Notes:
  - No image from the original implementation was reused.
  - Use `character-default` until the final image is available.
  - The default Character guideline is based on the observed original Party2
    character icon size, but the final asset may use an explicit override when
    the new implementation has a concrete requirement.

### IMG-002: Battle — Generic Encounter

- Status: Placeholder available
- Type: Battle
- File: `assets/images/placeholders/battle/battle-default.svg`
- Placeholder: `battle-default`
- Used by: Battle and Adventure presentation
- Purpose: Temporary illustration for a battle encounter.
- Size: 256 × 128 px
- Format: SVG
- Transparency: None
- Visual brief: Two neutral geometric combatants facing one another.
- Provenance: Created in this repository for development use; no third-party
  source or legacy asset was used.
- License status: Project-created placeholder; not a final asset.

### IMG-003: Adventure — Generic Route

- Status: Placeholder available
- Type: Adventure
- File: `assets/images/placeholders/adventure/adventure-default.svg`
- Placeholder: `adventure-default`
- Used by: Adventure presentation
- Purpose: Temporary illustration for an adventure route (adventures resolve immediately).
- Size: 256 × 128 px
- Format: SVG
- Transparency: None
- Visual brief: Neutral mountain route beneath a moon.
- Provenance: Created in this repository for development use; no third-party
  source or legacy asset was used.
- License status: Project-created placeholder; not a final asset.

### IMG-004: Job — Generic Emblem

- Status: Placeholder available
- Type: Job
- File: `assets/images/placeholders/job/job-default.svg`
- Placeholder: `job-default`
- Used by: Job presentation
- Purpose: Temporary icon shared by jobs until individual artwork exists.
- Size: 96 × 96 px
- Format: SVG
- Transparency: None
- Visual brief: Neutral star emblem with no character or franchise reference.
- Provenance: Created in this repository for development use; no third-party
  source or legacy asset was used.
- License status: Project-created placeholder; not a final asset.

### IMG-005: Battle — Temporary Transformation Appearances

- Status: Placeholder available; **artwork will be replaced** during future image implementation.
- Type: Character / Monster
- Files: `assets/images/placeholders/battle/forms.svg` (128×128 sheet, four64×64 views)
  and `assets/images/placeholders/battle/monster-default.svg` (64×64).
- Used by: Explicit Battle transformation appearance references; client integration remains pending.
- Purpose: Distinguish mechanical/winged forms by requesting-player gender and
  provide a missing-monster appearance fallback. These shapes and colors are
  development aids, not approved final character designs.
- Format: SVG; transparent backgrounds.
- Provenance: Newly authored geometric drawings in this repository; no legacy
  artwork, third-party source or generated bitmap was reused.
- License status: Project-created placeholders, not final assets.
- Replacement contract: Saved `Icon` values are appearance identity references,
  not resolved display URLs. Preserve the `mechanical-m`, `mechanical-f`,
  `winged-m`, `winged-f` fragments and existing references, or provide resolver
  aliases when replacing the art. Changing an illustration must not change
  job79's appearance-dependent callback or invalidate saved runs.
- Follow-up: The image catalog/manifest/resolver (#140, #654, #729) will replace
  this art and supply gem100's108 candidate positions. Missing positions use the
  fallback without changing the uniform draw or rerolling persisted outcomes.

## Integration and final assets

These files are original development placeholders, not completed production
assets. A public client and production manifest/resolver are pending #140,
#654, and #729. The character transparency brief describes the final target;
its present placeholder has an opaque background.

## First asset batch

IMG-001 through IMG-004 form the first implementation batch. Final artwork for
individual jobs, characters, monsters, stages, maps, effects, and UI elements
is tracked separately and must receive its own provenance and license record.
