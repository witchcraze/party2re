# Version 1.0 Feature Inventory

> Temporary Version 1 reconstruction document.
>
> This document describes the behavior and content areas to be reconstructed
> from `party2-main.zip`. It is not a source-code migration plan. The old
> implementation and its assets remain reference material only.

## Scope decision

Version 1.0 is complete when the existing project's meaningful game functions
have been newly implemented and the images required by those functions have
been newly produced or replaced with approved placeholders.

"Implemented" means behavior is available through the new application's
public contracts and is covered by appropriate tests. It does not mean that
the old CGI files, Perl structure, file layout, HTML, or persistence format are
copied or mechanically translated.

The old archive README states that the original source and license are
unknown. Therefore, the old source and images must not be incorporated into
the new implementation.

Foundational game logic, standard values, and calculated values may generally
be reused as behavioral requirements when independently reconstructed in the
new implementation. Distinctive asset names that evoke a specific game are
not reused; those names are reviewed individually and normally replaced with
generic terminology.

## Reference inventory

The archive currently contains approximately:

| Area | Observed quantity | Interpretation |
| --- | ---: | --- |
| Top-level CGI endpoints | 27 | Entry points, account, ranking, replay, administration, and other UI operations |
| Feature library CGI files | 74 | Feature and shared behavior candidates |
| Map-related CGI files | 112 | Map content and map-specific rules/data |
| Stage-related CGI files | 39 | Adventure/stage content |
| GIF/PNG/ICO images | 890 | Legacy visual references only |
| All archive files | 1,259 | Includes deployment files, HTML, scripts, data, and assets |

The quantities are inventory indicators, not Version 1 API counts.

## Feature groups

All groups below are Version 1.0 reconstruction requirements. The order is an
implementation order, not a statement that some groups are optional.

### A. Application foundation and account lifecycle

- character/player registration;
- login and session-related behavior;
- character profile and status display;
- player deletion and maintenance behavior;
- name changes and profile customization;
- notifications, news, and replay/history access;
- administrator operations.

Primary reference areas include `new_entry.cgi`, `login.cgi`, `player.cgi`,
`profile.cgi`, `delete.cgi`, `news.cgi`, `replay.cgi`, `admin.cgi`, and the
`profile`, `system`, `home`, and `custom_image` libraries.

### B. Character, progression, jobs, and skills

- level and experience;
- fundamental stats and status limits;
- jobs and job changes;
- job mastery/history;
- skills and custom skills;
- rebirth and related progression;
- sleep, fatigue, action limits, and recovery.

Primary reference areas include `job_change`, `job_master`, `skill`,
`custom_skill`, `reborn`, `sleep`, and the shared battle data definitions.

### C. Items, equipment, storage, and currency

- item definitions and owned item instances;
- weapons, armor, accessories, and consumable items;
- inventory and equipment rules;
- storage/depot;
- gold and other currencies;
- shops and item transactions;
- bank and gem store;
- blacksmith, enhancement, and related item operations.

Primary reference areas include `item`, `weapon`, `armor`, `accessory`,
`depot`, `store`, `goods`, `bank`, `gem_store`, and `blacksmith`.

### D. Adventure, maps, stages, and battle

- ordinary adventure and quest selection;
- stage requirements and rewards;
- map and dungeon progression;
- challenge content;
- monster encounters;
- battle actions, effects, skills, and results;
- battle records and replay data;
- player, guild, king, dungeon, and challenge battle modes.

Battle must be reconstructed as a reusable component. The reason a battle was
started must remain outside the battle implementation.

Primary reference areas include `quest`, `adventure_record`, `_battle`,
`vs_monster`, `vs_player`, `vs_guild`, `vs_king`, `vs_dungeon`,
`vs_challenge`, `stage/`, `map/`, and the battle/skill data files.

### E. Social and competitive systems

- guild creation and membership;
- guild administration and guild state;
- guild battles;
- player communication and public interaction;
- rankings, job rankings, and weekly rankings;
- contests and competitive records;
- rescue/helper behavior.

Primary reference areas include `guild`, `join_guild`, `guild_list.cgi`,
`park`, `ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`, `contest.cgi`,
and `helper`.

### F. Economy and side systems

- auction and free-market operations;
- casino entry and individual casino games;
- lottery and raffle systems;
- alchemy and recipes;
- farming, monster raising, and plantation behavior;
- collection and monster book records;
- medals, photos, events, chapel, gods, and wishes;
- exile and other town activities.

Primary reference areas include `auction`, `free`, `casino`,
`casino_doppel`, `casino_highlow`, `casino_indian`, `casino_slot`, `lot`,
`takarakuzi`, `alchemy`, `_alchemy_recipe`, `farm`, `plantation`,
`collection`, `_add_collection`, `_add_monster_book`, `photo`, `event`,
`medal`, `chapel`, `god`, `u_god`, `sp_change`, and `exile`.

### G. Presentation, assets, and operations

- UI-independent application operations;
- browser presentation and alternative-client boundary;
- character, monster, stage, map, effect, and UI images;
- image upload/custom-image rules where retained;
- logs, scheduled processing, and operational maintenance;
- reproducible local development and deployment.

The old Apache/CGI/Perl deployment is reference material only. The new
runtime will use Go, MariaDB for durable persistence, and Valkey where a
cache, transient state, queue, or coordination requirement is demonstrated.

## Dependency-oriented implementation slices

The following slices keep each change reviewable while preserving the full
Version 1 scope:

```text
A. Go application + MariaDB + Valkey development foundation
   |
   v
B. Player/Character creation and durable persistence
   |
   v
C. Progression, jobs, skills, items, inventory, and equipment
   |
   v
D. Scheduled activities and reusable battle component
   |
   v
E. Adventure, maps, stages, and rewards
   |
   +--> F. Social, guild, rankings, and competitive modes
   |
   +--> G. Economy and side systems
   |
   v
H. Client presentation, complete asset set, and release operations
```

The graph is approximate. A later Issue may split a slice further when its
acceptance criteria become concrete.

## Current implementation Issues

The next small implementation units are tracked as follows:

| Slice | Issue | Scope |
| --- | --- | --- |
| Character initialization | [#24](https://github.com/witchcraze/party2re/issues/24) | Initial identity, base stats, and starting currency |
| Progression | [#10](https://github.com/witchcraze/party2re/issues/10) | Level-up rules and experience thresholds |
| Items / Inventory | [#11](https://github.com/witchcraze/party2re/issues/11) | Item definitions, instances, and ownership |
| Battle | [#12](https://github.com/witchcraze/party2re/issues/12) | Reusable deterministic Battle contract |
| Adventure | [#13](https://github.com/witchcraze/party2re/issues/13) | One delayed Adventure flow using Battle |

Equipment, jobs, skills, maps, social systems, and economy features remain
separate follow-up work and should not be added to these Issues implicitly.

### Initial character values

The reference implementation establishes the following behavioral requirements
for a newly created character:

- level 1 and 0 experience;
- starting currency of 200;
- maximum HP in the inclusive range 30-32;
- maximum MP, current MP, attack, defense, and agility in the inclusive range
  6-8, with current HP and MP initialized to their maximum values.

The new implementation keeps these values as generic domain behavior. Job
identifiers and gender values are treated as data, while distinctive legacy
names and assets are not reused.

## Version 1 completion checklist

- [ ] Every feature group has an implementation Issue and acceptance criteria.
- [ ] Every Version 1 feature has domain or component tests.
- [ ] Durable state is stored in MariaDB where required by the feature.
- [ ] Valkey usage is documented per concrete cache, transient-state, queue, or
      coordination requirement; it is not used as an undiscriminated second
      database.
- [ ] Required images are listed in `docs/assets/required-images.md`.
- [ ] Every final image has known provenance and an approved license.
- [ ] No old source code or old image is included in the new implementation.
- [ ] The full application can be built and operated using the documented
      development workflow.

## Related documents

- [`../../STATUS.md`](../../STATUS.md)
- [`../../ROADMAP.md`](../../ROADMAP.md)
- [`../design/game-overview.md`](../design/game-overview.md)
- [`../assets/required-images.md`](../assets/required-images.md)
- [`../../AGENTS.md`](../../AGENTS.md)
