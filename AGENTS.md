# AGENTS.md

## Document hierarchy

Use the documents according to these roles:

- `AGENTS.md` — mandatory rules for agents and development.
- `README.md` — human-facing project introduction.
- `STATUS.md` — current project state; what is true now.
- `ROADMAP.md` — planned work and phase progression.
- `docs/architecture/` — enduring software architecture.
- `docs/design/` — enduring game/domain design.
- `docs/development/` — enduring development procedures.
- `.github/ISSUE_TEMPLATE/` and `.github/PULL_REQUEST_TEMPLATE.md` — mandatory ticket/review formats.

The distinction is important:

```text
AGENTS.md
  rules
    |
    +--> docs/architecture/   how the software is structured
    +--> docs/design/         what the game means
    +--> docs/development/    how work is performed
    |
    +--> STATUS.md            current state
    +--> ROADMAP.md           future work
```

Do not use `STATUS.md` or `ROADMAP.md` as substitutes for permanent architecture/design documentation.
Do not put temporary Version 1.0 reconstruction status into permanent architecture documents.

## Project purpose

This repository is a complete, clean-room reconstruction of the game known as Party2.

The original Party2 implementation is used only as a reference for understanding game behavior, rules, content, and design ideas. **No existing Party2 source code is to be reused.** The implementation language and architecture may differ completely from the original.

The goal is not merely to reproduce an old implementation. The goal is to create an OSS game that preserves the important characteristics of Party2 while providing a maintainable foundation for continued feature development.

One of the central characteristics identified during the initial investigation is that Party2 grew through the addition of many independent game features. The new architecture must therefore make **continued feature expansion a first-class design goal**.

---

## Development principles

### 1. Clean-room reconstruction

- Do not copy, port, translate, or mechanically rewrite existing Party2 source code.
- Do not preserve the old implementation's architecture merely for compatibility.
- Use the existing game only as a source of behavioral and functional requirements.
- Reconstruct the implementation from the desired domain model and requirements.
- Existing assets, including images, are not reused. Images and other visual assets will be recreated as new assets.

The original project's `README.md` is not treated as a source of requirements for this reconstruction.

`Created by Merino` may be acknowledged on the project page as the origin of the game on which this project is based.

### 2. Feature expansion is a primary architectural goal

New features should be possible without unnecessarily modifying the core or unrelated features.

When designing a new feature, ask:

> If we wanted to add another feature of the same kind next month, would this design make that easy?

Prefer:

- isolated feature modules;
- explicit responsibilities;
- stable contracts;
- domain events where they genuinely reduce coupling;
- data-driven content where appropriate.

Avoid:

- growing a central God object;
- feature-specific branches scattered through core code;
- direct access to another feature's internal implementation;
- abstractions created only for hypothetical future requirements.

Do not over-engineer. Extensibility must come from clear boundaries, not from abstraction for its own sake.

### 3. Components are defined independently of implementation language

The initial implementation language is **Go**.

However, the architecture must not depend on Go-specific implementation details at component boundaries. Define a component by:

1. responsibility;
2. inputs;
3. outputs;
4. state;
5. dependencies;
6. externally observable contract.

The implementation language is a property of the implementation, not the component's identity.

A future component may be rewritten in another language when there is a concrete reason to do so. Do not introduce multiple languages merely for architectural purity.

Do not introduce a network protocol, microservice boundary, or serialization layer solely because a component might someday use another language. Start with the simplest Go implementation that preserves the intended boundary.

### 4. Start as a modular monolith

Use a single repository and initially prefer a single application/process where practical.

Logical component boundaries are required, but physical service separation is not.

Do not introduce microservices unless a concrete requirement justifies them.

A future component should be extractable without requiring its internal implementation to be shared with unrelated components.

### 5. Core must remain small

The Core contains only concepts that are genuinely shared across much of the game.

Likely Core concepts include:

- Player
- Character
- Stats
- Progression
- Item definitions and instances
- Inventory
- Equipment
- Currency
- Game time
- Scheduled actions
- Domain events

This list is a starting point, not a permanent schema.

A feature-specific concept belongs in its feature unless there is a strong reason to promote it.

### 6. Features are first-class components

Examples include:

- Adventure
- Guild
- Casino
- Alchemy
- Auction
- Farming
- Collection
- Ranking
- Events
- other future game systems

A feature should own its feature-specific rules and state.

A feature may depend on public contracts of Core or other components, but should not reach into another feature's internal implementation or persistence model.

### 7. Domain components

Some systems are used by many features but are not necessarily part of the minimal Core.

Examples:

- Battle
- Adventure / Quest
- Time-based action processing

Treat these as independent components with explicit contracts.

For example, Battle should not need to know whether a battle was initiated by a quest, arena, guild battle, or another future feature.

---

## Domain model principles

### Character

Separate the account-level `Player` concept from the in-game `Character`.

Character owns character state and invariants, but should not become a God object containing every game system.

Avoid designs where Character directly implements every action such as starting quests, buying items, changing jobs, and managing guilds.

### Items

Separate:

- `ItemDefinition`: what an item is;
- `ItemInstance`: a concrete owned instance;
- `Inventory`: ownership;
- `Equipment`: equipped state.

This supports future systems such as enhancement, randomized properties, durability, trading, and special item behavior.

### Jobs and progression

Separate:

- `JobDefinition`: the definition of a job;
- `CharacterJob`: a character's relationship with a job, including mastery/history.

Job requirements, modifiers, and available skills should be modeled as rules/data rather than as a growing collection of special cases.

### Battle

Battle is an independent component.

At a conceptual level it contains:

- participants;
- state;
- actions;
- effects;
- result.

Battle must not contain feature-specific assumptions such as "this is a guild battle" or "this is a quest battle."

Higher-level features use the Battle component.

### Scheduled actions

Party2 contains an important asynchronous/time-based pattern: an action is started and a result becomes available later.

Model this as a reusable concept such as `ScheduledAction`, with:

- actor;
- action type;
- parameters;
- start time;
- execution time;
- state.

The scheduling mechanism itself must not contain the rules of individual features.

### Domain events

Use domain events when they provide meaningful decoupling.

Examples:

- `BattleFinished`
- `QuestCompleted`
- `CharacterLeveledUp`
- `ItemObtained`
- `JobChanged`
- `GuildJoined`

A publisher should not need to know which optional features consume an event.

Do not turn every operation into an event. Use direct calls when immediate results or strong transactional coupling are appropriate.

---

## Architecture review

Every substantial feature addition should receive both normal implementation review and an architecture-oriented review.

Review at least:

### Feature boundaries

- Is the feature's responsibility clear?
- Does it access another feature's internals?
- Is feature-specific state kept within the feature?

### Core boundaries

- Does this really belong in Core?
- Is feature-specific behavior leaking into Core?
- Is the Core becoming a collection of special cases?

### Extensibility

- Can another feature of the same category be added without large changes?
- Is the new feature unnecessarily coupled to existing features?
- Is a new extension point actually needed?

### Component contracts

- Are inputs and outputs clear?
- Are implementation details leaking across the boundary?
- Could the component eventually be replaced independently?

### Complexity

- Is the design simpler than the problem requires?
- Is abstraction justified by an actual requirement?
- Is "future-proofing" creating unnecessary complexity?

A useful review question is:

> What would have to change if we added a second implementation of this feature?

---

## Licensing

The final software license is not fixed yet.

Candidate licenses:

- MIT
- Apache-2.0
- AGPLv3

The final choice will be made during implementation after the project's actual dependencies and their license requirements have been reviewed.

For images and other creative assets, **Creative Commons (CC) licenses are candidates**. The exact license for each asset or asset category will be determined when the assets are created or incorporated.

When adding a dependency, verify its license before incorporating it. Avoid introducing third-party code or assets whose licensing is incompatible with the eventual project license or whose provenance cannot be established.

Do not reuse Party2's existing images.

---

## Project phase boundary

This project has two kinds of rules and information.

### Permanent project principles

The following principles continue after Version 1.0:

- feature expansion is a primary architectural goal;
- Core remains small;
- Feature Modules own feature-specific behavior and state;
- component boundaries are defined independently of implementation language;
- components communicate through explicit contracts;
- TDD is mandatory for non-trivial behavior;
- development is Issue / PR driven;
- Issue and PR templates are mandatory;
- substantial changes receive architecture review;
- dependencies and licenses are reviewed before incorporation.

These describe **how the project is developed**, not how the current migration is performed.

### Temporary Version 1.0 reconstruction phase

The project is currently rebuilding Party2 toward Version 1.0.

The following rules apply specifically to this phase:

- the existing Party2 implementation is a behavioral/design reference only;
- existing Party2 source code is not reused;
- existing Party2 images/assets are not reused;
- behavior is reconstructed through a clean implementation;
- differences from important reference behavior may be investigated during reconstruction;
- initial implementation uses Go;
- the project starts as a modular monolith.

These are transition requirements. They must not be mistaken for permanent product architecture.

In particular, the project is **not intended to remain a "refactoring project" after Version 1.0**.

The desired transition is:

```text
Reference Party2
      |
      v
Version 1.0 reconstruction
      |
      v
Stable OSS foundation
      |
      v
Normal feature development
```

After Version 1.0, historical implementation details should no longer drive ordinary development decisions.

The current phase and its remaining work are tracked in `STATUS.md` and `ROADMAP.md`.

## UI and API boundary

- Do not put game logic directly in GUI handlers. Major game operations should pass through the UI-independent application API/command boundary so they can be tested and alternative clients can be added later.

## Negative constraints

The following actions are explicitly prohibited unless a higher-level project decision changes the rule.

- Do not copy, port, translate, or mechanically rewrite existing Party2 source code.
- Do not reuse existing Party2 images or other visual assets.
- Do not begin substantial implementation without an Issue.
- Do not create an Issue or PR without using the repository template.
- Do not implement non-trivial behavior without appropriate tests.
- Do not mix unrelated refactoring into a feature or bug-fix ticket.
- Do not put feature-specific logic into Core merely for convenience.
- Do not access another Feature Module's private implementation.
- Do not add a dependency without following the dependency and license review process.
- Do not make a substantial architectural change without documenting the decision through an Issue.
- Do not treat `STATUS.md` or `ROADMAP.md` as substitutes for permanent architecture or design documentation.
- Do not optimize for code volume at the expense of tested, reviewable, mergeable changes.

When a requested action conflicts with one of these constraints, stop and resolve the conflict through the appropriate Issue or project decision rather than silently bypassing the rule.

## Agent operating rules

The following rules apply to every coding agent session.

### Before starting work

1. Read `AGENTS.md`.
2. Read `STATUS.md`.
3. Read `ROADMAP.md`.
4. Inspect the relevant Issue.
5. Read the relevant architecture/design documentation.
6. Inspect only the code and tests necessary for the Issue.

Do not begin substantial implementation from an informal request without an Issue.

### Branch strategy

Keep `main` as the integration branch. Do not implement Issue work directly on
`main`.

Before editing:

1. update a clean local `main` with `git pull --ff-only origin main`;
2. create one new branch for the Issue;
3. use a branch name that includes the Issue number and a short purpose, such
   as `feature/issue-4-character-persistence`;
4. confirm `git status` is clean before making changes.

Commit and push only the Issue's changes to its branch. Open a PR from that
branch to `main`, wait for CI and review, then merge the PR. After merging,
delete the local and remote Issue branch before starting another Issue. If
`main` advances while a PR is open, update the PR branch and rerun validation
before merging.

### Select a bounded task

Work on an existing Issue whenever possible.

If the requested work is too large:

- split it into smaller Issues;
- preserve independently testable acceptance criteria;
- do not silently expand the current Issue.

### TDD

For non-trivial behavior:

1. identify acceptance criteria;
2. write or update tests;
3. implement the smallest change satisfying the tests;
4. run focused tests;
5. run the broader test suite.

Prioritize tests of domain rules, invariants, observable component behavior, important integrations, and regressions.

### Issue and PR workflow

Every substantial change requires an Issue.

Every Issue **must use a repository Issue template**.

Every code change requires a PR.

Every PR **must use the repository PR template**.

Do not bypass templates because a task appears small or straightforward.

If an appropriate template does not exist, create or correct the template before proceeding, unless the task is explicitly an emergency repository recovery.

A PR should reference the Issue and explain:

- behavior changed;
- tests added/changed;
- tests run;
- architectural impact;
- documentation/status impact.

### Architectural changes

Agents may make ordinary local implementation decisions.

Do not silently make substantial architectural decisions.

Create or update an Issue when the work would change:

- Core responsibilities;
- component boundaries;
- dependency direction;
- public component contracts;
- persistence architecture;
- external API architecture;
- implementation-language boundaries;
- licensing strategy.

Document alternatives and trade-offs before adopting a materially different architecture.

### Dependency and license gate

Before adding a dependency:

1. determine why it is needed;
2. check whether the standard library or existing dependencies are sufficient;
3. inspect its license;
4. inspect relevant transitive licenses;
5. consider maintenance and security;
6. verify compatibility with the project's licensing strategy.

Do not incorporate code or assets with unknown provenance or licensing.

### Definition of Done

A ticket is complete only when applicable:

- acceptance criteria are satisfied;
- behavior is covered by tests;
- regression tests exist where appropriate;
- focused tests pass;
- broader tests pass;
- architecture remains valid;
- no unrelated changes were introduced;
- documentation/status is updated when necessary;
- PR template requirements are satisfied;
- review feedback is addressed;
- the repository remains buildable and understandable.

## Weekly token-budget strategy

The project is expected to be developed within a limited weekly free-token budget.

Optimize for **tested, reviewable, mergeable functionality**, not maximum generated code.

Prefer:

```text
one small Issue
    +
tests
    +
minimum implementation
    +
one focused PR
```

Avoid repeatedly rediscovering project direction or redesigning the entire system.

If implementation reveals a significant architectural question, stop the local implementation and create an appropriate decision Issue rather than consuming the token budget on speculative redesign.

## Reproducible design rationale

The following notes explain the decisions made during the initial Phase 0–2 design work. They are historical rationale, not additional permanent requirements. Current requirements are defined by the sections above and the current architecture/design documents.


The following notes preserve the reasoning that led to the current architecture. They are intentionally included so that a future developer or coding agent can reconstruct *why* these decisions were made rather than treating them as arbitrary rules.

### Why the old implementation is not being ported

The original implementation has a large amount of tightly coupled code, including game logic, persistence, request handling, and presentation concerns. The project is also intentionally free to change its implementation language.

Therefore, translating the old structure into Go would preserve many of the limitations that the reconstruction is intended to remove.

The old implementation is consequently treated as a behavioral specification/reference, not as a codebase to migrate.

### Why feature expansion is emphasized

The original game accumulated many systems over time: adventure, battles, jobs, skills, guilds, casino games, alchemy, auctions, farming, collection systems, events, and other additions.

This history suggests that the game's identity is partly expressed through **continued expansion of its game systems**.

Therefore, extensibility is not an optional engineering quality. It is part of the product's intended character.

### Why Core is intentionally small

If every new feature adds fields, flags, branches, and special cases to a shared central model, the new implementation will eventually reproduce the same coupling that motivated the reconstruction.

Keeping Core small makes the cost of adding a feature more local.

### Why Feature Modules are first-class

A large number of independent game systems means that the natural unit of future development is often a feature rather than a technical layer.

Feature Modules make ownership explicit and provide a natural unit for implementation, testing, and architecture review.

### Why Battle is independent

Multiple game systems can require combat. If combat is embedded inside Quest, Guild, or another feature, adding a new combat mode requires modifying existing feature code.

A separate Battle component allows different game systems to reuse combat without coupling their internal implementations.

### Why ScheduledAction is reusable

The original game includes actions whose result occurs after a delay. The same pattern can naturally support quests, crafting, farming, cooldowns, events, and future systems.

A reusable scheduling concept therefore provides an extension point without requiring each feature to invent its own time-processing mechanism.

### Why Domain Events are selective

Events can allow optional features such as rankings, achievements, collections, and statistics to react to core game outcomes without making the producer depend on every consumer.

However, universal eventization makes control flow harder to understand. Events are therefore a tool for meaningful decoupling, not a mandatory communication mechanism.

### Why Go is the initial language

The project benefits from starting with one implementation language while its architecture is still being established.

Go provides a straightforward initial implementation environment. There is no requirement that every future component remain in Go.

The important constraint is therefore not "all code must be Go", but "component boundaries must not unnecessarily depend on the implementation language."

### Why not start with microservices

The project has many potential features, but that does not mean each feature needs an independent process.

Starting as a modular monolith keeps development, testing, deployment, and AI-assisted development simple. A component can be extracted later if an actual requirement appears.

### Why architecture review is explicit

With a game designed for continuous feature expansion, a locally correct feature can still damage the long-term architecture.

A dedicated architecture review asks a different question from ordinary code review:

> Does this feature make the next feature easier or harder to implement?

That question is central to maintaining the project's intended extensibility.

---


## Test-driven development is mandatory

This project follows **test-driven development (TDD) as the default development method**.

Tests are not merely a verification step performed after implementation. They are part of the design process and should be written before or together with the implementation that they specify.

For any non-trivial behavior change, the expected workflow is:

```text
Issue
  ↓
Define behavior / acceptance criteria
  ↓
Write or update tests
  ↓
Implement the smallest change that satisfies the tests
  ↓
Run focused tests
  ↓
Run broader test suite
  ↓
Review
  ↓
PR
```

### What tests should prioritize

Tests should primarily protect **game behavior and domain rules**, not implementation details.

Prioritize, in this order:

1. **Domain invariants and game rules**
   - valid and invalid state transitions;
   - character progression;
   - battle outcomes;
   - item ownership/equipment rules;
   - job requirements;
   - currency/economy rules;
   - scheduled-action behavior;
   - feature-specific rules.

2. **Observable component behavior**
   - inputs and outputs;
   - state changes;
   - errors and failure conditions;
   - emitted domain events;
   - persistence behavior where persistence is part of the contract.

3. **Feature integration**
   - interactions between components through their public contracts;
   - important end-to-end game flows.

4. **Regression protection**
   - every discovered bug should normally result in a regression test before or alongside the fix.

Avoid tests that merely reproduce implementation details such as private helper structure, internal call order, or incidental data structures unless those details are themselves contractual.

### Test requirements for changes

A change is not considered complete merely because the code compiles.

For behavior changes:

- add or update tests that describe the intended behavior;
- ensure the tests fail for the missing behavior when practical;
- implement the smallest change necessary;
- run the relevant focused tests;
- run the broader test suite before the PR is considered complete.

When a test is intentionally omitted, the reason must be documented in the Issue or PR.

### Architecture and tests

Architecture decisions should also be evaluated through tests where practical.

A component boundary is healthier when its behavior can be tested through its public contract without depending on another component's private implementation.

Tests should help preserve the ability to replace a component implementation without rewriting unrelated tests.

---

## Issue-driven and PR-driven development is mandatory

All development work must be tracked through **GitHub Issues and Pull Requests** (or the repository's equivalent issue/PR system).

Do not begin substantial implementation work from an informal request alone.

The Issue is the unit of planned work. The PR is the unit of reviewed change.

### Issue requirements

Every implementation task must have an Issue.

Every Issue must use the repository's **Issue template**. Do not create ad-hoc Issues that bypass the template.

The Issue should define enough information for implementation to begin, including as appropriate:

- problem or desired behavior;
- scope;
- acceptance criteria;
- relevant design considerations;
- testing requirements;
- dependencies or prerequisites;
- out-of-scope items.

A vague Issue such as "implement guild" is insufficient. Break large work into smaller tickets with observable acceptance criteria.

### PR requirements

Every code change must be submitted through a Pull Request.

Every PR must use the repository's **PR template**. Do not bypass or replace the template with an informal PR description.

A PR should normally reference the Issue it implements.

The PR description must make it possible for a reviewer to determine:

- what behavior changed;
- why it changed;
- which tests were added or changed;
- which tests were run;
- whether architecture boundaries were affected;
- whether documentation or roadmap/status updates are required.

### Template usage is absolute

**Issue and PR templates are mandatory, not suggestions.**

Agents must not:

- create an Issue without selecting the appropriate template;
- create a PR without using the repository PR template;
- replace required template sections with a shorter custom description;
- close or bypass the Issue/PR workflow merely because a change appears small.

If an appropriate template does not exist, **stop and add/fix the template before proceeding with the work**, unless the task explicitly concerns emergency repository recovery.

### Ticket granularity

Prefer tickets that can be completed in a small, independently reviewable unit.

A ticket should ideally result in:

```text
one clear behavior
+
its tests
+
the minimum implementation
```

Large features should be decomposed into multiple Issues rather than implemented as one enormous ticket.

### PR scope

Keep PRs small enough that a reviewer can understand:

- the requirement;
- the behavior;
- the tests;
- the architectural impact.

Do not combine unrelated refactoring with feature work unless the refactoring is necessary for the ticket.

---

## Definition of Done

A ticket is not Done until all applicable items are satisfied:

- [ ] Acceptance criteria are satisfied.
- [ ] Tests cover the intended behavior.
- [ ] Regression tests exist for relevant bug fixes.
- [ ] Focused tests pass.
- [ ] The broader test suite passes.
- [ ] Architecture boundaries remain valid.
- [ ] No unnecessary abstraction or unrelated refactoring was introduced.
- [ ] Documentation is updated when the behavior or architecture requires it.
- [ ] The PR uses the repository template.
- [ ] The PR references the Issue.
- [ ] Review comments are resolved or explicitly addressed.
- [ ] The repository is left in a clean, buildable state.

---

## Weekly token-budget strategy

Because development is expected to operate within a limited weekly free-token budget, **ticket-driven TDD is also the primary mechanism for controlling AI work**.

Each weekly work unit should normally be one small Issue or a small group of tightly related Issues.

Agents should not spend tokens repeatedly rediscovering the project's direction.

Before implementation:

1. read `AGENTS.md`;
2. read `STATUS.md` and `ROADMAP.md`;
3. read the target Issue;
4. inspect only the relevant code and tests;
5. identify the acceptance criteria;
6. implement the tests first or establish the test changes immediately;
7. implement the smallest change;
8. run focused tests;
9. run broader tests;
10. prepare the PR using the mandatory template.

If the requested work is too large for one ticket, **stop and split the work into Issues** rather than silently expanding scope.

The goal is not to maximize the amount of code generated per week. The goal is to maximize the amount of **tested, reviewable, mergeable functionality** produced per week.

## Working rule for future agents

When requirements are ambiguous:

1. preserve the game's intended behavior;
2. prefer the smallest design that supports the current requirement;
3. keep feature-specific concerns inside the feature;
4. keep Core small;
5. avoid copying the old implementation's structure;
6. prefer explicit contracts over hidden coupling;
7. add abstractions only when they solve a demonstrated problem;
8. consider how the next feature of the same category would be implemented;
9. run focused tests before broad tests when iterating;
10. leave the repository in a clean, understandable state.

When an architectural decision is uncertain and could materially affect future feature development, stop and document the alternatives and trade-offs rather than silently choosing a complex solution.
