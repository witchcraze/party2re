package database

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestInventoryRepositoryConcurrentAdjacentSaveNoDeadlock(t *testing.T) {
	db := sparseIsolatedDB(t, "players", "characters", "character_depots", "inventory_items", "equipment_slots")
	ctx := context.Background()
	repo, err := NewInventoryRepository(db)
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

	// These are the only two characters in this DB and are adjacent in the index.
	characterIDs := []string{"00000000000000000000000000000001", "00000000000000000000000000000002"}
	expected := make([][]item.Instance, 2)
	for worker, characterID := range characterIDs {
		char, err := corecharacter.New(fmt.Sprintf("AdjInv%d", worker))
		if err != nil {
			t.Fatal(err)
		}
		char.ID, char.PlayerID = characterID, p.ID
		if err := chars.Save(ctx, char); err != nil {
			t.Fatal(err)
		}
		for _, definitionID := range []string{"weapon-01", "armor-01"} {
			inst, err := item.NewInstance(definitionID, 1)
			if err != nil {
				t.Fatal(err)
			}
			expected[worker] = append(expected[worker], inst)
		}
		if _, err := CreateTestInventoryWithItems(ctx, db, characterID, expected[worker]); err != nil {
			t.Fatal(err)
		}
		sortItemsByID(expected[worker])
	}

	for round := 0; round < 25; round++ {
		roundCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var ready sync.WaitGroup
		ready.Add(2)
		operations := make([]func() error, 2)
		for worker, characterID := range characterIDs {
			newItem, err := item.NewInstance([]string{"weapon-01", "armor-01"}[worker], 1)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			before := slices.Clone(expected[worker])
			after := append(slices.Clone(before), newItem)
			if round == 12 {
				after = nil // Exercise the empty-set deletion as well as id NOT IN.
			} else if len(after) > 8 {
				after = after[:3]
			}
			sortItemsByID(after)
			operations[worker] = func() error {
				signalReady := sync.OnceFunc(ready.Done)
				defer signalReady() // Release the peer even if locking fails.
				err := RunInTx(roundCtx, db, func(txCtx context.Context) error {
					if _, err := chars.FindByIDForUpdate(txCtx, characterID); err != nil {
						return err
					}
					inv, err := repo.FindByCharacterIDForUpdate(txCtx, characterID)
					if err != nil {
						return err
					}
					signalReady()
					ready.Wait()
					if !slices.Equal(inv.Items, before) {
						return fmt.Errorf("items before save: got %+v, want %+v", inv.Items, before)
					}
					if err := inv.Add(newItem); err != nil {
						return err
					}
					if round == 12 {
						inv.Items = nil
					} else if len(inv.Items) > 8 {
						inv.Items = inv.Items[:3]
					}
					return repo.Save(txCtx, inv)
				})
				if err == nil {
					expected[worker] = after // Only committed operations advance expected state.
				} else {
					cancel()
				}
				return err
			}
		}
		// RunRace joins both workers before reporting errors or closing DB connections.
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

func TestCharacterDeleteAndAdjacentInventorySaveNoDeadlock(t *testing.T) {
	db := sparseIsolatedDB(t)
	ctx := context.Background()
	invRepo, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	gemRepo, err := NewGemBoxRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	characterIDs := []string{"00000000000000000000000000000001", "00000000000000000000000000000002"}
	for round := 0; round < 10; round++ {
		var survivorInv, survivorDepot, survivorGems []item.Instance
		for worker, characterID := range characterIDs {
			char, err := corecharacter.New(fmt.Sprintf("Del%d_%d", round, worker))
			if err != nil {
				t.Fatal(err)
			}
			char.ID, char.PlayerID = characterID, p.ID
			if err := chars.Save(ctx, char); err != nil {
				t.Fatal(err)
			}
			var invItems, depItems, gemItems []item.Instance
			for _, defID := range []string{"weapon-01", "armor-01"} {
				inst, err := item.NewInstance(defID, 1)
				if err != nil {
					t.Fatal(err)
				}
				invItems = append(invItems, inst)
			}
			if _, err := CreateTestInventoryWithItems(ctx, db, characterID, invItems); err != nil {
				t.Fatal(err)
			}
			for _, defID := range []string{"weapon-31", "armor-34"} {
				inst, err := item.NewInstance(defID, 1)
				if err != nil {
					t.Fatal(err)
				}
				depItems = append(depItems, inst)
			}
			if _, err := CreateTestDepot(ctx, db, characterID, 5, depItems); err != nil {
				t.Fatal(err)
			}
			for _, defID := range []string{"gem_atk_1", "gem_heal_1"} {
				inst, err := item.NewInstance(defID, 1)
				if err != nil {
					t.Fatal(err)
				}
				gemItems = append(gemItems, inst)
			}
			if err := gemRepo.Save(ctx, gemstore.GemBox{
				CharacterID: characterID,
				Capacity:    15,
				Items:       slices.Clone(gemItems),
			}); err != nil {
				t.Fatal(err)
			}
			if worker == 0 {
				sortItemsByID(invItems)
				survivorInv = invItems
				survivorDepot = depItems
				survivorGems = gemItems
			}
		}

		// Worker 0 prunes and updates survivor's inventory, depot, and gem box while
		// Worker 1 deletes the adjacent character across all three tables.
		roundCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var ready sync.WaitGroup
		ready.Add(2)

		newInvItem, err := item.NewInstance("item-001", 1)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		newDepItem, err := item.NewInstance("weapon-40", 1)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		newGemItem, err := item.NewInstance("gem_mind_1", 1)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		opSave := func() error {
			signalReady := sync.OnceFunc(ready.Done)
			defer signalReady()
			err := RunInTx(roundCtx, db, func(txCtx context.Context) error {
				if _, err := chars.FindByIDForUpdate(txCtx, characterIDs[0]); err != nil {
					return err
				}
				inv, err := invRepo.FindByCharacterIDForUpdate(txCtx, characterIDs[0])
				if err != nil {
					return err
				}
				dep, err := depotRepo.FindByCharacterIDForUpdate(txCtx, characterIDs[0])
				if err != nil {
					return err
				}
				box, err := gemRepo.FindByCharacterIDForUpdate(txCtx, characterIDs[0])
				if err != nil {
					return err
				}
				signalReady()
				ready.Wait()
				inv.Items = append(inv.Items[:1], newInvItem)
				if err := invRepo.Save(txCtx, inv); err != nil {
					return err
				}
				dep.Items = append(dep.Items[:1], newDepItem)
				if err := depotRepo.Save(txCtx, dep); err != nil {
					return err
				}
				box.Items = append(box.Items[:1], newGemItem)
				return gemRepo.Save(txCtx, box)
			})
			if err != nil {
				cancel()
			}
			return err
		}
		opDelete := func() error {
			signalReady := sync.OnceFunc(ready.Done)
			defer signalReady()
			err := RunInTx(roundCtx, db, func(txCtx context.Context) error {
				if err := chars.DeleteSharedRows(txCtx, characterIDs[1]); err != nil {
					return err
				}
				if _, err := chars.FindByIDForUpdate(txCtx, characterIDs[1]); err != nil {
					return err
				}
				signalReady()
				ready.Wait()
				return chars.Delete(txCtx, characterIDs[1])
			})
			if err != nil {
				cancel()
			}
			return err
		}

		errs := testutil.RunRace(opSave, opDelete)
		cancel()
		for i, err := range errs {
			if err != nil {
				t.Errorf("round %d worker %d: %v", round, i, err)
			}
		}
		if t.Failed() {
			t.FailNow()
		}

		gotInv, err := invRepo.FindByCharacterID(ctx, characterIDs[0])
		if err != nil {
			t.Fatal(err)
		}
		wantInv := append(slices.Clone(survivorInv[:1]), newInvItem)
		sortItemsByID(wantInv)
		if !slices.Equal(gotInv.Items, wantInv) {
			t.Fatalf("round %d survivor inventory = %+v, want %+v", round, gotInv.Items, wantInv)
		}
		gotDepot, err := depotRepo.FindByCharacterID(ctx, characterIDs[0])
		if err != nil {
			t.Fatal(err)
		}
		wantDepot := append(slices.Clone(survivorDepot[:1]), newDepItem)
		if !slices.Equal(gotDepot.Items, wantDepot) {
			t.Fatalf("round %d survivor depot = %+v, want %+v", round, gotDepot.Items, wantDepot)
		}
		gotBox, err := gemRepo.FindByCharacterID(ctx, characterIDs[0])
		if err != nil {
			t.Fatal(err)
		}
		wantBox := append(slices.Clone(survivorGems[:1]), newGemItem)
		if !slices.Equal(gotBox.Items, wantBox) {
			t.Fatalf("round %d survivor gem box = %+v, want %+v", round, gotBox.Items, wantBox)
		}
		if _, err := chars.FindByID(ctx, characterIDs[1]); !errors.Is(err, corecharacter.ErrNotFound) {
			t.Fatalf("round %d deleted character FindByID error = %v, want ErrNotFound", round, err)
		}
		if err := chars.Delete(ctx, characterIDs[0]); err != nil {
			t.Fatal(err)
		}
	}
}

func sortItemsByID(items []item.Instance) {
	slices.SortFunc(items, func(a, b item.Instance) int {
		return strings.Compare(a.ID, b.ID)
	})
}
