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
- `Completed`: produced and integrated
- `Rejected`: no longer required

## Required Images

### IMG-001: Player Character — Idle

- Status: Completed
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

- Status: Completed
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

- Status: Completed
- Type: Adventure
- File: `assets/images/placeholders/adventure/adventure-default.svg`
- Placeholder: `adventure-default`
- Used by: Adventure presentation
- Purpose: Temporary illustration for a delayed adventure.
- Size: 256 × 128 px
- Format: SVG
- Transparency: None
- Visual brief: Neutral mountain route beneath a moon.
- Provenance: Created in this repository for development use; no third-party
  source or legacy asset was used.
- License status: Project-created placeholder; not a final asset.

### IMG-004: Job — Generic Emblem

- Status: Completed
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

## First asset batch

IMG-001 through IMG-004 form the first implementation batch. Final artwork for
individual jobs, characters, monsters, stages, maps, effects, and UI elements
is tracked separately and must receive its own provenance and license record.
