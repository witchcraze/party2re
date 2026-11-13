# Domain Design Specifications & SSOT Index

This directory serves as the **Single Source of Truth (SSOT)** for enduring game and domain rules in Party2 reconstruction (`party2re`), per [`.agents/rules/02-documentation-sync.md`](../../.agents/rules/02-documentation-sync.md).

Game behavior's authority is the original Party2 Perl CGI reference codebase (`party2/`). These documents keep the decisions, open gaps, not-yet-implemented rules and non-obvious legacy findings defined by [rule 02's design document content criteria](../../.agents/rules/02-documentation-sync.md#2-documentation-role-and-timing); implemented formulas and parameters live in code and tests. A Go test or merged issue does not establish legacy parity; see [known differences and audit scope](../migration/documentation-audit.md).

---

## Domain Architecture Map

```mermaid
graph TD
    subgraph Core ["Core Domain"]
        PAC["Player & Character"]
        PROG["Progression & Stats"]
        JOB["Jobs & Skills"]
        ITEM["Items & Equipment"]
        BAT["Battle Engine"]
    end

    subgraph Combat ["Combat & Exploration"]
        ADV["Adventure (10-Floor Crawl)"]
        DUN["Dungeon (Grid Maps)"]
        CHAL["Challenge (Survival)"]
        BOSS["Boss (Sealing Demon)"]
        PVP["PvP Colosseum"]
        GVG["GvG Guild Arena"]
        PTY["Party System"]
    end

    subgraph Economy ["Commerce & Economy"]
        DEP["Depot (Storage Hub)"]
        SHOP["Shops (NPC Standard & Secret)"]
        STR["Store (Player Boutiques)"]
        FLEA["Flea Market (P2P Stalls)"]
        AUC["Auction House"]
        BANK["Bank"]
        BLK["Black Market"]
        GEM["Gem Store"]
    end

    subgraph Life ["Life, Crafting & Entertainment"]
        HOME["Home, Resting & Mailbox"]
        TAV["Tavern & Delivery"]
        ALCH["Alchemy"]
        SMTH["Blacksmith"]
        FARM["Monster Ranch"]
        PLANT["Plantation"]
        CAS["Casino"]
        LOT["Lottery & Raffle"]
        GUILD["Guild System"]
    end

    Core --> Combat
    Core --> Economy
    Economy --> Life
```

---

## Canonical Design Document Sitemap

### Client boundary and scheduling
- [`action-preconditions.md`](action-preconditions.md) — Current catalog snapshot, availability gates, and known legacy differences.
- [Scheduling](../architecture/valkey-keyspace.md#scheduled-work-lifecycle) — unfinished work and terminal recovery.
- [`accessory-shop.md`](accessory-shop.md) — Accessory sales and synthesis.
- [`admin.md`](admin.md) — Administrative character inspection and controls.

### 1. Account, Identity & Character Foundation
- [`player-and-character.md`](player-and-character.md) — Account authentication, character identity, registration population limits (999 accounts), wallet (999,999G), vitality, crystal currency, and fatigue range.
- [`character-customization.md`](character-customization.md) — Naming hall, gender changes, title display, and custom font colors (`#RRGGBB`).
- [PAT authentication](../architecture/interfaces.md#personal-access-tokens-and-dual-authentication) — token lifecycle and dual authentication.
- [`player-deletion.md`](player-deletion.md) — Account deletion, asset cleanup, and maintenance lifecycle.
- [Maintenance mode](admin.md#system-maintenance-mode) — admin controls and request gating.
- [`notifications.md`](notifications.md) — Server news articles and character notification inboxes.

### 2. Character Progression, Jobs & Skills
- [`progression.md`](progression.md) — Cumulative experience thresholds ($10 \times \text{Level}^2$), Level 99 baseline, stats, and celestial OverLevel (Lv 150).
- [`jobs-and-skills.md`](jobs-and-skills.md) — Job catalog, Level 20 job changes, job mastery, and skill activation mechanics.
- [`items-and-equipment.md`](items-and-equipment.md) — 5-category item catalog, equipment invariants, and consumption helpers.
- [`custom_skill.md`](custom_skill.md) — Custom skill synthesis using 3-gem formulas and trigger phrase matching.
- [`god.md`](god.md) — Heaven and Underworld wish decisions and legacy pitfalls.

### 3. Combat, Adventure & Dungeon Exploration
- [`battle.md`](battle.md) — Deterministic turn resolver, multi-participant factions, status effects, formulas, and Battle Adapter.
- [`stages-and-monsters.md`](stages-and-monsters.md) — 28-stage adventure catalog and 286 clean-room monsters.
- [`adventure.md`](adventure.md) — Sequential 10-floor dungeon crawl, Floor 11 treasure room, adventure chronicle history, and milestone unlocks.
- [`dungeon.md`](dungeon.md) — 2D grid tile matrix exploration, hazard traps, `@ちず` scouting jobs, and escape portals.
- [`challenge.md`](challenge.md) — Continuous wave survival trial, enemy stat scaling, no inter-round healing, and Hall of Fame.
- [`boss.md`](boss.md) — 6-player king1–10 / 4-player king99 sealing battles, eligibility, Dejon banishment, and victory celebration crowds.
- [`party-system.md`](party-system.md) — 1–4 player co-op adventure lobbies, speed configuration, `need_join` condition gates, and synergy bonuses.
- [`pvp.md`](pvp.md) — Colosseum real-time 2..8 player Bet & Split arena, 9 team colors, and multi-round combat resolution.
- [`gvg.md`](gvg.md) — Guild vs Guild 2..8 player live matches, GP prize pools, target wins, and 7-tier championship medals.
- [`replay.md`](replay.md) — Original adventure scenes, personal archive ownership, public-read boundary, and current turn-log gaps.

### 4. Commerce, Storage & Economy
- [`depot.md`](depot.md) — Central storage hub (up to 500 slots), tiered expansion, direct item delivery, and dual-source consumption.
- [`shops.md`](shops.md) — Standard NPC shops (Weapon, Armor, Item) at 2x price / 50% resale, and Secret Underground Shop (@ヒミツジ, JobLv 7 gate, 3x price, puff-puff).
- [`store.md`](store.md) — Player boutique real-estate (50,000G in towns 1–4, 90-day cycle), 50 wallpapers, 15 furnitures, and depot barter/gold sales.
- [`fleamarket.md`](fleamarket.md) — Player-to-player free market stalls (server-wide 120 listing ceiling, direct depot transfer).
- [`auction.md`](auction.md) — Live P2P trade hall (`@おくる`/`@しらべる`) with uninitialized recipient depot auto-provisioning.
- [`bank.md`](bank.md) — Gold savings deposits and withdrawals with 999,999G wallet clamp.
- [`black-market.md`](black-market.md) — Rare item sacrifice for Rare/U-Rare points and Depot-delivered prizes.
- [`gemstore.md`](gemstore.md) — Dedicated gem box storage, 55+ synthesis formulas, and Depot orb appraisal.
- [`medal.md`](medal.md) — Small Medal depot exchange and character lifetime milestone achievements.
- [`achievements.md`](achievements.md) — Lifetime gameplay milestones, achievement categories, and tracking criteria.

### 5. Living, Towns & Crafting
- [`home.md`](home.md) — Private town estate construction (towns 1–4), companion phrase training, player mailbox (independent deletion), and free sleep recovery (`sleep.cgi`).
- [`town_park.md`](town_park.md) — Public bulletin board messaging, NPC dialogs, and daily fortune divinations (@町娘).
- [`tavern.md`](tavern.md) — Adventurer's Tavern clean-room naming and post-adventure standing delivery orders.
- [`alchemy.md`](alchemy.md) — 114 crafting recipes consuming depot materials, recipe compendium, and overnight home sleep synthesis.
- [`blacksmith.md`](blacksmith.md) — 12 authentic crystal weapon seals, equipment naming, and 3-slot dedicated weapon storage.
- [`monster.md`](monster.md) — Monster Grandpa ranch capacity: Home limit equal to the box, gifts limited by the sender's capacity.
- [`plantation.md`](plantation.md) — Seed cultivation with 6 seeds, 14 fertilizer reagents, midnight JST maturation, and depot harvest delivery.
- [`oracle-shop.md`](oracle-shop.md) — Oracle Shop (@ラクル) job-gated costume rentals, gender icons, private estate wallpaper boutique, and Black Market hint.
- [`chapel.md`](chapel.md) — 5 town church blessings with single-active prayer constraint and daily reset.
- [`altar-of-rebirth.md`](altar-of-rebirth.md) — Orb offering source, Ramia awakening and otherworld travel item wishes.

### 6. Social, Community & Gambling
- [`guild.md`](guild.md) — Guild founding, dynamic GP, custom roles, hex colors, broadcast callouts, and 20-day inactivity auto-disbandment.
- [`exile.md`](exile.md) — Player-run exile petitions (50+ job changes, 10-vote verdicts with 1-hour or 5-day sleep), reproduced with the original's quirks.
- [`eventplaza.md`](eventplaza.md) — Real-time 5-minute concurrency presence tracking, 3x markup traveling merchant bazaar, and victory celebration crowds.
- [`casino.md`](casino.md) — Multi-player room lobby (2..8 players), Indian Poker, High & Low, Doppelganger, solo 3-reel slot machine, and 18 authentic prizes routed to depot.
- [`lottery.md`](lottery.md) — Server-wide 20-ticket Takarakuji lottery with rollover jackpot, and Tavern Fukubiki raffle.
- [`photo-contest.md`](photo-contest.md) — 10-day seasonal cycles, screenshot submissions, community voting, prize delivery, and Hall of Fame.
- [`collection.md`](collection.md) — Illustrated monster encyclopedia and item discovery compendium with auto-record.
- [`ranking.md`](ranking.md) — 16 competitive leaderboards with Valkey snapshot caching and singleflight stampede guard.
- [`rescue-and-helper.md`](rescue-and-helper.md) — Emergency unstuck rescue with cooldown penalty, and 4-category helper quest board.
- [`activities.md`](activities.md) — Delayed background training actions with push-based worker resolution.
- [`game-overview.md`](game-overview.md) — High-level domain overview and feature expansion model.
