# Legacy CGI to Go Navigation Catalog

This catalog maps selected legacy CGI clusters to Go modules, transport, migration history, and design documents. Paths in the Legacy Script column are relative to the original Party2 root. It is a navigation inventory, not an exhaustive CGI coverage or behavioral-parity certificate.

It is structured into **7 Domain Clusters (CL-01 to CL-07)** for navigation. CL-02 has five individually selectable audit units, **CL-02A to CL-02E**, scoped by actions and state transitions rather than whole CGI files. Closed remediation issues are history, not evidence that every behavior is equivalent; see [the documentation audit](documentation-audit.md).

---

## 1. Critical Misnomer & Pitfall Warnings (取り違え厳禁)

| Misnomer Trap | Authentic Legacy Specification | Go Implementation Guideline |
|---|---|---|
| **`farm.cgi` (Monster Ranch)** | **モンスター牧場 (@モンジィ)**: 仲間モンスター保管（50〜350匹、転職100回以上の恩恵含む）、自宅ペット連携（8枠）、改名（8文字）、P2P譲渡、野生への解放。**作物・畑の要素は一切存在しない**。 | Dedicated to `internal/monster`. Fictional crop logic purged. (Issue #488, #787) |
| **`plantation.cgi` (Seed Cultivation)** | **種菜園 (@ロータス)**: 6種の種（赤/青/黄/緑/銀/金）、14種の特殊肥料、枯れ率計算、翌朝タイマー、預かり所（depot）への収穫物直送。 | Dedicated to `internal/plantation` (Issue #489). |
| **`reborn.cgi` / `altar.cgi` (No Rebirth)** | **復活の祭壇**: 6オーブ奉納・ラーミア復活・旅行アイテムの願い。レベル1リセットは存在しない。Lv99→150限界突破は別施設の天界 (`god.cgi`)。 | Purge fictional Rebirth system; restore OverLevel cap (Issue #470, #471). |
| **`guild.cgi` (No Donation Leveling)** | **ギルド拠点**: ゴールド寄付によるギルドLv1〜10上げは存在しない。**活動による動的GP**、自由役職命名（6文字）、申請承認制、HEXカラー。 | Purge fictional donation levels; restore activity GP & custom roles (Issue #490). |
| **`sleep.cgi` / `home.cgi` (No Paid Inn)** | **自宅での睡眠**: 有料の宿屋は存在しない。**自宅（または他人の家）で寝る**ことで無料全快・日次フラグリセット。 | Decommission `internal/inn`; integrate into `internal/home` (Issue #459). |
| **`free.cgi` (Flea Market)** | **フリーマーケット**: 旧ファイル名は `fleamarket.cgi` ではなく `lib/free.cgi`。預かり所から直接出品・引出。 | Implemented in `internal/fleamarket` (Issue #477). |
| **`secret.cgi` (Secret Shop)** | **秘密の店**: 旧ファイル名は `lib/secret.cgi`。転職7回以上（`job_lv >= 7`）、原典8アイテム3倍価格、満杯時Depot転送、ぱふぱふ（会話のみ）。 | Completed in `internal/secretshop` (Issue #462). |
| **`black_market.cgi` (Black Market)** | **闇市場**: ゴールド売買・相場変動・日次枠を全撤廃し、純粋なレアポイント生贄（手持ち/Depot）と24種景品Depot直送に回帰。 | Completed in `internal/blackmarket` (Issue #463). |

---

## 2. Domain Cluster Audit Catalog (全7クラスタ詳細マッピング)

### CL-01: Meta & Player Identity (基盤・アカウント・ナビゲーション)
- **Cluster Summary**: プレイヤーアカウント、キャラクター生成・管理、認証トークン、緊急脱出、管理者機能、ナビゲーション
- **Shared Dependencies**: `party2/lib/system.cgi` (ユーザーファイルI/O, セッション, アクセス制御)
- **Primary Domain Packages**: `internal/player/`, `internal/character/`, `internal/rescue/`, `internal/maintenance/`, `internal/id/`
- **Key Testing / Linter Focus**: AST Auth Wrappers (`internal/api/http/auth_lint_test.go`), IP抽出, 不正トークン破棄

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `index.cgi`, `_side_menu.cgi` | メインポータル・ナビゲーション | Client Presentation | Frontend / Static HTML | N/A | Planned | サーバー側ではなくフロントエンドUIで提供 |
| `login.cgi` | 認証・セッション発行・日次処理 | `internal/player/` | `internal/api/http/token.go`<br/>`migrations/010_players_sessions.sql`<br/>`migrations/053_player_api_tokens.sql` | `docs/design/player-and-character.md`<br/>`docs/design/api-tokens.md`<br/>`docs/api/paths/auth.json` | Fix closed; parity not certified (#775) | 対応履歴: #775。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `new_entry.cgi` | キャラクター新規作成 | `internal/player/`<br/>`internal/character/` | `internal/api/http/character.go`<br/>`migrations/002_characters.sql`<br/>`migrations/004_character_initial_state.sql` | `docs/design/player-and-character.md`<br/>`docs/api/paths/character.json` | Fix closed; parity not certified (#771) | 対応履歴: #771。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `player.cgi`, `player_old.cgi` | プレイヤーの軌跡 (Memory log & 4大コンプリート率) | *(Unimplemented)* | *(Unimplemented)* | `docs/design/player-and-character.md` | Reconciling (#773) | 原典の軌跡ログ(`memory.cgi`, `write_memory`)および4大コンプリート率(武器・防具・道具141・錬金)の取得APIが未実装 |
| `my.cgi`, `my2.cgi` | 外部ブログパーツ１・２ | Client / Widget API | *(Unimplemented)* | `docs/design/player-and-character.md` | Reconciling | 内部ダッシュボードではなく外部サイト貼り付け用JS(`document.write`)。ステータスミニカード・同居人台詞表示 |
| `profile.cgi`, `lib/profile.cgi` | 公開プロフィール表示・編集 | `internal/character/` | `internal/api/http/character.go`<br/>`migrations/047_character_profile_and_customization.sql` | `docs/design/player-and-character.md`<br/>`docs/api/paths/character.json` | Fix closed; parity not certified (#771) | 対応履歴: #771。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `delete.cgi` | キャラクター/アカウント削除 | `internal/player/`<br/>`internal/character/` | `internal/api/http/character.go`<br/>`migrations/049_player_deletion_and_maintenance.sql` | `docs/design/player-and-character.md`<br/>`docs/design/player-deletion.md`<br/>`docs/api/paths/player.json` | Fix closed; parity not certified (#774) | 対応履歴: #774。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `rescue.cgi` | 救済・スタック復帰 | `internal/rescue/` | `internal/api/http/helper_rescue.go` | `docs/design/rescue-and-helper.md`<br/>`docs/api/paths/rescue.json` | Completed (#772) | 架空の24時間以内再救出ペナルティ倍増(1200秒)の削除。町内安全時の無ペナルティ復帰ガード |
| `admin.cgi`, `maintenance.cgi` | 管理者操作・メンテナンス制御 | `internal/maintenance/` | `internal/api/http/maintenance.go` | `docs/design/system-maintenance.md`<br/>`docs/api/paths/admin.json` | Reconciling | メンテナンスモード切り替え、BAN、管理者による無ペナルティ強制復帰(`admin_refresh`)の追加 |
| `link.cgi` | 外部リンク・コミュニティ | Client Presentation | Frontend / Static HTML | N/A | Planned | 静的ナビゲーション・ワンクッション画面 |

---

### CL-02: Living, Housing & Towns (生活・拠点・預かり所)
- **Cluster Summary**: プレイヤーの生活拠点（自宅、睡眠）、手紙、町の探索、公園、預かり所（Depot）
- **Shared Dependencies**: `party2/lib/_npc_action.cgi`, `party2/lib/system.cgi`, `party2/lib/_data.cgi` (Homeで使うアイテム効果)
- **Primary Domain Packages**: `internal/home/`, `internal/depot/`, `internal/park/`, `internal/town/`, `internal/store/` (建設部分)
- **Key Testing / Linter Focus**: 有料宿屋の排除、Depotの排他制御・保管上限、日次フラグリセット

`CL-02A`〜`CL-02E`を個別の監査対象として指定する。親の`CL-02`を指定した場合は全5単位を順に調べ、各単位の結果を共有した後、下記の連動確認を行う。一部の監査だけで親全体をCompliantと判定しない。

同じ`lib/home.cgi`が複数単位に登場するのは、表示・睡眠・アイテム使用で操作と状態変化が異なるため。以下は監査範囲と参照先であり、ゲーム仕様の正本や実装済み証明ではない。外部スキルの`references/clusters.json`は候補キャッシュとして、このmappingに照合する。

#### CL-02A: Home, Mail & Park

- **Scope / Legacy**: `lib/home.cgi`の公開/本人表示、Home設定、`てがみをかく` / `てがみをよむ`、`からー`、`ことばをおしえる` / `ことばをわすれさせる`、ペット表示/会話、配送通知。`lib/park.cgi`の`うらない`、NPC会話・共通チャット。共有`lib/system.cgi`の手紙・ログ処理まで追う。
- **Go / HTTP**: `internal/home/service.go`、`internal/home/estate.go`の色設定、`internal/park/`、`internal/api/http/home.go`、`internal/api/http/home_estate.go`の設定/色関連、`internal/api/http/park.go`。
- **Contracts**: `docs/design/home.md`、`docs/design/town_park.md`、`docs/api/paths/home.json`、`docs/api/paths/letter.json`、`docs/api/paths/park.json`、`docs/api/paths/character.json` (色設定)。
- **Open follow-ups**: [#1094](https://github.com/witchcraze/party2re/issues/1094) (mailbox所有)、[#1110](https://github.com/witchcraze/party2re/issues/1110) (配送通知)、[#1115](https://github.com/witchcraze/party2re/issues/1115) (設定更新)、[#1116](https://github.com/witchcraze/party2re/issues/1116) (色更新)、[#1125](https://github.com/witchcraze/party2re/issues/1125) (ことば保持)、[#1126](https://github.com/witchcraze/party2re/issues/1126) (mail/park設定)。
- **Boundary**: Homeからの図鑑・ジョブマスター・プロフィール・冒険記録・特技設定・画像設定は遷移先と対象identityを確認する。遷移先の全仕様はCL-01/04/06/07の担当範囲。壁紙等の表示はここ、使用による変更はCL-02E。

#### CL-02B: Sleep & Recovery

- **Scope / Legacy**: `lib/home.cgi`の`ねる`、`lib/sleep.cgi`、共有dispatchの睡眠中操作制限。睡眠時間、訪問先の前提、起床回復、各機能のreset/完了と長期不在・再試行を追う。
- **Go / HTTP**: `internal/home/sleep.go`、`internal/core/timer/`、`internal/api/http/home_sleep.go`、`internal/api/http/action_home.go`、`internal/api/http/auth_helper.go`、`cmd/party2/wire.go`の回復連動配線。
- **Contracts**: `docs/design/home.md`、`docs/design/action-preconditions.md`、`docs/api/paths/home.json`。
- **Open follow-ups**: [#1117](https://github.com/witchcraze/party2re/issues/1117) (更新guard)、[#1118](https://github.com/witchcraze/party2re/issues/1118) (起床義務の寿命)、[#1119](https://github.com/witchcraze/party2re/issues/1119) (私有Home条件)、[#1123](https://github.com/witchcraze/party2re/issues/1123) (時間/人数consumer)。人数のidentity・時間窓はCL-01の[#1104](https://github.com/witchcraze/party2re/issues/1104)を参照する。
- **Boundary**: 満腹・祝福・錬金等は睡眠からの呼出し/失敗時整合を確認し、各機能全体の監査へ広げない。explicit Wakeとread-only GETの承認済み契約を維持する。

#### CL-02C: Depot & Transfers

- **Scope / Legacy**: `lib/depot.cgi`の`うる` / `まとめてうる`、`あずける` / `ひきだす`、`せいとん`、`おくる` (品/金)、`かくちょう`。共有`lib/system.cgi`の配送・未受取送金を含む。
- **Go / HTTP**: `internal/depot/`、`internal/database/depot_repository.go`、inventory/equipment保存境界、`internal/api/http/depot.go`。
- **Contracts**: `docs/design/depot.md`、`docs/design/items-and-equipment.md`、`docs/api/paths/depot.json`。
- **Open follow-ups**: [#1109](https://github.com/witchcraze/party2re/issues/1109) (未受取送金/金額保存)、[#1111](https://github.com/witchcraze/party2re/issues/1111) (整頓順)、[#1112](https://github.com/witchcraze/party2re/issues/1112) (装備品移動)、[#1127](https://github.com/witchcraze/party2re/issues/1127) (引出し/装備交換)。通知の受取側はCL-02Aの#1110と合わせて確認する。
- **Boundary**: 他機能からのDepot直送は入庫/容量/失敗時の契約を確認する。売買・景品・生産など送信元機能の全仕様は各クラスタが所有する。HomeでのDepot品消費はCL-02E。

#### CL-02D: Towns & Construction

- **Scope / Legacy**: `lib/town1.cgi`〜`lib/town4.cgi`、`lib/_town.cgi`の`たてる` / `ちぇっく` / `みせ` / `はいる`。町の探索、住宅/商店の建設、容量、価格、所有期限、GP連動、所有者への遷移を確認する。
- **Go / HTTP**: `internal/town/`、`internal/home/estate.go`の住宅建設/期限確認、`internal/store/service.go`の商店建設、`internal/api/http/home_estate.go`、`internal/api/http/store.go`。
- **Contracts**: `docs/design/home.md`、`docs/design/store.md`、`docs/api/paths/towns.json`、`docs/api/paths/houses.json`、`docs/api/paths/stores.json`。
- **Open follow-ups**: [#1113](https://github.com/witchcraze/party2re/issues/1113) (住宅上限)、[#1114](https://github.com/witchcraze/party2re/issues/1114) (商店上限)、[#1122](https://github.com/witchcraze/party2re/issues/1122) (住宅GP)。
- **Boundary**: 商店の建設・入店までが本単位で、出品/購入/内装販売はCL-03。私有Homeへの訪問・睡眠と有料の町住宅契約を同一条件にしない。

#### CL-02E: Home Item Use

- **Scope / Legacy**: `lib/home.cgi`の`つかう`と到達する`lib/_data.cgi`の効果。手持ち/Depotの両source、装備inspect、usage category、効果・前提・乱数・消費順・容量不足/失敗時をアイテムごとに確認する。
- **Go / HTTP**: `internal/home/item_usage.go`、`internal/home/recipe_usage.go`、`internal/home/costume_usage.go`、`internal/api/http/home_estate.go`のitem一覧/使用。
- **Contracts**: `docs/design/home.md`、`docs/design/blacksmith.md`、`docs/api/paths/character.json` (Home item一覧/使用)。
- **Open follow-ups**: [#1120](https://github.com/witchcraze/party2re/issues/1120) (木の実の現在値)、[#1121](https://github.com/witchcraze/party2re/issues/1121) (無刻印時の消費)、[#1124](https://github.com/witchcraze/party2re/issues/1124) (未対応dispatch/不明callback)。
- **Boundary**: 名前がrecipe poolにあるだけで対応済みとは数えない。レシピ・衣装・祭壇等への呼出しは効果/消費の境界まで追い、CL-03/04/05の全機能監査と分ける。未確定callbackや稼働config不在を仕様推測で埋めない。

#### Cross-unit checks

| Contract | Audit ownership / integration check |
| --- | --- |
| 品/金の送付 → Home通知 | CL-02Cの送付・commitとCL-02Aの通知生成/本人表示を一緒に確認する。#1110は#1109の後続。 |
| 睡眠 → 各更新APIのguard | CL-02Bが制限状態を所有し、CL-02Aの色設定、CL-02Dの建設、CL-02Eの使用入口まで確認する (#1117)。 |
| Home設定 → 有料住宅契約 | CL-02Aの設定更新がCL-02Dの契約・期限を上書きしないことを確認する (#1115)。 |
| 品の移動 → 装備/消費 | CL-02Cの所有移動・FK・交換とCL-02Eの使用/消費を区別し、同時操作・失敗時の品と装備状態を確認する (#1112/#1127)。 |
| 商店建設 → 売買 | CL-02DからCL-03へstore identity・入店条件を引き継ぐ。建設監査だけで商店売買全体のparityを認定しない。 |

The script-level pointers below remain navigation for the parent cluster. **Reconciling** means linked gaps or specifications remain; a closed historical fix does not certify its audit unit or CL-02 as a whole.

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/home.cgi` | 自宅拠点 (@自宅ペット) | `internal/home/` | `internal/api/http/home.go`<br/>`internal/api/http/home_estate.go`<br/>`internal/api/http/home_sleep.go`<br/>`migrations/034_player_home_and_mailbox.sql`<br/>`migrations/036_player_mailbox_independent_deletion.sql`<br/>`migrations/061_home_members_and_god_parity.sql`<br/>`migrations/063_home_estate_parity.sql` | `docs/design/home.md`<br/>`docs/api/paths/home.json`<br/>`docs/api/paths/letter.json` | Reconciling | CL-02A/B/E。対応履歴: #777, #778。未解決の所有・更新・睡眠・使用差分は各単位のfollow-upを参照。 |
| `lib/sleep.cgi` | 睡眠 (無料全快・日次リセット) | `internal/home/` | `internal/api/http/home_sleep.go` | `docs/design/home.md`<br/>`docs/api/paths/home.json` | Reconciling | CL-02B (#1117〜#1119, #1123)。#459は対応履歴。有料宿屋は非存在。時間/人数・起床義務・訪問条件に未解決事項あり。 |
| `lib/park.cgi` | 交流広場 (@町娘) | `internal/park/` | `internal/api/http/park.go`<br/>`migrations/032_town_park.sql` | `docs/design/town_park.md`<br/>`docs/api/paths/park.json` | Reconciling | CL-02A (#1126)。占い（22entry・27色）、NPC会話、チャット。文字数/log契約は未確定。10G回復は非存在。 |
| `lib/depot.cgi` | 預かり所 (@ニキータ) | `internal/depot/` | `internal/api/http/depot.go`<br/>`migrations/011_depot.sql`<br/>`migrations/055_depot_parity.sql` | `docs/design/depot.md`<br/>`docs/api/paths/depot.json` | Reconciling | CL-02C (#1109, #1111, #1112, #1127) と通知連動#1110。#460は対応履歴。容量・拡張・売却・整頓・配送・引出しを操作ごとに監査する。 |
| `lib/town1.cgi` .. `town4.cgi`, `lib/_town.cgi` | 町1〜町4の探索・建設 | `internal/home/` (住宅建設)<br/>`internal/town/`<br/>`internal/store/` (商店建設)<br/>Client Presentation | `internal/api/http/home_estate.go`<br/>`internal/api/http/store.go`<br/>`migrations/063_home_estate_parity.sql`<br/>`migrations/067_player_stores.sql` | `docs/design/home.md`<br/>`docs/design/store.md`<br/>`docs/api/paths/towns.json`<br/>`docs/api/paths/houses.json`<br/>`docs/api/paths/stores.json` | Reconciling | CL-02D (#1113, #1114, #1122)。#461, #466は対応履歴。町別上限・GP配線に未解決差分があり、設定/guardはCL-02A/Bと連動確認する。 |


---

### CL-03: Economy, Shops & Trading (商業・流通・装備強化)
- **Cluster Summary**: 武器・防具・道具・装飾品・宝石店・闇市・秘密の店、鍛冶屋、酒場、銀行、オークション、フリーマーケット、個人商店
- **Shared Dependencies**: `party2/lib/_data.cgi` (アイテム・装備マスタ定数), `party2/config.cgi`, `party2/lib/depot.cgi`
- **Primary Domain Packages**: `internal/shop/`, `internal/blacksmith/`, `internal/secretshop/`, `internal/blackmarket/`, `internal/gemstore/`, `internal/tavern/`, `internal/bank/`, `internal/auction/`, `internal/fleamarket/`, `internal/economy/`
- **Key Testing / Linter Focus**: 売却価格50%、MasterCard10%割引、刻印晶による武器刻印、Depot直結入出庫、並行購入デッドロック防止

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/weapon.cgi` | 武器屋 (@ブッキー) | `internal/shop/` | `internal/api/http/shop.go`<br/>`migrations/005_items_inventory.sql`<br/>`migrations/008_equipment.sql` | `docs/design/shops.md`<br/>`docs/design/items-and-equipment.md`<br/>`docs/api/paths/shop.json` | Compliant (#465) | 定価2倍販売・50%売却、ジョブレベル別カタログ拡張、手助けクエスト除外、Depot自動転送 |
| `lib/armor.cgi` | 防具屋 (@アマノ) | `internal/shop/` | `internal/api/http/shop.go` | `docs/design/shops.md`<br/>`docs/api/paths/shop.json` | Compliant (#465) | 定価2倍販売・50%売却、ジョブレベル別カタログ拡張、手助けクエスト除外、Depot自動転送 |
| `lib/item.cgi` | 道具屋 (@アイテムコ) | `internal/shop/` | `internal/api/http/shop.go` | `docs/design/shops.md`<br/>`docs/api/paths/shop.json` | Compliant (#465) | 定価2倍販売・50%売却、ジョブレベル別カタログ拡張、秘密の店ヒント |
| `lib/accessory.cgi` | 装飾品屋 (@ミラ) | `internal/shop/` | `internal/api/http/shop.go` | `docs/design/accessory-shop.md`<br/>`docs/api/paths/shop.json` | Implemented (#779); parity review remains | カタログ・倍率・48レシピ・秘薬保証を実装。成功確率の原典比較境界に差分あり。 |
| `lib/blacksmith.cgi` | 鍛冶屋 (@ブッキー) | `internal/blacksmith/` | `internal/api/http/blacksmith.go`<br/>`migrations/083_blacksmith_seals_and_storage.sql` | `docs/design/blacksmith.md`<br/>`docs/api/paths/blacksmith.json` | Compliant (#458) | 武器刻印12種(刻印晶消費)、装備名付け(武器・防具/20文字/サニタイズ)、専用武器預かり所(3枠/名前重複禁止/装備中引出不可) |
| `lib/secret.cgi` | 秘密の店 (@ヒミツジ) | `internal/secretshop/` | `internal/api/http/secretshop.go` | `docs/design/shops.md`<br/>`docs/api/paths/secretshop.json` | Compliant (#780) | 転職7回以上(`job_lv >= 7`)、原典8種3倍価格、手持ち直接購入時の図鑑自動登録連携(`RecordItemDiscovered`)、満杯時Depot転送(`depot.FindOrCreate`)、ぱふぱふ会話のみ |
| `lib/black_market.cgi` | 闇市場 (@闇商人) | `internal/blackmarket/` | `internal/api/http/blackmarket.go`<br/>`migrations/042_blackmarket_sacrifice_and_trade.sql`<br/>`migrations/065_drop_blackmarket_fictional_tables.sql` | `docs/design/black-market.md`<br/>`docs/api/paths/blackmarket.json` | Compliant (#463) | 純粋レアポイント物々交換、手持ち/Depot生贄、24種景品Depot直送、原典台詞 |
| `lib/gem_store.cgi` | 宝石店 (@ジェマ) | `internal/gemstore/` | `internal/api/http/gemstore.go` | `docs/design/gemstore.md`<br/>`docs/api/paths/gemstore.json` | Compliant (#464) | 宝石購入(5倍価格・GemBox格納)、売却(50%)、56種加工レシピ、5種未鑑定オーブ鑑定。ギフト時のメール通知のみ未実装 |
| `lib/store.cgi`, `lib/goods.cgi` | プレイヤーストア・オラクル屋 | `internal/store/` | `internal/api/http/store.go`<br/>`migrations/067_player_stores.sql` | `docs/design/store.md`<br/>`docs/api/paths/stores.json` | Compliant (#424) | 店舗建設(50,000G/90日)、ゴールド・物々交換出品、壁紙26種・家具15種内装 |
| `lib/bar.cgi` | ルイーダの酒場 (@ルイーダ) | `internal/tavern/` | `internal/api/http/tavern.go`<br/>`migrations/039_tavern.sql`<br/>`migrations/069_drop_fictional_delivery_tables.sql` | `docs/design/tavern.md`<br/>`docs/api/paths/tavern.json` | Compliant (#475) | 食事注文・福引券付与、出前予約・冒険完了時自動配達フック（架空NPCおつかい・小包便撤廃） |
| `lib/bank.cgi` | ゴールド銀行 (@タクシード) | `internal/bank/` | `internal/api/http/bank.go`<br/>`migrations/014_bank.sql`<br/>`migrations/064_character_bank_deposit.sql` | `docs/design/bank.md`<br/>`docs/api/paths/bank.json` | Compliant (#476) | 預金・引出、上限99兆G、所持金999,999G上限クランプ＆超過預金返還。架空の日次利息は非存在 |
| `lib/auction.cgi` | オークション (@ワイルド) | `internal/auction/` | `internal/api/http/auction.go`<br/>`migrations/020_auctions.sql` | `docs/design/auction.md`<br/>`docs/api/paths/auction.json` | Compliant (#474) | リアルタイムP2P送金・装備/アイテムDepot直送譲渡(`＠おくる`)、他PC調査(`＠しらべる`)。架空の入札・競売システムは非存在 |
| `lib/free.cgi` | フリーマーケット | `internal/fleamarket/` | `internal/api/http/fleamarket.go`<br/>`migrations/043_fleamarket.sql` | `docs/design/fleamarket.md`<br/>`docs/api/paths/fleamarket.json` | Compliant (#477) | Depotから直接出品(サーバー上限120件・個人上限5+OverFlea)、購入品・キャンセル品はDepotへ即時格納 |

---

### CL-04: Progression, Faith & Limits (成長・職業・限界突破・信仰)
- **Cluster Summary**: 転職、職業極め、願いの泉（SP強化）、特技設定、改名、画像変更、転生の祭壇（限界突破）、礼拝堂、天界・裏天界、小さなメダル
- **Shared Dependencies**: `party2/lib/_data.cgi` (職業・特技定義), `party2/lib/system.cgi`
- **Primary Domain Packages**: `internal/job/`, `internal/wishingwell/`, `internal/customskill/`, `internal/altar/`, `internal/chapel/`, `internal/god/`, `internal/medal/`, `internal/core/progression/`
- **Key Testing / Linter Focus**: 転生（Lv1リセット）の完全排除、Lv99→150限界突破、礼拝堂の単一祈願制約、SPステータス成長

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/job_change.cgi` | 転職所 (@ダーマ神官) | `internal/job/` | `internal/api/http/job.go`<br/>`migrations/007_character_jobs.sql`<br/>`migrations/059_job_change_parity.sql` | `docs/design/jobs-and-skills.md`<br/>`docs/api/paths/character.json` | Fix closed; parity not certified (#784) | 対応履歴: #784。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/job_master.cgi` | 職業極め所 | `internal/job/` | `internal/api/http/job.go` | `docs/design/jobs-and-skills.md`<br/>`docs/api/paths/character.json` | Fix closed; parity not certified (#785) | 対応履歴: #785。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/sp_change.cgi` | 願いの泉 (@女神) | `internal/wishingwell/` | `internal/api/http/wishingwell.go`<br/>`migrations/056_eliminate_rebirth_add_sp.sql` | `docs/design/wishing-well.md`<br/>`docs/api/paths/wishing_well.json` | Compliant (#468) | SPを消費して5大ステータス強化。OverLevel/JobMemoryガード |
| `lib/custom_skill.cgi` | 特技設定 (@マニャ) | `internal/customskill/` | `internal/api/http/custom_skill.go`<br/>`migrations/029_custom_skills.sql`<br/>`migrations/058_custom_skill_gem_synthesis.sql` | `docs/design/custom_skill.md`<br/>`docs/api/paths/customskill.json` | Compliant (#782, #850) | 3宝玉スロット合成、GemBox消費・返還接続、転職後キャパシティガード解除、CMP計算式復元済 |
| `lib/name_change.cgi` | 命名の館 (@アストロン) | `internal/character/` | `internal/api/http/character.go` | `docs/design/character-customization.md`<br/>`docs/api/paths/character.json` | Compliant | ゴールド手数料(名前変更: 500,000G、性転換: 10,000G)、重複ネーム検証、ギルド・フリマガード、現在職の性別制限検証 (#1164) |
| `lib/custom_image.cgi`, `lib/upload_image.cgi` | 画像設定所 | `internal/character/` | `internal/api/http/character.go` | `docs/design/character-customization.md`<br/>`docs/api/paths/character.json` | Fix closed; parity not certified (#786) | 対応履歴: #786。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/reborn.cgi` | 復活の祭壇 (@巫女) | `internal/altar/`<br/>`internal/core/progression/` | `internal/api/http/altar.go`<br/>`migrations/057_altar_of_rebirth.sql` | `docs/design/altar-of-rebirth.md`<br/>`docs/design/progression.md`<br/>`docs/api/paths/altar.json` | Compliant | ラーミア復活(6オーブ/30分)、4大旅行アイテムDepot直送。🚨 Lv1転生は架空として完全撤廃済。Lv99→150限界突破は天界(`god.cgi`)で実装 |
| `lib/chapel.cgi` | 礼拝堂 (@シスター) | `internal/chapel/` | `internal/api/http/chapel.go`<br/>`migrations/022_chapel.sql`<br/>`migrations/060_chapel_parity.sql` | `docs/design/chapel.md`<br/>`docs/api/paths/chapel.json` | Compliant (#783, #1163) | 祈願枠と効果期間の分離、同日睡眠後の再祈願拒否、日跨ぎ睡眠による枠解除・効果終了、残留chapel_reset安全化 (#1163) |
| `lib/god.cgi` | 天界 (@神) | `internal/god/` | `internal/api/http/god.go`<br/>`migrations/044_god_wishes_and_limit_breaks.sql`<br/>`migrations/061_home_members_and_god_parity.sql` | `docs/design/god.md`<br/>`docs/api/paths/god.json` | Compliant (#473) | 20種の願い(メイド・オルテガ・アバター・全ステータス+40)、Lv99→150限界突破 |
| `lib/u_god.cgi` | 裏天界 (@裏神) | `internal/god/` | `internal/api/http/god.go`<br/>`migrations/044_god_wishes_and_limit_breaks.sql` | `docs/design/god.md`<br/>`docs/api/paths/god.json` | Compliant (#473) | 5大保管庫拡張(牧場・預かり所+50/段、各最大5段階) |
| `lib/medal.cgi` | メダル王 (@メダル王) | `internal/medal/` | `internal/api/http/medal.go`<br/>`migrations/030_small_medals.sql`<br/>`migrations/050_achievements_and_medals.sql` | `docs/design/medal.md`<br/>`docs/design/achievements.md`<br/>`docs/api/paths/medal.json` | Compliant (#473) | 全15段階の小さなメダル景品交換、景品はDepotへ直送 |
| `lib/exile.cgi` | 荒らし追放騎士団 | `internal/maintenance/` (将来) | Pending | `docs/design/system-maintenance.md` | Backlog | プレイヤー自治による追放投票機能 |

---

### CL-05: Production, Cultivation & Monsters (生産・菜園・モンスター牧場)
- **Cluster Summary**: 錬金堂（レシピ合成）、モンスター牧場（仲間預託・ペット連携）、種菜園（栽培・肥料）
- **Shared Dependencies**: `party2/lib/_data.cgi`, `party2/lib/_alchemy_recipe.cgi`, `party2/lib/depot.cgi`
- **Primary Domain Packages**: `internal/alchemy/`, `internal/monster/`, `internal/plantation/`
- **Key Testing / Linter Focus**: 牧場から畑要素を全削除、錬金の翌朝タイマー・Depot直結、菜園の14種肥料・枯れ率計算

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/alchemy.cgi`, `lib/_alchemy_recipe.cgi` | 錬金堂 (@トロデ) | `internal/alchemy/` | `internal/api/http/alchemy.go`<br/>`migrations/070_alchemy_overnight_depot.sql` | `docs/design/alchemy.md`<br/>`docs/api/openapi.json` | Fix closed; parity not certified (#788) | 対応履歴: #788。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/farm.cgi` | モンスター牧場 (@モンジィ) | `internal/monster/` | `internal/api/http/monster.go`<br/>`migrations/045_monster_grandpa_and_pets.sql`<br/>`migrations/071_purge_farm_plots.sql` | `docs/design/monster.md`<br/>`docs/api/openapi.json` | Fix closed; parity not certified (#787) | 対応履歴: #787。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/plantation.cgi` | 種菜園 (@ロータス) | `internal/plantation/` | `internal/api/http/plantation.go`<br/>`migrations/072_plantation_plots.sql` | `docs/design/plantation.md`<br/>`docs/api/paths/character.json` | Compliant | 対応履歴: PR #489。6種の種、14種の肥料、枯れ率、翌朝収穫・Depot配送の仕様はDesign Docと原典を照合。 |

---

### CL-06: Combat, Adventure & Dungeons (冒険・ボス・戦闘・PVP/GVG・ダンジョン)
- **Cluster Summary**: 冒険（ダンジョン10階層）、封印の魔王、コア戦闘・スキル発動エンジン、闘技場、ギルド戦、パーティ共闘、連戦チャレンジ、手助け
- **Shared Dependencies**: `party2/lib/_battle.cgi`, `party2/lib/_skill.cgi`, `party2/lib/ActionCounter.pm`, `party2/lib/TurnEndProcessor.pm`, `party2/lib/_data.cgi`
- **Data Dependencies**: `party2/stage/0..27.cgi`, `party2/stage/king1..99.cgi`, `party2/challenge/0..8.cgi`, `party2/map/*`
- **Primary Domain Packages**: `internal/adventure/`, `internal/boss/`, `internal/core/battle/`, `internal/core/skill/`, `internal/pvp/`, `internal/gvg/`, `internal/dungeon/`, `internal/challenge/`, `internal/party/`, `internal/helperquest/`
- **Key Testing / Linter Focus**: 逃走ペナルティ、10階層制覇、挑戦条件・調査・封印、実時間ベッティング、パーティ相乗効果（2p=+10%, 3p=+20%, 4p=+30%）

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/vs_monster.cgi`, `lib/adventure.cgi`, `lib/adventure_record.cgi` | 冒険・階層探索 (Adventure) | `internal/adventure/` | `internal/api/http/adventure.go`<br/>`migrations/006_adventures.sql`<br/>`migrations/009_adventure_battle_rewards.sql`<br/>`migrations/037_adventure_chronicle.sql` | `docs/design/adventure.md`<br/>`docs/design/stages-and-monsters.md`<br/>`docs/api/paths/adventure.json` | Fix closed; parity not certified (#795) | 対応履歴: #795。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/vs_king.cgi`, `lib/_win_vs_king.cgi` | 封印の魔王 (Boss) | `internal/boss/` | `internal/api/http/combat.go`<br/>`migrations/025_boss_battles.sql` | `docs/design/boss.md`<br/>`docs/api/paths/boss.json` | Fix closed; parity not certified (#794) | 対応履歴: #794。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/_battle.cgi` | コア戦闘エンジン | `internal/core/battle/` | Core Engine | `docs/design/battle.md` | Fix closed; parity not certified (#790) | 対応履歴: #790。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/_skill.cgi` | スキル発動エンジン | `internal/core/skill/` | Core Engine | `docs/design/jobs-and-skills.md` | Fix closed; parity not certified (#791) | 対応履歴: #791。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/vs_player.cgi` | 闘技場 (PvP Arena) | `internal/pvp/` | `internal/api/http/pvp.go`<br/>`migrations/023_pvp_arena.sql` | `docs/design/pvp.md`<br/>`docs/api/paths/pvp.json` | Compliant (#481) | 8人9色の投票・オッズ分配・賞金精算準拠。非同期Eloは完全撤廃済 |
| `lib/vs_guild.cgi` | ギルド戦 (GvG Arena) | `internal/gvg/` | `internal/api/http/combat.go`<br/>`migrations/024_gvg_combat.sql` | `docs/design/gvg.md` | Compliant (#482) | 5勝ごとのトロフィー昇格（銅杯〜優勝杯）、ラウンドGP(+3)、参加GP(+4)、賞金精算準拠。非同期Eloは完全撤廃済 |
| `lib/vs_dungeon.cgi`, `party2/dungeon.cgi` | ダンジョン探索 | `internal/dungeon/` | `internal/api/http/adventure.go`<br/>`migrations/026_dungeon_exploration.sql` | `docs/design/dungeon.md`<br/>`docs/api/paths/dungeon.json` | Fix closed; parity not certified (#795) | 対応履歴: #795。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/vs_challenge.cgi`, `party2/challenge.cgi` | 連戦チャレンジ | `internal/challenge/` | `internal/api/http/combat.go`<br/>`migrations/028_endurance_challenge.sql` | `docs/design/challenge.md`<br/>`docs/api/paths/challenge.json` | Parity (#789) | 原典9ステージ(0最弱逆襲..8真最強王者)、+10%固定モンスター成長、10%宝箱階層トリガー、全滅時100%報酬獲得 |
| `lib/quest.cgi`, `party2/party.cgi` | パーティ共闘クエスト | `internal/party/` | `internal/api/http/party.go`<br/>`migrations/048_party_and_coop_quests.sql`<br/>`migrations/052_drop_parties_and_party_members.sql` | `docs/design/party-system.md`<br/>`docs/api/paths/party.json` | Fix closed; parity not certified (#793) | 対応履歴: #793。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/helper.cgi` | 手助けクエスト | `internal/helperquest/` | `internal/api/http/helper_rescue.go`<br/>`migrations/031_rescue_and_helper.sql` | `docs/design/rescue-and-helper.md`<br/>`docs/api/paths/helper.json` | Fix closed; parity not certified (#792) | 対応履歴: #792。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |

---

### CL-07: Social, Entertainment & Gambling (ギルド・娯楽・カジノ・ランキング・図鑑)
- **Cluster Summary**: ギルド（動的GP・役職）、写真館・コンテスト、イベント広場、宝くじ、福引、カジノ（スロット・ハイロー・インディアンポーカー・ドッペル）、図鑑、新聞、各種ランキング、リプレイ
- **Shared Dependencies**: `party2/lib/_data.cgi`, `party2/lib/system.cgi`, `party2/lib/_casino.cgi`
- **Primary Domain Packages**: `internal/guild/`, `internal/contest/`, `internal/eventplaza/`, `internal/lottery/`, `internal/casino/`, `internal/collection/`, `internal/notification/`, `internal/notification/`, `internal/ranking/`, `internal/replay/`
- **Key Testing / Linter Focus**: ギルド寄付レベル上げの排除、宝くじ20枚上限・キャリーオーバー、商人3倍価格、8人共有カジノ

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/guild.cgi`, `lib/join_guild.cgi`, `party2/guild_list.cgi` | ギルド拠点・運営 | `internal/guild/` | `internal/api/http/guild.go`<br/>`migrations/016_guilds.sql` | `docs/design/guild.md`<br/>`docs/api/paths/guilds.json` | Compliant | 活動動的GP、ギルド名（16文字以内）、役職命名（6文字以内）、HEXカラー設定、20日無稼働自動解散。 |
| `lib/photo.cgi`, `party2/contest.cgi`, `party2/screen_shot.cgi` | 写真館・コンテスト | `internal/contest/` | `internal/api/http/contest.go`<br/>`migrations/046_photo_contest_and_gallery.sql` | `docs/design/photo-contest.md`<br/>`docs/api/paths/contest.json` | Fix closed; parity not certified (#801) | 対応履歴: #801。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/event.cgi` | イベント広場 (@旅の商人) | `internal/eventplaza/` | `internal/api/http/eventplaza.go`<br/>`migrations/038_eventplaza.sql`<br/>`migrations/080_eventplaza_presences.sql` | `docs/design/eventplaza.md`<br/>`docs/api/paths/eventplaza.json` | Compliant | 5分リアルタイム出現トリガー、販売価格は定価の3倍 (`* 3`)、ヘルパー対象除外、預かり所自動配送。 |
| `lib/takarakuzi.cgi` | 宝くじ (@クラゲ) | `internal/lottery/` | `internal/api/http/lottery.go`<br/>`migrations/081_takarakuji.sql`<br/>`migrations/098_correct_lottery_equipment_collection.sql` | `docs/design/lottery.md`<br/>`docs/api/paths/lottery.json` | Fix closed; parity not certified (#800, #1239) | 対応履歴: #800, #1239。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/lot.cgi` | 福引 (@案内人) | `internal/lottery/` | `internal/api/http/lottery.go`<br/>`migrations/018_lottery.sql` | `docs/design/lottery.md`<br/>`docs/api/paths/lottery.json` | Fix closed; parity not certified (#800) | 対応履歴: #800。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/casino.cgi`, `party2/casino.cgi`, `lib/_casino.cgi` | カジノラウンジ | `internal/casino/` | `internal/api/http/casino.go`<br/>`migrations/017_casino.sql` | `docs/design/casino.md`<br/>`docs/api/paths/casino.json` | Fix closed; parity not certified (#799) | 対応履歴: #799。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/casino_slot.cgi` | スロットマシン | `internal/casino/` | `internal/api/http/casino.go` | `docs/design/casino.md`<br/>`docs/api/paths/casino.json` | Compliant | 5絵柄倍率、ギャンブラー200枚BET、Wish 5ボーナス、疲労度加算。 |
| `lib/casino_highlow.cgi` | ハイローゲーム | `internal/casino/` | `internal/api/http/casino.go` | `docs/design/casino.md`<br/>`docs/api/paths/casino.json` | Fix closed; parity not certified (#799) | 対応履歴: #799。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/casino_indian.cgi` | インディアンポーカー | `internal/casino/` | `internal/api/http/casino.go`<br/>`migrations/054_casino_poker_sessions.sql` | `docs/design/casino.md`<br/>`docs/api/paths/casino.json` | Fix closed; parity not certified (#799) | 対応履歴: #799。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/casino_doppel.cgi` | ドッペルゲンガー | `internal/casino/` | `internal/api/http/casino.go` | `docs/design/casino.md`<br/>`docs/api/paths/casino.json` | Fix closed; parity not certified (#799) | 対応履歴: #799。現在の仕様・残差はDesign Docおよび棚卸し記録を参照。 |
| `lib/collection.cgi`, `lib/_add_collection.cgi`, `lib/_add_monster_book.cgi`, `party2/view_monster.cgi` | 図鑑コレクション | `internal/collection/` | `internal/api/http/collection.go`<br/>`migrations/021_collection.sql`<br/>`migrations/087_character_collection_completions.sql`<br/>`migrations/095_monster_book_encounter_stats.sql` | `docs/design/collection.md`<br/>`docs/api/paths/collection.json` | Compliant (#797, #1247) | 戦闘・ボス討伐勝利時のモンスター図鑑記録（RecordMonsterDefeat）配線、初遭遇時ステータス・報酬スナップショット保存（#1247）、100%コンプリート時通知・伝説トリガー |
| `party2/news.cgi` | 新聞ログ | `internal/notification/` | `internal/api/http/notification.go`<br/>`migrations/033_news_and_notifications.sql` | `docs/design/notifications.md`<br/>`docs/api/paths/notification.json` | Compliant | 殿堂入り、イベント当選者、速報。 |
| `party2/ranking.cgi`, `week_ranking.cgi`, `job_ranking.cgi`, `legend.cgi` | サーバー各種ランキング・殿堂入り | `internal/ranking/` | `internal/api/http/ranking.go`<br/>`migrations/035_rankings_and_leaderboards.sql`<br/>`migrations/089_legend_and_week_ranking.sql`<br/>`migrations/097_job_popularity_stats.sql` | `docs/design/ranking.md`<br/>`docs/api/paths/ranking.json`<br/>`docs/api/paths/legends.json` | Compliant (#798, #1241) | 殿堂入り（legend.cgi 6称号永久記録）、週間転職ランキング（week_ranking.cgi 日曜0時週次ローテ・スナップショット凍結）、人気職業ランキング累積ポイント復元（job_ranking.cgi / #1241）、勝負師(cas_c)・錬金(alc_c)ランキング復元 |
| `party2/replay.cgi` | 戦闘リプレイ再生 | `internal/replay/` | `internal/api/http/replay.go`<br/>`migrations/027_battle_replays.sql` | `docs/design/replay.md`<br/>`docs/api/paths/replays.json` | Compliant (#796) | 戦闘リプレイ再生・個別マッチ履歴・グローバル最新フィードHTTPエンドポイント配線 |

---

## 3. Data & Master Files Catalog (`party2/stage/`, `challenge/`, `map/`)

| Script / Directory | Contents | Target Implementation in Go | Migration Status |
| :--- | :--- | :--- | :---: |
| `party2/stage/0.cgi` .. `27.cgi` | 28の通常冒険ステージ構成・敵出現テーブル | `internal/adventure/data/stages.json`<br/>`docs/design/stages-and-monsters.md` | Compliant |
| `party2/stage/king1.cgi` .. `king99.cgi` | 10体の封印魔王ステージ構成・耐性テーブル | `internal/boss/catalog.go`<br/>`docs/design/boss.md` | Compliant |
| `party2/challenge/0.cgi` .. `8.cgi` | 9段階の連戦チャレンジステージ構成 | `internal/challenge/data/challenge_tiers.json`<br/>`docs/design/challenge.md` | Compliant |
| `party2/map/0` .. `17` | ダンジョンマップ階層データ・タイル行列 | `internal/dungeon/catalog.go`<br/>`docs/design/dungeon.md` | Compliant |
