package dungeon_test

import (
	"context"
	"os"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func TestDungeonIntegrationFlow(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	battleEngine := corebattle.Engine{}
	service, err := dungeon.NewService(dungeonRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create a high-level test character capable of exploring Dungeon 1
	char, err := database.CreateTestCharacter(ctx, db, "Legendary Dungeon Crawler")
	if err != nil {
		t.Fatal(err)
	}

	// Buff stats
	updateStatsQuery := `
		UPDATE characters
		SET level = 30, hp = 800, max_hp = 800, attack = 200, defense = 150, agility = 100
		WHERE id = ?
	`
	if _, err := db.ExecContext(ctx, updateStatsQuery, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. List Dungeons
	overviews, err := service.ListDungeons(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListDungeons failed: %v", err)
	}
	if len(overviews) != 3 {
		t.Errorf("expected 3 default dungeons, got %d", len(overviews))
	}
	if !overviews[0].IsUnlocked {
		t.Errorf("expected Dungeon 1 to be unlocked for level 30 character")
	}

	// 3. Start Expedition on Dungeon 1
	exp, err := service.StartExpedition(ctx, char.ID, "dungeon-01")
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}
	if exp.CurrentFloor != 1 || exp.Status != dungeon.StatusExploring {
		t.Errorf("unexpected started expedition: %#v", exp)
	}

	// 4. Navigate on Floor 1:
	// Grid Floor 1:
	// S00T
	// 1101
	// 0X0D
	// 1000
	// (0,0) S -> East (1,0) '0' -> East (2,0) '0' -> South (2,1) '0' -> South (2,2) '0' -> East (3,2) 'D'
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (1, 0)
	if err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (2, 0)
	if err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (2, 1)
	if err != nil {
		t.Fatalf("step 3 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (2, 2)
	if err != nil {
		t.Fatalf("step 4 failed: %v", err)
	}
	stairRes, err := service.Move(ctx, char.ID, dungeon.DirectionEast) // to (3, 2) 'D' (Down stairs to Floor 2!)
	if err != nil {
		t.Fatalf("step 5 stairs failed: %v", err)
	}
	if stairRes.EventType != dungeon.EventStairs || stairRes.Expedition.CurrentFloor != 2 {
		t.Fatalf("expected stairs to floor 2, got %#v", stairRes)
	}

	// 5. Navigate on Floor 2 to Boss:
	// Grid Floor 2:
	// S0X0
	// 1010
	// 00T0
	// 101B
	// (0,0) S -> South (0,1) wall '1' -> East (1,0) '0' -> South (1,1) '0' -> South (1,2) '0' -> East (2,2) 'T' -> East (3,2) '0' -> South (3,3) 'B' (Boss!)
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (1, 0)
	if err != nil {
		t.Fatalf("floor 2 step 1 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (1, 1)
	if err != nil {
		t.Fatalf("floor 2 step 2 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (1, 2)
	if err != nil {
		t.Fatalf("floor 2 step 3 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (2, 2) 'T' (Treasure)
	if err != nil {
		t.Fatalf("floor 2 step 4 treasure failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (3, 2) '0'
	if err != nil {
		t.Fatalf("floor 2 step 5 failed: %v", err)
	}
	bossRes, err := service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (3, 3) 'B' (Boss Fight & Clear!)
	if err != nil {
		t.Fatalf("floor 2 boss fight failed: %v", err)
	}
	if !bossRes.IsFinished || bossRes.Expedition.Status != dungeon.StatusCleared {
		t.Fatalf("expected dungeon clear, got %#v", bossRes)
	}

	// 6. Verify Active Expedition Removed
	activeExp, _ := service.GetActiveExpedition(ctx, char.ID)
	if activeExp != nil {
		t.Errorf("expected active expedition to be removed after clear")
	}

	// 7. Verify History & Record in Database
	history, err := service.GetHistory(ctx, char.ID, 5)
	if err != nil || len(history) != 1 {
		t.Fatalf("expected 1 history record, got %d (err: %v)", len(history), err)
	}
	if history[0].Outcome != dungeon.StatusCleared {
		t.Errorf("expected history outcome cleared, got %v", history[0].Outcome)
	}

	record, err := service.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.HighestDungeonCleared != 1 || record.TotalExpeditions != 1 {
		t.Errorf("unexpected record statistics: %#v", record)
	}
}
