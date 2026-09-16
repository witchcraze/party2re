package pvp_test

import (
	"context"
	"os"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestPvPIntegration_DisbandRefundTransactional(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)

	c1, err := database.CreateTestCharacter(ctx, db, "PvP Leader")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacter(ctx, db, "PvP Member")
	if err != nil {
		t.Fatal(err)
	}

	// Give initial money: 1000G each
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 1000 WHERE id IN (?, ?)", c1.ID, c2.ID); err != nil {
		t.Fatal(err)
	}

	roomRepo := pvp.NewMemoryRoomRepository()
	battleEngine := mockBattleEngine{result: corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}}
	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		battleEngine,
		pvp.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Leader creates room (bet 200G)
	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Integration PvP Room",
		Bet:        200,
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Member joins room
	_, err = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	if err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	// Verify both characters have 800G in DB
	c1DB, _ := charRepo.FindByID(ctx, c1.ID)
	c2DB, _ := charRepo.FindByID(ctx, c2.ID)
	if c1DB.Money != 800 || c2DB.Money != 800 {
		t.Fatalf("expected 800G in DB after bets, got c1=%d, c2=%d", c1DB.Money, c2DB.Money)
	}

	// Leader leaves room -> triggers disband and transactional refund
	err = svc.LeaveRoom(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("LeaveRoom failed: %v", err)
	}

	// Verify both characters have 1000G restored in DB
	c1DBAfter, _ := charRepo.FindByID(ctx, c1.ID)
	c2DBAfter, _ := charRepo.FindByID(ctx, c2.ID)
	if c1DBAfter.Money != 1000 || c2DBAfter.Money != 1000 {
		t.Fatalf("expected 1000G in DB after refund, got c1=%d, c2=%d", c1DBAfter.Money, c2DBAfter.Money)
	}
}

func TestPvPIntegration_MatchPrizeDistributionTransactional(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)

	c1, err := database.CreateTestCharacter(ctx, db, "PvP Winner")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacter(ctx, db, "PvP Loser")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 500 WHERE id IN (?, ?)", c1.ID, c2.ID); err != nil {
		t.Fatal(err)
	}

	roomRepo := pvp.NewMemoryRoomRepository()
	battleEngine := mockBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome:    corebattle.OutcomeWin,
			WinnerSide: pvp.ColorRed,
			WinnerTeam: pvp.ColorRed,
			Turns:      1,
		},
	}
	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		battleEngine,
		pvp.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Integration Prize Room",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)

	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if !res.MatchCompleted {
		t.Fatal("expected match to end")
	}

	// Total prize pool was 200G. Winner c1 should have 400 + 200 = 600G and PvPWins=1
	c1DB, _ := charRepo.FindByID(ctx, c1.ID)
	c2DB, _ := charRepo.FindByID(ctx, c2.ID)
	if c1DB.Money != 600 {
		t.Fatalf("expected winner money 600 in DB, got %d", c1DB.Money)
	}
	if c1DB.PvPWins != 1 {
		t.Fatalf("expected winner PvPWins 1 in DB, got %d", c1DB.PvPWins)
	}
	if c2DB.Money != 400 {
		t.Fatalf("expected loser money 400 in DB, got %d", c2DB.Money)
	}
}
