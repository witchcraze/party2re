package challenge_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type participantReadCharacters struct {
	battle.CharacterRepository
	chars *mockCharRepo
}

func (r participantReadCharacters) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.chars.FindByID(ctx, id)
}

type participantReadInventory struct {
	battle.InventoryRepository
	value coreinventory.Inventory
	err   error
}

func (r participantReadInventory) FindByCharacterID(context.Context, string) (coreinventory.Inventory, error) {
	return r.value, r.err
}

type participantReadEquipment struct {
	battle.EquipmentRepository
	value coreequipment.Equipment
	err   error
}

func (r participantReadEquipment) FindByCharacterID(context.Context, string) (coreequipment.Equipment, error) {
	return r.value, r.err
}

type observedRoundEngine struct {
	corebattle.Engine
	calls int
}

func (e *observedRoundEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls++
	return e.Engine.ResolvePartyBattle(req)
}

type observedRoundRepository struct {
	*mockChallengeRepo
	hofCalls, finalizations int
}

func (r *observedRoundRepository) GetHallOfFame(ctx context.Context, tier string) (*challenge.HallOfFameEntry, error) {
	r.hofCalls++
	return r.mockChallengeRepo.GetHallOfFame(ctx, tier)
}

func (r *observedRoundRepository) FinalizeSession(ctx context.Context, run challenge.ChallengeSession, exp, gold int, items []string, streak int) error {
	r.finalizations++
	return r.mockChallengeRepo.FinalizeSession(ctx, run, exp, gold, items, streak)
}

func TestChallengeParticipantReadErrorsPreserveRound(t *testing.T) {
	storageErr := errors.New("participant storage unavailable")
	for _, buffered := range []bool{false, true} {
		for _, scenario := range []string{"empty", "equipped", "inventory error", "equipment error", "inventory canceled", "equipment canceled"} {
			if buffered && scenario == "empty" {
				continue // A consumed item must exist in the durable inventory.
			}
			name := "initial/"
			if buffered {
				name = "buffered/"
			}
			t.Run(name+scenario, func(t *testing.T) {
				ctx := context.Background()
				char := createPartyTestChar("hero", 30, 200, 1000, 1000)
				char.Stats.MP, char.Stats.MaxMP = 30, 50
				chars := &mockCharRepo{chars: map[string]corecharacter.Character{"hero": char}}
				invs := participantReadInventory{value: coreinventory.Inventory{CharacterID: "hero"}}
				equips := participantReadEquipment{value: coreequipment.Equipment{CharacterID: "hero"}}
				if scenario != "empty" {
					invs.value.Items = []coreitem.Instance{{ID: "amulet", DefinitionID: battle.ItemDokuroAmulet, Quantity: 2}}
					equips.value.Slots = map[coreitem.Slot]string{coreitem.SlotAccessory: "amulet"}
				}
				var wantErr error
				switch scenario {
				case "inventory error":
					wantErr, invs.err = storageErr, storageErr
				case "equipment error":
					wantErr, equips.err = storageErr, storageErr
				case "inventory canceled":
					wantErr, invs.err = context.Canceled, context.Canceled
				case "equipment canceled":
					wantErr, equips.err = context.Canceled, context.Canceled
				}
				builder := battle.NewService(battle.WithCharacterRepository(participantReadCharacters{chars: chars}),
					battle.WithInventoryRepository(invs), battle.WithEquipmentRepository(equips))
				store := challenge.NewMemorySessionRepository()
				repo := &observedRoundRepository{mockChallengeRepo: newMockChallengeRepo()}
				engine := &observedRoundEngine{}
				svc, err := challenge.NewService(repo, chars, engine, challenge.WithParticipantBuilder(builder), challenge.WithActiveSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				run, err := svc.StartSession(ctx, "hero", "0")
				if err != nil {
					t.Fatal(err)
				}
				if buffered {
					mp := 7
					run.CurrentRound, run.CharacterCurrentHP = 2, 150
					run.AccumulatedExp, run.AccumulatedGold = 22, 6
					m := &run.Members[0]
					m.CharacterCurrentHP, m.CharacterCurrentMP = 150, &mp
					m.AccumulatedExp, m.AccumulatedGold = 22, 6
					m.ConsumedItems = []corebattle.ConsumedItem{{ID: battle.ItemDokuroAmulet, Quantity: 1}}
					saveRunFixture(t, ctx, repo, store, run)
				}
				before, err := store.GetActiveSession(ctx, "hero")
				if err != nil {
					t.Fatal(err)
				}
				durableBefore := challenge.EncodeJSON(repo.sessions[run.ID])
				result, _, err := svc.AdvanceRound(ctx, "hero", run.ID, run.Revision)
				if !errors.Is(err, wantErr) {
					t.Fatalf("error=%v, want %v", err, wantErr)
				}
				after, err := store.GetActiveSession(ctx, "hero")
				if err != nil {
					t.Fatal(err)
				}
				if wantErr != nil {
					if !reflect.DeepEqual(before, after) || engine.calls != 0 || repo.hofCalls != 0 || repo.finalizations != 0 || result != nil {
						t.Fatalf("failed read changed round or executed side effects: before=%+v after=%+v engine=%d hof=%d finalize=%d", before, after, engine.calls, repo.hofCalls, repo.finalizations)
					}
					if challenge.EncodeJSON(repo.sessions[run.ID]) != durableBefore || !reflect.DeepEqual(chars.chars["hero"], char) {
						t.Fatal("failed read changed durable session or character")
					}
				} else if result == nil || !result.Won || after == nil || after.CurrentRound != before.CurrentRound+1 || after.AccumulatedExp <= before.AccumulatedExp || after.AccumulatedGold <= before.AccumulatedGold || engine.calls != 1 {
					t.Fatalf("healthy read did not advance round: result=%+v after=%+v", result, after)
				}
			})
		}
	}
}
