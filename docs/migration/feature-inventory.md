# Version 1.0 Feature Inventory

> Temporary Version 1 reconstruction document.
>
> This document describes the behavior and content areas reconstructed
> from the `party2-main/` reference codebase. It is not a source-code migration plan. The old
> implementation and its assets remain reference material only.

## Scope Decision

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

## Reference Inventory

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

---

## Feature Groups & Reconstruction Status

All groups below are Version 1.0 reconstruction requirements.

### A. Application Foundation and Account Lifecycle
- [x] Character/player registration & password hashing (#21)
- [x] Login and session authentication lifecycle in Valkey Master (#21, #366, #378)
- [x] Character profile and status display (#87)
- [x] Player-character ownership verification linkage (#131)
- [x] Personal Access Token (API Key) generation and dual authentication (#163)
- [x] Player deletion and maintenance behavior (#134, #190, #367)
- [x] Name changes and profile customization (#198)
- [x] Notifications, news, and player notification inbox (#67)
- [x] Administrator operations (#190)

### B. Character, Progression, Jobs, and Skills
- [x] Level and cumulative experience progression (#10)
- [x] Fundamental stats and initial character bounds (#24)
- [x] Job definitions, data catalog, and job change history (#17, #38, #50)
- [x] Job-based stat growth formulas (#31)
- [x] Job prerequisite tree (_is_need_job), milestone counters (kill_m, kill_p, hero_c, mao_c, cas_c, job_lv), atomic equipped armor consumption for FireFighter, exempt item possession gating for Gambler, CMP tier growth calculation, and Onion Knight SP-based dynamic stat growth (#784, #830)
- [x] Skill definitions, costs, and availability conditions (#18)
- [x] Job mastery, level-20 job changes, item costs, mastered-job memory exchange with OverLevel guard and gender compatibility restrictions, future memory snapshots ("よびおこす"), and 72-job completion title/news notification with "すっぴん" job unlock (#467, #470, #831)
- [x] Job mastery catalog endpoint (職業極め所, job_master.cgi) with 87-job progress and complete percentage calculation (#785)
- [x] Home resting, sleep recovery, and tired reset (#62, #459)
- [x] Custom skill assignment (#69)
- [x] Wishing Well (願いの泉, @女神) SP sacrifice exchange for permanent stat growth (#468)
- [x] Standardize vitality & fatigue state clamping and combat recovery helpers (#684)
- [x] Endgame wishes, stat boosts, and Lv99+ / storage limit breaks (#187)

### C. Items, Equipment, Storage, and Currency
- [x] Item definitions, 5-category data catalog, and instance ownership (#11, #51)
- [x] Weapons, armor, shields, accessories, and consumables (#51)
- [x] Inventory slot management and 5-slot equipment rules (#19)
- [x] Character Item Depot storage for items with dynamic capacity (#58, #558, #559, #678)
- [x] Gold currency wallet & transactions (999,999G cap) (#24)
- [x] Item Shop purchase & 50% resale transactions (#55, #465)
- [x] Bank accounts, gold deposits, and withdrawals with wallet clamp (#71, #476)
- [x] Blacksmith weapon seals (12 authentic seals consuming crystals), equipment naming, and 3-slot weapon storage (#458, #632)
- [x] Gem store, synthesis recipes, and dual-source (inventory & depot) weighted orb appraisals (#72, #317, #464, #560)
- [x] Black Market underground trade and rare point barter (#142, #463)
- [x] Small Medal collection and rare reward exchange (#160, #473)
- [x] Character lifetime milestone achievements and commemorative medals (#70, #358, #473)
- [x] Centralized transactional reward item delivery engine with configurable depot overflow routing (#679)
- [x] Unified dual-source item resolution and consumption across Inventory and Depot with deterministic Rank 3 -> Rank 5 lock ordering (#683)
- [x] Crystal currency mutation encapsulation and AST linter protection (#685)
- [x] Centralized depot FindOrCreate helper and dynamic capacity refresh across item delivery and trade modules (#712, #725)

### D. Adventure, Maps, Stages, and Battle
- [x] Reusable deterministic Battle component & turn resolver (#12, #20, #36, #480, #826)
- [x] Data-driven Stage Catalog (28 stages) and Monster Catalog (286 clean-room monsters) (#56)
- [x] Multi-stage adventure and dungeon exploration: Authentic 10-floor dungeon crawl loop, Floor 11 Treasure Room, depot overflow fallback, and immediate crawl execution (#57, #74, #478, #655)
- [x] Multiplayer party formation, co-op adventures, synergy bonuses, speed configs (3/18/25), need_join condition checks, distributed party adventure locking, and group quests (#188, #478, #653, #656, #709, #793)
- [x] Adventure history logs, stage clear stats, and milestone progression unlocks (#199)
- [x] Push-based background ScheduledAction completion via Valkey Worker (#106, #109, #110)
- [x] Colosseum PvP: Real-time 8-player Bet & Split combat, 9 team colors, multi-round party battle resolution, and distributed room locking (#75, #481, #594, #644, #652, #661)
- [x] Guild versus Guild (GvG) Combat: Live multi-round matches, GP prize pools, target wins, and 7-tier victory medals & championship cups (#77, #482, #594, #644, #652, #675)
- [x] 4-player Party Sealing Boss Battles, Dejon banishment, Hero Count increments, and victory celebration banquets (#73, #479, #657)
- [x] Dungeon Exploration & Continuous Endurance Challenge multi-player runs, map scouting (@ちず), Hall of Fame records, and Valkey Master run buffers (#162, #404, #405, #483, #597, #600, #657)
- [x] Battle replay records and match history viewer (#66, #796)
- [x] Standardized Battle Adapter: Character/Party to Battle Participant Mapping, equipment stat scaling, recipient-targeted item drop routing, and Post-Battle State Application (#496, #593, #596, #599, #605, #643, #663)
- [x] Post-battle milestone counter increments: MonsterKills (kill_m) on defeating strong enemies (&is_strong) and MaoCount (mao_c) on unsealing the demon king (Stage EX / 封印の地) (#823)

### E. Social and Competitive Systems
- [x] Guild creation, membership application/approval workflow, dynamic Guild Points, hex colors, custom titles, broadcast callouts, and inactivity disbandment worker (#76, #490, #591, #633)
- [x] Player communication, park, and public interactions (#78)
- [x] Player private home, mailbox, and letter correspondence (#159, #461, #680)
- [x] Helper and player rescue assistance (#79, #213, #659, #680)
- [x] Rankings (level, job, wealth, pvp victory, helper, medals, casino wins, alchemy syntheses, weekly job changes, authentic monster kills, demon king defeats, and hero achievements; purged fabricated battle victories) and permanent Hall of Fame (legend.cgi) with automatic completion induction hooks across Collection, Job Mastery, and Alchemy (#63, #280, #454, #470, #798, #824, #825)
- [x] Photo Contest, screenshots, seasonal voting, prize settlement error propagation, scheduled settlement action wiring, and Hall of Fame (#186, #660, #801)

### F. Economy and Side Systems
- [x] Alchemy: Free Overnight Depot-linked Synthesis & Compendium (#60, #487)
- [x] Player Auction house and free-market operations (#80, #474)
- [x] Casino mini-games: Multi-Player Room Lobby (Candidate C), Indian Poker, High & Low, Doppelganger, Slot Machine, Prize Exchange, distributed room locking, CasinoWins tracking (cas_c), Gambler job unlock gating, and escape fatigue (#81, #82, #141, #397, #408, #453, #486, #590, #630, #635, #642, #708, #799)
- [x] Lottery: Server-wide 20-cap Takarakuji lottery with pessimistic row locking, and Fukubiki raffle (#83, #484, #485, #631, #800)
- [x] Plantation seed cultivation with 6 seeds, 14 fertilizers, overnight maturation, and Depot delivery (#84, #489, #816)
- [x] Collection and Monster Book encyclopedia with combat defeat recording, canonical thresholds (180/141), and 100% completion news (#85, #797)
- [x] Chapel prayers and blessings (#86, #472)
- [x] Event Plaza, traveling merchant bazaar, and victory celebration banquets linked to Valkey presence (#161, #491, #634)
- [x] Secret Underground Shop and NPC @ヒミツジ (#192, #462)
- [x] Adventurer's Tavern, culinary menu, food delivery standing orders, and post-adventure fullness reset (#185, #475, #595, #634)
- [x] Flea Market player-to-player item stalls and SQL CAS status guard (#194, #398, #477)
- [x] Monster Grandpa & Monster Ranch pet companion storage and stabling (#193, #488, #787)
- [x] Altar of Rebirth: 6-Orb Offering, Ramia Awakening, and Otherworld Travel Item Wishes (#471, #503)
- [x] Player Store & Town Boutiques: store construction, depot listings, and interior styling (#424, #466)
- [x] Oracle Shop: authentic item purchasing (@kau), consumable costume usage with gender parity, home wallpaper boutique, and Black Market discovery hint (#721, #724)

### G. Presentation, Assets, and Operations
- [x] UI-independent HTTP JSON Application API layer (OpenAPI 3.1) (#87, #254, #266, #690)
- [x] HTTP edge security: trusted proxy CIDR allowlist for spoof-proof rate limiting and aligned CORS preflight methods and headers (#691)
- [x] MariaDB durable persistence & migration automation (`001`–`085`) (#124, #399)
- [x] Valkey worker queue with AOF+RDB persistence (#106)
- [x] Structured JSON logging with credential masking (#49)
- [x] Unified local verification script and pre-push hook (#121)
- [x] Minimal production container image published via GHCR (#88, #128)
- [x] Initial SVG placeholder assets (#39)
- [x] Documentation streamlining and SSOT consolidation (#697, #720)
- [x] Fail-fast MariaDB and Valkey startup connectivity validation and clean resource teardown (#728, #693)
- [x] Standardized HTTP response envelopes and error formatting (Envelope Pattern) (#741)
- [x] Presentation markup decoupling, UI-agnostic domain services, and presentation AST static analysis linter (#727)
- [x] ActionURLResolver: ActionID to HTTP endpoint mapping and HATEOAS discovery (#740)
- [ ] Web presentation UI / client implementation (#140)
- [ ] Production asset production and license attribution (#143, #202)

---

## Version 1 Completion Checklist

- [ ] Every feature group has an implementation Issue and acceptance criteria.
- [x] Every Version 1 feature has domain or component tests.
- [x] Durable state is stored in MariaDB where required by the feature.
- [x] Valkey usage is documented per concrete cache, transient-state, queue, or coordination requirement.
- [ ] Required images are listed in `docs/assets/required-images.md`.
- [ ] Every final image has known provenance and an approved license.
- [x] No old source code or old image is included in the new implementation.
- [x] The full application can be built and operated using the documented development workflow.

---

## Related Documents

- [`../../STATUS.md`](../../STATUS.md)
- [`../../ROADMAP.md`](../../ROADMAP.md)
- [`../architecture/components.md`](../architecture/components.md)
- [`../design/game-overview.md`](../design/game-overview.md)
- [`../assets/required-images.md`](../assets/required-images.md)
- [`../../AGENTS.md`](../../AGENTS.md)
