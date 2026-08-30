package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/delivery"
	"github.com/witchcraze/party2re/internal/id"
)

func TestDeliveryRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char1, err := database.CreateTestCharacter(ctx, db, "CourierSender")
	if err != nil {
		t.Fatal(err)
	}

	char2, err := database.CreateTestCharacter(ctx, db, "CourierRecipient")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Save and Get Quests
	questID := id.New()
	q := &delivery.Quest{
		ID:               questID,
		ClientName:       "薬草師のミレイユ",
		ClientMessage:    "薬草を届けてください",
		TargetItemID:     "item-001",
		TargetItemName:   "薬草",
		RequiredQuantity: 3,
		RecipientName:    "見習い調合師",
		Destination:      "薬草研究所",
		RewardGold:       180,
		RewardExp:        90,
		RewardItemID:     "item-007",
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
	}

	if err := repo.SaveQuest(ctx, q); err != nil {
		t.Fatalf("SaveQuest failed: %v", err)
	}

	fetchedQuest, err := repo.GetQuestByID(ctx, questID)
	if err != nil {
		t.Fatalf("GetQuestByID failed: %v", err)
	}
	if fetchedQuest.TargetItemID != "item-001" || fetchedQuest.RewardGold != 180 {
		t.Errorf("unexpected quest data: %+v", fetchedQuest)
	}

	available, err := repo.GetAvailableQuests(ctx, now)
	if err != nil {
		t.Fatalf("GetAvailableQuests failed: %v", err)
	}
	if len(available) == 0 {
		t.Errorf("expected available quests, got 0")
	}

	// 2. Character Deliveries
	delID := id.New()
	del := &delivery.CharacterDelivery{
		ID:          delID,
		CharacterID: char1.ID,
		QuestID:     questID,
		Status:      delivery.StatusInProgress,
		AcceptedAt:  now,
	}

	if err := repo.SaveCharacterDelivery(ctx, del); err != nil {
		t.Fatalf("SaveCharacterDelivery failed: %v", err)
	}

	activeDels, err := repo.GetActiveCharacterDeliveries(ctx, char1.ID)
	if err != nil {
		t.Fatalf("GetActiveCharacterDeliveries failed: %v", err)
	}
	if len(activeDels) != 1 || activeDels[0].ID != delID {
		t.Fatalf("expected 1 active delivery with id %s, got %+v", delID, activeDels)
	}

	// Update delivery to completed
	completedAt := now.Add(time.Minute)
	del.Status = delivery.StatusCompleted
	del.CompletedAt = &completedAt
	if err := repo.UpdateCharacterDelivery(ctx, del); err != nil {
		t.Fatalf("UpdateCharacterDelivery failed: %v", err)
	}

	allDels, err := repo.GetCharacterDeliveries(ctx, char1.ID)
	if err != nil {
		t.Fatalf("GetCharacterDeliveries failed: %v", err)
	}
	if len(allDels) != 1 || allDels[0].Status != delivery.StatusCompleted {
		t.Errorf("expected 1 completed delivery, got %+v", allDels)
	}

	// 3. Parcels
	parcelID := id.New()
	parcel := &delivery.Parcel{
		ID:                   parcelID,
		SenderCharacterID:    char1.ID,
		SenderCharacterName:  char1.Name,
		RecipientCharacterID: char2.ID,
		ItemID:               "item-001",
		ItemName:             "薬草",
		ItemQuantity:         2,
		GoldAmount:           150,
		Message:              "荷物です！",
		CourierFee:           50,
		Status:               delivery.ParcelStatusPending,
		CreatedAt:            now,
	}

	if err := repo.SaveParcel(ctx, parcel); err != nil {
		t.Fatalf("SaveParcel failed: %v", err)
	}

	fetchedParcel, err := repo.GetParcelByID(ctx, parcelID)
	if err != nil {
		t.Fatalf("GetParcelByID failed: %v", err)
	}
	if fetchedParcel.GoldAmount != 150 || fetchedParcel.Status != delivery.ParcelStatusPending {
		t.Errorf("unexpected parcel: %+v", fetchedParcel)
	}

	incoming, err := repo.GetIncomingParcels(ctx, char2.ID)
	if err != nil || len(incoming) != 1 {
		t.Fatalf("expected 1 incoming parcel for char2, got %v, err: %v", incoming, err)
	}

	sent, err := repo.GetSentParcels(ctx, char1.ID)
	if err != nil || len(sent) != 1 {
		t.Fatalf("expected 1 sent parcel for char1, got %v, err: %v", sent, err)
	}

	// Update parcel
	claimedAt := now.Add(2 * time.Minute)
	parcel.Status = delivery.ParcelStatusClaimed
	parcel.ClaimedAt = &claimedAt
	if err := repo.UpdateParcel(ctx, parcel); err != nil {
		t.Fatalf("UpdateParcel failed: %v", err)
	}

	incomingAfterClaim, err := repo.GetIncomingParcels(ctx, char2.ID)
	if err != nil || len(incomingAfterClaim) != 0 {
		t.Errorf("expected 0 pending incoming parcels after claim, got %v", incomingAfterClaim)
	}
}
