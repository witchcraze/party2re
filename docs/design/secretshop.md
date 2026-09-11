# Secret Underground Shop & NPC @ヒミツジ Design

## Overview

The Secret Underground Shop Module (`internal/secretshop`) reconstructs the legacy secret shop (`secret.cgi` / `item.cgi:himitsunomise`) where veteran adventurers can discover an underground merchant managed by the mysterious talking sheep NPC `@ヒミツジ` (Himitsuji).

The secret shop provides 8 rare consumable items at premium pricing (3x base market rate), humorous sheep NPC dialogue interactions, and a flavor-only `@ぱふぱふ` (Puff-Puff) massage interaction.

---

## Domain Rules & Systems

### 1. Discovery & Access Qualification

Access to the secret underground shop is restricted to characters who satisfy discovery qualifications according to legacy CGI (`item.cgi:himitsunomise`):

- **Job Level Requirement**: `JobLevel >= 7` (転職回数7回以上)
- **Access Control**: Characters failing this criterion receive `ErrAccessDenied` (HTTP 403 Forbidden).

---

### 2. Genuine Rare Goods Catalog & Helper Quest Exclusion (`internal/secretshop/data/secret_items.json`)

The secret shop stocks the exact 8 items specified in legacy `secret.cgi:sales`:

| Item ID | Definition ID | Name | Clean-Room / Legacy Name | Category | Base Price | Secret Price (3x) | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `secret_item_herbal_root` | `item-010` | 薬草の根っこ | 薬草の根っこ (旧: パデキアの根っこ) | Consumable | 250 G | **750 G** | 大地の生命力を宿した薬草の根。戦闘中に仲間の傷や状態異常を治療する。 |
| `secret_item_magic_mirror` | `item-015` | 魔法の鏡 | 魔法の鏡 | Consumable | 300 G | **900 G** | 魔法の壁を展開し、敵の呪文を跳ね返す神秘の鏡。 |
| `secret_item_ruby_of_protection` | `item-080` | 守りのルビー | 守りのルビー | Consumable | 500 G | **1,500 G** | 魔法の光で仲間を包み込み、受ける魔法ダメージを軽減する宝石。 |
| `secret_item_silver_harp` | `item-078` | 銀のたてごと | 銀のたてごと | Consumable | 1,400 G | **4,200 G** | 美しい音色を奏でて魔物を呼び寄せる銀製の竪琴。 |
| `secret_item_staff_of_change` | `item-043` | へんげの杖 | へんげの杖 | Consumable | 1,000 G | **3,000 G** | 使用者の姿をモンスターの姿に変貌させる不思議な杖。 |
| `secret_item_philosophers_enlightenment` | `item-027` | 賢者の悟り | 賢者の悟り | Consumable | 10,000 G | **30,000 G** | 深遠なる知識と悟りを開いた証。上位職への転職条件を満たす秘宝。 |
| `secret_item_spirits_ward` | `item-030` | 精霊の守り | 精霊の守り | Consumable | 5,000 G | **15,000 G** | 精霊たちの強大な加護が宿るお守り。上位職への転職条件を満たす秘宝。 |
| `secret_item_counts_blood` | `item-031` | 伯爵の血 | 伯爵の血 | Consumable | 5,000 G | **15,000 G** | 高貴なる闇の血脈を宿す秘薬。上位職への転職条件を満たす秘宝。 |

#### Helper Quest Exclusion Filter
To maintain quest balance, items that are actively requested by ongoing Helper Quests (`internal/helper`) are automatically hidden from the shop listing and cannot be purchased during that period (`ErrItemUnavailableInHelperQuest`).

---

### 3. Transactional Purchasing & Depot Routing (`PurchaseItem`)

Purchasing items is fully transactional and concurrency-safe (`secret.cgi:kau`):
- Runs inside `txProvider.RunInTx`.
- Acquires pessimistic locks on character record and inventory stack (`SELECT ... FOR UPDATE`).
- Verifies character eligibility (`JobLevel >= 7`), item availability, and sufficient funds.
- Calculates total price with overflow protection (`safeMultiply`).
- **Inventory Hand Slot vs Depot Routing**:
  - If inventory consumable slot is empty and purchase quantity is 1:
    - Item is added directly to character inventory.
    - Dialogue: `"{ItemName}メェ〜。持ってけメェ〜"`
  - If inventory consumable slot is occupied or purchase quantity is greater than 1:
    - Item is transferred directly to character Depot (`internal/depot`).
    - If depot is full, returns `ErrDepotFull` (HTTP 400 Bad Request).
    - Dialogue: `"{ItemName}は{CharacterName}メェ〜の預かり所の方に投げましたメェ〜"`
- Deducts gold atomically via `economy.Exchange`.

---

### 4. NPC Interactions & Puff-Puff Flavor Service

The secret shop sheep NPC `@ヒミツジ` provides several unique interactions:

1. **Talk (`POST /characters/{id}/secretshop/talk`)**:
   - Returns randomized sheep dialogues:
     - *"バレちゃったメェ〜。他の人には秘密だメェ〜。"*
     - *"値段は高いメェ〜けれど、他では手に入らないレアものだメェ〜。"*
     - *"メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜。"*
     - *"ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜。"*
     - *"＠ぱふぱふはサービスだメェ〜。"*

2. **Inspect (`POST /characters/{id}/secretshop/inspect`)**:
   - Returns NPC lore background:
     - *"@ヒミツジ「オイラは羊の@ヒミツジだメェ〜。羊の国から来たよ…ゴホッゴホッ…羊の国から来たメェ〜」"*

3. **Puff-Puff Service (`POST /characters/{id}/secretshop/puffpuff`)**:
   - Under legacy parity (`secret.cgi:pafupafu`), puff-puff is a purely humorous interaction with NO state or stat modifications:
     - *"パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ {CharacterName} わしのパフパフは気持ちいいだろう"*
   - No HP or MP recovery.

---

## API Endpoints

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/characters/{id}/secretshop` | Bearer | Get secret shop status and available items |
| `POST` | `/characters/{id}/secretshop/talk` | Bearer | Talk with NPC @ヒミツジ |
| `POST` | `/characters/{id}/secretshop/inspect` | Bearer | Inspect NPC @ヒミツジ |
| `POST` | `/characters/{id}/secretshop/puffpuff` | Bearer | Receive Puff-Puff flavor message (no healing) |
| `POST` | `/characters/{id}/secretshop/purchase` | Bearer | Purchase rare items from the shop |
