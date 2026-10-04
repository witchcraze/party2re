# Image Guidelines

> This document defines general image requirements by asset type.
>
> The initial values below were established after inspecting the original
> Party2 image assets. They are used as practical starting points for the
> reconstruction; they are not requirements to reproduce the original
> implementation's pixel dimensions.
>
> These guidelines may remain useful beyond Version 1, although the asset
> production and management workflow may be redesigned later.

## Asset Types

### Character

Used for small character/player representations.

- Typical size: 20 × 20 px
- Observed legacy upper size: 40 × 40 px
- Aspect ratio: Flexible
- Transparency: Supported

The original Party2 character icon set is predominantly 20 × 20 px, with a
small number of larger variants.

#### Placeholder Types

- `character-default`
  - General-purpose character placeholder.
- `character-silhouette`
  - Used when the character's appearance should not convey specific information.
- `character-unknown`
  - Used when a character exists but its appearance is not yet determined.

Default placeholder: `character-default`

### Monster / Enemy

Used for monster, enemy, and similar entity representations.

- Typical size: 40 × 40 px
- Observed legacy upper size: 140 × 140 px
- Aspect ratio: Flexible
- Transparency: Supported

The original Party2 monster assets have a wide range of sizes. 40 × 40 px is
the dominant size, while larger assets exist for some special entities.

#### Placeholder Types

- `monster-default`
  - General-purpose monster placeholder.
- `monster-silhouette`
  - Used when the appearance should not convey specific information.
- `monster-unknown`
  - Used when the enemy is known but its appearance is not yet determined.

Default placeholder: `monster-default`

### Effect

Used for small visual effects or effect indicators.

- Typical size: 30 × 30 px
- Observed legacy upper size: 50 × 50 px
- Aspect ratio: Flexible
- Transparency: Supported

#### Placeholder Types

- `effect-default`
- `effect-unknown`

Default placeholder: `effect-default`

### Mark / Small Indicator

Used for very small status, category, or decorative indicators.

- Typical size: 13 × 13 px
- Observed legacy upper size: 15 × 14 px
- Aspect ratio: Flexible
- Transparency: Supported

This is an observation of original assets, not an enforced limit for the new UI. Record concrete presentation requirements in the asset work queue.

#### Placeholder Types

- `mark-default`
- `mark-unknown`

Default placeholder: `mark-default`

### Background / Tile

Used for map, stage, field, and other background elements.

- Typical size: 40 × 40 px
- Observed legacy upper size: 180 × 180 px
- Aspect ratio: Flexible

The original Party2 background assets are predominantly 40 × 40 px tiles, with
larger rectangular assets used for some special backgrounds.

#### Placeholder Types

- `background-default`
  - General-purpose background placeholder.
- `background-empty`
  - Neutral empty background.
- `background-unknown`
  - Used when the background content has not yet been determined.

Default placeholder: `background-default`

### UI Graphic

Used for buttons, labels, badges, banners, logos, and other non-gameplay UI
graphics.

- Typical size: 50 × 16 px
- Observed legacy upper size: 468 × 60 px
- Aspect ratio: Flexible

The original project contains many small UI graphics as well as larger title
and banner graphics. Individual assets should use an explicit override when
their intended role is materially different from the typical UI graphic.

#### Placeholder Types

- `ui-default`
- `ui-unknown`

Default placeholder: `ui-default`

## Individual Overrides

Individual assets should normally follow the requirements of their asset type.

An explicit size override may be used when the intended usage requires it.
The reason for the override should be recorded in `required-images.md`.

Do not copy the original asset dimensions merely for compatibility with the
old implementation. If the new design has a concrete reason to use another
size, document the override or revise the asset-type guideline.

## Placeholder Images

When a required image is not yet available, implementation must use the
placeholder associated with its asset type instead of leaving the image
reference unresolved.

Placeholder images are temporary development assets and must not be treated as
final game assets.

Placeholder definitions may be documented before the corresponding files are
created. Actual placeholder files should be added when they become necessary.

Expected placeholder location:

    assets/images/placeholders/<asset-type>/<placeholder-name>.<format>

Existing repository-created placeholders are SVGs; other names listed above
are reserved suggestions, not files that already exist. The inventory and
provenance belong in [required-images.md](required-images.md):

| Type | Existing placeholder | Size |
|---|---|---|
| Character | `character/character-default.svg` | 128 × 128 |
| Battle | `battle/battle-default.svg` | 256 × 128 |
| Adventure | `adventure/adventure-default.svg` | 256 × 128 |
| Job | `job/job-default.svg` | 96 × 96 |

These recorded sizes override the observed legacy icon dimensions for temporary
illustrations. They do not settle final UI display sizes or final artwork.
