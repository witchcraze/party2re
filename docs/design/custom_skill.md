# Custom Skill Gem Synthesis

## Overview

Each character has one original custom skill. The skill is created at home by
selecting up to three gems from the character's gem box, naming the skill, and
optionally assigning an activation phrase. This is not a job-skill loadout:
job mastery, priorities, and four-slot equipment are not part of this feature.

## Rules

- A gem selection contains `gem1`, `gem2`, and `gem3`; empty positions are
  allowed.
- The sum of gem slot costs must be at most 3.
- The sum of gem CMP costs must not exceed the character's maximum MP.
- Selected gems are removed from inventory atomically. Gems from the previous
  configuration are returned before the new selection is consumed.
- The skill name is required, at most 60 Unicode characters, cannot contain
  whitespace or `;<>`, and cannot equal a reserved command name:
  `こうげき`, `ぼうぎょ`, `てんしょん`, `ささやき`, `にげる`, `すくしょ`,
  or `すすむ`.
- The activation phrase is optional, at most 60 Unicode characters, and cannot
  contain `;<>`.

## Persistence and API

`character_custom_skills` stores the name, phrase, computed CMP cost, and three
gem IDs. `POST /characters/{id}/custom-skills` accepts `name`, `comment`, and
`gems` (or `gem1`/`gem2`/`gem3`). `GET` returns the saved `custom_skill`.
Inventory and custom-skill writes share one transaction boundary.

Combat execution of the synthesized skill remains a separate concern.
