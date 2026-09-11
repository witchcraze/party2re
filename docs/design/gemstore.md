# 宝石店・宝石合成・鑑定 (Gem Store & Synthesis) 設計書

## 1. 概要 (Overview)

旧 Party2 における `gem_store.cgi`（宝石店・NPC `@ジェマ`）の機能を Clean-Room 手法により再構築したサブシステムである。
キャラクターレベルに応じた宝珠・天珠の購入、インベントリ内宝石の売却、他プレイヤーへの宝石譲渡、2種類の素材（宝石または特殊消費アイテム）を消費した55種類以上の上位宝石への特殊合成（加工）、未鑑定宝珠の鑑定機能を提供する。

---

## 2. Legacy 1:1 `@actions` 照合表

| Legacy Action | Legacy サブルーチン | Modern Go ドメインメソッド | Modern HTTP エンドポイント | 動作と不変条件 |
|---|---|---|---|---|
| `みる (宝珠箱)` | `gem_box.cgi` | `GemStoreService.GetGemBox` | `GET /characters/{id}/gembox` | 宝珠箱内容取得（動的容量、所持宝珠リスト） |
| `せいとん` | `gem_box.cgi` | `GemStoreService.SortGemBox` | `POST /characters/{id}/gembox/sort` | カタログ昇順（ID・マスター順）で宝珠箱を整頓・永続化 |
| `かう` | `&kau` | `GemStoreService.BuyGem` | `POST /characters/{id}/gemstore/buy` | レベル要件検証 $\rightarrow$ 価格計算（定価 $\times$ 5） $\rightarrow$ 所持金減算 $\rightarrow$ 宝珠箱 (`GemBox`) 追加（満杯時拒否） |
| `うる` | `&uru` | `GemStoreService.SellGem` | `POST /characters/{id}/gemstore/sell` | 宝珠箱所有確認 $\rightarrow$ 売却額計算（50%価格、最低1G） $\rightarrow$ 宝珠箱から消費 $\rightarrow$ 所持金加算 |
| `おくる` | `&okuru` | `GemStoreService.SendGem` | `POST /characters/{id}/gemstore/send` | 自身送信防止 $\rightarrow$ 送受信者ID昇順ロック $\rightarrow$ 送信者宝珠箱から消費 $\rightarrow$ 受信者宝珠箱付与（満杯時拒否） |
| `かこう` | `&kako` | `GemStoreService.SynthesizeGem` | `POST /characters/{id}/gemstore/synthesize` | レシピ素材2種（宝珠箱/インベントリ/倉庫）所有確認 $\rightarrow$ 消費 $\rightarrow$ 宝珠箱へ上位宝石付与（満杯時拒否） |
| `かんてい` | `&kantei` | `GemStoreService.AppraiseItem` | `POST /characters/{id}/gemstore/appraise` | 未鑑定宝珠（インベントリ）を消費 $\rightarrow$ 宝珠箱へ上位宝石付与（満杯時拒否） |
| `みる` / `はなす` | `words` / メニュー | `GemStoreService.GetCatalog`<br>`GemStoreService.GetRecipes`<br>`GemStoreService.GetDialogue` | `GET /gemstore/catalog`<br>`GET /gemstore/recipes`<br>`GET /gemstore/dialogue` | 転職・レベル別購入可能宝石、全合成レシピ、店主 `@ジェマ` 会話メッセージの取得 |

---

## 3. ドメインモデル & カタログ (Domain Model & Catalog)

### 3.1 宝石 (`Gem`)
- `ID`: 宝石識別子 (e.g. `gem_atk_1`, `gem_sky_atk_1`, `gem_awakening_sky`)
- `Name`: 表示名 (e.g. `攻撃の宝珠Ⅰ`, `攻撃の天珠Ⅰ`, `覚醒の天珠`)
- `Price`: 定価 (G)
- `RequiredLevel`: 購入に必要なキャラクターレベル / 転職回数 (1, 10, 30, 50, 100)
- `SlotCost`: スキルスロット消費数 (宝珠: 1, 天珠: 2)
- `MPCost`: 戦闘時消費CMP (CMP = 集中魔力)
- `Description`: 効果説明文

### 3.2 専用宝珠箱 (`GemBox`) と動的容量計算
旧 Party2 の `gem_box.cgi` に完全準拠した専用ストレージ。通常インベントリとは独立して管理され、キャラクターの転職回数 (`job_lv`) に応じて容量が動的にスケールする。

- **初期容量 (未転職 / `job_lv <= 0`)**: 5 スロット
- **転職進行時 (`1 <= job_lv < 20`)**: `job_lv * 5 + 5` スロット
- **最大容量 (`job_lv >= 20`)**: 100 スロット

### 3.3 合成レシピ (`Recipe`)
- `ID`: レシピ識別子 (e.g. `recipe_atk_2`, `recipe_combo_sky`, `recipe_awakening_sky`)
- `ResultName`: 完成品宝石名
- `Material1`: 必要素材1 (宝石または特殊アイテム)
- `Material2`: 必要素材2 (宝石または特殊アイテム)

### 3.4 未鑑定宝珠の鑑定プール (Unidentified Orb Appraisal Pools)
未鑑定宝珠（`_data.cgi` No. 251〜255）を鑑定した際、レガシーの `@nums` 定義に完全準拠した重み付き確率で宝石が選定される。

1. **光る宝珠 (`光る宝珠`, No. 251)**: 全34エントリ
   - 重み1: 攻撃の天珠Ⅰ, 全攻撃の天珠, 魔撃の天珠, 全魔撃の天珠
   - 重み2: 攻撃の宝珠Ⅱ, 連撃の宝珠, 全攻撃の宝珠Ⅰ, 全攻撃の宝珠Ⅱ, 防御装の宝珠Ⅰ, 魔撃の宝珠Ⅰ, 魔撃の宝珠Ⅱ, 全魔撃の宝珠, 全息撃の宝珠, 攻倍化の宝珠Ⅰ, 攻倍化の宝珠Ⅱ, 守倍化の宝珠Ⅰ, 守倍化の宝珠Ⅱ, 早倍化の宝珠Ⅰ, 早倍化の宝珠Ⅱ
2. **ひび割れた宝珠 (`ひび割れた宝珠`, No. 252)**: 全45エントリ
   - 重み1: 回復の天珠Ⅰ, 全回復の天珠, 覚醒の宝珠
   - 重み2: 回復の宝珠, 攻軽装の宝珠, 魔軽装の宝珠, 息軽装の宝珠, 攻無装の宝珠, 魔無装の宝珠Ⅰ, 魔無装の宝珠Ⅱ, 攻反装の宝珠, 魔反装の宝珠Ⅰ, 魔反装の宝珠Ⅱ, 息反装の宝珠, 回復装の宝珠, 心眼の宝珠, 猛毒解の宝珠, 麻痺解の宝珠, 睡眠解の宝珠, 混乱解の宝珠, 動封解の宝珠, 攻封解の宝珠, 魔封解の宝珠, 異常解の宝珠
3. **多彩色の宝珠 (`多彩色の宝珠`, No. 253)**: 全17エントリ
   - 重み1: 森閑の天珠
   - 重み2: 攻撃の宝珠Ⅱ, 攻撃の天珠Ⅰ, 攻撃の天珠Ⅱ, 連撃の天珠, 全攻撃の天珠, 魔撃の天珠, 全魔撃の天珠, 命吸収の天珠
4. **黒ずんだ宝珠 (`黒ずんだ宝珠`, No. 254)**: 全15エントリ
   - 重み1: 覚醒の天珠
   - 重み2: 魔吸収の天珠, 防御装の天珠, 攻反装の天珠, 蘇生の天珠Ⅰ, 爆撃の天珠, 召喚の天珠, 覚醒の宝珠
5. **妖しい宝珠 (`妖しい宝珠`, No. 255)**: 全16エントリ
   - 重み1: 烈撃の天珠, 連烈の天珠, 全回復の天珠, 魔回復の天珠Ⅱ, 蘇生の天珠Ⅱ, 蘇生の天珠Ⅲ, 神気の天珠, 森閑の天珠
   - 重み2: 攻撃の宝珠Ⅱ, 攻撃の天珠Ⅰ, 攻撃の天珠Ⅱ, 覚醒の天珠

---

## 4. トランザクション境界と排他ロック順序 (Transaction & Locking)

システム全体のデッドロックを防止するため、以下の確定的なロック順序（Locking Hierarchy）を厳格に順守する。

```text
[Tier 2] characters (昇順: min(id1, id2) -> max(id1, id2))
    |
[Tier 3] inventory_items (昇順: min(character_id) -> max(character_id))
    |
[Tier 5] character_depots (昇順: min(character_id) -> max(character_id))
    |
[Tier 8] character_gem_boxes (昇順: min(character_id) -> max(character_id))
```

すべての状態更新は `txProvider.RunInTx(ctx, ...)` によるアンビエントトランザクション伝播下でアトミックに実行される。

---

## 5. セキュリティと認可 (Security & Authorization)

- **セッション認証**: すべてのキャラクター操作エンドポイントは Bearer トークン認証必須。
- **所有権認可**: 操作対象キャラクターの `player_id` がセッションプレイヤーと一致するか検証（403 Forbidden）。
- **IDOR 防止**: 送信・購入・売却・合成・鑑定すべてにおいて、操作者のインベントリと所持金を直接検証。
