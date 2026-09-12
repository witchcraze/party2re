# Tavern Post-Adventure Food Delivery Design Specification

## Overview

The Food Delivery ("でりばりー") subsystem is part of the Adventurer's Tavern module (`internal/tavern`, `docs/design/tavern.md`).
In original Party2 (`party2/lib/bar.cgi`, `party2/lib/_battle.cgi`), delivery is NOT an NPC courier quest or parcel delivery system. It is a standing meal reservation made at the Tavern: an adventurer pre-orders food or drinks before heading out to adventure. When an adventure concludes, the tavern automatically delivers the meal, restoring HP and MP, deducting the meal cost, and awarding bonus raffle tickets for the town lottery.

> [!NOTE]
> Fictional NPC delivery quests and player-to-player parcel mail previously implemented in `internal/delivery` have been completely purged (#475) in accordance with the clean-room migration policy.

---

## 1. Domain Architecture & Invariants

```mermaid
sequenceDiagram
    autonumber
    actor Player
    participant Tavern as Tavern Service
    participant Adv as Adventure Service
    participant DB as MariaDB / Tx

    Player->>Tavern: ReserveDelivery(character_id, item_id)
    Tavern->>DB: Save DeliveryReservation (0G upfront)
    Note over Player,Adv: Player departs on Adventure
    Player->>Adv: Start & Claim Adventure
    Adv->>DB: Resolve battle & commit rewards
    Adv->>Tavern: PostAdventureHook -> ClaimDelivery(character_id)
    alt Character has sufficient Gold
        Tavern->>DB: Deduct meal Price, Restore HP/MP, Award Tickets, Delete Reservation
        Tavern-->>Adv: Meal delivered & consumed
    else Insufficient Gold
        Tavern-->>Adv: Skip delivery (no charge, no heal)
    end
```

### Invariants:
1. **Zero Upfront Reservation Cost**: Reserving a delivery meal costs 0 G upfront (`party2/lib/bar.cgi:140`). Payment occurs upon successful delivery at adventure completion.
2. **Single Active Reservation**: A character may hold at most one active delivery reservation at a time (`tavern_deliveries` keyed by `character_id`). Reserving another meal overwrites the active reservation.
3. **Cancellation Without Fee**: Players can cancel a pending delivery reservation at any time with zero penalty or fee (`party2/lib/bar.cgi:149`).
4. **Automated Post-Adventure Trigger**: When an adventure completes (`adventure.Claim`), the registered `PostAdventureHook` automatically invokes `ClaimDelivery`:
   - If the player has sufficient funds (`Money >= Price`), the meal cost is deducted, HP and MP are restored (clamped to `MaxHP` / `MaxMP`), raffle tickets are awarded, and the delivery reservation is consumed.
   - If funds are insufficient, delivery is skipped without deducting gold or applying restorative effects, and the error is treated as non-fatal so adventure rewards are not impeded.
5. **Direct Manual Claim**: Characters may also manually claim a pending delivery meal via `POST /characters/{id}/tavern/delivery/claim`.

---

## 2. API Endpoints

All food delivery endpoints are consolidated under the Tavern domain (`/characters/{id}/tavern/delivery`):

| Method | Path | Summary | Authentication |
|---|---|---|---|
| `POST` | `/characters/{id}/tavern/delivery` | Reserve a tavern meal for post-adventure delivery | Bearer Token / Session |
| `GET` | `/characters/{id}/tavern/delivery` | Inspect currently active delivery reservation | Bearer Token / Session |
| `DELETE` | `/characters/{id}/tavern/delivery` | Cancel active delivery reservation | Bearer Token / Session |
| `POST` | `/characters/{id}/tavern/delivery/claim` | Manually claim and consume reserved delivery meal | Bearer Token / Session |

---

## 3. Database Schema

The subsystem uses `tavern_deliveries` (`migrations/039_tavern.sql`):

```sql
CREATE TABLE IF NOT EXISTS tavern_deliveries (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    item_id VARCHAR(64) NOT NULL,
    item_name VARCHAR(128) NOT NULL,
    price INT NOT NULL,
    hp_heal INT NOT NULL,
    mp_heal INT NOT NULL,
    tickets INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tavern_deliveries_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);
```

Legacy fictional tables `delivery_quests`, `character_deliveries`, and `delivery_parcels` have been purged via `migrations/069_drop_fictional_delivery_tables.sql`.
