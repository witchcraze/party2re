# Design Rationale

> This document records the background and reasoning behind the project's major design decisions.
>
> `AGENTS.md` is the source of truth for current rules and constraints. This document preserves
> historical context and explains why those rules and boundaries were chosen.
>
> Do not treat historical rationale as an independent requirement when it conflicts with the
> current rules in `AGENTS.md`.

## How to use this document

- Use this document when a design decision needs to be understood or reconsidered.
- Record material alternatives and trade-offs here when new architectural decisions are made.
- Keep current requirements and mandatory rules in `AGENTS.md` or the appropriate authoritative
  design document.
- When a decision changes, update the current source of truth first and then update this rationale.

---

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
