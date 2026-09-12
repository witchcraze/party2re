package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

func TestApplyPostBattleResult_BanishedFatigue(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	txProv := &mockTxProvider{}

	charRepo.characters["c1"] = corecharacter.Character{
		ID:    "c1",
		Name:  "Hero1",
		Tired: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	charRepo.characters["c2"] = corecharacter.Character{
		ID:    "c2",
		Name:  "Hero2",
		Tired: 20,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	invRepo.inventories["c1"] = coreinventory.Inventory{CharacterID: "c1"}
	invRepo.inventories["c2"] = coreinventory.Inventory{CharacterID: "c2"}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithTransactionProvider(txProv),
	)

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"c1", "c2"},
		BattleResult: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			RemainingHP: map[string]int{
				"c1": 50,
				"c2": 0,
			},
			BanishedIDs: map[string]bool{
				"c2": true,
			},
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// c1 was not banished -> Tired remains 10
	if resp.UpdatedCharacters["c1"].Tired != 10 {
		t.Errorf("expected c1 Tired to be 10, got %d", resp.UpdatedCharacters["c1"].Tired)
	}

	// c2 was banished by dejon -> Tired increases by +30% -> 20 + 30 = 50
	if resp.UpdatedCharacters["c2"].Tired != 50 {
		t.Errorf("expected c2 Tired to be 50, got %d", resp.UpdatedCharacters["c2"].Tired)
	}
}
