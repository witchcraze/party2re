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

- Status: Required
- Type: Character
- Placeholder: `character-default`
- Used by: Character component
- Purpose: Main player character displayed in the default/idle state
- Description:
  - A single player character standing in an idle pose.
  - Transparent background.
  - The character should remain recognizable at the typical display size.
- Size override: None
- Notes:
  - This is a sample entry demonstrating the expected format.
  - No image from the original implementation should be reused.
  - Use `character-default` until the final image is available.
  - The default Character guideline is based on the observed original Party2
    character icon size, but the final asset may use an explicit override when
    the new implementation has a concrete requirement.
