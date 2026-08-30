# Adventurer's Tavern & Barkeep @エレナ Design

## Overview

The Tavern Module (`internal/tavern`) clean-room reconstructs the legacy tavern/bar system (`bar.cgi` / `ルイーダの酒場` -> clean-room `冒険者の酒場`), managed by the friendly tavern barkeep NPC `@エレナ` (Elena).

The tavern serves as a community hub for adventurers where they can order food and drinks to restore HP and MP, earn bonus raffle tickets for the town lottery, schedule pre-ordered delivery meals for post-adventure recovery, and chat with Elena for lore and gameplay hints.

---

## Domain Rules & Systems

### 1. Menu Catalog (`internal/tavern/data/menu.json`)

The tavern serves 14 distinct culinary offerings organized across 4 categories:

| ID | Item Name | Category | Price | HP Heal | MP Heal | Raffle Tickets | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `tavern_water` | 癒しの湧き水 | Drink | 20 G | 40 | 0 | 1 | 清らかな名水。喉を潤しわずかにHPを回復する。 |
| `tavern_fruit_juice` | もぎたて果実ジュース | Drink | 50 G | 0 | 30 | 1 | 新鮮な果実を搾ったジュース。すっきりとした甘さでMPを回復する。 |
| `tavern_tomato_juice` | 完熟トマトジュース | Drink | 100 G | 10 | 80 | 2 | 栄養満点の濃厚トマトジュース。HPとMPを回復する。 |
| `tavern_coffee` | 深煎り焙煎コーヒー | Drink | 200 G | 0 | 250 | 3 | 香ばしい挽きたてコーヒー。冴えわたる苦味でMPを大きく回復する。 |
| `tavern_herb_tea` | 薬草ハーブティー | Drink | 500 G | 0 | 500 | 5 | 厳選された薬草で淹れたお茶。精神を安らげMPを大幅に回復する。 |
| `tavern_ice_cream` | 濃厚バニラアイス | Dessert | 100 G | 30 | 30 | 2 | ひんやり甘いバニラアイス。HPとMPを適度に回復する。 |
| `tavern_pudding` | カスタードプリン | Dessert | 200 G | 60 | 60 | 3 | 昔ながらのしっかり固めプリン。カラメルの風味が元気をくれる。 |
| `tavern_parfait` | 特製フルーツパフェ | Dessert | 300 G | 100 | 100 | 4 | 季節の果物を贅沢に盛り付けたパフェ。冒険者の疲れを癒やす。 |
| `tavern_curry` | スパイス香る特製カレー | Food | 400 G | 250 | 0 | 5 | 秘伝のスパイスを煮込んだカレー。スタミナが湧いてHPを回復する。 |
| `tavern_pasta` | 挽肉とトマトのパスタ | Food | 600 G | 300 | 50 | 6 | モチモチ麺と濃厚トマトソースのパスタ。HPとMPを回復する。 |
| `tavern_omelet_rice` | ふわとろオムライス | Food | 750 G | 500 | 100 | 7 | 黄金色の卵で包んだオムライス。HPを大きく回復する。 |
| `tavern_salisbury_steak` | 極上ジューシーハンバーグ | Food | 900 G | 800 | 0 | 8 | 溢れる肉汁の絶品ハンバーグ。HPを大量に回復する。 |
| `tavern_rib_steak` | 厳選リブアイステーキ | Food | 1,000 G | 999 | 0 | 10 | 厚切りリブアイを炭火で焼き上げた最高峰ステーキ。HPを完全回復する。 |
| `tavern_full_course` | 酒場名物グランフルコース | Course | 3,000 G | 999 | 999 | 15 | 酒場の粋を集めた豪華フルコース料理。HPとMPを完全回復し大量の福引券を進呈。 |

---

### 2. Immediate Consumption (`OrderMeal`)

When ordering food or drinks directly inside the tavern:
- **Fullness Check**: If `status.IsFull == true`, the character cannot eat (`ErrAlreadyFull` / HTTP 409 Conflict).
- **Gold Validation**: Checks `char.Money >= item.Price` with pessimistic locking.
- **Restoration**: HP and MP are restored according to item values up to `MaxHP` and `MaxMP`.
- **Raffle Ticket Reward**: Awarded raffle tickets are atomically added to `character_lottery.raffle_tickets`.
- **Fullness Update**: Marks `is_full = true`, records `last_eaten_at`, increments `total_meals_eaten`, and accumulates `total_gold_spent`.

---

### 3. Delivery Reservation System (`ReserveDelivery` / `ClaimDelivery`)

To allow characters heading out to adventure to secure an immediate meal upon return:
- **Reservation (`POST /characters/{id}/tavern/delivery`)**:
  - Validates character funds and menu item selection.
  - Saves reservation details into `tavern_deliveries`.
  - Can be inspected with `GET` or cancelled with `DELETE`.
- **Claim & Consume (`POST /characters/{id}/tavern/delivery/claim`)**:
  - Re-validates gold funds at claim time inside a pessimistic transaction.
  - Deducts gold, restores HP/MP, awards raffle tickets, sets character fullness state to true, and removes the pending delivery entry.

---

### 4. Barkeep NPC Dialogue (`Talk`)

Conversing with `@エレナ` provides randomized lore, dining recommendations, and advice:
- "いらっしゃい！冒険者の酒場へようこそ。美味しいご飯と飲み物を用意してるわよ♪"
- "食材にはMPを回復させる魔法の聖水や、HPを回復させる新鮮な薬草が含まれているのよ。"
- "HPを回復させたいならボリューム満点のご飯やデザートを食べていくといいわ。"
- "MPを回復させたいなら香り高いコーヒーやハーブティーを飲んでいくといいわね。"
- "お腹がいっぱいのときは無理に食べちゃダメよ？冒険で身体を動かしてからまた来てね！"
- "冒険終わりに温かい食事を食べたいなら、事前に「でりばりー」を予約しておくと便利よ♪"
- "お酒は大人になってからね！未成年にはもぎたて果実ジュースがおすすめよ。"

---

## REST Endpoints Summary

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/tavern/menu` | List all available drinks, desserts, foods, and courses. |
| `GET` | `/characters/{id}/tavern` | Get current character stats, fullness status, and delivery. |
| `POST` | `/characters/{id}/tavern/order` | Order and consume a meal immediately. |
| `POST` | `/characters/{id}/tavern/delivery` | Reserve a meal for delivery. |
| `GET` | `/characters/{id}/tavern/delivery` | Inspect active delivery reservation. |
| `DELETE` | `/characters/{id}/tavern/delivery` | Cancel active delivery reservation. |
| `POST` | `/characters/{id}/tavern/delivery/claim` | Claim and eat the reserved delivery meal. |
| `POST` | `/characters/{id}/tavern/talk` | Chat with tavern barkeep Elena for lore and hints. |
