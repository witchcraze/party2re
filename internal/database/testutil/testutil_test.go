package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

func TestTestutilReExports(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. CreateTestPlayerWithUsername
	uname := fmt.Sprintf("tu_user_%s", id.New()[:8])
	p, err := CreateTestPlayerWithUsername(ctx, db, uname)
	if err != nil {
		t.Fatalf("CreateTestPlayerWithUsername failed: %v", err)
	}
	if p.ID == "" || p.Username != uname {
		t.Fatalf("expected player username %s, got %s", uname, p.Username)
	}

	// 2. CreateTestCharacterWithFunds
	c, err := CreateTestCharacterWithFunds(ctx, db, "TestutilHero", 8888)
	if err != nil {
		t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
	}
	if c.Money != 8888 || c.Name != "TestutilHero" {
		t.Fatalf("expected money 8888 and name TestutilHero, got %+v", c)
	}

	// 3. CreateTestGuildWithLeader
	gname := fmt.Sprintf("TU_Gld_%s", id.New()[:8])
	g, leader, err := CreateTestGuildWithLeader(ctx, db, gname, 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}
	if g.LeaderCharacterID != leader.ID || g.Name != gname {
		t.Fatalf("expected leader ID and name match: %s vs %s, %s vs %s", g.LeaderCharacterID, leader.ID, g.Name, gname)
	}

	// 4. CreateTestInventoryWithItems
	it, _ := coreitem.NewInstance("potion-01", 5)
	inv, err := CreateTestInventoryWithItems(ctx, db, c.ID, []coreitem.Instance{it})
	if err != nil {
		t.Fatalf("CreateTestInventoryWithItems failed: %v", err)
	}
	if len(inv.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(inv.Items))
	}

	// 5. CreateTestDepot
	dep, err := CreateTestDepot(ctx, db, c.ID, 3000, []coreitem.Instance{it})
	if err != nil {
		t.Fatalf("CreateTestDepot failed: %v", err)
	}
	if dep.Gold != 3000 {
		t.Fatalf("expected gold 3000, got %d", dep.Gold)
	}

	// 6. RunRace2
	err1, err2 := RunRace2(
		func() error { return nil },
		func() error { return nil },
	)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected race errors: %v, %v", err1, err2)
	}
}
