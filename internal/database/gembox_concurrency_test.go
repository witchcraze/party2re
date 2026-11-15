package database

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestGemBoxRepositoryConcurrentAdjacentSaveNoDeadlock(t *testing.T) {
	db := sparseIsolatedDB(t, "players", "characters", "character_depots", "character_gem_boxes", "gem_box_items")
	ctx := context.Background()
	repo, err := NewGemBoxRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	characterIDs := []string{"00000000000000000000000000000001", "00000000000000000000000000000002"}
	expected := make([][]item.Instance, 2)
	for worker, characterID := range characterIDs {
		char, err := corecharacter.New(fmt.Sprintf("AdjGem%d", worker))
		if err != nil {
			t.Fatal(err)
		}
		char.ID, char.PlayerID = characterID, p.ID
		if err := chars.Save(ctx, char); err != nil {
			t.Fatal(err)
		}
		for _, defID := range []string{"gem_atk_1", "gem_heal_1"} {
			inst, err := item.NewInstance(defID, 1)
			if err != nil {
				t.Fatal(err)
			}
			expected[worker] = append(expected[worker], inst)
		}
		if err := repo.Save(ctx, gemstore.GemBox{
			CharacterID: characterID,
			Capacity:    15,
			Items:       slices.Clone(expected[worker]),
		}); err != nil {
			t.Fatal(err)
		}
	}

	for round := 0; round < 25; round++ {
		roundCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var ready sync.WaitGroup
		ready.Add(2)
		operations := make([]func() error, 2)
		for worker, characterID := range characterIDs {
			newItem, err := item.NewInstance([]string{"gem_atk_1", "gem_heal_1"}[worker], 1)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			before := slices.Clone(expected[worker])
			after := append(slices.Clone(before), newItem)
			if round == 12 {
				after = nil
			} else if len(after) > 8 {
				after = after[:3]
			}
			operations[worker] = func() error {
				signalReady := sync.OnceFunc(ready.Done)
				defer signalReady()
				err := repo.RunInTx(roundCtx, func(txCtx context.Context) error {
					box, err := repo.FindByCharacterIDForUpdate(txCtx, characterID)
					if err != nil {
						return err
					}
					signalReady()
					ready.Wait()
					if !slices.Equal(box.Items, before) {
						return fmt.Errorf("items before save: got %+v, want %+v", box.Items, before)
					}
					if err := box.AddItem(newItem); err != nil {
						return err
					}
					if round == 12 {
						box.Items = nil
					} else if len(box.Items) > 8 {
						box.Items = box.Items[:3]
					}
					return repo.Save(txCtx, box)
				})
				if err == nil {
					expected[worker] = after
				} else {
					cancel()
				}
				return err
			}
		}
		errs := testutil.RunRace(operations...)
		cancel()
		for worker, err := range errs {
			if err != nil {
				t.Errorf("character %s round %d: %v", characterIDs[worker], round, err)
			}
		}
		if t.Failed() {
			t.FailNow()
		}
	}
	for worker, characterID := range characterIDs {
		got, err := repo.FindByCharacterID(ctx, characterID)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.Items, expected[worker]) {
			t.Errorf("character %s final items: got %+v, want %+v", characterID, got.Items, expected[worker])
		}
	}
}
