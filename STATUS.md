# Status

Last updated: Issue #55 — Item Shop purchase and sale operations

## Current phase

**Version 1.0 Reconstruction / Refactoring — In Progress**

Phase 0〜2（ゲーム理解・アーキテクチャ・ドメインモデル）は完了しています。

現在は、これらの設計を実装へ移しながら、Version 1.0に必要なゲームの基盤と主要な挙動を新規実装する過程です。
Version 1.0の完成条件は、既存プロジェクトの意味のあるゲーム機能を新規実装として再構築し、必要な画像を新規制作または承認済みプレースホルダーで準備することです。旧ソースコード・旧画像の移植は完成条件に含めません。
この「リファクタリング／再構築フェーズ」は永続的なプロジェクト方針ではありません。Version 1.0を成立させるための一時的な開発フェーズです。

### 永続するもの

以下はVersion 1.0完成後も継続します。

- Feature拡張を中心とした設計
- Core / Component / Featureの責務分離
- コンポーネント境界を実装言語から独立させる考え方
- TDD
- Issue / PR driven development
- Architecture Review
- 小さなチケット単位での開発
- 依存ソフトウェアとライセンスの管理

### 一時的なもの

以下はVersion 1.0の再構築に必要な対応です。

- 既存Party2の調査
- 旧実装との挙動比較
- 旧実装を使用しない新規実装への置き換え
- 既存アセットの再制作
- Version 1.0のための初期機能再構築
- 旧実装に由来する制約の整理・除去

Version 1.0完成後は、これらを通常の開発作業として扱わず、必要に応じて個別のIssueとして扱います。

## Next action

Phase 3 foundation, Character persistence, delayed activities, initial character
state, level progression, job-based stat growth rules, items, inventory,
reusable Battle contract, detailed Battle resolution, the first
Adventure-to-Battle reward loop, job definitions/history, skill usage
conditions, equipment slots, and initial assets are in place. Player account
and session lifecycle is implemented through Issue #21. Activity and
Adventure claim reward application is atomic under concurrency through Issue
#35.

The current Compose workflow remains the development and integration-test
environment. CI also publishes Go coverage reports for later review without
enforcing a percentage threshold.

Application operations now have an injectable standard-library `log/slog`
contract with JSON output, correlation fields, and secret-safe handling. Player
account operations use it without logging passwords, session values, or
credentials.

Issue #50 and Issue #51 validate every loaded Job and Item definition through
catalog-wide field, lookup, availability, slot, and boundary tests. Both
catalogs are verified continuously in CI.

Issue #62 introduced character resting and inn recovery.

Issue #106 introduced the reusable ScheduledAction processing mechanism
(Valkey-backed). The mechanism is available for feature modules to use:

- `internal/core/scheduling` — domain model with state machine and `Validate()`
- `internal/scheduling` — `Service` (enqueue), `Worker` (periodic poll + dispatch),
  `ActionHandler` interface for feature modules
- `internal/valkey` — Valkey client wrapper (`PARTY2_VALKEY_ADDR` env var)
- Valkey is included in the Docker Compose environment with AOF + RDB persistence
- Corrupted or oversized data from Valkey is rejected before any lock or dispatch

Feature modules connect to the mechanism by implementing `ActionHandler` and
calling `worker.RegisterHandler(actionType, handler)` at startup. No changes
to the scheduling mechanism itself are required when adding a new action type.

Issue #109 migrated Activity training to ScheduledAction push processing:

- `StartTraining` enqueues a ScheduledAction (`activity:training_complete`)
- `TrainingHandler` processes completion by claiming and applying experience rewards
- Fallback path works if Valkey is unavailable (manual `Claim` still functions)

Issue #110 migrated Adventure completion to ScheduledAction push processing:

- `Start` enqueues a ScheduledAction (`adventure:complete`)
- `AdventureCompletionHandler` resolves battle, applies rewards, and persists results at worker execution time
- Fallback path works if Valkey is unavailable (manual `Claim` resolves and claims)

Issue #55 introduced the Item Shop purchase and sale operations:

- `internal/shop` — Service for item purchases (gold deduction and inventory addition) and sales (inventory removal and 50% price gold payout)
- `internal/database.ShopRepository` — Atomic single-transaction commit for character wallet and inventory updates
- `docs/design/shops.md` — Language-agnostic specification of shop purchase and resale rules

## Confirmed decisions

- Existing Party2 source code will not be reused.
- Existing Party2 assets/images will not be reused.
- Existing Party2 is a behavioral/design reference.
- `Created by Merino` may be acknowledged on the project page as the origin of the game.
- Initial implementation language is Go.
- Components are conceptually language-independent.
- Future replacement of individual components by another language is allowed.
- Start as a modular monolith.
- Do not introduce microservices or remote protocols without a concrete requirement.
- Core should remain small.
- Feature Modules are first-class.
- Battle is a reusable independent component.
- Scheduled actions are a reusable concept for delayed game activities.
- Domain events are available for meaningful decoupling, but should be used selectively.
- Architecture review is required for substantial feature additions.
- Software license candidates: MIT, Apache-2.0, AGPLv3.
- Creative asset candidates: Creative Commons licenses.
- Final licensing will be determined after implementation dependencies are known.
- Durable persistence will use MariaDB.
- Valkey will be used only for concrete cache, transient-state, queue, or coordination requirements.
- Session storage is currently MariaDB as the smallest implementation for the
  existing persistence setup; moving sessions to Valkey remains a follow-up
  once transient-state infrastructure is needed.
- Initial Go target is Go 1.26.7, subject to updating the pinned patch version when the project deliberately changes its supported toolchain.

## Current conceptual model

```text
Core
  Player
  Character
  Progression
  Item
  Inventory
  Equipment
  Currency
  Time / Scheduling
  Domain Events

Shared Components
  Battle
  Adventure / Quest

Features
  Guild
  Casino
  Alchemy
  Auction
  Farming
  Collection
  Ranking
  Events
  ...
```

## Not yet decided

- exact Go package layout;
- database product;
- API framework;
- frontend technology;
- final project license;
- final asset licenses;
- first production feature set;
- deployment architecture.

Do not make these decisions merely for completeness. Decide them when the implementation requires them.

## Document references

- `AGENTS.md` — rules that apply to current and future development.
- `docs/architecture/` — permanent architecture.
- `docs/design/` — permanent game/design model.
- `docs/development/` — permanent development workflow.
- `ROADMAP.md` — phase and future-work planning.
