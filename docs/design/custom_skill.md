# Custom Skill Gem Synthesis

## Overview

Each character has one original custom skill. The skill is created at home by
selecting up to three gems from the character's gem box, naming the skill, and
optionally assigning an activation phrase. This is not a job-skill loadout:
job mastery, priorities, and four-slot equipment are not part of this feature.

## Rules

- A gem selection has three positions; empty positions are allowed.
- The sum of gem slot costs must be at most 3.
- Selected gems are removed from the character's gem box (`character_gem_boxes` / `gem_box_items`)
  atomically. Gems from the previous configuration are returned to the gem box before the new selection
  is consumed.
- Gem swapping and unsetting operate exclusively on gems already owned by the character. Custom
  skill configuration does not enforce deposit capacity limits on the GemBox (preventing lockout when a
  job change resets dynamic capacity below existing gem inventory).
- The skill name is required, at most 60 Unicode characters, cannot contain
  whitespace or `;<>`, and cannot equal a reserved command name:
  `こうげき`, `ぼうぎょ`, `てんしょん`, `ささやき`, `にげる`, `すくしょ`,
  or `すすむ`.
- The activation phrase is optional, at most 60 Unicode characters, and cannot
  contain `;<>`.

## Persistence and API

`character_custom_skills` stores the name, phrase, computed CMP cost, and three
gem IDs, set with `custom_skill_set` in the own-Home custom skill scene
([Home](home.md#observation-visibility-and-navigation)). Gem box storage and custom-skill writes share one transaction boundary.

Combat execution of the synthesized skill remains a separate concern.
