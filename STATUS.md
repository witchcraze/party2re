# Status

Last updated: Issue #472 — [Refactor] Chapel: Eliminate fictional Donations, restore Monster befriending prayer, and enforce single-active-wish constraint

## Current phase

**Version 1.0 Reconstruction / Refactoring — In Progress**

Phase 0〜4（ゲーム理解・アーキテクチャ・ドメインモデル・骨格・垂直スライス）は完了しています。

現在は **Phase 5+（個別機能の段階的再構築）** にあり、Version 1.0に必要な主要ゲームシステムをクリーンルーム再構築として新規実装しています。
Version 1.0の完成条件は、既存プロジェクトの意味のあるゲーム機能を新規実装として再構築し、必要な画像を新規制作または承認済みプレースホルダーで準備することです。旧ソースコード・旧画像の移植は完成条件に含めません。

---

## Current Component State (What is True Now)

### Architecture & Repository Intelligence (Guidance Layer - PoC)
- **Guidance Layer (.arch/)**: シンボルアンカー（`path#Symbol`）ベースのモジュール詳細定義（`.arch/modules/*.json`）、共有テーブル逆引きインデックス（`.arch/shared_tables/*.json`、`characters`, `inventory_items`, `bank_accounts`, `guilds`）、Mermaid全体トポロジー図（`docs/architecture/guidance-layer.md`）。外部依存不要のGo + JSON + Markdown構成。
- **Module Selection Criteria & Target Tiers**: 4つの選定基準（C1: トランザクション深度, C2: 行ロック階層, C3: エスクロー/共有状態, C4: 非同期Worker）に基づくトリアージ。Tier 1（高リスク8機能: `tavern`, `delivery`, `bank`, `auction`, `guild`, `shop`, `blacksmith`, `adventure`）、Tier 2（オンデマンド）、Tier 3（除外）の運用スコープを確立。
- **Automated Mechanical Verification**: Go AST シンボルリント（`internal/architecture/arch_test.go`）による高速静的シンボル実在性チェック、`RunInTx` 呼び出し実在検証、本番ファイル行数リミット（生産コード ≤ 500行、`cmd/*/main.go` ≤ 150行）のラチェット方式自動ガード（`internal/architecture/file_size_lint_test.go`）、インターフェース直接メソッド数リミット（ドメインインターフェース ≤ 10直接メソッド）のラチェット方式自動ガード（`internal/architecture/interface_size_lint_test.go`）、および孤立メソッド・未使用定数・未使用DTO構造体フィールドの機械的デッドコード検知（`internal/architecture/deadcode_lint_test.go`, `internal/architecture/unused_definitions_lint_test.go`）（`make check` / `make arch-lint` 統合）。`internal/tavern/tavern.go`（620行→210行）および `internal/boss/boss.go`（633行→228行）の分割リファクタリングにより500行制限をクリアし `whitelistedLegacyFileLimits` は13から11ファイルへラチェットダウン。また孤立メソッド監査（Issue #428）およびゼロホワイトリストラチェット化（Issue #438）によりデッドコード孤立メソッドは完全ゼロを恒久保証。さらに Issue #439 により ISP インターフェースサイズ制限を導入し、パイロットとして `contest.ContestRepository`（28メソッド）を5つの責務別サブインターフェース（Photo/Round/Entry/Vote/Legend）へ分割・埋め込み合成（Composite Interface）へリファクタリング完了（`whitelistedLegacyInterfaceLimits` 残余5件）。
- **Continuous Performance Verification & Benchmark Framework (`docs/development/benchmarking.md`)**: クリティカルパス（AST静的解析リント、戦闘シミュレーション、Valkeyセッション操作）を網羅する `Benchmark*` スイート、標準実行スクリプト（`scripts/benchmark.sh`）、`Makefile` ターゲット（`make bench`）、およびベースライン比較・リグレッション自動検知CLI（`scripts/compare_benchmarks.go`）。
- **Valkey Keyspace Taxonomy & Operational SSOT (`docs/architecture/valkey-keyspace.md`)**: システム全体のValkeyキー空間（`party2:<namespace>:<entity>[:<id>]`）、TTLポリシー、所有モジュール、Luaスクリプト運用基準（1ms未満バジェット、O(log N)上限、`KEYS *` 禁止、Cluster Hash Tagging `{...}` 規約、インメモリ等価性、`lua/*.lua` 外部ファイル化・`//go:embed` コンパイル時埋め込み）、TTLスコア付きSorted Set（ZSET）遅延パージ標準。Go ASTリンター（`internal/architecture/valkey_lint_test.go`）により機械的検証。
- **Core Domain Invariant Static Analysis Linter Suite**: Go AST 静的構文解析リンター（`internal/core/core_lint_test.go`）による全生産コードファイルの検査。Progression、Currency & Economy、Job State、Inventory、Equipment、Battle Participant Identityの全6重要ドメイン不変条件に対する直接構造体フィールド操作を機械的に禁止し、Core標準カプセル化ヘルパー経由の操作を100%強制。

### Core & Shared Components
- **Player** (`internal/core/player`, `internal/player`): アカウント登録・パスワードハッシュ（bcrypt コスト12、GPU耐性・メモリ困難性担保）・セッション管理（Valkey Master `party2:session:<token>` 7日間TTL自動失効、Sorted Set `party2:player:sessions:<player_id>` による遅延パージ `ZREMRANGEBYSCORE`）・Personal Access Token（APIキー `p2_sk_...`、SHA-256ダイジェスト永続化、デュアル認証、所有権検証付き失効 `DELETE /player/tokens/{id}`）・アカウント完全削除（所有キャラクター全件クリーンアップ、Valkeyセッション破棄、PATカスケード削除、MariaDB 35+テーブル連鎖削除）。
- **Character** (`internal/core/character`, `internal/character`): `player_id` 外部キーによるアカウント紐付け、初期ステータス、能力値計算、スキルポイント（SP）、キャラクター一覧取得、キャラクター個別削除（所有権認可、外部ドメイン `CleanupHook` 実行、MariaDB 35+サブリソーステーブルの完全カスケード削除）、命名の館（名前変更・性別/外観変更）、プロフィール自己紹介コメント・アバター画像管理。通貨・メダルの安全なカプセル化（`AddMoney`, `DeductMoney`, `AddSmallMedals`, `DeductSmallMedals`、上限キャップ・負数ガード・残高オーバードラフト防止）。架空の転生（Rebirth）は完全撤廃。
- **Progression** (`internal/core/progression`): レベルアップ（累積経験値テーブル `level * level * 10`）、レベルアップ時のSP加算（`$m{sp}++`、スキルの宝珠による25%追加ボーナス）、SP到達時の職業スキル自動習得判定（`skill.RequiredSP == character.SP`）、原典準拠のステータス成長率再ロール（$v > 9$ 時 `rand(1, 9)`・HP最低+1保証）、天界OverLevel限界突破（Lv150）対応、ASTリンターによるCore標準ヘルパー（`progression.ApplyExperienceWithJobFull`）強制。
- **Job & Skill** (`internal/core/job`, `internal/job`, `internal/core/skill`): クリーンルーム規約に完全準拠したJSONカタログ（`jobs.json`）、Lv20転職（能力値半減・Lv1/Exp0・転職回数・前職SP復元）、最終スキルSP到達マスタリー、特殊職のアイテム消費、思い出しによるマスター職交換、将来用メモリ枠（「よびおこす」未来のカケラによるステータススナップショット保存・復元）、全72職コンプリート時の称号・全体イベントニュース通知および特殊職「すっぴん」解禁、SP到達スキル習得、スキル発動・MPコスト計算。
- **Item, Inventory, Equipment** (`internal/core/item`, `internal/inventory`, `internal/equipment`): 5カテゴリJSONカタログ（武器・防具・盾・アクセ・消費/素材）、スロット装備、所持枠管理、統一アイテム定義プロバイダー（`coreitem.DefinitionProvider`）、インベントリアイテム更新（`inv.Update`）、装備スロットカプセル化（`equip.Equip`, `equip.Unequip`）。
- **Battle** (`internal/core/battle`): 決定論的ターン制戦闘解決、勝敗・報酬決定（経験値・ゴールド・アイテム・ちいさなメダル）、構造化ターンログ出力、戦闘参加者（Participant）標準アダプタ/ビルダー（`NewParticipantFromCharacter`, `ParticipantBuilder`）。従来の1v1戦闘（`Resolve`）との100%下位互換を維持しつつ、オリジナルParty2 CGI（`_battle.cgi`, `_skill.cgi`）完全パリティのマルチターン・パーティ戦闘エンジン（`ResolvePartyBattle`）を実装（Issue #480）。最大4人の味方パーティ vs 1〜N体の敵グループ、敏捷性（Agility）降順ターン解決、MP消費の職業スキル（単体/全体・属性・回復）、CMP消費・詠唱セリフ出力付き合成スキル（GemEffect連鎖）、属性フィールド状態（火・水・風・土・光・闇の相性補正 +30%/-20%・反属性フィールド無効化・ターン減算）、および致命傷時の復活トリガー（ファラオ、不死身、闘気の盾、ドクロのお守り、呪いの復活）の完全再現を達成。ファイルサイズ上限厳守のため単一責務ファイル（`party_battle.go`, `field.go`, `defeat.go`, `action.go`）に分割完了。
- **Scheduling** (`internal/core/scheduling`, `internal/scheduling`): Valkeyバックエンドの遅延アクションキュー＆分散排他ロックWorker。`valkeytest` による完全オフライン単体テスト導入（Issue #282）により、`Worker.Run()` / `processActions()` および `ValkeyRepository` 全メソッド（Schedule, FetchDue, AcquireLock, Save, CancelByActorID）の全分岐を網羅しパッケージカバレッジ **19.0% → 92.7%** へ向上。
- **Database & Transaction Orchestration** (`internal/database`, `internal/economy`, `internal/core/event`, `internal/testutil`, `internal/database/testutil`): 全リポジトリのトランザクション伝播モデル（`RunInTx` と `ExecutorFromContext`）、コンテキスト内トランザクション再利用、決定論的行ロック獲得順序（Shared -> Players -> Characters (昇順) -> Inventory/Equipment -> Jobs -> Depots -> Bank -> Guilds (昇順) -> 各種機能テーブル）のAST強制（`internal/database/lock_hierarchy_lint_test.go`）、共通2者間IDソート排他ロックユーティリティ（`id.Sort2`）、**横断的アプリケーション実行時プリミティブ層**（`docs/architecture/cross-domain-primitives.md`、`economy.TransactionRunner`、`economy.ExecuteTransaction`、`economy.Run[T]`）、インプロセス2フェーズドメインイベントディスパッチャ（`internal/core/event.Dispatcher`、In-Tx同期＋Post-Commit非同期）、標準エンティティファクトリおよび汎用並行ストレステストハーネス（`RunConcurrentStressTest`, `RunRace`, `RunRace2`）。`internal/economy/economy.go` は責務別に4ファイルへ分割リファクタリングされ500行制限をクリア（`whitelistedLegacyFileLimits` から完全除外）。
- **Standardized Pagination & Common Utilities** (`internal/pagination`, `internal/id`, `internal/validation`, `internal/api/http/middleware`): 単一責務の共通パッケージ配置、暗号学的一意ID生成（`internal/id`）、汎用ジェネリックページネーション（`internal/pagination`、オフセット `Page[T]` およびキーセット・カーソル `CursorPage[T]`）。広場掲示板、冒険履歴、戦闘リプレイ、手紙、宅配便への水平展開。

### Feature Modules
- **Activity** (`internal/activity`): 訓練機能（Valkey Worker push型＋手動Claimフォールバック）。
- **Adventure** (`internal/adventure`): 28ステージ（`stages.json`）、286体モンスター（`monsters.json`）、戦闘解決、ドロップ報酬（メダル含む）、Valkey Worker連携、過去冒険履歴一覧（オフセット/カーソル両対応）、冒険戦績クロニクル（トライモード/イメージ/カーム/ハード/アバター/エクストリーム）。勝利時フック（`VictoryHook`）による実績進捗連携。
- **Medal & Lifetime Achievements** (`internal/medal`): 小さなメダル交換所（減算消費方式、`economy.Service` 連携、`TransactionProvider` と行ロックによる完全アトミック整合性）、生涯マイルストーン実績・記念勲章コレクションシステム（オブザーバーフック連携による進捗自動記録、二重受取防止排他ロック、記念勲章・メダル報酬付与）。
- **Shop** (`internal/shop`): アイテム売買（50%売却）、1回最大取引数量制限（`MaxTransactionQuantity = 9999`）、整数オーバーフロー安全乗算（`safeMultiply`）、`economy.Service` 連携、`TransactionProvider` と決定論的行ロック階層（`characters` -> `inventory_items`）による完全アトミック整合性。
- **Depot** (`internal/depot`): 倉庫（アイテム預入・引出・動的容量計算・拡張・売却・整頓・郵送）。架空のゴールド預託を完全撤廃し、オリジナルPerl CGI仕様（`system.cgi:get_depot_c`）に準拠した動的容量計算（Base(JobLv: 5〜150) + ExDepot(0〜20: +5〜+100) + OverDepot(0〜5: +50〜+250) = 最大500枠）、段階的拡張コストテーブル（200k〜999k）、アイテム売却（50%価格・単体および一括アトミック売却）、アイテム整頓（武器Kind 1 -> 防具Kind 2 -> アイテムKind 3 -> DefinitionID昇順）、アイテム・ゴールド郵送（2者間昇順行ロック `id.Sort2` によるデッドロックフリーな直接転送）、アイテム引出時の図鑑（Collection）自動登録、およびスタックアイテム預入時の枠数判定順序不具合（Issue #452）の解消を達成（Issue #460, #452）。横断的ランタイムプリミティブ `economy.TransactionRunner` / `ExecuteTransaction` による整合性担保とRank 2 (`characters`) -> Rank 3 (`inventory_items`) -> Rank 5 (`character_depots`) の行ロック階層遵守。
- **Blacksmith** (`internal/blacksmith`): 鍛冶屋（+1〜+10装備強化、成功率曲線、横断的ランタイムプリミティブ `economy.TransactionRunner` / `ExecuteTransaction` への移行完了。手動行ロック・SQLボイラープレートを完全排除し、Rank 2 (`characters`) -> Rank 3 (`inventory_items`) 決定論的ロック階層と費用・素材消費・インベントリ更新のアトミック整合性を保証）。
- **Alchemy** (`internal/alchemy`): 錬金術（112レシピ `recipes.json`）、素材合成（`TransactionProvider` と行ロックによる素材消費・合成物付与のアトミック整合性）。
- **Bank** (`internal/bank`): 銀行（預金・引出・プレイヤー間送金、`FOR UPDATE` 排他ロック）。
- **Inn** (`internal/inn`): 宿屋・休息（HP/MP全回復。横断的ランタイムプリミティブ `economy.TransactionRunner` / `ExecuteTransaction` へのパイロット移行完了。手動行ロック・SQLボイラープレートを完全排除し、決定論的ロック階層とHP/MP全回復のアトミック整合性を保証）。
- **Guild** (`internal/guild`): ギルド設立（5,000 G）、階層役職管理（Leader, Officer, Member）、加入・脱退・追放・役職変更・リーダー権限譲渡、ゴールド寄付によるEXP獲得とレベルアップ（最大Lv10 / 定員拡大、行ロックによるロストアップデート防止）、お知らせ掲示板、単一ギルド所属制約。
- **Casino** (`internal/casino`): カジノコイン両替（1 Coin = 20 G、横断的ランタイムプリミティブ `economy.TransactionRunner` / `ExecuteTransaction` への移行完了。手動行ロック・SQLボイラープレートを排除し、Rank 2 (`characters`) -> Rank 8 (`casino_accounts`) 決定論的ロック階層と通貨変換のアトミック整合性を保証）、インディアンポーカー（セッション永続化 `casino_poker_sessions`、進行中カードマスキング、コール/勝負/降り）、スロットマシン（3リール・5絵柄、777 100倍ジャックポット、レート設定）、ドッペルゲンガー（8種マーク一致・倍率設定）、ハイロー（大小予測、倍々モード）。`HighLowSession.Step()`（95.0%）および `PlayIndianPokerAction()`（95.0%）の単体テスト網羅率向上、重複エラー `ErrInsufficientCoin` の一本化、および孤立メソッド削除完了。
- **Lottery & Raffle** (`internal/lottery`): 福引（通常3枚・特賞〜6等・ハズレ、裏福引300枚・各色オーブ）、定期4桁数字宝くじ（1等100,000 Gジャックポット、下3桁/2桁/1桁返還、所有権認可・トランザクション安全な当籤受取処理）。
- **Farm & Plantation** (`internal/farm`): 4区画農園（種蒔き、水やり、肥料、実時間経過成熟判定・枯れ判定、収穫報酬精算）。Unit of Work トランザクション（`FOR UPDATE` 行ロック）によるアトミック化。
- **Auction & Marketplace** (`internal/auction`): プレイヤー間アイテム出品、入札時のゴールドエスクロー、高値更新時の自動返金、即決購入、出品期間満了時の自動精算、出品キャンセル（所有権認可 403 Forbidden）、`FOR UPDATE` 排他ロック。
- **Collection & Monster Book** (`internal/collection`): モンスター図鑑（討伐記録・コンプリート率計算）、アイテム図鑑（獲得アイテム・カテゴリ別記録・コンプリート率計算）。
- **Chapel & Blessings** (`internal/chapel`): 礼拝堂（シスターNPC、5種の原典準拠祈り（お金、強さ、モンスター仲間化率+50%、宝箱ドロップ、カジノコイン）、単一の祈り排他制約（「祈りに大事なのは、数でなく気持ちなのです」/ 409 Conflict）、架空の寄付機能 `POST /characters/{id}/chapel/donate` および `donation_gold_total` の完全撤廃）。
- **Player versus Player Arena** (`internal/pvp`): 闘技場・対人対戦（PvP、標準Eloレーティング K=32/初期1000、近傍マッチメイキング・同一アカウント談合防止、勝敗・対戦履歴・防衛ログ永続化、経験値・ゴールド報酬）。
- **Guild versus Guild Combat** (`internal/gvg`): ギルド対抗戦（GvG、標準Eloレーティング K=32/初期1000、5段階勝利メダル・王者杯昇格システム、ギルドポイントGP、ギルドEXP獲得・レベルアップ連動、対戦履歴永続化）。
- **King & World Boss Battles** (`internal/boss`): 封印戦・ワールドボス（全10段階キングボス＋太古の創世神Tier、レベル制限・前提段階クリア・1日3回挑戦制限、初回討伐ボーナス・ドロップ報酬、討伐数リーダーボード、挑戦履歴永続化、討伐時のイベント広場祝宴連動。ファイルサイズ上限遵守のため `catalog.go`, `records.go`, `battle.go`, `boss.go` に責務分割完了）。
- **Dungeon Exploration** (`internal/dungeon`): ダンジョン探索（多層グリッドマップ探索、モンスター遭遇戦闘、トラップ・宝箱イベント、階段降下、フロアボス決戦、一時報酬台帳バッファリングと脱出・踏破時の一括アトミック確定、全滅時戦利品没収、探索履歴永続化）。Valkey Master による進行中探索状態バッファリング（Candidate D、`party2:dungeon:{char:<id>}:state|rewards`、スライディング2時間TTL、アトミックLuaスクリプト `dungeon_step.lua`、探索中SQL書き込み完全ゼロ化、Two-Phase Settlement によるMariaDB確定後パージ）。`Move()`（44.9%→92.3%）のタイルイベント（階段・罠・宝箱・ボス戦闘・安全脱出・ターン切れ全滅）およびエラーパスの単体テスト網羅率向上完了。
- **Battle Replays & Match History** (`internal/replay`): 戦闘リプレイ・対戦履歴（全戦闘モードのターン別アクションログ・ダメージ値・残りHPスナップショットの記録・忠実再生、標準化レコーダー、プレイヤー別履歴・全体最新一覧（キーセット・カーソル対応）、自動プルーニング）。
- **Continuous Endurance Challenge** (`internal/challenge`): 連戦チャレンジ・サバイバル戦闘（全4段階Tier `challenge_tiers.json`、ラウンド進行に伴う累進スケーリング、インターラウンドHP回復、マイルストーンアイテムドロップ、途中撤退全額確定 vs 敗北50%救済、リーダーボード、所有権認可）。Valkey Master による進行中セッションバッファリング（Candidate D、`party2:challenge:{char:<id>}:session|rewards`、スライディング2時間TTL、アトミックLuaスクリプト `challenge_round.lua`、ラウンド進行中SQL書き込み完全ゼロ化、Two-Phase Settlement によるMariaDB確定後パージ）。
- **Custom Skill Gem Synthesis** (`internal/custom_skill`): 宝石箱から最大3個を選んで調合するオリジナルスキル（CMP/スロット制限、名前・セリフ検証、旧宝石返却を含むアトミック交換）。戦闘発動処理は別スコープ。
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`): 手助けクエスト（納品依頼、通常・レア・ギルド専用、錬金素材・幸福袋・GP報酬、有効依頼アイテムのショップ除外連携）および緊急救出処理（状態リセット、Valkey タスク自動キャンセル、クールダウン/睡眠ペナルティ）。
- **Town Park & Public Bulletin Board** (`internal/park`): 交流広場・公開掲示板（発言投稿・文字色指定・宛先指定・HTMLサニタイズ・レートリミット、最新投稿ページネーション（キーセット・カーソル対応）、NPC占い）。`TalkToNPC()`（50.0%→100.0%）および `Divinate()`（50.0%→100.0%）のキャラクター不在・リポジトリ障害エラーパスの単体テスト網羅率向上完了。
- **News & Player Notifications** (`internal/notification`): ニュース・お知らせ＆プレイヤー通知インボックス（全体告知、カテゴリ別お知らせ、プレイヤー別メッセージ受信箱、既読・未読管理、一括既読化、未読件数照会）。
- **Player Private Home & Mailbox** (`internal/home`): 自宅・私有地管理（壁紙・テーマ・一言設定、訪問者記録、手紙送受信・受信箱/送信箱（オフセット/カーソル対応）・未読件数、独立削除フラグ、仲間ペット言葉教え・挨拶会話、送金・譲渡通知台帳）。
- **Player Leaderboards & Character Rankings** (`internal/ranking`): ランキング・リーダーボード（12カテゴリ、決定論的タイブレーク・ページネーション、インメモリTTLキャッシュ、Valkey分散スナップショットキャッシュ、Singleflightキャッシュスタンピード抑止、定期更新Workerアクション、永続スナップショット `ranking_snapshots`）。
- **Event Plaza, Traveling Merchant Bazaar & Victory Banquets** (`internal/eventplaza`): イベント広場・行商人バザー＆ボス討伐祝宴（人口連動行商人Tier判定、希少アイテムバザーカタログ `bazaar.json`、アトミック購入トランザクション、ボス討伐連動祝宴・乾杯参加ゴールド報酬・重複乾杯防止 `banquet_toasts`、キャラクター所有権検証）。
- **Secret Underground Shop & NPC @ヒミツジ** (`internal/secretshop`): 秘密の店（資格判定 Lv15以上または転生者、希少消費アイテムカタログ `secret_items.json`、3倍価格プレミアム設定、アトミック購入トランザクション、NPC会話・詳細情報・ぱふぱふサービス回復）。
- **Adventurer's Tavern, Menu Orders, Delivery Reservations & NPC @エレナ** (`internal/tavern`): 冒険者の酒場（14種飲食メニューカタログ `menu.json`、HP/MP回復＆満腹度管理、購入時福引券ボーナス付与、冒険後自動回復デリバリー予約・受取・キャンセル機能、NPC会話。ファイルサイズ上限遵守のため `dialogue.go`, `delivery.go`, `order.go`, `tavern.go` に責務分割完了）。
- **Town Black Market, Contraband Trading, Dynamic Pricing & NPC @ヤミジ** (`internal/blackmarket`): 裏路地の闇市（資格判定 Lv10以上、10種禁制品カタログ `blackmarket_items.json`、4種市場相場状態、1日購入制限クォータ、レアアイテム捧げものリサイクル `SacrificeItem`、限定景品交換 `TradePrize`、アトミックトランザクション）。`GetMarketState()`（27.8%→100.0%）の時間帯ローテーション・リポジトリ障害フォールバック・相場既定値補完・カスタム値保持の単体テスト網羅率向上完了。
- **Town Delivery Quests & Player Courier Service** (`internal/delivery`): 町のでりばりー依頼＆プレイヤー間宅配便（NPC配送依頼、最大3件同時受領、報酬アトミック精算、およびプレイヤー間宅配便、手数料50 G、受取待ち・発送履歴（キーセット・カーソル対応）、受取・発送キャンセル/返金、CAS条件付きステータス更新 `WHERE id = ? AND status = 'pending'` による二重処理防止）。`CancelParcel()`（54.5%→87.9%）、`CompleteDelivery()`（71.0%→87.1%）、`ClaimParcel()`（73.2%→85.4%）のエラーパス・アイテム返還・ボーナス報酬受取フローの単体テスト網羅率向上完了。
- **Flea Market & Player Item Stalls** (`internal/fleamarket`): フリーマーケット＆露店取引（最大5件同時出品、1〜999,999 G固定価格出品、出品時インベントリ消費・キャンセル時安全返却、ID昇順排他ロックによるデッドロック防止、SQL CAS述語 `WHERE id = ? AND status = 'active'` と `RowsAffected() == 1` 検証によるアトミック移転）。
- **Gem Store, Jewel Synthesis & Appraisal** (`internal/gemstore`): 宝石店・宝珠/天珠販売・特殊合成加工・他プレイヤー譲渡・未鑑定宝珠鑑定（レベル別カタログ `gems.json`、55種以上の上位合成レシピ `recipes.json`、5種未鑑定宝珠の重み付きランダム鑑定プール `orb_appraisals.json`、決定論的行ロック階層によるアトミック整合性）。
- **Endgame God Wishes & Limit Breaks** (`internal/god`): 天界・裏天界の願い事＆限界突破（ステータス+40・所持金・メダル等の願い事、Lv99到達時レベル上限150限界突破 `over_level`、倉庫枠拡張 `over_depot`、モンスター預入枠拡張 `over_monster`、職業記憶枠拡張 `over_future`、フリマ出品上限拡張 `over_flea`、店舗出品上限拡張 `over_store`）。
- **Monster Grandpa & Pet Companions** (`internal/monster`): モンスター預かり所＆自宅ペット仲間（最大50〜300体預入 `character_monsters`、自宅ペット同居最大8体、命名制約、他プレイヤーへの譲渡、野生への解放、行ロックによるトランザクション整合性）。
- **Photo Contest, Screenshots & Gallery** (`internal/contest`): フォトコン会場（キャラクター別スクリーンショット保存・ギャラリー最大20枚、コンテストエントリー・題名バリデーション・連続制限、投票・応援コメント・自己投票禁止・1人1票、10日周期定期集計、上位3名賞金・メダル・GP付与、1位投票者メダル配布、歴代1位殿堂入り `contest_legends` 永久アーカイブ）。巨大リポジトリインターフェース（28メソッド）をISPに基づき5つの責務別サブインターフェース（Photo/Round/Entry/Vote/Legend）へ分割し、埋め込み合成インターフェース `ContestRepository` として再構築完了。
- **Multiplayer Party & Co-op Quests** (`internal/party`): パーティ結成・冒険（最大4人編成、合言葉パスワード、参加条件バリデーション、Ready同期、リーダー権限（キック・解散）、協力戦闘解決、シナジーボーナス、報酬分配、HP1生存保証）。Valkey Master による待機ロビー管理（`party2:party:lobby:<party_id>` 15分TTL自動失効、60秒Readyカウントダウン、ZSETロビー一覧）、アトミックLuaスクリプト、MariaDB `party_adventure_logs` への恒久冒険ログ永続化。
- **Altar of Rebirth & Ramia Awakening** (`internal/altar`): 復活の祭壇（6色のオーブ（s, r, b, g, y, p）奉納、伝説の不死鳥ラーミァ復活祈り `@いのる`・30分間滞在記録 `altar_ramia_awakenings`、4種の異世界旅行アイテム願い `@ねがう`（真実の鏡、マダムの招待状、宝の地図、闇のランプ）、インベントリ上限時の預かり所（Depot）自動転送、オーブ状態クリア、決定論的行ロック階層（`characters` -> `inventory_items` -> `character_depots` -> `altar_ramia_awakenings`）による完全アトミック整合性）。
- **Wishing Well & SP Stat Growth** (`internal/wishingwell`): 願いの泉（@女神、`sp_change.cgi`）。スキルポイント（SP）を捧げて基礎能力値（MHP・MMPは1 SPにつき+2、攻撃・守備・素早さは1 SPにつき+1）を恒久的に成長させる原典仕様の完全再現。思い出し中（JobMemory active）および天界限界突破（OverLevel）時の利用制限ガード、ステータス上限クランプ、決定論的行ロック階層（Rank 2 `characters`）によるアトミック整合性担保。
- **System Maintenance Mode** (`internal/maintenance`): メンテナンスモード管理（`GET /maintenance`, `POST /admin/maintenance`, `PUT /admin/maintenance`、管理者APIキーによる有効化/無効化・告知メッセージ・終了予定時刻設定、HTTPミドルウェアによる503 Service Unavailable遮断、Valkey Master / In-Memory キャッシュによる毎リクエストのSQLクエリ排除、MariaDBバックアップ `system_maintenance`）。

### API & Transport
- **Server Entrypoint, Configuration & Lifecycle Orchestration** (`cmd/party2`): 構成分離（`config.go`, `main.go`, `services_core.go`, `services_econ.go`, `services_cmbt.go`, `services_soc.go`, `services_misc.go`, `wire.go`）、型付けされた設定構造体インジェクション（`database.Config`, `valkey.Config`, `Config`）による並行テスト分離（`t.Parallel()` 完全対応）、MariaDB・Valkey・全ドメインリポジトリおよびサービス・スケジューリングWorker・HTTP APIルーター（全35種Option）の統合初期化、ドメインイベントフック一元集約（`wire.go`）、Graceful Shutdown（`http.Server.Shutdown(ctx)`、Worker Contextキャンセル待機、リソース安全開放）、起動・停止のJSON構造化ログ。
- **HTTP JSON API & OpenAPI 3.1 Specification** (`internal/api/http`, `docs/api/base.json`, `docs/api/paths/*.json`): Go標準 `net/http` によるREST風エンドポイント（全203ルート・221オペレーション）。モジュール分割仕様（40ファイル）と自動バンドル（`docs/api/openapi.json` およびバイナリ埋め込み）、CI自動テストによるASTベースのルート網羅率100%検証、セッション認証およびPAT（APIキー）デュアル認証、管理者APIキー認可（`X-Admin-Key`、定数時間比較）、キャラクター所有権認可検証（403 Forbidden、全サブリソースIDOR防御）、標準セキュリティヘッダー、CORSミドルウェア、Valkey/In-Memory 分散レートリミット（429 Too Many Requests、ValkeyLimiter 96.9%・extractClientIP 100% カバレッジ担保）、メンテナンスモードミドルウェア（503 Service Unavailable）。

### Infrastructure & Operations
- **Database**: MariaDB（マイグレーション `migrations/001_initial.sql` 〜 `057_altar_of_rebirth.sql`、`make db-migrate` / `make db-reset`、永続権威 MariaDB Master、コネクションプール設定 `MaxOpenConns`・`MaxIdleConns`・`ConnMaxLifetime`・`ConnMaxIdleTime` の環境変数設定対応）。
- **Valkey**: 遅延アクションキュー・排他ロック・分散レートリミット・ランキングスナップショットキャッシュ（AOF+RDB永続化）。RFC #356 に基づく揮発性ステートのプライマリストア（Valkey Master: セッション、メンテナンス状態、待機ロビー、ダンジョン・連戦進行中バッファ）境界策定。統一キー空間仕様（SSOT: `docs/architecture/valkey-keyspace.md`）策定および AST 機械検証（`internal/architecture/valkey_lint_test.go`）。Lua スクリプト運用基準・Hash Tagging 規約・インメモリ等価性 SSOT 策定。一時ランバッファ（Candidate D: ダンジョン探索 #404・連戦サバイバル #405 完了）のValkey Master移行およびLuaスクリプト契約（SSOT: `docs/architecture/transient-run-state.md`）。ワールドボスHPのリアルタイム共有HP低減PoC完了（SSOT: `docs/architecture/transient-boss-hp.md`）。外部Valkeyコンテナ不要のオフラインモックハーネス（`valkeytest.MockClient`, `valkeytest.NewBuilder`, RESPビルダー群 `MakeIntSliceResult`, `MakeStringResult`, `MakeErrorResult` 等）を `internal/testutil/valkeytest` として標準化完了（Issue #444）。
- **Logging**: Go標準 `log/slog` によるJSON構造化ログ、秘密情報自動マスキング。
- **Verification**: `Makefile` (`make check`, `make fmt`, `make vet`, `make lock-lint`, `make openapi-sync`, `make openapi-check`, `make openapi-scaffold`, `make test-stress`, `make bench`, `make check-clean`)、OpenAPI 3.1 仕様書自動同期 CLI（`scripts/sync_openapi.go`）、CIガード、Go AST 静的解析テストスイート（トランザクション伝播、行ロック階層順序、サービス層 `RunInTx`、Valkey キー空間仕様＆`KEYS *` 禁止、Luaスクリプト外部ファイル化＆埋め込み保証、HTTP 所有権認可、Core ドメイン不変条件、未参照・孤立メソッド／未使用定数／未使用DTO構造体フィールドのデッドコード機械検知、本番ファイル行数・エントリポイントサイズ制約（生産 ≤ 500行、main.go ≤ 150行）のラチェット自動検査、全リンターへの高速バイト事前フィルタ適用）。
- **Deployment**: Distroless (`gcr.io/distroless/static-debian13:nonroot`) ベースの最小本番イメージ（GHCR自動公開）。

---

## Immediate Priorities (Next Actions)

1. **Transient State Migration & Refactoring**:
   - Issue #411: Establish cross-domain application runtime primitives to abstract currency, item, locking, and event rules
2. **Client Presentation & Web UI**:
   - Issue #140: Web Presentation UI and browser client implementation
3. **Production Asset Pipeline & Final Licensing**:
   - Issue #143, #202: Production visual asset creation, mapping, and license attribution catalog

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
