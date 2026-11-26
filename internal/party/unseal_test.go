package party

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	battleadapter "github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type failingPartySettler struct{}

func (failingPartySettler) ApplyPostBattleResult(context.Context, battleadapter.ApplyPostBattleRequest) (battleadapter.ApplyPostBattleResponse, error) {
	return battleadapter.ApplyPostBattleResponse{}, errors.New("settlement failed")
}

func TestStartPartyAdventure_Stage19UnsealOpensRoomsAfterCommit(t *testing.T) {
	for _, fail := range []bool{false, true} {
		ctx := context.Background()
		stages, err := adventure.InitialStageCatalog()
		if err != nil {
			t.Fatal(err)
		}
		monsters, _ := adventure.InitialMonsterCatalog()
		charRepo := newMockCharacterRepository()
		var settler PostBattleSettler = &capturingPartyBattleSettler{}
		if fail {
			settler = failingPartySettler{}
		}
		svc, err := NewService(newMockPartyRepository(), charRepo, &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}, stages, monsters,
			fixedBattleEngine{outcome: corebattle.OutcomeWin}, WithDepotRepository(newMockDepotRepo()), WithPostBattleSettler(settler), WithRNG(func(int) int { return 0 }))
		if err != nil {
			t.Fatal(err)
		}
		var calls []string
		var party []corebattle.LifetimeActor
		svc.SetUnsealHook(func(_ context.Context, name string, unsealers []corebattle.LifetimeActor) {
			calls, party = append(calls, name), unsealers
		})
		member := corecharacter.Character{ID: "a-member", Name: "Member", Level: 80, JobLevel: 30, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000}}
		leader := corecharacter.Character{ID: "z-leader", Name: "Leader", Level: 99, JobLevel: 40, Stats: corecharacter.Stats{HP: 50000, MaxHP: 50000}}
		charRepo.chars[member.ID], charRepo.chars[leader.ID] = member, leader
		detail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{Speed: 18, Name: "封印隊", StageID: "stage-19", MaxMembers: 2})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.JoinParty(ctx, detail.Party.ID, member.ID, ""); err != nil {
			t.Fatal(err)
		}
		_, err = svc.StartPartyAdventure(ctx, detail.Party.ID, leader.ID)
		if fail {
			if err == nil || len(calls) != 0 {
				t.Fatalf("rolled-back settlement opened rooms: err=%v calls=%v", err, calls)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || calls[0] != "封印隊" || len(party) != 2 || party[0].ID != leader.ID || party[0].ExperienceReward != 139 || party[1].GoldReward != 40 {
			t.Fatalf("expected one unseal by the lobby with its party leader first: calls=%v party=%+v", calls, party)
		}
	}
}
