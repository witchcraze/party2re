# Complete Legacy CGI to Go Architecture Mapping Catalog

This catalog is the Single Source of Truth (SSOT) mapping every single Perl CGI script from the legacy Party2 codebase (`/home/witchcraze/dev/party2/party2/`) to its corresponding Go domain module, HTTP endpoint, database migration, and design/API document.

It is structured into **7 Domain Clusters (CL-01 to CL-07)** with complete file paths to enable zero-discovery subagent audit orchestration.

---

## 1. Critical Misnomer & Pitfall Warnings (取り違え厳禁)

| Misnomer Trap | Authentic Legacy Specification | Go Implementation Guideline |
| :--- | :--- | :--- |
| **`farm.cgi` (Monster Ranch)** | **モンスター牧場 (@モンジィ)**: 仲間モンスター保管（30/32匹）、自宅ペット連携（8枠）、改名（8文字）、P2P譲渡、野生への解放。**作物・畑の要素は一切存在しない**。 | Dedicated to `internal/monsterranch` (or clean `internal/farm`). Crop logic must be purged. (Issue #488) |
| **`plantation.cgi` (Seed Cultivation)** | **種菜園 (@ロータス)**: 6種の種（赤/青/黄/緑/銀/金）、14種の特殊肥料、枯れ率計算、翌朝タイマー、預かり所（depot）への収穫物直送。 | Dedicated to `internal/plantation` (Issue #489). |
| **`reborn.cgi` / `altar.cgi` (No Rebirth)** | **転生の祭壇**: レベル1リセット（転生）は存在しない。**Lv99→150の限界突破（OverLevel）**および裏天界解放のみ。 | Purge fictional Rebirth system; restore OverLevel cap (Issue #470, #471). |
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
| `login.cgi` | 認証・セッション発行 | `internal/player/` | `internal/api/http/token.go`<br/>`migrations/010_players_sessions.sql`<br/>`migrations/053_player_api_tokens.sql` | `docs/design/player-and-character.md`<br/>`docs/design/api-tokens.md`<br/>`docs/api/paths/auth.json` | Compliant | セッションCookieからBearer API Tokenへ安全にモダナイズ |
| `new_entry.cgi` | キャラクター新規作成 | `internal/player/`<br/>`internal/character/` | `internal/api/http/character.go`<br/>`migrations/002_characters.sql`<br/>`migrations/004_character_initial_state.sql` | `docs/design/player-and-character.md`<br/>`docs/api/paths/character.json` | Compliant | 初期ステータス（種族・性別・初期アイテム付与）の厳格な一致 |
| `my.cgi`, `my2.cgi` | キャラクターダッシュボード | `internal/character/` | `internal/api/http/character.go` | `docs/design/player-and-character.md`<br/>`docs/api/paths/character.json` | Compliant | 装備・インベントリ・現在状態の俯瞰表示 |
| `status.cgi` | 詳細ステータス・マスタリー表示 | `internal/character/` | `internal/api/http/character.go` | `docs/design/player-and-character.md`<br/>`docs/api/paths/character.json` | Compliant | パラメータ計算、熟練度、称号一覧 |
| `profile.cgi`, `lib/profile.cgi` | 公開プロフィール表示・編集 | `internal/character/` | `internal/api/http/character.go`<br/>`migrations/047_character_profile_and_customization.sql` | `docs/design/player-and-character.md`<br/>`docs/api/paths/character.json` | Compliant | 自己紹介文、好物、対戦アイコン設定 |
| `delete.cgi` | キャラクター/アカウント削除 | `internal/player/` | `internal/api/http/character.go`<br/>`migrations/049_player_deletion_and_maintenance.sql` | `docs/design/player-deletion.md`<br/>`docs/api/paths/player.json` | Compliant | 関連リソース（Depot, 自宅, 手紙等）のカスケード削除整合性 |
| `rescue.cgi` | 救済・スタック復帰 | `internal/rescue/` | `internal/api/http/helper_rescue.go` | `docs/design/rescue-and-helper.md`<br/>`docs/api/paths/rescue.json` | Compliant | エラー・進行不能時に町へ座標を安全リセット |
| `admin.cgi`, `maintenance.cgi` | 管理者操作・メンテナンス制御 | `internal/maintenance/` | `internal/api/http/maintenance.go` | `docs/design/system-maintenance.md`<br/>`docs/api/paths/admin.json` | Compliant | メンテナンスモード切り替え、緊急停止、BAN |
| `link.cgi` | 外部リンク・コミュニティ | Client Presentation | Frontend / Static HTML | N/A | Planned | 静的ナビゲーション |

---

### CL-02: Living, Housing & Towns (生活・拠点・預かり所)
- **Cluster Summary**: プレイヤーの生活拠点（自宅、睡眠）、手紙、町の探索、公園、預かり所（Depot）
- **Shared Dependencies**: `party2/lib/_npc_action.cgi`, `party2/lib/system.cgi`
- **Primary Domain Packages**: `internal/home/`, `internal/depot/`, `internal/park/`
- **Key Testing / Linter Focus**: 有料宿屋の排除、Depotの排他制御・保管上限、日次フラグリセット

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/home.cgi` | 自宅拠点 (@自宅ペット) | `internal/home/` | `internal/api/http/home.go`<br/>`migrations/034_player_home_and_mailbox.sql`<br/>`migrations/036_player_mailbox_independent_deletion.sql`<br/>`migrations/061_home_members_and_god_parity.sql` | `docs/design/home.md`<br/>`docs/api/paths/home.json` | Reconciling (#459, #461) | 自宅増築、言葉を教える、ペット8枠、手紙の独立削除管理 |
| `lib/sleep.cgi` | 睡眠 (無料全快・日次リセット) | `internal/home/` | `internal/api/http/home.go` | `docs/design/resting.md`<br/>`docs/api/paths/home.json` | Compliant (#459) | 🚨 有料の「宿屋 (`internal/inn`)」は架空として完全撤廃。自宅・他人の家で寝て無料回復＆日次リセット |
| `lib/park.cgi` | 公園 (@メリーヌ) | `internal/park/` | `internal/api/http/park.go`<br/>`migrations/032_town_park.sql` | `docs/design/town_park.md`<br/>`docs/api/paths/park.json` | Compliant | 10G少額回復、チャット掲示板 |
| `lib/depot.cgi` | 預かり所 (@モリー) | `internal/depot/` | `internal/api/http/depot.go`<br/>`migrations/011_depot.sql`<br/>`migrations/055_depot_parity.sql` | `docs/design/depot.md`<br/>`docs/api/paths/depot.json` | Reconciling (#460) | あずける/ひきだす/すべてあずける。他機能（錬金・菜園・オークション）のハブ保管庫 |
| `lib/town1.cgi` .. `town4.cgi`, `lib/_town.cgi` | 町1〜町4の探索・建設 | `internal/home/` (建設)<br/>Client Presentation | `internal/api/http/home.go` | `docs/design/home.md` | Reconciling (#461, #466) | 自宅建設（500G）、個人商店建設（10000G）、町の発展度 |

---

### CL-03: Economy, Shops & Trading (商業・流通・装備強化)
- **Cluster Summary**: 武器・防具・道具・装飾品・宝石店・闇市・秘密の店、鍛冶屋、酒場、銀行、オークション、フリーマーケット、個人商店
- **Shared Dependencies**: `party2/lib/_data.cgi` (アイテム・装備マスタ定数), `party2/config.cgi`, `party2/lib/depot.cgi`
- **Primary Domain Packages**: `internal/shop/`, `internal/blacksmith/`, `internal/secretshop/`, `internal/blackmarket/`, `internal/gemstore/`, `internal/tavern/`, `internal/bank/`, `internal/auction/`, `internal/fleamarket/`, `internal/economy/`
- **Key Testing / Linter Focus**: 売却価格50%、MasterCard10%割引、強化上限+10、Depot直結入出庫、並行購入デッドロック防止

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/weapon.cgi` | 武器屋 | `internal/shop/` | `internal/api/http/shop.go`<br/>`migrations/005_items_inventory.sql`<br/>`migrations/008_equipment.sql` | `docs/design/shops.md`<br/>`docs/design/items-and-equipment.md`<br/>`docs/api/paths/shop.json` | Reconciling (#465) | レベル制限、売却価格50% |
| `lib/armor.cgi` | 防具屋 | `internal/shop/` | `internal/api/http/shop.go` | `docs/design/shops.md`<br/>`docs/api/paths/shop.json` | Reconciling (#465) | レベル制限、売却価格50% |
| `lib/item.cgi`, `lib/goods.cgi` | 道具屋 | `internal/shop/` | `internal/api/http/shop.go` | `docs/design/shops.md`<br/>`docs/api/paths/shop.json` | Reconciling (#465) | MasterCard所持時の10%割引 |
| `lib/accessory.cgi` | 装飾品屋 | `internal/shop/` | `internal/api/http/shop.go` | `docs/design/shops.md`<br/>`docs/api/paths/shop.json` | Reconciling (#465) | 特殊効果アクセサリーのステータス計算 |
| `lib/blacksmith.cgi` | 鍛冶屋 (@コスタ) | `internal/blacksmith/` | `internal/api/http/handler.go`<br/>`migrations/012_item_enhancement.sql` | `docs/design/blacksmith.md` | Reconciling (#458) | 強化+1..+10、刻印、名付け（全角8文字） |
| `lib/secret.cgi` | 秘密の店 (@ヒミツジ) | `internal/secretshop/` | `internal/api/http/secretshop.go` | `docs/design/secretshop.md`<br/>`docs/api/paths/secretshop.json` | Completed (#462) | 転職7回以上(`job_lv >= 7`)、原典8種3倍価格、満杯時Depot転送、ぱふぱふ会話のみ |
| `lib/black_market.cgi` | 闇市場 (@闇商人) | `internal/blackmarket/` | `internal/api/http/blackmarket.go`<br/>`migrations/042_blackmarket_sacrifice_and_trade.sql`<br/>`migrations/065_drop_blackmarket_fictional_tables.sql` | `docs/design/black-market.md`<br/>`docs/api/paths/blackmarket.json` | Completed (#463) | 純粋レアポイント物々交換、手持ち/Depot生贄、24種景品Depot直送、原典台詞 |
| `lib/gem_store.cgi` | 宝石店 (@ルル) | `internal/gemstore/` | `internal/api/http/gemstore.go` | `docs/design/gemstore.md`<br/>`docs/api/paths/gemstore.json` | Reconciling (#464) | 宝石購入およびスキル枠への宝玉合成 |
| `lib/store.cgi`, `party2/store.cgi` | プレイヤー個人店舗 | `internal/shop/` (bazaar) | `internal/api/http/shop.go` | `docs/design/shops.md` | Reconciling (#466) | 看板設定、記帳、出品枠上限 |
| `lib/bar.cgi` | ルイーダの酒場 (@ルイーダ) | `internal/tavern/` | `internal/api/http/tavern.go`<br/>`migrations/039_tavern.sql`<br/>`migrations/069_drop_fictional_delivery_tables.sql` | `docs/design/tavern.md`<br/>`docs/design/delivery.md`<br/>`docs/api/paths/tavern.json` | Completed (#475) | 食事注文・福引券付与、出前予約・冒険完了時自動配達フック（架空NPCおつかい・小包便撤廃） |
| `lib/bank.cgi` | ゴールド銀行 | `internal/bank/` | `internal/api/http/handler.go`<br/>`migrations/014_bank.sql` | `docs/design/bank.md` | Reconciling (#476) | 預金・引出・送金、999,999,999G上限、日次利息計算 |
| `lib/auction.cgi` | オークション | `internal/auction/` | `internal/api/http/auction.go`<br/>`migrations/020_auctions.sql` | `docs/design/auction.md`<br/>`docs/api/paths/auction.json` | Reconciling (#474) | 出品・入札・落札、落札品はDepotへ直送 |
| `lib/free.cgi` | フリーマーケット | `internal/fleamarket/` | `internal/api/http/fleamarket.go`<br/>`migrations/043_fleamarket.sql` | `docs/design/fleamarket.md`<br/>`docs/api/paths/fleamarket.json` | Reconciling (#477) | Depotから直接出品・購入品はDepotへ即時格納 |

---

### CL-04: Progression, Faith & Limits (成長・職業・限界突破・信仰)
- **Cluster Summary**: 転職、職業極め、願いの泉（SP強化）、特技設定、改名、画像変更、転生の祭壇（限界突破）、礼拝堂、天界・裏天界、小さなメダル
- **Shared Dependencies**: `party2/lib/_data.cgi` (職業・特技定義), `party2/lib/system.cgi`
- **Primary Domain Packages**: `internal/job/`, `internal/wishingwell/`, `internal/custom_skill/`, `internal/altar/`, `internal/chapel/`, `internal/god/`, `internal/medal/`, `internal/core/progression/`
- **Key Testing / Linter Focus**: 転生（Lv1リセット）の完全排除、Lv99→150限界突破、礼拝堂の単一祈願制約、SPステータス成長

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/job_change.cgi` | 転職所 (@ダーマ神官) | `internal/job/` | `internal/api/http/job.go`<br/>`migrations/007_character_jobs.sql`<br/>`migrations/059_job_change_parity.sql` | `docs/design/jobs-and-skills.md`<br/>`docs/api/paths/job.json` | Reconciling (#467) | Lv30以上条件、転職時ステータス補正 |
| `lib/job_master.cgi` | 職業極め所 | `internal/job/` | `internal/api/http/job.go` | `docs/design/jobs-and-skills.md`<br/>`docs/api/paths/job.json` | Reconciling (#467) | 職業マスターパッシブボーナス |
| `lib/sp_change.cgi` | 願いの泉 (@女神) | `internal/wishingwell/` | `internal/api/http/wishingwell.go`<br/>`migrations/056_eliminate_rebirth_add_sp.sql` | `docs/design/wishing-well.md`<br/>`docs/api/paths/wishing_well.json` | Compliant (#468) | SPを消費して5大ステータス強化 |
| `lib/custom_skill.cgi` | 特技設定 (@マニャ) | `internal/custom_skill/` | `internal/api/http/custom_skill.go`<br/>`migrations/029_custom_skills.sql`<br/>`migrations/058_custom_skill_gem_synthesis.sql` | `docs/design/custom_skill.md`<br/>`docs/api/paths/custom_skill.json` | Reconciling (#469) | 呪文・特技詠唱文設定、宝玉スロット合成 |
| `lib/name_change.cgi` | 命名の館 (@アストロン) | `internal/character/` | `internal/api/http/character.go` | `docs/design/character-customization.md`<br/>`docs/api/paths/character.json` | Compliant | ゴールド手数料、重複ネーム検証 |
| `lib/custom_image.cgi`, `lib/upload_image.cgi` | 画像設定所 | `internal/character/` | `internal/api/http/character.go` | `docs/design/character-customization.md`<br/>`docs/api/paths/character.json` | Compliant | カスタムアバターURL・アイコン設定 |
| `lib/altar.cgi`, `lib/reborn.cgi` | 転生の祭壇 (@精霊ルビス) | `internal/altar/`<br/>`internal/core/progression/` | `internal/api/http/altar.go`<br/>`migrations/013_rebirth.sql`<br/>`migrations/057_altar_of_rebirth.sql` | `docs/design/altar-of-rebirth.md`<br/>`docs/design/progression.md`<br/>`docs/api/paths/altar.json` | Reconciling (#470, #471) | 🚨 Lv1転生は架空。Lv99→150限界突破（OverLevel）のみ実装 |
| `lib/chapel.cgi` | 礼拝堂 (@シスター) | `internal/chapel/` | `internal/api/http/chapel.go`<br/>`migrations/022_chapel.sql`<br/>`migrations/060_chapel_parity.sql` | `docs/design/chapel.md`<br/>`docs/api/paths/chapel.json` | Compliant (#472) | 5つの願い（単一祈願排他制約）、翌朝リセット |
| `lib/god.cgi` | 天界 (@神) | `internal/god/` | `internal/api/http/god.go`<br/>`migrations/044_god_wishes_and_limit_breaks.sql`<br/>`migrations/061_home_members_and_god_parity.sql` | `docs/design/god.md`<br/>`docs/api/paths/god.json` | Reconciling (#473) | ステータス種付与、天界の加護 |
| `lib/u_god.cgi` | 裏天界 (@裏神) | `internal/god/` | `internal/api/http/god.go`<br/>`migrations/044_god_wishes_and_limit_breaks.sql` | `docs/design/god.md`<br/>`docs/api/paths/god.json` | Reconciling (#473) | 天界牧場拡張+2（30匹→32匹） |
| `lib/medal.cgi` | メダル王 (@メダル王) | `internal/medal/` | `internal/api/http/medal.go`<br/>`migrations/030_small_medals.sql`<br/>`migrations/050_achievements_and_medals.sql` | `docs/design/medal.md`<br/>`docs/design/achievements.md`<br/>`docs/api/paths/medal.json` | Reconciling (#473) | 小さなメダル交換、景品はDepotへ直送 |
| `lib/exile.cgi` | 荒らし追放騎士団 | `internal/maintenance/` (将来) | Pending | `docs/design/system-maintenance.md` | Backlog | プレイヤー自治による追放投票機能 |

---

### CL-05: Production, Cultivation & Monsters (生産・菜園・モンスター牧場)
- **Cluster Summary**: 錬金堂（レシピ合成）、モンスター牧場（仲間預託・ペット連携）、種菜園（栽培・肥料）
- **Shared Dependencies**: `party2/lib/_data.cgi`, `party2/lib/_alchemy_recipe.cgi`, `party2/lib/depot.cgi`
- **Primary Domain Packages**: `internal/alchemy/`, `internal/monsterranch/` (旧 `internal/farm/`), `internal/plantation/`, `internal/monster/`
- **Key Testing / Linter Focus**: 牧場から畑要素を全削除、錬金の翌朝タイマー・Depot直結、菜園の14種肥料・枯れ率計算

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/alchemy.cgi`, `lib/_alchemy_recipe.cgi` | 錬金堂 (@トロデ) | `internal/alchemy/` | `internal/api/http/alchemy.go`<br/>`migrations/070_alchemy_overnight_depot.sql` | `docs/design/alchemy.md`<br/>`docs/api/openapi.json` | Complete (#487) | ✅ 復元完了: 完全無料、Depot素材消費、翌朝完成（自宅休息完了）、Depot直送、図鑑100%称号。 |
| `lib/farm.cgi` | モンスター牧場 (@モンジィ) | `internal/monsterranch/` (旧 `internal/farm/`) | `internal/api/http/farm.go`<br/>`internal/api/http/monster.go`<br/>`migrations/019_farm.sql`<br/>`migrations/045_monster_grandpa_and_pets.sql` | `docs/design/farm.md`<br/>`docs/design/monster.md`<br/>`docs/api/paths/farm.json`<br/>`docs/api/paths/monster.json` | Reconciling (#488) | 🚨 創作: 4面畑・水やり・収穫。<br/>正: モンスター預託（30/32枠）、自宅ペット連携（8枠）、改名（8文字）、P2P譲渡。 |
| `lib/plantation.cgi` | 種菜園 (@ロータス) | `internal/plantation/` | Pending HTTP Handler | `docs/design/plantation.md` (Issue #489) | Reconciling (#489) | 6種の種、14種の肥料、水やりと枯れ率計算、翌朝収穫Depot直結。 |

---

### CL-06: Combat, Adventure & Dungeons (冒険・ボス・戦闘・PVP/GVG・ダンジョン)
- **Cluster Summary**: 冒険（ダンジョン10階層）、封印の魔王、コア戦闘・スキル発動エンジン、闘技場、ギルド戦、パーティ共闘、連戦チャレンジ、手助け
- **Shared Dependencies**: `party2/lib/_battle.cgi`, `party2/lib/_skill.cgi`, `party2/lib/ActionCounter.pm`, `party2/lib/TurnEndProcessor.pm`, `party2/lib/_data.cgi`
- **Data Dependencies**: `party2/stage/0..27.cgi`, `party2/stage/king1..99.cgi`, `party2/challenge/0..8.cgi`, `party2/map/*`
- **Primary Domain Packages**: `internal/adventure/`, `internal/boss/`, `internal/core/battle/`, `internal/core/skill/`, `internal/pvp/`, `internal/gvg/`, `internal/dungeon/`, `internal/challenge/`, `internal/party/`, `internal/helper/`
- **Key Testing / Linter Focus**: 逃走ペナルティ、10階層制覇、王の証と世界樹の葉、実時間ベッティング、パーティ相乗効果（2p=+10%, 3p=+20%, 4p=+30%）

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/vs_monster.cgi`, `lib/adventure.cgi`, `lib/adventure_record.cgi` | 冒険・階層探索 (Adventure) | `internal/adventure/` | `internal/api/http/adventure.go`<br/>`migrations/006_adventures.sql`<br/>`migrations/009_adventure_battle_rewards.sql`<br/>`migrations/037_adventure_chronicle.sql` | `docs/design/adventure-chronicle.md`<br/>`docs/design/stages-and-monsters.md`<br/>`docs/api/paths/adventure.json` | Reconciling (#478) | 🚨 創作: 1時間タイマー放置遠征。<br/>正: 10階層の能動的ダンジョン探索、ボス撃破、逃走時ゴールド半減。 |
| `lib/vs_king.cgi`, `lib/_win_vs_king.cgi` | 封印の魔王 (Boss) | `internal/boss/` | `internal/api/http/combat.go`<br/>`migrations/025_boss_battles.sql` | `docs/design/boss.md`<br/>`docs/api/paths/boss.json` | Reconciling (#479) | 🚨 創作: 1日3回入場制限。<br/>正: 王の証（消費アイテム）による挑戦、世界樹の葉1回蘇生。 |
| `lib/_battle.cgi` | コア戦闘エンジン | `internal/core/battle/` | Core Engine | `docs/design/battle.md` | Reconciled (#480) | ダメージ計算式、命中率、クリティカル、状態異常計算。 |
| `lib/_skill.cgi` | スキル発動エンジン | `internal/core/skill/` | Core Engine | `docs/design/jobs-and-skills.md` | Reconciling (#469) | 特技発動確率、MP/HP消費、属性補正。 |
| `lib/vs_player.cgi` | 闘技場 (PvP Arena) | `internal/pvp/` | `internal/api/http/pvp.go`<br/>`migrations/023_pvp_arena.sql` | `docs/design/pvp.md`<br/>`docs/api/paths/pvp.json` | Reconciling (#481) | 🚨 創作: 非同期Eloランキング。<br/>正: リアルタイム観戦・賭け（ベッティング）、日次報酬。 |
| `lib/vs_guild.cgi` | ギルド戦 (GvG Arena) | `internal/gvg/` | `internal/api/http/combat.go`<br/>`migrations/024_gvg_combat.sql` | `docs/design/gvg.md` | Reconciling (#482) | 🚨 創作: 非同期Elo対戦。<br/>正: リアルタイム勝ち抜きトーナメント、砦防衛。 |
| `lib/vs_dungeon.cgi`, `party2/dungeon.cgi` | ダンジョン探索 | `internal/dungeon/` | `internal/api/http/adventure.go`<br/>`migrations/026_dungeon_exploration.sql` | `docs/design/dungeon.md`<br/>`docs/api/paths/dungeon.json` | Reconciling (#483) | パーティ共闘探索、トラップ宝箱判定。 |
| `lib/vs_challenge.cgi`, `party2/challenge.cgi` | 連戦チャレンジ | `internal/challenge/` | `internal/api/http/combat.go`<br/>`migrations/028_endurance_challenge.sql` | `docs/design/challenge.md`<br/>`docs/api/paths/challenge.json` | Reconciling (#483) | 多重ラウンド勝ち抜きサバイバル、中間報酬。 |
| `lib/quest.cgi`, `party2/party.cgi` | パーティ共闘クエスト | `internal/party/` | `internal/api/http/party.go`<br/>`migrations/048_party_and_coop_quests.sql`<br/>`migrations/052_drop_parties_and_party_members.sql` | `docs/design/party-system.md`<br/>`docs/api/paths/party.json` | Compliant | 同期マルチバトル、相乗効果補正（2p=+10%, 3p=+20%, 4p=+30%）。 |
| `lib/helper.cgi` | 手助けクエスト | `internal/helper/` | `internal/api/http/helper_rescue.go`<br/>`migrations/031_rescue_and_helper.sql` | `docs/design/rescue-and-helper.md`<br/>`docs/api/paths/helper.json` | Backlog | プレイヤー間の救難・アイテム配送依頼掲示板。 |

---

### CL-07: Social, Entertainment & Gambling (ギルド・娯楽・カジノ・ランキング・図鑑)
- **Cluster Summary**: ギルド（動的GP・役職）、写真館・コンテスト、イベント広場、宝くじ、福引、カジノ（スロット・ハイロー・インディアンポーカー・ドッペル）、図鑑、新聞、各種ランキング、リプレイ
- **Shared Dependencies**: `party2/lib/_data.cgi`, `party2/lib/system.cgi`, `party2/lib/_casino.cgi`
- **Primary Domain Packages**: `internal/guild/`, `internal/contest/`, `internal/eventplaza/`, `internal/lottery/`, `internal/casino/`, `internal/collection/`, `internal/news/`, `internal/notification/`, `internal/ranking/`, `internal/replay/`
- **Key Testing / Linter Focus**: ギルド寄付レベル上げの排除、宝くじ20枚上限・キャリーオーバー、商人3倍価格、8人共有カジノ

| Legacy Script | Authentic Role / Action | Go Domain Implementation | HTTP Handler & Migrations | Design Doc & OpenAPI | Status | Pitfalls / Parity Traps |
| :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| `lib/guild.cgi`, `party2/join_guild.cgi`, `party2/guild_list.cgi` | ギルド拠点・運営 | `internal/guild/` | `internal/api/http/handler.go`<br/>`migrations/016_guilds.sql` | `docs/design/guild.md` | Reconciling (#490) | 🚨 創作: ゴールド寄付によるギルドLv1〜10上げ。<br/>正: 活動動的GP、役職命名（6文字）、HEXカラー設定。 |
| `lib/photo.cgi`, `party2/contest.cgi`, `party2/screen_shot.cgi` | 写真館・コンテスト | `internal/contest/` | `internal/api/http/contest.go`<br/>`migrations/046_photo_contest_and_gallery.sql` | `docs/design/photo-contest.md`<br/>`docs/api/paths/contest.json` | Compliant | 10日周期コンテスト、投票、殿堂入り。 |
| `lib/event.cgi` | イベント広場 (@旅の商人) | `internal/eventplaza/` | `internal/api/http/eventplaza.go`<br/>`migrations/038_eventplaza.sql` | `docs/design/eventplaza.md`<br/>`docs/api/paths/eventplaza.json` | Reconciling (#491) | リアルタイム出現トリガー、販売価格は定価の3倍 (`* 3`)。 |
| `lib/takarakuzi.cgi` | 宝くじ (@案内人) | `internal/lottery/` | `internal/api/http/lottery.go`<br/>`migrations/018_lottery.sql` | `docs/design/lottery.md`<br/>`docs/api/paths/lottery.json` | Reconciling (#484) | 🚨 創作: 4桁数字選択制。<br/>正: 連番・バラ20枚上限、定期抽選、キャリーオーバー。 |
| `lib/lot.cgi` | 福引 (@案内人) | `internal/lottery/` | `internal/api/http/lottery.go`<br/>`migrations/018_lottery.sql` | `docs/design/lottery.md`<br/>`docs/api/paths/lottery.json` | Reconciling (#485) | 🚨 創作: ゴールド購入・天井システム。<br/>正: 福引券消費、種・オーブ景品、等級保証。 |
| `lib/casino.cgi`, `party2/casino.cgi`, `lib/_casino.cgi` | カジノラウンジ | `internal/casino/` | `internal/api/http/casino.go`<br/>`migrations/017_casino.sql` | `docs/design/casino-*.md`<br/>`docs/api/paths/casino.json` | Reconciling (#486) | 8人共有ラウンジ、コイン購入・換金、景品Depot直送。 |
| `lib/casino_slot.cgi` | スロットマシン | `internal/casino/` | `internal/api/http/casino.go` | `docs/design/casino-slot-machine.md`<br/>`docs/api/paths/casino.json` | Reconciling (#486) | 3リール、プログレッシブ・ジャックポット。 |
| `lib/casino_highlow.cgi` | ハイローゲーム | `internal/casino/` | `internal/api/http/casino.go` | `docs/design/casino-highlow.md`<br/>`docs/api/paths/casino.json` | Compliant | カード大小予想、連勝倍率。 |
| `lib/casino_indian.cgi` | インディアンポーカー | `internal/casino/` | `internal/api/http/casino.go`<br/>`migrations/054_casino_poker_sessions.sql` | `docs/design/casino-indian-poker.md`<br/>`docs/api/paths/casino.json` | Compliant | ブラフ・コール・フォールド判定。 |
| `lib/casino_doppel.cgi` | ドッペルゲンガー | `internal/casino/` | `internal/api/http/casino.go` | `docs/design/casino-doppel.md`<br/>`docs/api/paths/casino.json` | Compliant | 8絵柄マッチングゲーム。 |
| `lib/collection.cgi`, `lib/_add_collection.cgi`, `lib/_add_monster_book.cgi`, `party2/view_monster.cgi` | 図鑑コレクション | `internal/collection/` | `internal/api/http/collection.go`<br/>`migrations/021_collection.sql` | `docs/design/collection.md`<br/>`docs/api/paths/collection.json` | Compliant | モンスター図鑑・アイテム図鑑・撃破数記録。 |
| `party2/news.cgi` | 新聞ログ | `internal/news/` | `internal/api/http/notification.go`<br/>`migrations/033_news_and_notifications.sql` | `docs/design/notifications.md`<br/>`docs/api/paths/notification.json` | Compliant | 殿堂入り、イベント当選者、速報。 |
| `party2/ranking.cgi`, `week_ranking.cgi`, `job_ranking.cgi`, `legend.cgi` | サーバー各種ランキング | `internal/ranking/` | `internal/api/http/ranking.go`<br/>`migrations/035_rankings_and_leaderboards.sql` | `docs/design/ranking.md`<br/>`docs/api/paths/ranking.json` | Compliant | Valkeyスナップショットキャッシュ連携、週次集計。 |
| `party2/replay.cgi` | 戦闘リプレイ再生 | `internal/replay/` | `internal/api/http/handler.go`<br/>`migrations/027_battle_replays.sql` | `docs/design/replay.md` | Compliant | 構造化されたターンバイターンの戦闘再生。 |

---

## 3. Data & Master Files Catalog (`party2/stage/`, `challenge/`, `map/`)

| Script / Directory | Contents | Target Implementation in Go | Migration Status |
| :--- | :--- | :--- | :---: |
| `party2/stage/0.cgi` .. `27.cgi` | 28の通常冒険ステージ構成・敵出現テーブル | `internal/adventure/data/stages.json`<br/>`docs/design/stages-and-monsters.md` | Compliant |
| `party2/stage/king1.cgi` .. `king99.cgi` | 10体の封印魔王ステージ構成・耐性テーブル | `internal/boss/data/kings.json`<br/>`docs/design/boss.md` | Compliant |
| `party2/challenge/0.cgi` .. `8.cgi` | 9段階の連戦チャレンジステージ構成 | `internal/challenge/data/challenge_tiers.json`<br/>`docs/design/challenge.md` | Compliant |
| `party2/map/0` .. `17` | ダンジョンマップ階層データ・タイル行列 | `internal/dungeon/data/maps.json`<br/>`docs/design/dungeon.md` | Compliant |
