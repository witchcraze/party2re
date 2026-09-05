package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

func TestPartyRepository_AdventureLog(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Verify NewPartyRepository validation
	if _, err := NewPartyRepository(nil); err == nil {
		t.Fatal("expected error when db is nil, got nil")
	}

	partyRepo, err := NewPartyRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	pID := id.New()
	otherPID := id.New()
	baseTime := time.Now().UTC().Truncate(time.Second)

	// 2. Save multiple adventure logs for pID and another party
	log1 := party.PartyAdventureLog{
		ID:                  id.New(),
		PartyID:             pID,
		StageID:             "forest",
		Outcome:             "win",
		Turns:               3,
		TotalEXP:            150,
		TotalGold:           75,
		SynergyBonusPercent: 10,
		DetailsJSON:         `{"outcome":"win","turns":3}`,
		CreatedAt:           baseTime.Add(-10 * time.Minute),
	}
	if err := partyRepo.SaveAdventureLog(ctx, log1); err != nil {
		t.Fatalf("SaveAdventureLog(log1) failed: %v", err)
	}

	log2 := party.PartyAdventureLog{
		ID:                  id.New(),
		PartyID:             pID,
		StageID:             "cavern",
		Outcome:             "loss",
		Turns:               5,
		TotalEXP:            30,
		TotalGold:           10,
		SynergyBonusPercent: 5,
		DetailsJSON:         `{"outcome":"loss","turns":5}`,
		CreatedAt:           baseTime,
	}
	if err := partyRepo.SaveAdventureLog(ctx, log2); err != nil {
		t.Fatalf("SaveAdventureLog(log2) failed: %v", err)
	}

	otherLog := party.PartyAdventureLog{
		ID:                  id.New(),
		PartyID:             otherPID,
		StageID:             "graveyard",
		Outcome:             "win",
		Turns:               2,
		TotalEXP:            200,
		TotalGold:           100,
		SynergyBonusPercent: 15,
		DetailsJSON:         "",
		CreatedAt:           baseTime,
	}
	if err := partyRepo.SaveAdventureLog(ctx, otherLog); err != nil {
		t.Fatalf("SaveAdventureLog(otherLog) failed: %v", err)
	}

	// 3. Retrieve logs for pID and verify ordering and fields
	logs, err := partyRepo.GetAdventureLogsByPartyID(ctx, pID)
	if err != nil {
		t.Fatalf("GetAdventureLogsByPartyID failed: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs for party %s, got %d", pID, len(logs))
	}

	// Ordered by created_at DESC (log2 first, then log1)
	if logs[0].ID != log2.ID {
		t.Errorf("expected first log to be log2 (%s), got %s", log2.ID, logs[0].ID)
	}
	if logs[0].StageID != "cavern" || logs[0].Outcome != "loss" || logs[0].Turns != 5 || logs[0].TotalEXP != 30 || logs[0].TotalGold != 10 || logs[0].SynergyBonusPercent != 5 || logs[0].DetailsJSON != log2.DetailsJSON {
		t.Errorf("unexpected log[0] content: %+v", logs[0])
	}

	if logs[1].ID != log1.ID {
		t.Errorf("expected second log to be log1 (%s), got %s", log1.ID, logs[1].ID)
	}
	if logs[1].StageID != "forest" || logs[1].Outcome != "win" || logs[1].Turns != 3 || logs[1].TotalEXP != 150 || logs[1].TotalGold != 75 || logs[1].SynergyBonusPercent != 10 || logs[1].DetailsJSON != log1.DetailsJSON {
		t.Errorf("unexpected log[1] content: %+v", logs[1])
	}

	// 4. Retrieve logs for otherPID
	otherLogs, err := partyRepo.GetAdventureLogsByPartyID(ctx, otherPID)
	if err != nil {
		t.Fatalf("GetAdventureLogsByPartyID(otherPID) failed: %v", err)
	}
	if len(otherLogs) != 1 {
		t.Fatalf("expected 1 log for otherPID, got %d", len(otherLogs))
	}
	if otherLogs[0].ID != otherLog.ID || otherLogs[0].DetailsJSON != "" {
		t.Errorf("unexpected otherLog content: %+v", otherLogs[0])
	}

	// 5. Query non-existent party ID
	emptyLogs, err := partyRepo.GetAdventureLogsByPartyID(ctx, id.New())
	if err != nil {
		t.Fatalf("GetAdventureLogsByPartyID(non-existent) failed: %v", err)
	}
	if len(emptyLogs) != 0 {
		t.Fatalf("expected 0 logs for non-existent party, got %d", len(emptyLogs))
	}
}
