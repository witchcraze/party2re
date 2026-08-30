# Secret Underground Shop & NPC @ヒミツジ Design

## Overview

The Secret Underground Shop Module (`internal/secretshop`) reconstructs the legacy secret shop (`secret.cgi` / `item.cgi:ひみつのみせ`) where veteran adventurers can discover an underground merchant managed by the mysterious talking sheep NPC `@ヒミツジ` (Himitsuji).

The secret shop provides rare consumable goods and accessories at premium pricing (3x base market rate), humorous NPC dialogue interactions, and a restorative `@ぱふぱふ` (Puff-Puff) massage service.

---

## Domain Rules & Systems

### 1. Discovery & Access Qualification

Access to the secret underground shop is restricted to characters who satisfy discovery qualifications:

- **Level Requirement**: Level `>= 15`
- **Rebirth Exception**: Any character with `RebirthCount > 0` qualifies regardless of level.
- **Access Control**: Characters failing these criteria receive `ErrAccessDenied` (HTTP 403 Forbidden).

---

### 2. Rare Goods Catalog & Helper Quest Exclusion (`internal/secretshop/data/secret_items.json`)

The secret shop stocks 8 exclusive or rare consumables and accessories:

| Item ID | Definition ID | Name | Category | Base Price | Secret Price (3x) | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `secret_item_philosopher_stone` | `item-004` | 賢者の石 | Consumable | 1,500 G | **4,500 G** | 錬金術の極致とされる神秘の石。味方全員の傷を癒やす力がある。 |
| `secret_item_sacred_tree_dewdrop` | `item-005` | 世界樹の雫 | Consumable | 1,200 G | **3,600 G** | 霊木から滴る神聖な雫。味方全員のHPを完全回復する。 |
| `secret_item_sacred_tree_leaf` | `item-006` | 世界樹の葉 | Consumable | 1,000 G | **3,000 G** | 死者を蘇生させる奇跡の葉。力尽きた仲間を復活させる。 |
| `secret_item_elven_elixir` | `item-007` | エルフの飲み薬 | Consumable | 2,000 G | **6,000 G** | 森のエルフ秘伝の霊薬。MPを全回復する。 |
| `secret_item_prayer_ring` | `item-008` | 祈りの指輪 | Accessory | 2,500 G | **7,500 G** | 祈りを捧げることで使用者のMPを回復する不思議な指輪。 |
| `secret_item_dark_rosary` | `item-009` | 破邪のロザリオ | Accessory | 3,000 G | **9,000 G** | 邪悪を打ち払うロザリオ。致命的な呪いや即死攻撃を防ぐ。 |
| `secret_item_life_ring` | `item-010` | 命の指輪 | Accessory | 3,500 G | **10,500 G** | 生命力を活性化させ、歩くたびにHPを微量回復する指輪。 |
| `secret_item_holy_water` | `item-011` | 聖水 | Consumable | 300 G | **900 G** | 邪悪な魔物を寄せ付けなくする聖なる水。 |

#### Helper Quest Exclusion Filter
To maintain quest balance, items that are actively requested by ongoing Helper Quests (`internal/helper`) are automatically hidden from the shop listing and cannot be purchased during that period (`ErrItemUnavailableInHelperQuest`).

---

### 3. Transactional Purchasing (`PurchaseItem`)

Purchasing items is fully transactional and concurrency-safe:
- Runs inside `txProvider.RunInTx`.
- Acquires pessimistic locks on character record and inventory stack (`SELECT ... FOR UPDATE`).
- Verifies character eligibility, item availability, and sufficient funds.
- Calculates total price with overflow protection (`safeMultiply`).
- Creates new item instances (`coreitem.NewInstance`) and appends them to inventory.
- Deducts gold and persists changes atomically.

---

### 4. NPC Interactions & Restorative Services

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
   - Returns the humorous legacy message:
     - *"パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ わしのパフパフは気持ちいいだろう"*
   - Restores a minor amount of health and mana (+10 HP, +5 MP capped at Max HP/MP).

---

## API Endpoints

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/characters/{id}/secretshop` | Bearer | Get secret shop status and available items |
| `POST` | `/characters/{id}/secretshop/talk` | Bearer | Talk with NPC @ヒミツジ |
| `POST` | `/characters/{id}/secretshop/inspect` | Bearer | Inspect NPC @ヒミツジ |
| `POST` | `/characters/{id}/secretshop/puffpuff` | Bearer | Receive Puff-Puff massage service & minor healing |
| `POST` | `/characters/{id}/secretshop/purchase` | Bearer | Purchase rare items from the shop |
