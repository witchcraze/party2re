# 宝石店・宝石合成・鑑定 (Gem Store & Synthesis) 設計書

## 1. 概要 (Overview)

旧 Party2 における `gem_store.cgi`（宝石店・NPC `@ジェマ`）の機能を Clean-Room 手法により再構築したサブシステムである。
キャラクターレベルに応じた宝珠・天珠の購入、インベントリ内宝石の売却、他プレイヤーへの宝石譲渡、2種類の素材（宝石または特殊消費アイテム）を消費した55種類以上の上位宝石への特殊合成（加工）、未鑑定宝珠の鑑定機能を提供する。

---

## 2. Legacy 1:1 `@actions` 照合表

| Legacy Action | Legacy サブルーチン | Modern Go ドメインメソッド | Gateway | 動作と不変条件 |
|---|---|---|---|---|
| 入店時の宝珠箱表示 | `gem_store.cgi:88,111–126` | `GemStoreService.GetGemBox` | `gemstore` scene の `gem_box` | 宝珠箱内容（転職回数から導く容量、所持宝珠リスト） |
| `せいとん` | `gem_store.cgi:266–281` / `seiton` | `GemStoreService.SortGemBox` | `gemstore_sort` | 現行Goはカタログ順で宝珠箱を整頓・永続化。旧実装は保存kind・番号の数値順 |
| `かう` | `gem_store.cgi:129–171` / `kau` | `GemStoreService.BuyGem` | `gemstore_buy`、候補は scene の `catalog` | レベル要件検証 $\rightarrow$ 価格計算（定価 $\times$ 5） $\rightarrow$ 所持金減算 $\rightarrow$ 宝珠箱 (`GemBox`) 追加（満杯時拒否） |
| `うる` | `gem_store.cgi:177–214` / `uru` | `GemStoreService.SellGem` | `gemstore_sell`、候補は scene の `gem_box` | 宝珠箱から1個を消費し定価の半額を加算。現行Goの最低1Gは旧実装にない差分 |
| `おくる` | `gem_store.cgi:219–261` / `okuru` | `GemStoreService.SendGem` | `gemstore_send` | 自身送信防止 $\rightarrow$ 送受信者ID昇順ロック $\rightarrow$ 送信者宝珠箱から消費 $\rightarrow$ 受信者宝珠箱付与（満杯時拒否）。旧実装の受信者は名前指定 |
| `かこう` | `gem_store.cgi:286–332` / `kako` | `GemStoreService.SynthesizeGem` | `gemstore_synthesize`、候補は scene の `recipes` | レシピ素材2種（宝珠箱/インベントリ/倉庫）所有確認 $\rightarrow$ 消費 $\rightarrow$ 宝珠箱へ上位宝石付与（満杯時拒否）。倉庫読取エラー（SQL/キャンセル）は即時中断、未作成倉庫（`depot.ErrNotFound`）のみ許容 |
| `かんてい` | `gem_store.cgi:337–398` / `kantei` | `GemStoreService.AppraiseItem` | `gemstore_appraise`、候補は depot scene | 未鑑定宝珠（インベントリまたは預かり所）を消費（スタック時 `ConsumeOne`） $\rightarrow$ 宝珠箱へ上位宝石付与（満杯時拒否）。倉庫読取エラー（SQL/キャンセル）は即時中断、未作成倉庫（`depot.ErrNotFound`）のみ許容 |
| `はなす` | `gem_store.cgi:89–95` / `@words` | `GemStoreService.GetDialogue` | scene の `dialogue` | 店主 `@ジェマ` の会話文。旧実装は1行を無作為に選び、5行目に販売中の宝石名を埋め込む。Goは固定5行を返す |

### Gatewayと参照・未移行操作

6操作はすべて共通の[コマンド境界](../architecture/client-agent-api.md#input-and-execution-boundary)で実行し、`gemstore` scene が参照を担う。宝石店のREST（購入・売却・配送・合成・鑑定・整頓POST、宝珠箱GET、匿名のカタログ・レシピ・会話GET）は登録しない。入力・結果の正本は[OpenAPI](../api/base.json)と[HTTPアダプター](../../internal/api/http/action_gemstore.go)。操作者は認証済みパスの所有キャラクターに限定し、対象は明示する。売却・配送は宝珠箱のインスタンスIDまたは定義ID（定義IDならサービスが最初の一致を1個選ぶ）、購入は宝石ID、合成はレシピID、配送先は受信者のキャラクターID。所有確認、価格、容量、整頓順とトランザクションは既存サービスが担う。

scene は所有者の宝珠箱、操作者の転職回数で購入できる宝石、全レシピ、会話文を返す。いずれも固定カタログか容量上限のある一覧なのでページングしない。`catalog` の `price` は定価で、購入時はその5倍を支払う。観測は購入・配送・合成・鑑定を行わない。旧カタログGETは任意の転職回数を指定できたが、scene は旧店頭と同じく操作者の段階だけを表示する。

旧dispatchは `party2/lib/gem_store.cgi:100–106`。対象未指定時の候補表示は scene と明示入力で代替する。旧名指定はGoのID指定に置き換える。最低売却価格、数値順とカタログ順の一致、会話の無作為選択、旧NPC `しらべる`・cooldown・presence・logの再現は [#947](https://github.com/witchcraze/party2re/issues/947) の残存照合事項とし、transport移行を完全なゲーム仕様互換の根拠にしない。購入条件は #1135、レシピ解決は #1134、満杯時配送・受信上限は #1139 が扱う。

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

### 4.1 倉庫 (Depot) 読取エラーハンドリング規約

合成 (`SynthesizeGem`) および鑑定 (`AppraiseItem`) において、構成済み `DepotRepository` に対する排他読取 (`FindByCharacterIDForUpdate`) は以下の規約に従う。

- **未作成倉庫 (`depot.ErrNotFound`)**: 正常系として許容し、倉庫アイテム 0 件の空倉庫として処理を継続する。インベントリまたは宝珠箱のみで消費・鑑定が完結した場合であっても、未作成の倉庫レコードは新規作成・保存されない。
- **一時障害・SQL エラー・コンテキストキャンセル**: 倉庫データが正しく読取不能な障害とみなし、直ちにエラーを伝播してトランザクションを中断・ロールバックする。インベントリ素材や所持品のみで要件を満たす場合であっても、倉庫読取障害を握りつぶして部分観測で状態更新を実行してはならない。これにより、素材欠落とみなす誤判定や障害発生時の部分更新を防止する。

---

## 5. セキュリティと認可 (Security & Authorization)

- **セッション認証**: すべてのキャラクター操作エンドポイントは Bearer トークン認証必須。
- **所有権認可**: 操作対象キャラクターの `player_id` がセッションプレイヤーと一致するか検証（403 Forbidden）。
- **IDOR 防止**: 送信・購入・売却・合成・鑑定すべてにおいて、操作者のインベントリと所持金を直接検証。
