package database_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/lottery"
)

func lotteryIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})

	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated lottery repository tests")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})

	name := "party2_lottery_repo_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error(err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	tables := []string{
		"players",
		"characters",
		"character_depots",
		"character_lottery",
		"takarakuji_rounds",
		"takarakuji_tickets",
	}
	for _, table := range tables {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestLotteryRepository_Integration(t *testing.T) {
	db := lotteryIsolatedDB(t)

	repo, err := database.NewLotteryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character with 100,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "TakarakujiRepoTester")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 100000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Add 10 raffle tickets (e.g. from tavern meal or god wish)
	tickets, err := repo.AddRaffleTickets(ctx, char.ID, 10)
	if err != nil {
		t.Fatalf("AddRaffleTickets failed: %v", err)
	}
	if tickets != 10 {
		t.Errorf("tickets=%d, want 10", tickets)
	}

	// 3. Use 3 raffle tickets
	remaining, err := repo.UseRaffleTickets(ctx, char.ID, 3)
	if err != nil {
		t.Fatalf("UseRaffleTickets failed: %v", err)
	}
	if remaining != 7 {
		t.Errorf("remaining=%d, want 7", remaining)
	}

	// 4. Create Takarakuji round
	drawDate := time.Now().Add(10 * 24 * time.Hour).UTC()
	round, err := repo.CreateTakarakujiRound(ctx, lottery.TakarakujiRound{
		DrawDate:     drawDate,
		IsDrawn:      false,
		Prize1ItemID: "item-129",
		Prize1Amount: 1,
		Prize2ItemID: "weapon-40",
		Prize2Amount: 2,
		Prize3ItemID: "item-126",
		Prize3Amount: 3,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTakarakujiRound failed: %v", err)
	}
	if round.RoundID == 0 {
		t.Fatal("expected non-zero RoundID")
	}

	// 5. Get active round
	active, err := repo.GetActiveTakarakujiRound(ctx)
	if err != nil {
		t.Fatalf("GetActiveTakarakujiRound failed: %v", err)
	}
	if active.RoundID != round.RoundID {
		t.Errorf("active.RoundID = %d, want %d", active.RoundID, round.RoundID)
	}

	// 6. Check purchased before buying
	hasBought, err := repo.HasCharacterPurchasedTakarakuji(ctx, round.RoundID, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasBought {
		t.Error("expected hasBought to be false")
	}

	// 7. Purchase Takarakuji ticket (30,000 gold)
	tkt, updatedChar, err := repo.PurchaseTakarakujiTicket(ctx, round.RoundID, char.ID, lottery.TakarakujiCostGold)
	if err != nil {
		t.Fatalf("PurchaseTakarakujiTicket failed: %v", err)
	}
	if tkt.ID == "" || updatedChar.Money != 100000-lottery.TakarakujiCostGold {
		t.Errorf("ticket ID = %s, money = %d, want money = %d", tkt.ID, updatedChar.Money, 100000-lottery.TakarakujiCostGold)
	}

	// 8. Check purchased after buying
	hasBought, err = repo.HasCharacterPurchasedTakarakuji(ctx, round.RoundID, char.ID)
	if err != nil || !hasBought {
		t.Errorf("expected hasBought to be true, got %v, err: %v", hasBought, err)
	}

	// 9. Second purchase attempt -> ErrAlreadyPurchased
	_, _, err = repo.PurchaseTakarakujiTicket(ctx, round.RoundID, char.ID, lottery.TakarakujiCostGold)
	if !errors.Is(err, lottery.ErrAlreadyPurchased) {
		t.Errorf("expected ErrAlreadyPurchased, got: %v", err)
	}

	// 10. List round tickets
	roundTickets, err := repo.ListRoundTakarakujiTickets(ctx, round.RoundID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roundTickets) != 1 {
		t.Errorf("len(roundTickets) = %d, want 1", len(roundTickets))
	}

	// 11. Settle round
	winningItem := "item-129"
	tkt.WonRank = 1
	tkt.WonItemID = &winningItem
	if err := repo.SettleTakarakujiRound(ctx, round.RoundID, time.Now().UTC(), []lottery.TakarakujiTicket{tkt}); err != nil {
		t.Fatalf("SettleTakarakujiRound failed: %v", err)
	}

	// 12. Check settled ticket
	charTkt, err := repo.GetCharacterTakarakujiTicket(ctx, round.RoundID, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if charTkt.WonRank != 1 || charTkt.WonItemID == nil || *charTkt.WonItemID != "item-129" {
		t.Errorf("charTkt = %+v", charTkt)
	}
}
