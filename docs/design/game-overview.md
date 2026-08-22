# Game Overview

## Purpose

This document records the behavioral understanding of Party2 that should guide the reconstruction.

It is a specification-oriented document, not a description of the old source-code structure.

## Core player loop

The central loop identified during Phase 0 is approximately:

```text
Create / obtain character
        |
        v
Prepare character
  (job / equipment / items)
        |
        v
Choose an activity
        |
        v
Adventure / battle / other feature
        |
        v
Receive rewards
  (experience / currency / items / records)
        |
        v
Character progression
        |
        v
Unlock or attempt additional content
        |
        +------> social / economic / collection systems
```

A particularly important characteristic is that many activities can be time-based: the player starts an action and a result becomes available later.

## Character development

Important concepts observed in the original game include:

- level and experience;
- stats;
- jobs;
- job history/mastery;
- skills;
- equipment;
- inventory;
- items;
- long-term records and collections.

The reconstruction should preserve the gameplay concepts where they are considered important, but should not reproduce the old data structures.

## Battle

Battle is a major reusable game system.

Potential consumers include:

- quests;
- dungeons;
- challenges;
- player-versus-player activities;
- guild battles;
- special encounters.

The battle system should not know the reason a battle was initiated.

## Jobs and skills

Jobs are more than a character label. They can involve requirements, progression/mastery, stat effects, and skill availability.

The new design should represent job definitions separately from a character's job history/state.

## Items and equipment

The game contains a large item/equipment ecosystem.

The reconstruction should distinguish:

- item definitions;
- concrete item instances;
- ownership;
- equipped state.

This leaves room for future mechanics such as enhancement, randomized properties, trading, durability, and special items.

## Social and competitive systems

Important categories include:

- guilds;
- guild battles;
- player communication;
- rankings;
- player-versus-player content.

These should be implemented as features using shared domain components where appropriate.

## Economy and side systems

The original game contains many systems beyond the core adventure loop, including categories such as:

- shops;
- banking;
- auctions / marketplaces;
- casino games;
- alchemy / crafting;
- farming / cultivation;
- collections;
- events;
- lotteries and other side games.

The exact set is not assumed to be required in the first release.

## Feature expansion as a product characteristic

One of the key conclusions of Phase 0 is that the game is characterized not only by its existing feature set, but also by its ability to accumulate new systems over time.

Therefore, the reconstruction should not attempt to finish every historical feature before becoming useful.

Instead, build a small, coherent core and add features incrementally.

## What is intentionally not preserved

The following are not treated as requirements:

- the original source-code structure;
- the original implementation language;
- the original persistence format;
- the original HTML/UI implementation;
- the original image assets;
- accidental implementation constraints.

The target is the game's meaningful behavior and design, not its historical implementation.

## Related documents

- [`../architecture/overview.md`](../architecture/overview.md) — how the game is structured in software.
- [`../architecture/components.md`](../architecture/components.md) — component ownership.
- [`../architecture/feature-modules.md`](../architecture/feature-modules.md) — feature expansion model.
- [`../../AGENTS.md`](../../AGENTS.md) — mandatory development rules.
- [`../../STATUS.md`](../../STATUS.md) — current reconstruction status.
