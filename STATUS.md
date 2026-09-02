# Status
 
Last updated: Issue #327 — Design and introduce keyset / cursor-based pagination for high-volume endpoints

## Current phase

**Version 1.0 Reconstruction / Refactoring — In Progress**

Phase 0〜4（ゲーム理解・アーキテクチャ・ドメインモデル・骨格・垂直スライス）は完了しています。

現在は **Phase 5+（個別機能の段階的再構築）** にあり、Version 1.0に必要な主要ゲームシステムをクリーンルーム再構築として新規実装しています。
Version 1.0の完成条件は、既存プロジェクトの意味のあるゲーム機能を新規実装として再構築し、必要な画像を新規制作または承認済みプレースホルダーで準備することです。旧ソースコード・旧画像の移植は完成条件に含めません。

---

## Current Component State (What is True Now)

### Architecture & Repository Intelligence (Guidance Layer - PoC)
- **Guidance Layer (.arch/)**: シンボルアンカー（`path#Symbol`）ベースのモジュール詳細定義（`.arch/modules/*.json`）および高 Fan-in 共有テーブルの逆引きインデックス（`.arch/shared_tables/*.json`、`characters`, `inventory_items`, `bank_accounts`, `guilds`）、GitHub ネイティブな Mermaid 全体トポロジー図（`docs/architecture/guidance-layer.md`）。外部 Node.js 依存や HTML 成果物を全廃し、純粋な Go + JSON + Markdown で完結。影響範囲・ロック順序をゼロトークンで特定しコンテキストトークン消費を 90% 削減。
- **Module Selection Criteria & Target Tiers**: 4つの選定基準（C1: トランザクション深度, C2: 行ロック階層, C3: エスクロー/共有状態, C4: 非同期Worker）に基づくトリアージを実施（`docs/architecture/guidance-layer.md`）。Tier 1（高リスク8機能: `tavern`, `delivery`, `bank`, `auction`, `guild`, `shop`, `blacksmith`, `adventure`）、Tier 2（オンデマンド）、Tier 3（除外）の運用スコープを確立。
- **Automated Mechanical Verification**: Go AST シンボルリント（`internal/architecture/arch_test.go`）による 0.05 秒の静的シンボル実在性チェック（モジュール定義および共有テーブル逆引き定義）およびトランザクション境界シンボルにおける `RunInTx` 呼び出しの実在検証（`go test ./...` および `scripts/verify.sh` step `[4/7]` 統合）。外部ランタイム不要の純粋な Go 標準構文解析器による機械的テスト。



### Core & Shared Components
- **Player** (`internal/core/player`, `internal/player`): アカウント登録・パスワードハッシュ・セッション管理・アカウント完全削除（`DELETE /players/me`, `DELETE /players/{id}`、パスワード再認証、所有キャラクター全件のクリーンアップフック実行および35+テーブル連鎖削除、MariaDBトランザクション整合性）。
- **Character** (`internal/core/character`, `internal/character`): `player_id` 外部キーによるアカウント紐付け、初期ステータス、能力値計算、転生（Rebirth +5永続ボーナス）、プレイヤー別キャラクター一覧取得、キャラクター個別削除（`DELETE /characters/{id}`、所有権認可チェック、外部ドメイン `CleanupHook` 実行、MariaDB 35+サブリソーステーブルの完全カスケード削除）、命名の館（NPC `@マリナン`、名前変更 500,000 G・ギルド/フリマ制約・重複防止・ニュース配信、性別/外観変更 10,000 G）、プロフィール自己紹介コメント（最大160文字）・アバター画像アップロード/設定（最大2 MB）・カスタムメタデータ管理（`character_profiles`）。
- **Progression** (`internal/core/progression`): レベルアップ（累積経験値テーブル `level * level * 10`）、OverLevel限界突破（Lv150）対応、成長率適用、Go AST 静的解析リンター（`internal/core/progression/progression_lint_test.go`）による全機能モジュールでの直接フィールド操作禁止・Core標準ヘルパー（`progression.ApplyExperience`）強制。
- **Job & Skill** (`internal/core/job`, `internal/job`, `internal/core/skill`): クリーンルーム規約に完全準拠したJSONカタログ（`jobs.json`、特定フランチャイズ固有語を排除し汎用ファンタジー名へ標準化）、転職、Lv99マスタリー、スキル発動・コスト計算。
- **Item, Inventory, Equipment** (`internal/core/item`, `internal/inventory`, `internal/equipment`): 5カテゴリJSONカタログ（武器・防具・盾・アクセ・消費/素材）、スロット装備、所持枠管理、統一アイテム定義プロバイダーインターフェース（`coreitem.DefinitionProvider` / `coreitem.ItemDefinitionProvider`）の一元化。
- **Battle** (`internal/core/battle`): 決定論的ターン制戦闘解決、勝敗・報酬決定（経験値・ゴールド・アイテム・ちいさなメダル）、構造化ターンログ出力、戦闘参加者（Participant）標準アダプタ/ビルダー（`NewParticipantFromCharacter`, `NewParticipantFromCharacterWithHP`, `ParticipantBuilder`）。
- **Scheduling** (`internal/core/scheduling`, `internal/scheduling`): Valkeyバックエンドの遅延アクションキュー＆分散排他ロックWorker。
- **Database & Transaction Orchestration** (`internal/database`): 全32リポジトリのトランザクション伝播モデル（`RunInTx` と `ExecutorFromContext`）への完全統一、トランザクション境界外からの直接 `r.db.BeginTx` 呼び出しの完全排除、コンテキスト内トランザクション再利用、マルチモジュール統合テスト（コミット原子性・ロールバック整合性・ネストトランザクション伝播検証）、決定論的行ロック獲得順序（`players` -> `characters` (昇順) -> `inventory_items` -> `character_jobs` -> `character_depots` -> `bank_accounts` -> `guilds` (昇順) -> 各種機能テーブル）によるデッドロック防止の標準化、および高並行性ストレステスト・デッドロック検出ベンチマークスイート（`internal/database/concurrency_stress_test.go`、`make test-stress`）による50並行ワーカー・1,000複合トランザクション負荷下での0デッドロック・データ保存不変条件の自動検証。さらに、全サブリソースリポジトリ（連戦セッション・宝くじ・オークション出品・ダンジョン探索・私有地手紙/挨拶台帳・祝勝祝宴乾杯台帳等）における所有権スコープ付きSQL（`WHERE id = ? AND character_id = ?`）の厳格適用によるIDOR完全防止。
- **Standardized Pagination & Common Utilities** (`internal/pagination`, `internal/id`, `internal/validation`, `internal/api/http/middleware`): 単一責務の共通パッケージ配置方針（Rule of Three、セキュリティ/認可の即時共通化指針）および暗号学的に安全なID生成ユーティリティ（`internal/id`）。汎用ジェネリックページネーションパッケージ（`internal/pagination`）によるリクエストパラメータ正規化（`Normalize`, `Parse`, `ParseRequest`, `ParseCursorRequest`）、標準オフセットレスポンスコンテナ（`Page[T]` `{items, total, limit, offset}`）、および高スループットストリーム向けキーセット/カーソルレスポンスコンテナ（`CursorPage[T]` `{items, next_cursor, prev_cursor, limit, has_more}`、不透明Base64カーソルトークン暗号化・復号化）、全HTTP API一覧エンドポイントにおける統一エンベロープ適用および OpenAPI 3.1 仕様完全同期。


### Feature Modules
- **Activity** (`internal/activity`): 訓練機能（Valkey Worker push型＋手動Claimフォールバック）。
- **Adventure** (`internal/adventure`): 28ステージ（`stages.json`）、286体モンスター（`monsters.json`）、戦闘解決、ドロップ報酬（メダル含む）、Valkey Worker連携、過去冒険履歴一覧（`GET /characters/{id}/adventures`）、冒険戦績クロニクル・ステージ別クリア統計・マイルストーンアンロック状態（`GET /characters/{id}/adventure-chronicle`、トライモード/イメージ設定/カームモード/ハードモード/アバター設定/エクストリームモード）。
- **Medal** (`internal/medal`): 小さなメダル交換所（減算消費方式、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによるメダル消費・アイテム付与の完全アトミックトランザクション整合性・並行性レース防止）およびダンジョン宝箱・踏破、ワールドボス討伐・初回クリアによるメダル獲得連携。
- **Shop** (`internal/shop`): アイテム売買（50%売却）、1回あたり最大取引数量制限（`MaxTransactionQuantity = 9999`）、整数オーバーフロー安全乗算（`safeMultiply` / `ErrPriceOverflow`）、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる購入・売却のアトミックトランザクション整合性・並行性レース保護。
- **Depot** (`internal/depot`): 倉庫（アイテム・ゴールド預入・引出）、トランザクション整合性。
- **Blacksmith** (`internal/blacksmith`): 鍛冶屋（+1〜+10装備強化、成功率曲線、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる費用・強化素材消費・インベントリ更新の完全アトミックトランザクション整合性・二重消費防止）。
- **Alchemy** (`internal/alchemy`): 錬金術（112レシピ `recipes.json`）、素材合成（`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる必要素材消費・合成物付与の完全アトミックトランザクション整合性・並行合成競合防止）。
- **Bank** (`internal/bank`): 銀行（預金・引出・プレイヤー間送金、`FOR UPDATE` 排他ロック）。
- **Inn** (`internal/inn`): 宿屋・休息（HP/MP全回復、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる宿泊費減算と全回復の完全アトミックトランザクション整合性・残高オーバードラフト防止）。
- **Guild** (`internal/guild`): ギルド設立（5,000 G）、階層役職管理（Leader, Officer, Member）、加入・脱退・追放・役職変更・リーダー権限譲渡、ゴールド寄付によるEXP獲得とギルドレベルアップ（最大Lv10 / 定員拡大、MariaDB `SELECT ... FOR UPDATE` 行ロックによる並行寄付時のロストアップデート完全防止）、お知らせ掲示板、単一ギルド所属制約。
- **Casino** (`internal/casino`): カジノコイン両替（1 Coin = 20 G）、インディアンポーカー（52枚標準トランプモデル、ブラインド賭け、NPCディーラーAI、最大5ラウンド・レート上昇、ショーダウン勝敗判定・配当精算）、スロットマシン（3リール・5絵柄、777 100倍ジャックポット、レート設定 $1〜$200）、ドッペルゲンガー（8種マーク一致・秘密選択、4x/6x/8x 倍率設定）、ハイロー（トランプ数字大小予測、2倍配当、連勝継続倍々モード）。全ゲームにおいて `DeductBetAndCreditPayout` による条件付きアトミックベット減算・配当付与トランザクション処理と MariaDB 行レベルロックによって並行性エクスプロイト（残高0での無限無料スピン・不正配当獲得）を完全防止。
- **Lottery & Raffle** (`internal/lottery`): 福引（通常3枚・特賞〜6等・ハズレ、裏福引300枚・各色オーブ）、定期4桁数字宝くじ（1等100,000 Gジャックポット、下3桁・下2桁・下1桁返還、所有権認可・トランザクション安全な当籤受取処理 `WHERE id = ? AND character_id = ?`）。
- **Farm & Plantation** (`internal/farm`): 4区画農園（薬草・マンドラゴラ・月光草・黄金の果実の種蒔き、水やり収穫数+1、肥料成長時間半減、実時間経過成熟判定・枯れ判定、収穫報酬精算）。MariaDB Unit of Work トランザクション（`FOR UPDATE` 行ロック）による種・肥料アイテム消費とプロット状態遷移の完全アトミック化により、前提条件不備時のアイテム空消費を防止し並行植え付けのレースコンディションを完全解消。
- **Auction & Marketplace** (`internal/auction`): プレイヤー間アイテム出品（開始価格・即決価格・出品期間）、入札時のゴールドエスクロー、高値更新時の自動即時返金、即決購入（即時成立・売上送金）、出品期間満了時の自動落札・返却精算、出品キャンセル（出品者所有権認可 403 Forbidden）、`FOR UPDATE` 排他ロック。
- **Collection & Monster Book** (`internal/collection`): モンスター図鑑（討伐記録・初回/最新討伐日時・コンプリート率計算）、アイテム図鑑（獲得アイテム・カテゴリ別記録・コンプリート率計算）。
- **Chapel & Blessings** (`internal/chapel`): 教会（祈り・祝福登録、ゴールド寄付、戦闘・冒険報酬バフ補正計算）。
- **Player versus Player Arena** (`internal/pvp`): 闘技場・対人対戦（PvP、標準Eloレーティング K=32/初期1000、近傍マッチメイキング・同一アカウント談合防止、勝敗・対戦履歴・防衛ログのMariaDB永続化、経験値・ゴールド報酬付与）。
- **Guild versus Guild Combat** (`internal/gvg`): ギルド対抗戦（GvG、標準Eloレーティング K=32/初期1000、5段階勝利メダル・王者杯昇格システム、ギルドポイントGP、ギルドEXP獲得・レベルアップ連動、対戦履歴・ラウンド詳細ログのMariaDB永続化）。
- **King & World Boss Battles** (`internal/boss`): 封印戦・ワールドボス（全10段階キングボス＋太古の創世神Tier、レベル制限・前提段階クリア・1日3回挑戦制限、初回討伐ボーナス・ドロップ報酬、討伐数（英雄度）・最高到達Tierリーダーボード、挑戦履歴のMariaDB永続化、討伐時のイベント広場祝宴連動）。
- **Dungeon Exploration** (`internal/dungeon`): ダンジョン探索（多層グリッドマップ探索、モンスター遭遇戦闘、トラップ・宝箱イベント、階段降下、フロアボス決戦、一時報酬台帳バッファリングと脱出・踏破時の一括アトミック確定、全滅時の戦利品没収、探索履歴・踏破記録のMariaDB永続化、探索者キャラクターIDスコープ検証）。
- **Battle Replays & Match History** (`internal/replay`): 戦闘リプレイ・対戦履歴（全戦闘モードのターン別アクションログ・ダメージ値・残りHPスナップショットの記録・忠実再生、標準化レコーダー `ReplayRecorder` / `RecordMatchFromResult` / `RecordCharacterVsCharacter` / `RecordCharacterVsMonster` / `RecordParticipantVsParticipant`、プレイヤー別対戦履歴・全体最新リプレイ一覧、保持期間経過レコードの自動プルーニング、MariaDB永続化）。
- **Continuous Endurance Challenge** (`internal/challenge`): 連戦チャレンジ・サバイバル戦闘（初級・中級・上級・奈落の全4段階Tier JSONカタログ `challenge_tiers.json`、ラウンド進行に伴うステータス累進スケーリング、インターラウンド20% HP回復、5連勝区切りのマイルストーンアイテムドロップ、途中撤退による全額確定精算 vs 敗北時の50%救済精算・アイテム没収、最高到達ラウンド別リーダーボード、挑戦セッション所有権認可・`ErrForbidden` 403 Forbidden、MariaDB永続化）。
- **Custom Skill Loadout & Slot Management** (`internal/custom_skill`): カスタムスキル・スロット管理（JSONスキルカタログ `skills.json`、現在職・マスター職・宝石汎用スキルの装備制限バリデーション、スロット枠数管理、発動優先度 1〜10、重複装備防止、全戦闘モード向けアクティブスキル供給、MariaDB永続化）。
- **Player Rescue & Helper Quests** (`internal/helper`, `internal/rescue`): 手助けクエスト（何でも屋 @リッカによる武器・防具・道具・モンスター納品依頼、通常・レア・ギルド専用クエスト、錬金素材・幸福袋・GP報酬、6日間期限、有効依頼アイテムのショップ除外連携）および緊急救出処理（スタック時・エラー時の状態リセット、Valkey スケジューリングタスク自動キャンセル連携、クールダウン/睡眠ペナルティ、MariaDB永続化）。
- **Town Park & Public Bulletin Board** (`internal/park`): 交流広場・公開掲示板（プレイヤー発言投稿・文字色指定・宛先指定・HTMLサニタイズ・連投レートリミット、最新投稿ページネーション、@町娘NPC会話・20種運勢＆27色ラッキーカラー占い、MariaDB永続化）。
- **News & Player Notifications** (`internal/notification`): ニュース・お知らせ＆プレイヤー通知インボックス（全体告知 `news.cgi`、カテゴリ別お知らせ配信、プレイヤー別非同期メッセージ受信箱、既読・未読管理、一括既読化、未読件数照会、MariaDB永続化）。
- **Player Private Home & Mailbox** (`internal/home`): 自宅・私有地管理（`home.cgi`、壁紙・テーマ・一言設定、訪問者記録、プレイヤー間手紙送受信・受信箱/送信箱・未読件数、送信者・受信者の独立削除フラグ `is_deleted_by_sender`/`is_deleted_by_recipient` と双方削除時の完全パージ、仲間ペット言葉教え・挨拶会話、送金・譲渡通知台帳、MariaDB永続化）。
- **Player Leaderboards & Character Rankings** (`internal/ranking`): ランキング・リーダーボード機能（`ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`、レベル・プレイヤー総資産・キャラクター所持金・戦闘通算勝利数・PvP闘技場勝利数・ボス討伐数・冒険勝利数・職マスター数・職業人気分布・手助け達成数・転生回数・ちいさなメダル所持数の12カテゴリ、決定論的タイブレーク・ページネーション、インメモリTTLキャッシュ、Valkey分散スナップショットキャッシュ、Singleflightキャッシュスタンピード抑止、バックグラウンドWorker定期更新アクション `party2:ranking:refresh`、永続スナップショット `ranking_snapshots`、MariaDB永続化）。
- **Event Plaza, Traveling Merchant Bazaar & Victory Banquets** (`internal/eventplaza`): イベント広場・行商人バザー＆ボス討伐祝宴（人口連動行商人Tier判定、希少アイテム・素材・装備バザーカタログ `bazaar.json`、並行性安全なアトミック購入トランザクション、キングボス討伐連動の祝勝祝宴自動開催・24時間有効期限・乾杯参加ゴールド報酬・重複乾杯防止 `banquet_toasts`、全変異エンドポイントにおけるセッション認証およびキャラクター所有権検証によるIDOR完全防止、MariaDB永続化）。
- **Secret Underground Shop & NPC @ヒミツジ** (`internal/secretshop`): 秘密の店・羊NPC @ヒミツジ（資格判定 Lv15以上または転生者、希少消費アイテム・アクセサリカタログ `secret_items.json`、3倍価格プレミアム設定、手助け依頼アイテム除外フィルタ連携、並行性安全なアトミック購入トランザクション、NPC羊会話・詳細情報・@ぱふぱふサービスと微小回復 HP+10/MP+5）。
- **Adventurer's Tavern, Menu Orders, Delivery Reservations & NPC @エレナ** (`internal/tavern`): 冒険者の酒場・看板娘NPC @エレナ（14種飲食メニューカタログ `menu.json`、HP/MP回復＆満腹度管理、購入時福引券ボーナス付与、冒険後自動回復用デリバリー予約・受取・キャンセル機能、NPC会話、MariaDB永続化）。
- **Town Black Market, Contraband Trading, Dynamic Pricing & NPC @ヤミジ** (`internal/blackmarket`): 裏路地の闇市・闇ブローカーNPC @ヤミジ（資格判定 Lv10以上、10種禁制品アイテムカタログ `blackmarket_items.json`、4種ダイナミック市場相場状態 `Quiet`, `HotDemand`, `Crackdown`, `Bargain` と価格・買取倍率補正、1日購入制限クォータ、レアアイテム・超レアアイテム捧げものリサイクルシステム `SacrificeItem` によるレアポイント/裏レアポイント獲得、限定武器・防具・装飾品・消費アイテム景品交換 `TradePrize`、並行性安全なアトミック購入・売却・捧げもの・交換トランザクション、NPC会話・相場噂話情報、MariaDB永続化 `blackmarket_character_purchases`, `blackmarket_market_state`, `blackmarket_character_points`）。
- **Town Delivery Quests & Player Courier Service** (`internal/delivery`): 町のでりばりー依頼＆プレイヤー間宅配便（薬草・ポーション・装備等のNPC配送依頼、最大3件同時受領、インベントリ消費・ゴールド/EXP/アイテム報酬アトミック精算、およびプレイヤー間アイテム/ゴールド宅配便、50 G手数料、受取・発送キャンセル/返金、`GetParcelByIDForUpdate` および CAS 条件付きステータス更新 `WHERE id = ? AND status = 'pending'` による受取/キャンセル並行実行時の二重支払い・複製レースコンディション完全防止、MariaDB永続化）。
- **Flea Market & Player Item Stalls** (`internal/fleamarket`): フリーマーケット＆プレイヤー露店取引（`free.cgi`、最大5件同時出品、1〜999,999 G固定価格出品、出品時インベントリ消費・キャンセル時安全返却、`GetListingByIDForUpdate` および買い手・売り手キャラクターID昇順ソート排他ロックによる並行購入時のデッドロック完全防止とゴールド・アイテムアトミック移転、MariaDB永続化 `fleamarket_listings`）。
- **Gem Store, Jewel Synthesis & Appraisal** (`internal/gemstore`): 宝石店・宝珠/天珠販売・特殊合成加工・他プレイヤー譲渡・未鑑定宝珠鑑定（`gem_store.cgi`, `_data.cgi` No. 251–255, NPC `@ジェマ`、レベル別カタログ `gems.json`、55種以上の上位宝石合成レシピ `recipes.json`、5種未鑑定宝珠の重み付きレガシー準拠ランダム鑑定プール `orb_appraisals.json`、スレッドセーフRNG/決定論的シード注入、50%売却、自身送信防止＆昇順ロック譲渡、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる完全アトミックトランザクション整合性）。
- **Endgame God Wishes & Limit Breaks** (`internal/god`): 天界・裏天界の願い事＆限界突破システム（`god.cgi`, `u_god.cgi`, NPC `@神`, `@神?`、ステータス+40・所持金・小さなメダル・全快等の天界願い事、Lv99到達時レベル上限150限界突破 `over_level`、倉庫預入枠拡張 `over_depot`、モンスター預入枠拡張 `over_monster`、職業記憶枠拡張 `over_future`、フリマ出品数上限拡張 `over_flea`、店舗出品数上限拡張 `over_store`、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる完全アトミックトランザクション整合性）。
- **Monster Grandpa & Pet Companions** (`internal/monster`): モンスター預かり所＆自宅ペット仲間システム（`farm.cgi` / `monster.cgi`, NPC `@モンジィ`、最大50体〜限界突破時最大300体預入 `character_monsters`、自宅ペット同居最大8体 `MaxHomePets = 8`、同居ペット同名重複チェック、命名文字制約、他プレイヤーへのモンスター譲渡、野生への解放、`TransactionProvider` と `SELECT ... FOR UPDATE` 行ロックによる完全アトミックトランザクション整合性）。
- **Photo Contest, Screenshots & Gallery** (`internal/contest`): フォトコン会場・NPC `@ワコール`（`photo.cgi` / `contest.cgi`、キャラクター別スクリーンショット保存・ギャラリー最大20枚、コンテストエントリー・題名バリデーション・連続エントリー制限、開催中コンテスト作品への投票・応援コメント・自己投票禁止・1人1票制約、10日周期定期集計・5作品未満延期、上位3名賞金15,000/7,000/3,000 G・小さなメダル10/6/3枚・ギルドポイント700/300/100 GP付与、1位投票者全員への小さなメダル1枚配布、歴代1位作品の殿堂入り `contest_legends` 永久アーカイブ、結果発表お知らせ配信、`TransactionProvider` と行ロックによる完全アトミックトランザクション整合性・MariaDB永続化）。
- **Multiplayer Party & Co-op Quests** (`internal/party`): パーティ結成・冒険中のパーティー（`quest.cgi`, `party.cgi`、最大4人編成、パーティ名バリデーション、合言葉パスワード設定、参加条件チェック（Lv上限・下限、HP下限）、単一アクティブパーティ制約、準備完了（Ready）同期、リーダー権限（キック・解散）、マルチプレイヤー協力戦闘解決（`internal/core/battle` 拡張）、協力シナジーボーナス（+10%〜+30% EXP/ゴールドボーナス）、報酬分配（レベルアップ・OverLevel限界突破（Lv150）対応・ドロップ品一括付与）、冒険ログ永続化、`TransactionProvider` と行ロックによる完全アトミックトランザクション整合性・MariaDB永続化 `parties`, `party_members`, `party_adventure_logs`）。
- **System Maintenance Mode** (`internal/maintenance`): メンテナンスモード管理（`GET /maintenance`, `POST /admin/maintenance`, `PUT /admin/maintenance`、管理者APIキーによる有効化/無効化・告知メッセージ・終了予定時刻設定、HTTPミドルウェアによる503 Service Unavailable遮断、`/health`, `/openapi.json`, `/maintenance` および管理者バイパス、MariaDB永続化 `system_maintenance`）。

### API & Transport
- **Server Entrypoint & Lifecycle Orchestration** (`cmd/party2`): MariaDB・Valkey・全ドメインリポジトリおよびサービス・スケジューリングWorker・HTTP APIルーター（全34種Option）の統合初期化、`ADDR` / `PORT` 環境変数解決（デフォルト `:8080`）、GoroutineベースのHTTPサーバー＆Worker実行、OSシグナル（`SIGINT`, `SIGTERM`）受信時のタイムアウト付きGraceful Shutdown（`http.Server.Shutdown(ctx)`、Worker Contextキャンセル待機、DB/Valkeyリソース安全開放）、起動・停止のJSON構造化ログ。
- **HTTP JSON API & OpenAPI 3.1 Specification** (`internal/api/http`, `docs/api/openapi.json`): Go標準 `net/http` によるREST風エンドポイント（全182ルート：`/health`, `/openapi.json`, `/maintenance`, `/admin/maintenance`, `/players`, `/players/me`, `/players/{id}`, `/sessions`, `/characters`, `/characters/{id}`, `/jobs`, `/characters/{id}/change-job`, `/characters/{id}/rebirth`, `/characters/{id}/inn`, `/characters/{id}/custom-skills*`, `/characters/{id}/chapel*`, `/characters/{id}/farm*`, `/characters/{id}/collections/*`, `/characters/{id}/lottery/*`, `/characters/{id}/casino/*`, `/challenges/*`, `/characters/{id}/bosses*`, `/characters/{id}/dungeons*`, `/characters/{id}/pvp*`, `/auctions*`, `/fleamarket/listings*`, `/characters/{id}/fleamarket/listings*`, `/gemstore/*`, `/characters/{id}/gemstore/*`, `/god/*`, `/characters/{id}/god/*`, `/monster/*`, `/characters/{id}/monsters*`, `/contest/*`, `/characters/{id}/photos*`, `/characters/{id}/contest/*`, `/naming-hall/*`, `/characters/{id}/name`, `/characters/{id}/gender`, `/characters/{id}/profile`, `/characters/{id}/avatar`, `/characters/{id}/adventures`, `/characters/{id}/adventure-chronicle`, `/adventures`, `/parties*`, `/shop/*`, `/park/*`, `/medals/*`, `/helpers/*`, `/rescues/*`, `/news/*`, `/notifications/*`, `/homes/*`, `/letters/*`, `/rankings/*`, `/eventplaza*`, `/characters/{id}/secretshop*`, `/tavern/*`, `/characters/{id}/tavern*`, `/characters/{id}/blackmarket*`, `/characters/{id}/delivery*`）。全エンドポイントを網羅した機械可読 OpenAPI 3.1.0 スキーマ仕様（`docs/api/openapi.json`）の提供と `GET /openapi.json` による常時配信、CI自動テスト（`internal/api/http/openapi_test.go`）によるルート網羅率100%検証・スキーマバリデーション・ドキュメント同期ガード、および Go AST 静的解析テスト（`internal/api/http/auth_lint_test.go`）によるキャラクター・プレイヤー操作全エンドポイントでの標準認証・所有権検証ラッパー適用の自動機械検証。セッション認証、管理者APIキー認可（`X-Admin-Key` / `Authorization: Bearer <key>`、タイミング攻撃耐性をもつ定数時間比較、`POST /news`, `POST /rankings/refresh`, `POST /contest/settle`, `POST /admin/maintenance` を保護）、キャラクター・プレイヤー所有権認可検証（403 Forbidden、アカウント削除・キャラクター削除・連戦セッション・宝くじ・冒険Claim・冒険クロニクル・オークション出品取消・宅配便・フリマ出品取消・宝石店操作・天界願い事操作・モンスター預かり所/ペット操作・フォトコン作品/写真操作・名前変更/性別変更/プロフィール更新/アバターアップロード・パーティ結成/加入/脱退/キック/解散/Ready/出発・イベント広場購入/乾杯等のサブリソースIDOR完全防御）、標準セキュリティレスポンスヘッダー（nosniff, DENY, strict-origin-when-cross-origin, CSP none）、CORS ポリシーミドルウェア（許可 Origin ホワイトリスト、`OPTIONS` プリフライトキャッシュ、`*` ワイルドカード抑止）、Valkey/In-Memory 分散レートリミットミドルウェア（公開認証エンドポイント・一般エンドポイント別制限、429 Too Many Requests、Retry-After / X-RateLimit-* ヘッダー、fail-open耐障害性）、メンテナンスモードミドルウェア（503 Service Unavailable、システム/管理者バイパス）、64 KiB リクエスト制限、未知フィールド拒否、構造化エラーレスポンス。

### Infrastructure & Operations
- **Database**: MariaDB（マイグレーション `migrations/001_initial.sql` 〜 `049_player_deletion_and_maintenance.sql`、`make db-migrate` / `make db-reset`）。
- **Valkey**: 遅延アクションキュー・排他ロック・分散レートリミット・ランキングスナップショットキャッシュ（AOF+RDB永続化）。
- **Logging**: Go標準 `log/slog` によるJSON構造化ログ、秘密情報自動マスキング。
- **Verification**: `Makefile` (`make check`, `make fmt`, `make vet`, `make openapi-sync`, `make openapi-check`, `make test-stress`, `make check-clean`)、OpenAPI 3.1 仕様書自動同期・フォーマット CLI（`scripts/sync_openapi.go`）、CIガード（OpenAPI 3.1構文・全ルート網羅テスト）、Go AST 静的解析テスト（全リポジトリにおける `ExecutorFromContext` 必須・`BeginTx` 禁止検証 `internal/database/tx_lint_test.go`、サービス層 `RunInTx` 呼び出し検証 `internal/architecture/arch_test.go`、HTTP 所有権認可検証 `internal/api/http/auth_lint_test.go`、Core 成長ヘルパー適用強制 `internal/core/progression/progression_lint_test.go`）、Git pre-push hook による自動検証、`scripts/stress_test.sh` による高並行ストレステスト。
- **Deployment**: Distroless (`gcr.io/distroless/static-debian13:nonroot`) ベースの最小本番イメージ（GHCR自動公開）。

---

## Immediate Priorities (Next Actions)

1. **Remaining Version 1.0 Feature Modules**:
   - Personal Access Token (API Key) generation and authentication (Issue #163)
   - Web Presentation UI / Client (Issue #140)

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
