package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

func TestBuildParticipant_ItemDefinitionIDs_StackExpansionLimits(t *testing.T) {
	ctx := context.Background()
	chars, invs, equips := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()

	char := corecharacter.Character{
		ID:    "char-items",
		JobID: "job-01",
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 100, Defense: 100, Agility: 100},
	}
	chars.characters[char.ID] = char

	// 1. Normal consumables (e.g. item-001 herbs) with large stack quantity (500)
	// MUST NOT expand ItemDefinitionIDs by 500 entries (bounded to exactly 1).
	herbInst, err := coreitem.NewInstance("item-001", 500)
	if err != nil {
		t.Fatal(err)
	}

	// 2. ItemTreasurePocket with moderate quantity (3) MUST expand to 3 entries
	pocketInst, err := coreitem.NewInstance(battle.ItemTreasurePocket, 3)
	if err != nil {
		t.Fatal(err)
	}

	// 3. ItemTreasurePocket with huge quantity (500) MUST be capped to MaxTreasurePocketParticipantAllowance (64)
	hugePocketInst, err := coreitem.NewInstance(battle.ItemTreasurePocket, 500)
	if err != nil {
		t.Fatal(err)
	}

	// Subtest A: Herb (500) and moderate Pocket (3)
	invA, _ := coreinventory.New(char.ID)
	invA.Items = []coreitem.Instance{herbInst, pocketInst}
	invs.inventories[char.ID] = invA
	equips.equipments[char.ID] = coreequipment.Equipment{CharacterID: char.ID}

	svc := battle.NewService(
		battle.WithCharacterRepository(chars),
		battle.WithInventoryRepository(invs),
		battle.WithEquipmentRepository(equips),
	)

	partA, err := svc.BuildParticipant(ctx, char.ID)
	if err != nil {
		t.Fatalf("BuildParticipant failed: %v", err)
	}

	herbCount := 0
	pocketCount := 0
	for _, id := range partA.ItemDefinitionIDs {
		switch id {
		case "item-001":
			herbCount++
		case battle.ItemTreasurePocket:
			pocketCount++
		}
	}
	if herbCount != 1 {
		t.Fatalf("expected normal consumable item-001 to appear exactly once, got %d", herbCount)
	}
	if pocketCount != 3 {
		t.Fatalf("expected ItemTreasurePocket to expand to stack quantity 3, got %d", pocketCount)
	}

	// Subtest B: Huge Pocket stack (500) capped at MaxTreasurePocketParticipantAllowance (64)
	invB, _ := coreinventory.New(char.ID)
	invB.Items = []coreitem.Instance{hugePocketInst}
	invs.inventories[char.ID] = invB

	partB, err := svc.BuildParticipant(ctx, char.ID)
	if err != nil {
		t.Fatalf("BuildParticipant failed: %v", err)
	}

	hugePocketCount := 0
	for _, id := range partB.ItemDefinitionIDs {
		if id == battle.ItemTreasurePocket {
			hugePocketCount++
		}
	}
	if hugePocketCount != battle.MaxTreasurePocketParticipantAllowance {
		t.Fatalf("expected huge pocket stack to be capped at %d, got %d",
			battle.MaxTreasurePocketParticipantAllowance, hugePocketCount)
	}
}
