# Status

Last updated: Issue #82 — Casino Indian Poker Mini-Game

## Current phase

**Version 1.0 Reconstruction / Refactoring — In Progress**

Phase 0〜4（ゲーム理解・アーキテクチャ・ドメインモデル・骨格・垂直スライス）は完了しています。

現在は **Phase 5+（個別機能の段階的再構築）** にあり、Version 1.0に必要な主要ゲームシステムをクリーンルーム再構築として新規実装しています。
Version 1.0の完成条件は、既存プロジェクトの意味のあるゲーム機能を新規実装として再構築し、必要な画像を新規制作または承認済みプレースホルダーで準備することです。旧ソースコード・旧画像の移植は完成条件に含めません。

---

## Current Component State (What is True Now)

### Core & Shared Components
- **Player** (`internal/core/player`, `internal/player`): アカウント登録・パスワードハッシュ・セッション管理（MariaDB永続化）。
- **Character** (`internal/core/character`, `internal/character`): `player_id` 外部キーによるアカウント紐付け、初期ステータス、能力値計算、転生（Rebirth +5永続ボーナス）、プレイヤー別キャラクター一覧取得。
- **Progression** (`internal/core/progression`): レベルアップ（累積経験値テーブル `level * level * 10`）、成長率適用。
- **Job & Skill** (`internal/core/job`, `internal/job`, `internal/core/skill`): JSONカタログ（`jobs.json`）、転職、Lv99マスタリー、スキル発動・コスト計算。
- **Item, Inventory, Equipment** (`internal/core/item`, `internal/inventory`, `internal/equipment`): 5カテゴリJSONカタログ（武器・防具・盾・アクセ・消費/素材）、スロット装備、所持枠管理。
- **Battle** (`internal/core/battle`): 決定論的ターン制戦闘解決、勝敗・報酬決定。
- **Scheduling** (`internal/core/scheduling`, `internal/scheduling`): Valkeyバックエンドの遅延アクションキュー＆分散排他ロックWorker。

### Feature Modules
- **Activity** (`internal/activity`): 訓練機能（Valkey Worker push型＋手動Claimフォールバック）。
- **Adventure** (`internal/adventure`): 28ステージ（`stages.json`）、286体モンスター（`monsters.json`）、戦闘解決、ドロップ報酬、Valkey Worker連携。
- **Shop** (`internal/shop`): アイテム売買（50%売却）、トランザクション整合性。
- **Depot** (`internal/depot`): 倉庫（アイテム・ゴールド預入・引出）、トランザクション整合性。
- **Blacksmith** (`internal/blacksmith`): 鍛冶屋（+1〜+10装備強化、成功率曲線、費用・素材消費）。
- **Alchemy** (`internal/alchemy`): 錬金術（112レシピ `recipes.json`）、素材合成。
- **Bank** (`internal/bank`): 銀行（預金・引出・プレイヤー間送金、`FOR UPDATE` 排他ロック）。
- **Inn** (`internal/inn`): 宿屋・休息（HP/MP全回復）。
- **Guild** (`internal/guild`): ギルド設立（5,000 G）、階層役職管理（Leader, Officer, Member）、加入・脱退・追放・役職変更・リーダー権限譲渡、ゴールド寄付によるEXP獲得とギルドレベルアップ（最大Lv10 / 定員拡大）、お知らせ掲示板、単一ギルド所属制約。
- **Casino** (`internal/casino`): カジノコイン両替（1 Coin = 20 G）、インディアンポーカー（52枚標準トランプモデル、ブラインド賭け、NPCディーラーAI、最大5ラウンド・レート上昇、ショーダウン勝敗判定・配当精算、トランザクション整合性）。

### API & Transport
- **HTTP JSON API** (`internal/api/http`): Go標準 `net/http` によるREST風エンドポイント（`/health`, `/players`, `/sessions`, `/characters`, `/adventures`, `/shop/*`）。セッション認証、キャラクター所有権認可検証（403 Forbidden）、標準セキュリティレスポンスヘッダー（nosniff, DENY, strict-origin-when-cross-origin, CSP none）、CORS ポリシーミドルウェア（許可 Origin ホワイトリスト、`OPTIONS` プリフライトキャッシュ、`*` ワイルドカード抑止）、64 KiB リクエスト制限、未知フィールド拒否、構造化エラーレスポンス。

### Infrastructure & Operations
- **Database**: MariaDB（マイグレーション `migrations/001_initial.sql` 〜 `017_casino.sql`、`make db-migrate` / `make db-reset`）。
- **Valkey**: 遅延アクションキュー・排他ロック（AOF+RDB永続化）。
- **Logging**: Go標準 `log/slog` によるJSON構造化ログ、秘密情報自動マスキング。
- **Verification**: `Makefile` (`make check`, `make fmt`, `make check-clean`)、Git pre-push hook による自動検証。
- **Deployment**: Distroless (`gcr.io/distroless/static-debian13:nonroot`) ベースの最小本番イメージ（GHCR自動公開）。

---

## Immediate Priorities (Next Actions)

1. **Casino Mini-Games** (Issue #81): スロットマシンの実装。
2. **Core Domain Specifications** (Issue #136): `docs/design/` 配下への Core 言語非依存仕様書（戦闘、成長、ジョブ、スキル、アイテム）の整備。
3. **Remaining Version 1.0 Feature Modules**:
   - Casino Mini-Games（スロットマシン、ハイロー、ドッペル）
   - Guild Battles（GvG戦闘エンジン）
   - Auction / Free Market（プレイヤー間出品・入札）
   - Farm / Plantation（栽培・モンスター育成）
   - Collection & Monster Book（図鑑・収集記録）
   - Rankings（レベル、ジョブ、週間ランキング）
   - Web Presentation UI / Client

---

## Confirmed decisions

- Existing Party2 source code will not be reused.
- Existing Party2 assets/images will not be reused.
- Existing Party2 is a behavioral/design reference.
- `Created by Merino` may be acknowledged on the project page as the origin of the game.
- Initial implementation language is Go (Go 1.26.7).
- Components are conceptually language-independent.
- Future replacement of individual components by another language is allowed.
- Start as a modular monolith.
- Do not introduce microservices or remote protocols without a concrete requirement.
- Core should remain small.
- Feature Modules are first-class components.
- Battle is a reusable independent component.
- Scheduled actions use Valkey-backed Worker queue with push-processing and fallback.
- Durable persistence uses MariaDB.
- API layer uses Go standard library `net/http` JSON handlers.
- Production container uses Distroless minimal image.
- Domain events are available for meaningful decoupling, but should be used selectively.
- Architecture review is required for substantial feature additions.

---

## Pending Decisions / Open Questions

- frontend technology / web client framework;
- final software license (candidates: MIT, Apache-2.0, AGPLv3);
- final creative asset licenses (candidates: Creative Commons);
- moving session storage from MariaDB to Valkey (when transient-state cache is needed);
- final asset production and management pipeline.

Do not make these decisions merely for completeness. Decide them when the implementation requires them.

---

## Document references

- `AGENTS.md` — rules that apply to current and future development.
- `docs/architecture/` — permanent architecture.
- `docs/design/` — permanent game/design model.
- `docs/development/` — permanent development workflow.
- `ROADMAP.md` — phase and future-work planning.
- `docs/migration/feature-inventory.md` — Version 1.0 feature inventory.
