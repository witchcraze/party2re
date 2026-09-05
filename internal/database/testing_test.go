package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

func TestSanitizeTestName(t *testing.T) {
	// 1. Empty name should use defaultPrefix and suffix <= maxLen
	nameEmpty := sanitizeTestName("", 32, "testpref")
	if len(nameEmpty) > 32 {
		t.Errorf("empty name exceeded maxLen: %d", len(nameEmpty))
	}
	if !strings.HasPrefix(nameEmpty, "testpref_") {
		t.Errorf("empty name should start with defaultPrefix: %s", nameEmpty)
	}

	// 2. Normal name within maxLen should be kept as-is
	normal := "NormalHero"
	if res := sanitizeTestName(normal, 32, "Hero"); res != normal {
		t.Errorf("normal name should be kept as-is: got %s, want %s", res, normal)
	}

	// 3. Overly long name (>32 chars) should be truncated to <= 32 chars
	superLong := "ThisIsAnExtremelyLongCharacterNameExceedingThirtyTwoChars"
	truncated := sanitizeTestName(superLong, 32, "Long")
	if len(truncated) > 32 {
		t.Errorf("truncated name exceeded 32 chars: length=%d, name=%s", len(truncated), truncated)
	}
	if truncated != superLong[:32] {
		t.Errorf("truncated name mismatch: got %s, want %s", truncated, superLong[:32])
	}
}

func TestDatabaseTestEntityFactories(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. CreateTestPlayerWithUsername
	t.Run("CreateTestPlayerWithUsername", func(t *testing.T) {
		uname := fmt.Sprintf("p_usr_%s", id.New()[:8])
		p, err := CreateTestPlayerWithUsername(ctx, db, uname)
		if err != nil {
			t.Fatalf("CreateTestPlayerWithUsername error: %v", err)
		}
		if p.ID == "" || p.Username != uname {
			t.Errorf("unexpected player attributes: %+v", p)
		}
	})

	// 2. CreateTestCharacterWithFunds
	var charID string
	t.Run("CreateTestCharacterWithFunds", func(t *testing.T) {
		c, err := CreateTestCharacterWithFunds(ctx, db, "RichHero", 7777)
		if err != nil {
			t.Fatalf("CreateTestCharacterWithFunds error: %v", err)
		}
		if c.ID == "" || c.Money != 7777 || c.Name != "RichHero" {
			t.Errorf("unexpected character attributes: %+v", c)
		}
		charID = c.ID
	})

	// 3. CreateTestGuildWithLeader
	t.Run("CreateTestGuildWithLeader", func(t *testing.T) {
		gname := fmt.Sprintf("ApexG_%s", id.New()[:8])
		g, leader, err := CreateTestGuildWithLeader(ctx, db, gname, 15000)
		if err != nil {
			t.Fatalf("CreateTestGuildWithLeader error: %v", err)
		}
		if g.ID == "" || g.Name != gname || g.LeaderCharacterID != leader.ID {
			t.Errorf("unexpected guild attributes: %+v", g)
		}
		if leader.Money != 15000 {
			t.Errorf("unexpected leader money: %d", leader.Money)
		}
	})

	// 4. CreateTestInventoryWithItems
	t.Run("CreateTestInventoryWithItems", func(t *testing.T) {
		item1, _ := coreitem.NewInstance("herb-01", 3)
		item2, _ := coreitem.NewInstance("potion-01", 2)
		inv, err := CreateTestInventoryWithItems(ctx, db, charID, []coreitem.Instance{item1, item2})
		if err != nil {
			t.Fatalf("CreateTestInventoryWithItems error: %v", err)
		}
		if inv.CharacterID != charID || len(inv.Items) != 2 {
			t.Errorf("unexpected inventory: %+v", inv)
		}
	})

	// 5. CreateTestDepot
	t.Run("CreateTestDepot", func(t *testing.T) {
		depItem, _ := coreitem.NewInstance("elixir-01", 1)
		dep, err := CreateTestDepot(ctx, db, charID, 5000, []coreitem.Instance{depItem})
		if err != nil {
			t.Fatalf("CreateTestDepot error: %v", err)
		}
		if dep.CharacterID != charID || dep.Gold != 5000 || len(dep.Items) != 1 {
			t.Errorf("unexpected depot: %+v", dep)
		}
	})
}
