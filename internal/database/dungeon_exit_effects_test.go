package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Escape is legacy nigeru for every member (_battle.cgi:1208–1220): fatigue round×3+1,
// is_eat cleared, vitality carried and capped, item105/106 bonus, used items cleared; once per run.
func TestDungeonRepository_FinalizeExpedition_EscapeExitEffects(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tavern, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	const round = 5
	shoes, _ := coreitem.NewInstance("item-105", 1)
	rooster, _ := coreitem.NewInstance("item-106", 1)
	herb, _ := coreitem.NewInstance("item-101", 1)
	held := []coreitem.Instance{shoes, rooster, herb}
	members := make([]dungeon.ExpeditionMember, 3)
	for i, name := range []string{"Exit Shoes", "Exit Rooster", "Exit Herb"} {
		c, err := database.CreateTestCharacter(ctx, db, name+" "+id.New()[:8])
		if err != nil {
			t.Fatal(err)
		}
		c.Stats.MaxHP, c.Stats.HP, c.Stats.MaxMP, c.Stats.MP, c.Tired = 100, 100, 50, 50, 2
		if err := chars.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := invs.Save(ctx, coreinventory.Inventory{CharacterID: c.ID, Items: []coreitem.Instance{held[i]}}); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if err := equips.Save(ctx, coreequipment.Equipment{CharacterID: c.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: held[i].ID}}); err != nil {
				t.Fatal(err)
			}
		}
		status, err := tavern.GetCharacterStatus(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		status.IsFull = true
		if err := tavern.UpsertCharacterStatus(ctx, status); err != nil {
			t.Fatal(err)
		}
		members[i] = dungeon.ExpeditionMember{CharacterID: c.ID, CurrentHP: 40, MaxHP: 100, CurrentMP: 200, MaxMP: 50}
	}
	// The herb holder fell and used the held herb during the run.
	members[2].CurrentHP = 0
	members[2].ConsumedItems = []corebattle.ConsumedItem{{ID: "item-101", InstanceID: herb.ID, Quantity: 1}}

	before := make(map[string]int, 3)
	for _, m := range members {
		c, _ := chars.FindByID(ctx, m.CharacterID)
		before[m.CharacterID] = c.Money
	}
	history := dungeon.DungeonExpeditionHistory{
		ID: id.New(), CharacterID: members[0].CharacterID, DungeonID: "0",
		Outcome: dungeon.StatusEscaped, CreatedAt: time.Now().UTC(), ExitRound: round,
	}
	leader, _ := chars.FindByID(ctx, members[0].CharacterID)
	errA, errB := testutil.RunRace2(
		func() error { l := leader; return dungeonRepo.FinalizeExpedition(ctx, history, &l, nil, members...) },
		func() error { l := leader; return dungeonRepo.FinalizeExpedition(ctx, history, &l, nil, members...) },
	)
	if (errA == nil) == (errB == nil) || !errors.Is(errors.Join(errA, errB), dungeon.ErrExpeditionAlreadyFinished) {
		t.Fatalf("concurrent Escape: want exactly one settlement, got %v / %v", errA, errB)
	}
	// A retry after commit is also a no-op.
	if err := dungeonRepo.FinalizeExpedition(ctx, history, &leader, nil, members...); !errors.Is(err, dungeon.ErrExpeditionAlreadyFinished) {
		t.Fatalf("retry: got %v", err)
	}

	for i, m := range members {
		c, err := chars.FindByID(ctx, m.CharacterID)
		if err != nil {
			t.Fatal(err)
		}
		wantHP := 40
		if i == 2 {
			wantHP = 0
		}
		if c.Tired != 2+round*3+1 || c.Stats.HP != wantHP || c.Stats.MP != 50 {
			t.Errorf("member %d: Tired=%d HP=%d MP=%d, want %d/%d/50", i, c.Tired, c.Stats.HP, c.Stats.MP, 2+round*3+1, wantHP)
		}
		status, err := tavern.GetCharacterStatus(ctx, m.CharacterID)
		if err != nil || status.IsFull {
			t.Errorf("member %d: meal flag not reset: %+v %v", i, status, err)
		}
		gold := c.Money - before[m.CharacterID]
		switch i {
		case 0:
			want := leader
			if _, err := progression.ApplyExperience(&want, round*20); err != nil {
				t.Fatal(err)
			}
			if c.Experience != want.Experience || c.Level != want.Level {
				t.Errorf("item-105: EXP/Lv %d/%d, want %d/%d", c.Experience, c.Level, want.Experience, want.Level)
			}
		case 1:
			if gold < 0 || gold >= round*77 {
				t.Errorf("item-106: gold +%d, want [0,%d)", gold, round*77)
			}
		case 2:
			inv, _ := invs.FindByCharacterID(ctx, m.CharacterID)
			if _, found := inv.Find(herb.ID); found {
				t.Error("used held item was not cleared")
			}
		}
	}
}
