# Legacy CGI to Go Navigation Catalog

This catalog maps selected legacy CGI clusters to Go domains, transport sources, and design documents. Paths in the Legacy Script column are relative to the original Party2 root. It is a navigation inventory, not an exhaustive CGI coverage or behavioral-parity certificate.

It is structured into **7 Domain Clusters (CL-01 to CL-07)** for navigation. CL-02 has five individually selectable audit units, **CL-02A to CL-02E**, scoped by actions and state transitions rather than whole CGI files. Closed remediation issues are history, not evidence that every behavior is equivalent; see [the documentation audit](documentation-audit.md).

---

## 1. Critical Misnomer & Pitfall Warnings (取り違え厳禁)

| Misnomer Trap | Authentic Legacy Specification | Go Implementation Guideline |
|---|---|---|
| **`farm.cgi` (Monster Ranch)** | **モンスター牧場 (@モンジィ)**: 仲間モンスター保管（50〜350匹、転職100回以上の恩恵含む）、自宅ペット連携（8枠）、改名（8文字）、P2P譲渡、野生への解放。**作物・畑の要素は一切存在しない**。 | Dedicated to `internal/monster`. Fictional crop logic purged. |
| **`plantation.cgi` (Seed Cultivation)** | **種菜園 (@ロータス)**: 6種の種（赤/青/黄/緑/銀/金）、14種の特殊肥料、枯れ率計算、翌朝タイマー、預かり所（depot）への収穫物直送。 | Dedicated to `internal/plantation`. |
| **`reborn.cgi` / `altar.cgi` (No Rebirth)** | **復活の祭壇**: 6オーブ奉納・ラーミア復活・旅行アイテムの願い。レベル1リセットは存在しない。Lv99→150限界突破は別施設の天界 (`god.cgi`)。 | Purge fictional Rebirth system; restore OverLevel cap in celestial god facility (`god.cgi`). |
| **`guild.cgi` (No Donation Leveling)** | **ギルド拠点**: ゴールド寄付によるギルドLv1〜10上げは存在しない。**活動による動的GP**、自由役職命名（6文字）、申請承認制、HEXカラー。 | Purge fictional donation levels; restore activity GP & custom roles. |
| **`sleep.cgi` / `home.cgi` (No Paid Inn)** | **自宅での睡眠**: 有料の宿屋は存在しない。**自宅（または他人の家）で寝る**ことで無料全快・日次フラグリセット。 | Decommissioned separate inn package; integrated into `internal/home`. |
| **`free.cgi` (Flea Market)** | **フリーマーケット**: 旧ファイル名は `fleamarket.cgi` ではなく `lib/free.cgi`。預かり所から直接出品・引出。 | Dedicated to `internal/fleamarket`. |
| **`secret.cgi` (Secret Shop)** | **秘密の店**: 旧ファイル名は `lib/secret.cgi`。転職7回以上（`job_lv >= 7`）、原典8アイテム3倍価格、満杯時Depot転送、ぱふぱふ（会話のみ）。 | Owned by `internal/secretshop`. |
| **`black_market.cgi` (Black Market)** | **闇市場**: ゴールド売買・相場変動・日次枠を全撤廃し、純粋なレアポイント生贄（手持ち/Depot）と24種景品Depot直送に回帰。 | Owned by `internal/blackmarket`. |

---

## 2. Domain Cluster Audit Catalog (全7クラスタ詳細マッピング)

### CL-01: Meta & Player Identity (基盤・アカウント・ナビゲーション)
- **Cluster Summary**: プレイヤーアカウント、キャラクター生成・管理、認証トークン、緊急脱出、管理者機能、ナビゲーション
- **Shared Dependencies**: `party2/lib/system.cgi` (ユーザーファイルI/O, セッション, アクセス制御 / 更新連打ペナルティ廃止・レートリミット置換承認は [design/player-and-character.md](../design/player-and-character.md#27-request-throttling-and-reload-sleep-penalty-reconciliation-approved-departure) 参照)
- **Primary Domain Packages**: `internal/player/`, `internal/character/`, `internal/rescue/`, `internal/maintenance/`, `internal/id/`
- **Key Testing / Linter Focus**: AST Auth Wrappers (`internal/api/http/auth_lint_test.go`), IP抽出, 不正トークン破棄

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :---: | :--- |
| `index.cgi`, `_side_menu.cgi` | メインポータル・ナビゲーション | Client Presentation | N/A | Planned | サーバー側ではなくフロントエンドUIで提供 |
| `login.cgi` | 認証・セッション発行・日次処理 | `internal/player/` | [design/player-and-character.md](../design/player-and-character.md)<br/>[PAT contract](../architecture/interfaces.md#personal-access-tokens-and-dual-authentication)<br/>[api/paths/auth.json](../api/paths/auth.json) | Approved reload-sleep departure (#1106); review against source | 短時間アクセス・更新連打ペナルティ（睡眠加算）は廃止し、HTTP 429（`internal/ratelimit`）およびアカウントBANへ置換。睡眠タイマーは通信頻度で汚染しない（#1106承認）。詳細は [design/player-and-character.md](../design/player-and-character.md#27-request-throttling-and-reload-sleep-penalty-reconciliation-approved-departure) 参照。 |
| `new_entry.cgi` | キャラクター新規作成 | `internal/player/`<br/>`internal/character/` | [design/player-and-character.md](../design/player-and-character.md)<br/>[api/paths/character.json](../api/paths/character.json) | Reconciled registration population limits (#1105); review against source | 定員上限（`max_entry`）はPlayerアカウント単位の環境変数 `PARTY2_MAX_PLAYERS` による定員判定（409 Conflict）として設計（#1105承認）。キャラ名は1〜8文字・空白拒否（`new_entry.cgi:110–114`）。詳細は [design/player-and-character.md](../design/player-and-character.md#28-registration-population-limit--capacity-reconciliation-1105) 参照。 |
| `player.cgi`, `player_old.cgi` | プレイヤーの軌跡 (Memory log & 4大コンプリート率) | *(Unimplemented)* | [design/player-and-character.md](../design/player-and-character.md) | Reconciling (#773) | 原典の軌跡ログ(`memory.cgi`, `write_memory`)および4大コンプリート率(武器・防具・道具141・錬金)の取得APIが未実装 |
| `my.cgi`, `my2.cgi` | 外部ブログパーツ１・２ | Client / Widget API | [design/player-and-character.md](../design/player-and-character.md) | Reconciling | 内部ダッシュボードではなく外部サイト貼り付け用JS(`document.write`)。ステータスミニカード・同居人台詞表示 |
| `profile.cgi`, `lib/profile.cgi` | 公開プロフィール表示・編集 | `internal/character/` | [design/player-and-character.md](../design/player-and-character.md)<br/>[api/paths/character.json](../api/paths/character.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `delete.cgi` | キャラクター/アカウント削除 | `internal/player/`<br/>`internal/character/` | [design/player-and-character.md](../design/player-and-character.md)<br/>[design/player-deletion.md](../design/player-deletion.md)<br/>[api/paths/player.json](../api/paths/player.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `rescue.cgi` | 救済・スタック復帰 | `internal/rescue/` | [design/rescue-and-helper.md](../design/rescue-and-helper.md)<br/>[api/paths/rescue.json](../api/paths/rescue.json) | Review against source | 架空の24時間以内再救出ペナルティ倍増(1200秒)の削除。町内安全時の無ペナルティ復帰ガード |
| `admin.cgi`, `maintenance.cgi` | 管理者操作・メンテナンス制御 | `internal/maintenance/` | [Maintenance](../design/admin.md#system-maintenance-mode)<br/>[api/paths/admin.json](../api/paths/admin.json) | Reconciling | メンテナンスモード切り替え、BAN、管理者による無ペナルティ強制復帰(`admin_refresh`)の追加 |
| `link.cgi` | 外部リンク・コミュニティ | Client Presentation | N/A | Planned | 静的ナビゲーション・ワンクッション画面 |

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
- **Contracts**: [design/home.md](../design/home.md)、[design/town_park.md](../design/town_park.md)、[api/paths/home.json](../api/paths/home.json)、[api/paths/letter.json](../api/paths/letter.json)、[api/paths/park.json](../api/paths/park.json)、[api/paths/character.json](../api/paths/character.json) (色設定)。
- **Open follow-ups**: [#1094](https://github.com/witchcraze/party2re/issues/1094) (mailbox所有)、[#1110](https://github.com/witchcraze/party2re/issues/1110) (配送通知)、[#1115](https://github.com/witchcraze/party2re/issues/1115) (設定更新)、[#1116](https://github.com/witchcraze/party2re/issues/1116) (色更新)、[#1125](https://github.com/witchcraze/party2re/issues/1125) (ことば保持)、[#1126](https://github.com/witchcraze/party2re/issues/1126) (mail/park設定)。
- **Boundary**: Homeからの図鑑・ジョブマスター・プロフィール・冒険記録・特技設定・画像設定は遷移先と対象identityを確認する。遷移先の全仕様はCL-01/04/06/07の担当範囲。壁紙等の表示はここ、使用による変更はCL-02E。

#### CL-02B: Sleep & Recovery

- **Scope / Legacy**: `lib/home.cgi`の`ねる`、`lib/sleep.cgi`、共有dispatchの睡眠中操作制限。睡眠時間、訪問先の前提、起床回復、各機能のreset/完了と長期不在・再試行を追う。
- **Go / HTTP**: `internal/home/sleep.go`、`internal/core/timer/`、`internal/api/http/home_sleep.go`、`internal/api/http/action_home.go`、`internal/api/http/auth_helper.go`、`cmd/party2/wire.go`の回復連動配線。
- **Contracts**: [design/home.md](../design/home.md)、[design/action-preconditions.md](../design/action-preconditions.md)、[api/paths/home.json](../api/paths/home.json)。
- **Open follow-ups**: [#1117](https://github.com/witchcraze/party2re/issues/1117) (更新guard)、[#1118](https://github.com/witchcraze/party2re/issues/1118) (起床義務の寿命)、[#1119](https://github.com/witchcraze/party2re/issues/1119) (私有Home条件)、[#1123](https://github.com/witchcraze/party2re/issues/1123) (時間/人数consumer)。人数のidentity・時間窓はCL-01の[#1104](https://github.com/witchcraze/party2re/issues/1104)を参照する。
- **Boundary**: 満腹・祝福・錬金等は睡眠からの呼出し/失敗時整合を確認し、各機能全体の監査へ広げない。explicit Wakeとread-only GETの承認済み契約を維持する。

#### CL-02C: Depot & Transfers

- **Scope / Legacy**: `lib/depot.cgi`の`うる` / `まとめてうる`、`あずける` / `ひきだす`、`せいとん`、`おくる` (品/金)、`かくちょう`。共有`lib/system.cgi`の配送・未受取送金を含む。
- **Go / HTTP**: `internal/depot/`、`internal/database/depot_repository.go`、inventory/equipment保存境界、`internal/api/http/depot.go`。
- **Contracts**: [design/depot.md](../design/depot.md)、[design/items-and-equipment.md](../design/items-and-equipment.md)、[api/paths/depot.json](../api/paths/depot.json)。
- **Open follow-ups**: [#1109](https://github.com/witchcraze/party2re/issues/1109) (未受取送金/金額保存)、[#1111](https://github.com/witchcraze/party2re/issues/1111) (整頓順)、[#1112](https://github.com/witchcraze/party2re/issues/1112) (装備品移動)、[#1127](https://github.com/witchcraze/party2re/issues/1127) (引出し/装備交換)。通知の受取側はCL-02Aの#1110と合わせて確認する。
- **Boundary**: 他機能からのDepot直送は入庫/容量/失敗時の契約を確認する。売買・景品・生産など送信元機能の全仕様は各クラスタが所有する。HomeでのDepot品消費はCL-02E。

#### CL-02D: Towns & Construction

- **Scope / Legacy**: `lib/town1.cgi`〜`lib/town4.cgi`、`lib/_town.cgi`の`たてる` / `ちぇっく` / `みせ` / `はいる`。町の探索、住宅/商店の建設、容量、価格、所有期限、GP連動、所有者への遷移を確認する。
- **Go / HTTP**: `internal/town/`、`internal/home/estate.go`の住宅建設/期限確認、`internal/store/service.go`の商店建設、`internal/api/http/home_estate.go`、`internal/api/http/store.go`。
- **Contracts**: [design/home.md](../design/home.md)、[design/store.md](../design/store.md)、[api/paths/towns.json](../api/paths/towns.json)、[api/paths/houses.json](../api/paths/houses.json)、[api/paths/stores.json](../api/paths/stores.json)。
- **Open follow-ups**: [#1113](https://github.com/witchcraze/party2re/issues/1113) (住宅上限)、[#1114](https://github.com/witchcraze/party2re/issues/1114) (商店上限)、[#1122](https://github.com/witchcraze/party2re/issues/1122) (住宅GP)。
- **Boundary**: 商店の建設・入店までが本単位で、出品/購入/内装販売はCL-03。私有Homeへの訪問・睡眠と有料の町住宅契約を同一条件にしない。

#### CL-02E: Home Item Use

- **Scope / Legacy**: `lib/home.cgi`の`つかう`と到達する`lib/_data.cgi`の効果。手持ち/Depotの両source、装備inspect、usage category、効果・前提・乱数・消費順・容量不足/失敗時をアイテムごとに確認する。
- **Go / HTTP**: `internal/home/item_usage.go`、`internal/home/recipe_usage.go`、`internal/home/costume_usage.go`、`internal/api/http/home_estate.go`のitem一覧/使用。
- **Contracts**: [design/home.md](../design/home.md)、[design/blacksmith.md](../design/blacksmith.md)、[api/paths/character.json](../api/paths/character.json) (Home item一覧/使用)。
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

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/home.cgi` | 自宅拠点 (@自宅ペット) | `internal/home/` | [design/home.md](../design/home.md)<br/>[api/paths/home.json](../api/paths/home.json)<br/>[api/paths/letter.json](../api/paths/letter.json) | Reconciling | CL-02A/B/E。未解決の所有・更新・睡眠・使用差分は各監査単位および所有文書を参照。 |
| `lib/sleep.cgi` | 睡眠 (無料全快・日次リセット) | `internal/home/` | [design/home.md](../design/home.md)<br/>[api/paths/home.json](../api/paths/home.json) | Reconciling | CL-02B (#1117〜#1119, #1123)。有料宿屋は非存在。時間/人数・起床義務・訪問条件の契約は CL-02B および [design/home.md](../design/home.md) 参照。 |
| `lib/park.cgi` | 交流広場 (@町娘) | `internal/park/` | [design/town_park.md](../design/town_park.md)<br/>[api/paths/park.json](../api/paths/park.json) | Reconciling | CL-02A (#1126)。占い（22entry・27色）、NPC会話、チャット。文字数/log契約は未確定。10G回復は非存在。 |
| `lib/depot.cgi` | 預かり所 (@ニキータ) | `internal/depot/` | [design/depot.md](../design/depot.md)<br/>[api/paths/depot.json](../api/paths/depot.json) | Reconciling | CL-02C (#1109, #1111, #1112, #1127) と通知連動#1110。容量・拡張・売却・整頓・配送・引出しを操作ごとに監査する。 |
| `lib/town1.cgi` .. `town4.cgi`, `lib/_town.cgi` | 町1〜町4の探索・建設 | `internal/home/` (住宅建設)<br/>`internal/town/`<br/>`internal/store/` (商店建設)<br/>Client Presentation | [design/home.md](../design/home.md)<br/>[design/store.md](../design/store.md)<br/>[api/paths/towns.json](../api/paths/towns.json)<br/>[api/paths/houses.json](../api/paths/houses.json)<br/>[api/paths/stores.json](../api/paths/stores.json) | Reconciling | CL-02D (#1113, #1114, #1122)。町別上限・GP配線に未解決差分があり、設定/guardはCL-02A/Bと連動確認する。 |


---

### CL-03: Economy, Shops & Trading (商業・流通・装備強化)
- **Cluster Summary**: 武器・防具・道具・装飾品・宝石店・闇市・秘密の店、鍛冶屋、酒場、銀行、オークション、フリーマーケット、個人商店
- **Shared Dependencies**: `party2/lib/_data.cgi` (アイテム・装備マスタ定数), `party2/config.cgi`, `party2/lib/depot.cgi`
- **Primary Domain Packages**: `internal/shop/`, `internal/blacksmith/`, `internal/secretshop/`, `internal/blackmarket/`, `internal/gemstore/`, `internal/tavern/`, `internal/bank/`, `internal/auction/`, `internal/fleamarket/`, `internal/economy/`
- **Key Testing / Linter Focus**: 売却価格50%、MasterCard10%割引、刻印晶による武器刻印、Depot直結入出庫、並行購入デッドロック防止

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/weapon.cgi` | 武器屋 (@ブッキー) | `internal/shop/` | [design/shops.md](../design/shops.md)<br/>[design/items-and-equipment.md](../design/items-and-equipment.md)<br/>[api/paths/shop.json](../api/paths/shop.json) | Review against source | 定価2倍販売・50%売却、ジョブレベル別カタログ拡張、手助けクエスト除外、Depot自動転送 |
| `lib/armor.cgi` | 防具屋 (@アマノ) | `internal/shop/` | [design/shops.md](../design/shops.md)<br/>[api/paths/shop.json](../api/paths/shop.json) | Review against source | 定価2倍販売・50%売却、ジョブレベル別カタログ拡張、手助けクエスト除外、Depot自動転送 |
| `lib/item.cgi` | 道具屋 (@アイテムコ) | `internal/shop/` | [design/shops.md](../design/shops.md)<br/>[api/paths/shop.json](../api/paths/shop.json) | Review against source | 定価2倍販売・50%売却、ジョブレベル別カタログ拡張、秘密の店ヒント |
| `lib/accessory.cgi` | 装飾品屋 (@ミラ) | `internal/shop/` | [design/accessory-shop.md](../design/accessory-shop.md)<br/>[api/paths/shop.json](../api/paths/shop.json) | Review against source | カタログ・倍率・48レシピ・秘薬保証。成功確率の原典比較境界に差分あり。 |
| `lib/blacksmith.cgi` | 鍛冶屋 (@ブッキー) | `internal/blacksmith/` | [design/blacksmith.md](../design/blacksmith.md)<br/>[api/paths/blacksmith.json](../api/paths/blacksmith.json) | Review against source | 武器刻印12種(刻印晶消費)、装備名付け(武器・防具/20文字/サニタイズ)、専用武器預かり所(3枠/名前重複禁止/装備中引出不可) |
| `lib/secret.cgi` | 秘密の店 (@ヒミツジ) | `internal/secretshop/` | [design/shops.md](../design/shops.md)<br/>[api/paths/secretshop.json](../api/paths/secretshop.json) | Review against source | 転職7回以上(`job_lv >= 7`)、原典8種3倍価格、手持ち直接購入時の図鑑自動登録連携(`RecordItemDiscovered`)、満杯時Depot転送(`depot.FindOrCreate`)、ぱふぱふ会話のみ |
| `lib/black_market.cgi` | 闇市場 (@闇商人) | `internal/blackmarket/` | [design/black-market.md](../design/black-market.md)<br/>[api/paths/blackmarket.json](../api/paths/blackmarket.json) | Review against source | 純粋レアポイント物々交換、手持ち/Depot生贄、24種景品Depot直送、原典台詞 |
| `lib/gem_store.cgi` | 宝石店 (@ジェマ) | `internal/gemstore/` | [design/gemstore.md](../design/gemstore.md)<br/>[api/paths/gemstore.json](../api/paths/gemstore.json) | Review against source | 宝石購入(5倍価格・GemBox格納)、売却(50%)、56種加工レシピ、5種未鑑定オーブ鑑定。ギフト時のメール通知のみ未実装 |
| `lib/store.cgi`, `lib/goods.cgi` | プレイヤーストア・オラクル屋 | `internal/store/` | [design/store.md](../design/store.md)<br/>[api/paths/stores.json](../api/paths/stores.json) | Review against source | 店舗建設(50,000G/90日)、ゴールド・物々交換出品、壁紙50種（最大50,000G実控除）・看板(5,000G残高要・控除なし)・家具15種内装(1,000G残高要・控除なし)。詳細は [design/store.md](../design/store.md#3-store-customization-charges-and-legacy-parity-decision-1146) 参照。 |
| `lib/bar.cgi` | ルイーダの酒場 (@ルイーダ) | `internal/tavern/` | [design/tavern.md](../design/tavern.md)<br/>[api/paths/tavern.json](../api/paths/tavern.json) | Review against source | 食事注文・福引券付与、出前予約・冒険完了時自動配達フック（架空NPCおつかい・小包便撤廃） |
| `lib/bank.cgi` | ゴールド銀行 (@タクシード) | `internal/bank/` | [design/bank.md](../design/bank.md)<br/>[api/paths/bank.json](../api/paths/bank.json) | Review against source | 預金・引出、上限99兆G、所持金999,999G上限クランプ＆超過預金返還。架空の日次利息は非存在 |
| `lib/auction.cgi` | オークション (@ワイルド) | `internal/auction/` | [design/auction.md](../design/auction.md)<br/>[api/paths/auction.json](../api/paths/auction.json) | Review against source | リアルタイムP2P送金・装備/アイテムDepot直送譲渡(`＠おくる`)、他PC調査(`＠しらべる`)。架空の入札・競売システムは非存在 |
| `lib/free.cgi` | フリーマーケット | `internal/fleamarket/` | [design/fleamarket.md](../design/fleamarket.md)<br/>[api/paths/fleamarket.json](../api/paths/fleamarket.json) | Review against source | Depotから直接出品(サーバー上限120件・個人上限5+OverFlea)、購入品・キャンセル品はDepotへ即時格納 |

---

### CL-04: Progression, Faith & Limits (成長・職業・限界突破・信仰)
- **Cluster Summary**: 転職、職業極め、願いの泉（SP強化）、特技設定、改名、画像変更、転生の祭壇（限界突破）、礼拝堂、天界・裏天界、小さなメダル
- **Shared Dependencies**: `party2/lib/_data.cgi` (職業・特技定義), `party2/lib/system.cgi`
- **Primary Domain Packages**: `internal/job/`, `internal/wishingwell/`, `internal/customskill/`, `internal/altar/`, `internal/chapel/`, `internal/god/`, `internal/medal/`, `internal/core/progression/`
- **Key Testing / Linter Focus**: 転生（Lv1リセット）の完全排除、Lv99→150限界突破、礼拝堂の単一祈願制約、SPステータス成長

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/job_change.cgi` | 転職所 (@ダーマ神官) | `internal/job/` | [design/jobs-and-skills.md](../design/jobs-and-skills.md)<br/>[api/paths/character.json](../api/paths/character.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/job_master.cgi` | 職業極め所 | `internal/job/` | [design/jobs-and-skills.md](../design/jobs-and-skills.md)<br/>[api/paths/character.json](../api/paths/character.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/sp_change.cgi` | 願いの泉 (@女神) | `internal/wishingwell/` | [design/wishing-well.md](../design/wishing-well.md)<br/>[api/paths/wishing_well.json](../api/paths/wishing_well.json) | Review against source | SPを消費して5大ステータス強化。OverLevel/JobMemoryガード |
| `lib/custom_skill.cgi` | 特技設定 (@マニャ) | `internal/customskill/` | [design/custom_skill.md](../design/custom_skill.md)<br/>[api/paths/customskill.json](../api/paths/customskill.json) | Review against source | 3宝玉スロット合成、GemBox消費・返還接続、転職後キャパシティガード解除、CMP計算式復元 |
| `lib/name_change.cgi` | 命名の館 (@アストロン) | `internal/character/` | [design/character-customization.md](../design/character-customization.md)<br/>[api/paths/character.json](../api/paths/character.json) | Review against source | ゴールド手数料(名前変更: 500,000G、性転換: 10,000G)、名前文字数1〜8文字・空白拒否（`name_change.cgi:121–125`）、重複ネーム検証、ギルド・フリマガード、現在職の性別制限検証 |
| `lib/custom_image.cgi`, `lib/upload_image.cgi` | 画像設定所 | `internal/character/` | [design/character-customization.md](../design/character-customization.md)<br/>[api/paths/character.json](../api/paths/character.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/reborn.cgi` | 復活の祭壇 (@巫女) | `internal/altar/`<br/>`internal/core/progression/` | [design/altar-of-rebirth.md](../design/altar-of-rebirth.md)<br/>[design/progression.md](../design/progression.md)<br/>[api/paths/altar.json](../api/paths/altar.json) | Review against source | ラーミア復活(6オーブ/30分)、4大旅行アイテムDepot直送。🚨 Lv1転生は架空として完全撤廃済。Lv99→150限界突破は天界(`god.cgi`)で実装 |
| `lib/chapel.cgi` | 礼拝堂 (@シスター) | `internal/chapel/` | [design/chapel.md](../design/chapel.md)<br/>[api/paths/chapel.json](../api/paths/chapel.json) | Review against source | 祈願枠と効果期間の分離、同日睡眠後の再祈願拒否、日跨ぎ睡眠による枠解除・効果終了、残留chapel_reset安全化。詳細は [design/chapel.md](../design/chapel.md) 参照。 |
| `lib/god.cgi` | 天界 (@神) | `internal/god/` | [design/god.md](../design/god.md)<br/>[api/paths/god.json](../api/paths/god.json) | Review against source | 20種の願い(メイド・オルテガ・アバター・全ステータス+40: 現在HP/MP維持)、Lv99→150限界突破 |
| `lib/u_god.cgi` | 裏天界 (@裏神) | `internal/god/` | [design/god.md](../design/god.md)<br/>[api/paths/god.json](../api/paths/god.json) | Review against source | 5大保管庫拡張(牧場・預かり所+50/段、各最大5段階) |
| `lib/medal.cgi` | メダル王 (@メダル王) | `internal/medal/` | [design/medal.md](../design/medal.md)<br/>[design/achievements.md](../design/achievements.md)<br/>[api/paths/medal.json](../api/paths/medal.json) | Review against source | 全15段階の小さなメダル景品交換、景品はDepotへ直送 |
| `lib/exile.cgi` | 荒らし追放騎士団 | `internal/maintenance/` (将来) | [Maintenance](../design/admin.md#system-maintenance-mode) | Backlog | プレイヤー自治による追放投票機能 |

---

### CL-05: Production, Cultivation & Monsters (生産・菜園・モンスター牧場)
- **Cluster Summary**: 錬金堂（レシピ合成）、モンスター牧場（仲間預託・ペット連携）、種菜園（栽培・肥料）
- **Shared Dependencies**: `party2/lib/_data.cgi`, `party2/lib/_alchemy_recipe.cgi`, `party2/lib/depot.cgi`
- **Primary Domain Packages**: `internal/alchemy/`, `internal/monster/`, `internal/plantation/`
- **Key Testing / Linter Focus**: 牧場から畑要素を全削除、錬金の翌朝タイマー・Depot直結、菜園の14種肥料・枯れ率計算

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/alchemy.cgi`, `lib/_alchemy_recipe.cgi` | 錬金堂 (@トロデ) | `internal/alchemy/` | [design/alchemy.md](../design/alchemy.md)<br/>[api/openapi.json](../api/openapi.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/farm.cgi` | モンスター牧場 (@モンジィ) | `internal/monster/` | [design/monster.md](../design/monster.md)<br/>[api/openapi.json](../api/openapi.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/plantation.cgi` | 種菜園 (@ロータス) | `internal/plantation/` | [design/plantation.md](../design/plantation.md)<br/>[api/paths/character.json](../api/paths/character.json) | Review against source | 6種の種、14種の肥料、枯れ率、翌朝収穫・Depot配送の仕様はDesign Docと原典を照合。 |

---

### CL-06: Combat, Adventure & Dungeons (冒険・ボス・戦闘・PVP/GVG・ダンジョン)
- **Cluster Summary**: 冒険（ダンジョン10階層）、封印の魔王、コア戦闘・スキル発動エンジン、闘技場、ギルド戦、パーティ共闘、連戦チャレンジ、手助け
- **Shared Dependencies**: `party2/lib/_battle.cgi`, `party2/lib/_skill.cgi`, `party2/lib/ActionCounter.pm`, `party2/lib/TurnEndProcessor.pm`, `party2/lib/_data.cgi`
- **Data Dependencies**: `party2/stage/0..27.cgi`, `party2/stage/king1..99.cgi`, `party2/challenge/0..8.cgi`, `party2/map/*`
- **Primary Domain Packages**: `internal/adventure/`, `internal/boss/`, `internal/core/battle/`, `internal/core/skill/`, `internal/pvp/`, `internal/gvg/`, `internal/dungeon/`, `internal/challenge/`, `internal/party/`, `internal/helperquest/`
- **Key Testing / Linter Focus**: 逃走ペナルティ、10階層制覇、挑戦条件・調査・封印、実時間ベッティング、パーティ相乗効果（2p=+10%, 3p=+20%, 4p=+30%）

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/vs_monster.cgi`, `lib/adventure.cgi`, `lib/adventure_record.cgi` | 冒険・階層探索 (Adventure) | `internal/adventure/` | [design/adventure.md](../design/adventure.md)<br/>[design/stages-and-monsters.md](../design/stages-and-monsters.md)<br/>[api/paths/adventure.json](../api/paths/adventure.json) | Partial; interaction missing (#1323) | 開始一要求で全戦闘・階層・調査を自動完了する重大gap。原典 `vs_monster.cgi:35–82` / `_battle.cgi:561–659,1169–1178` / `_npc_action.cgi:89–155` の明示action・対象・行動間隔・NPC反撃・個別退出を復元する。#478の1時間待機撤去は操作省略の承認ではなく、#1014は既存transport接続のみ。 |
| `lib/vs_king.cgi`, `lib/_win_vs_king.cgi` | 封印の魔王 (Boss) | `internal/boss/` | [design/boss.md](../design/boss.md)<br/>[api/paths/boss.json](../api/paths/boss.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/_battle.cgi` | コア戦闘エンジン | `internal/core/battle/` | [design/battle.md](../design/battle.md) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/_skill.cgi` | スキル発動エンジン | `internal/core/skill/` | [design/jobs-and-skills.md](../design/jobs-and-skills.md) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/vs_player.cgi` | 闘技場 (PvP Arena) | `internal/pvp/` | [design/pvp.md](../design/pvp.md)<br/>[api/paths/pvp.json](../api/paths/pvp.json) | Review against source | 8人9色の投票・オッズ分配・賞金精算準拠。非同期Eloは完全撤廃済 |
| `lib/vs_guild.cgi` | ギルド戦 (GvG Arena) | `internal/gvg/` | [design/gvg.md](../design/gvg.md) | Review against source | 5勝ごとのトロフィー昇格（銅杯〜優勝杯）、ラウンドGP(+3)、参加GP(+4)、賞金精算準拠。非同期Eloは完全撤廃済 |
| `lib/vs_dungeon.cgi`, `party2/dungeon.cgi` | ダンジョン探索 | `internal/dungeon/` | [design/dungeon.md](../design/dungeon.md)<br/>[api/paths/dungeon.json](../api/paths/dungeon.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/vs_challenge.cgi`, `party2/challenge.cgi` | 連戦チャレンジ | `internal/challenge/` | [design/challenge.md](../design/challenge.md)<br/>[api/paths/challenge.json](../api/paths/challenge.json) | Review against source | 原典9ステージ(0最弱逆襲..8真最強王者)、+10%固定モンスター成長、10%宝箱階層トリガー、全滅時100%報酬獲得 |
| `lib/quest.cgi`, `party2/party.cgi` | パーティ共闘クエスト | `internal/party/` | [design/party-system.md](../design/party-system.md)<br/>[api/paths/party.json](../api/paths/party.json) | Partial; interaction missing (#1323) | Valkey lobbyとSQL精算は存在するが、冒険中の操作・継続state・本人ごとの退出は未再現。原典 `_battle.cgi:1184–1278` の退出をlobby leaveや全員同時精算で代替しない。#948の観測/command移行と分けて復元する。 |
| `lib/helper.cgi` | 手助けクエスト | `internal/helperquest/` | [design/rescue-and-helper.md](../design/rescue-and-helper.md)<br/>[api/paths/helper.json](../api/paths/helper.json) | Partial; zero bootstrap selected (#1226) | `みる` → `ListQuests`、`かいけつ` → `CompleteQuest`。自動bootstrapは0件（歴史上の初期運用件数は不明、#1226承認）。期限切れtarget完了・非target交換・販売除外は [Helper lifecycle契約](../design/rescue-and-helper.md#lifecycle-and-expiration) を、抽選pool・重み・報酬契約は [rescue-and-helper.md](../design/rescue-and-helper.md#quest-variations--probabilities) を参照。 |

---

### CL-07: Social, Entertainment & Gambling (ギルド・娯楽・カジノ・ランキング・図鑑)
- **Cluster Summary**: ギルド（動的GP・役職）、写真館・コンテスト、イベント広場、宝くじ、福引、カジノ（スロット・ハイロー・インディアンポーカー・ドッペル）、図鑑、新聞、各種ランキング、リプレイ
- **Shared Dependencies**: `party2/lib/_data.cgi`, `party2/lib/system.cgi`, `party2/lib/_casino.cgi`
- **Primary Domain Packages**: `internal/guild/`, `internal/contest/`, `internal/eventplaza/`, `internal/lottery/`, `internal/casino/`, `internal/collection/`, `internal/notification/`, `internal/ranking/`, `internal/replay/`
- **Key Testing / Linter Focus**: ギルド寄付レベル上げの排除、宝くじ20枚上限・キャリーオーバー、商人3倍価格、8人共有カジノ

| Legacy Script | Authentic Role / Action | Go Domain Implementation | Design Doc & OpenAPI | Review boundary | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/guild.cgi`, `lib/join_guild.cgi`, `party2/guild_list.cgi` | ギルド拠点・運営 | `internal/guild/` | [design/guild.md](../design/guild.md)<br/>[api/paths/guilds.json](../api/paths/guilds.json) | Review against source | 活動動的GP、ギルド名（16文字以内）、役職命名（6文字以内）、HEXカラー設定、20日無稼働自動解散。 |
| `lib/photo.cgi`, `party2/contest.cgi`, `party2/screen_shot.cgi` | 写真館・コンテスト | `internal/contest/` | [景品の原典根拠・採用契約](../design/photo-contest.md#44-prize-structure--rewards)<br/>[api/paths/contest.json](../api/paths/contest.json) | Partial (#1250) | 原典に合わせGold受取待ち・メダルDepot配送を採用。入賞者本人9/5/2枚、優勝投票者各1枚。賞品表10/6/3枚への補正やscope外変数への追加1枚配送は採用しない。Goのwallet/counter即時加算との実装差が残るため、仕様判断のみでCompliantとしない。 |
| `lib/event.cgi`, `lib/_win_vs_king.cgi` | イベント広場 (@旅の商人)・封印祝賀会 | `internal/eventplaza/` | [祝賀会の原典根拠・採用契約](../design/eventplaza.md#3-victory-celebrations-legacy-contract-and-implementation-gaps)<br/>[api/paths/eventplaza.json](../api/paths/eventplaza.json) | Reconciled for listed actions (#1248) | `かう` → `ListAvailableBazaarItems` / `PurchaseBazaarItem`。商人の3倍価格・ヘルパー除外・預かり所配送、封印祝賀会の群衆・商人判定契約は [design/eventplaza.md](../design/eventplaza.md#3-victory-celebrations-legacy-contract-and-implementation-gaps) 参照（#1248承認）。全体parityは未認定。 |
| `lib/takarakuzi.cgi` | 宝くじ (@クラゲ) | `internal/lottery/` | [design/lottery.md](../design/lottery.md)<br/>[api/paths/lottery.json](../api/paths/lottery.json) | Intentional draw departure approved (#1249); full parity not certified | `かう` → `BuyTakarakujiTicket`、`しょうひん` → `GetTakarakujiStatus`。末尾参加枠除外を採用せず全枠抽選を明示承認（#1249承認）。確率は [Design §3–4](../design/lottery.md#3-prize-catalog--candidate-lineup) が所有。倉庫満杯時は #1139。 |
| `lib/lot.cgi` | 福引 (@案内人) | `internal/lottery/` | [design/lottery.md](../design/lottery.md)<br/>[api/paths/lottery.json](../api/paths/lottery.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/casino.cgi`, `party2/casino.cgi`, `lib/_casino.cgi` | カジノラウンジ | `internal/casino/` | [design/casino.md](../design/casino.md)<br/>[api/paths/casino.json](../api/paths/casino.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/casino_slot.cgi` | スロットマシン | `internal/casino/` | [design/casino.md](../design/casino.md)<br/>[api/paths/casino.json](../api/paths/casino.json) | Review against source | 5絵柄倍率、ギャンブラー200枚BET、Wish 5ボーナス、疲労度加算。 |
| `lib/casino_highlow.cgi` | ハイローゲーム | `internal/casino/` | [design/casino.md](../design/casino.md)<br/>[api/paths/casino.json](../api/paths/casino.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/casino_indian.cgi` | インディアンポーカー | `internal/casino/` | [design/casino.md](../design/casino.md)<br/>[api/paths/casino.json](../api/paths/casino.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/casino_doppel.cgi` | ドッペルゲンガー | `internal/casino/` | [design/casino.md](../design/casino.md)<br/>[api/paths/casino.json](../api/paths/casino.json) | Review against source | 仕様・残差は所有 Design Doc と原典を照合する。 |
| `lib/collection.cgi`, `lib/_add_collection.cgi`, `lib/_add_monster_book.cgi`, `party2/view_monster.cgi` | 図鑑コレクション | `internal/collection/` | [design/collection.md](../design/collection.md)<br/>[api/paths/collection.json](../api/paths/collection.json) | Review against source | 戦闘・ボス討伐勝利時のモンスター図鑑記録（RecordMonsterDefeat）配線、初遭遇時ステータス・報酬スナップショット保存、100%コンプリート時通知・伝説トリガー確定境界と永続失敗回復。詳細は [design/collection.md](../design/collection.md) 参照。 |
| `party2/news.cgi` | 新聞ログ | `internal/notification/` | [design/notifications.md](../design/notifications.md)<br/>[api/paths/notification.json](../api/paths/notification.json) | Review against source | 殿堂入り、イベント当選者、速報。 |
| `party2/ranking.cgi`, `week_ranking.cgi`, `job_ranking.cgi`, `legend.cgi` | サーバー各種ランキング・殿堂入り | `internal/ranking/` | [design/ranking.md](../design/ranking.md)<br/>[api/paths/ranking.json](../api/paths/ranking.json)<br/>[api/paths/legends.json](../api/paths/legends.json) | Approved weekly timing departure (#1252); durable recurrence/recovery (#1314) | 殿堂入り（6称号永久記録）、人気職業・勝負師・錬金ランキング復元。週間転職ランキングはJST日曜0時予定・遅延分一括確定を意図的変更として承認（#1252承認）。週次継続・次回予約復旧手順は [design/ranking.md](../design/ranking.md#3-weekly-job-change-ranking-week_rankingcgi) 参照。 |
| `party2/replay.cgi` | 個人／公開リプレイ再生 | `internal/replay/` | [design/replay.md](../design/replay.md)<br/>[api/paths/replays.json](../api/paths/replays.json) | Partial; scene/capture undecided (#1320) | 受取キャラクターの所有者のみ閲覧可能。原典の通常冒険での画面記録・退出本人への最新スロット上書き保存は未再現（#1320）。詳細は [design/replay.md](../design/replay.md) 参照。 |

---

## 3. Data & Master Files Catalog (`party2/stage/`, `challenge/`, `map/`)

| Script / Directory | Contents | Target Implementation in Go | Migration Status |
| :--- | :--- | :--- | :---: |
| `party2/stage/0.cgi` .. `27.cgi` | 28の通常冒険ステージ構成・敵出現テーブル | `internal/adventure/data/stages.json`<br/>[design/stages-and-monsters.md](../design/stages-and-monsters.md) | Compliant |
| `party2/stage/king1.cgi` .. `king99.cgi` | 10体の封印魔王ステージ構成・耐性テーブル | `internal/boss/catalog.go`<br/>[design/boss.md](../design/boss.md) | Compliant |
| `party2/challenge/0.cgi` .. `8.cgi` | 9段階の連戦チャレンジステージ構成 | `internal/challenge/data/challenge_tiers.json`<br/>[design/challenge.md](../design/challenge.md) | Compliant |
| `party2/map/0` .. `17` | ダンジョンマップ階層データ・タイル行列 | `internal/dungeon/catalog.go`<br/>[design/dungeon.md](../design/dungeon.md) | Compliant |
