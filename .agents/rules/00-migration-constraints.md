---
name: Migration and Clean-Room Constraints
description: Absolute constraints for transitioning from the original Party2 implementation.
---

# Migration Constraints (Version 1.0 Temporary Phase)

This project is currently rebuilding Party2 toward Version 1.0. These are transition requirements and must not be mistaken for permanent product architecture.

## 1. Clean-room reconstruction

- Do not copy, port, translate, or mechanically rewrite existing Party2 source code.
- Do not preserve the old implementation's architecture merely for compatibility.
- Use the existing game only as a source of behavioral and functional requirements.
- Reconstruct the implementation from the desired domain model and requirements.
- The existing Party2 implementation is a behavioral/design reference only.

## 2. Handling Ambiguity

- **Do not invent behavior silently:** When investigating the old implementation, if the intended behavior is unclear, undocumented, or if conflicting evidence exists, do not silently guess or invent the behavior. Record the ambiguity and create an Issue, request a decision from the user, or implement a safe placeholder while raising the question.

## 3. Asset and Intellectual Property Rules

- Existing Party2 source code is not reused.
- Existing Party2 images, visual assets, and sounds are not reused.
- **Copyrighted / Distinctive Names:** Asset names or job/skill names that contain distinctive proper names or other terms strongly associated with a specific game (e.g., Final Fantasy, Dragon Quest) are **not reused**. Such names must be replaced with generic terminology (e.g., replace "赤魔道士" with generic terms).

## 4. Legacy CGI `@actions` Array & Subroutine Reconciliation
To prevent unintended feature dropouts during clean-room migration:
- **Extract Action Dispatch Tables**: When analyzing legacy CGI scripts in `/home/witchcraze/dev/party2`, extract all elements of `@actions`, `$actions{...}` dispatch tables, and action branches (`if ($in{action} eq '...')`).
- **1:1 Mapping Verification**: Create an explicit 1:1 reconciliation mapping table between legacy actions and proposed/implemented Go domain methods & HTTP endpoints.
- **Explicit Rationale for Omitted/Superseded Actions**: If a legacy action is intentionally omitted, superseded (e.g. replaced by modern session auth / REST conventions), or postponed, explicitly record the reason in the PR or Issue (e.g., `Superseded by session middleware`, `Postponed to Issue #<number>`).

## 5. Phase Boundary

The desired transition is:
`Reference Party2` -> `Version 1.0 reconstruction` -> `Stable OSS foundation` -> `Normal feature development`

After Version 1.0, historical implementation details should no longer drive ordinary development decisions, and this rule file may be safely removed.
