package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type mapCharacterReader struct {
	char corecharacter.Character
	err  error
}

func (r *mapCharacterReader) FindByID(context.Context, string) (corecharacter.Character, error) {
	return r.char, r.err
}

type mapInventoryReader struct {
	inv    coreinventory.Inventory
	err    error
	actors []string
}

func (r *mapInventoryReader) FindByCharacterID(_ context.Context, actor string) (coreinventory.Inventory, error) {
	r.actors = append(r.actors, actor)
	return r.inv, r.err
}

// This reader fails immediately if observation attempts a transition.
type mapRunReader struct {
	dungeon.ActiveExpeditionStore
	run *dungeon.ActiveExpedition
	err error
}

func (r *mapRunReader) GetActiveExpedition(context.Context, string) (*dungeon.ActiveExpedition, error) {
	return r.run, r.err
}

func TestMapObservationSourcesAndContinuations(t *testing.T) {
	ctx := context.Background()
	run := &dungeon.ActiveExpedition{ID: "run", CharacterID: "hero", DungeonID: dungeon.NativeMap0ID, MapVariant: 1, Revision: 4, Status: dungeon.StatusExploring, TurnsRemaining: 20}
	store := &mapRunReader{run: run}
	chars := &mapCharacterReader{char: corecharacter.Character{ID: "hero", JobID: "job-09"}}
	inventory := &mapInventoryReader{inv: coreinventory.Inventory{Items: []coreitem.Instance{{DefinitionID: "item-197", Quantity: 1}}}}
	svc, err := dungeon.NewService(newMockDungeonRepo(), chars, &corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store), dungeon.WithInventoryProvider(inventory))
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.ObserveExpedition(ctx, "hero")
	if err != nil || view.Map.Radius != 3 || !slices.Equal(view.Directions, []dungeon.Direction{dungeon.DirectionNorth, dungeon.DirectionSouth, dungeon.DirectionEast, dungeon.DirectionWest}) || !view.Controls.CanMove || view.Controls.CanFight || !view.Controls.CanEscape {
		t.Fatalf("solo map: %+v %v", view, err)
	}
	for _, item := range []string{"197", "item-197", "scope_goggles", "スコープ"} {
		inventory.inv.Items[0].DefinitionID = item
		if got, err := svc.ObserveExpedition(ctx, "hero"); err != nil || got.Map.Radius != 3 {
			t.Fatalf("item %s: %+v %v", item, got, err)
		}
	}
	before := *run
	for _, source := range []string{"run", "character", "inventory"} {
		failure := errors.New(source + " unavailable")
		switch source {
		case "run":
			store.err = failure
		case "character":
			chars.err = failure
		case "inventory":
			inventory.err = failure
		}
		if got, err := svc.ObserveExpedition(ctx, "hero"); got != nil || !errors.Is(err, failure) {
			t.Fatalf("partial %s: %+v %v", source, got, err)
		}
		store.err, chars.err, inventory.err = nil, nil, nil
	}
	if !reflect.DeepEqual(before, *run) {
		t.Fatal("observation mutated run")
	}
	inventory.inv.Items = nil
	run.Members = []dungeon.ExpeditionMember{{CharacterID: "hero", JobID: "job-01"}, {CharacterID: "scout", JobID: "job-79", ItemIDs: []string{"item-197"}}}
	for _, tc := range []struct {
		actor  string
		radius int
		move   bool
	}{{"hero", 1, true}, {"scout", 3, false}} {
		got, err := svc.ObserveExpedition(ctx, tc.actor)
		if err != nil || got.Map.Radius != tc.radius || got.Controls.CanMove != tc.move || (tc.actor == "scout" && (got.Controls.CanFight || got.Controls.CanEscape || len(got.Directions) != 0)) {
			t.Fatalf("actor %s: %+v %v", tc.actor, got, err)
		}
	}
	if got, err := svc.ObserveExpedition(ctx, "outsider"); got != nil || !errors.Is(err, dungeon.ErrForbidden) {
		t.Fatalf("private run: %+v %v", got, err)
	}
	run.Encounter = &dungeon.ActiveEncounter{}
	got, err := svc.ObserveExpedition(ctx, "hero")
	if err != nil || got.Controls.CanMove || !got.Controls.CanFight || !got.Controls.CanEscape || len(got.Directions) != 0 {
		t.Fatalf("combat: %+v %v", got, err)
	}
	run.Encounter = nil
	run.TurnsRemaining = 0
	got, err = svc.ObserveExpedition(ctx, "hero")
	if err != nil || got.Controls.CanMove || len(got.Directions) != 0 || !got.Controls.CanEscape {
		t.Fatalf("exhausted: %+v %v", got, err)
	}
	for _, status := range []dungeon.ExpeditionStatus{dungeon.StatusEscaped, dungeon.StatusWipedOut} {
		run.Status = status
		chars.err, inventory.err = errors.New("terminal map must not read character"), errors.New("terminal map must not read inventory")
		got, err := svc.ObserveExpedition(ctx, "hero")
		if err != nil || got.Map != nil || got.Controls.CanMove || !got.Controls.CanFight || !got.Controls.CanEscape || got.Status != status {
			t.Fatalf("terminal %s: %+v %v", status, got, err)
		}
	}
	store.run = nil
	if got, err := svc.ObserveExpedition(ctx, "hero"); got != nil || err != nil {
		t.Fatalf("ended: %+v %v", got, err)
	}
}

func TestMapObservationRejectsInvalidGeometry(t *testing.T) {
	for _, grid := range [][]string{{}, {"S"}, {"S0", "0"}} {
		store := &mapRunReader{run: &dungeon.ActiveExpedition{ID: "run", CharacterID: "hero", DungeonID: "broken", Status: dungeon.StatusExploring}}
		svc, err := dungeon.NewService(newMockDungeonRepo(), &mockCharRepo{}, &corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons([]dungeon.Dungeon{{ID: "broken", Variants: []dungeon.Floor{{Width: 2, Height: 2, Grid: grid}}}}))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := svc.ObserveExpedition(context.Background(), "hero"); got != nil || err == nil {
			t.Fatalf("partial broken map: %+v %v", got, err)
		}
	}
}

func TestDungeonStartDoesNotSaveAnIncompleteVisibilitySnapshot(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("inventory unavailable")
	inventory := &mapInventoryReader{err: failure}
	store := dungeon.NewMemoryExpeditionRepository()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{"hero": createTestChar("hero", 10, 100, 30, 20)}}
	svc, err := dungeon.NewService(newMockDungeonRepo(), chars, &corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store), dungeon.WithInventoryProvider(inventory))
	if err != nil {
		t.Fatal(err)
	}
	if run, err := svc.StartExpedition(ctx, "hero", dungeon.NativeMap0ID); run != nil || !errors.Is(err, failure) {
		t.Fatalf("partial start: %+v %v", run, err)
	}
	if run, err := store.GetActiveExpedition(ctx, "hero"); run != nil || err != nil {
		t.Fatalf("saved partial roster: %+v %v", run, err)
	}
}

func TestMapObservationRecoversMissingSavedInventory(t *testing.T) {
	ctx := context.Background()
	for _, origin := range []string{"old-json", "empty-snapshot", "start-without-provider", "start-with-provider"} {
		for _, ownsGoggles := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/goggles=%t", origin, ownsGoggles), func(t *testing.T) {
				store := dungeon.NewMemoryExpeditionRepository()
				hero := createTestChar("hero", 10, 100, 30, 20)
				hero.JobID = "job-01"
				chars := &mockCharRepo{chars: map[string]corecharacter.Character{"hero": hero}}
				inv := &mapInventoryReader{}
				if ownsGoggles {
					inv.inv.Items = []coreitem.Instance{{ID: "scope", DefinitionID: "item-197", Quantity: 1}}
				}
				run := dungeon.ActiveExpedition{ID: "run", CharacterID: "hero", DungeonID: dungeon.NativeMap0ID, MapVariant: 1, Status: dungeon.StatusExploring, TurnsRemaining: 20}
				switch origin {
				case "old-json":
					run.Members = dungeon.DecodeMembers(`[{"character_id":"hero","job_id":"job-01"},{"character_id":"ally","job_id":"job-79","item_ids":["item-197"]}]`)
				case "empty-snapshot":
					run.Members = []dungeon.ExpeditionMember{{CharacterID: "hero", JobID: "job-01", ItemIDs: []string{}}, {CharacterID: "ally", JobID: "job-79", ItemIDs: []string{"item-197"}}}
					encoded := dungeon.EncodeMembers(run.Members)
					if strings.Contains(encoded, `"item_ids":[]`) {
						t.Fatal("empty and missing inventory unexpectedly distinguishable")
					}
					run.Members = dungeon.DecodeMembers(encoded)
				default:
					opts := []dungeon.Option{dungeon.WithActiveExpeditionStore(store)}
					if origin == "start-with-provider" {
						opts = append(opts, dungeon.WithInventoryProvider(inv))
					}
					starter, err := dungeon.NewService(newMockDungeonRepo(), chars, &corebattle.Engine{}, opts...)
					if err != nil {
						t.Fatal(err)
					}
					started, err := starter.StartExpedition(ctx, "hero", dungeon.NativeMap0ID)
					if err != nil {
						t.Fatal(err)
					}
					run = *started
					run.Members = dungeon.DecodeMembers(dungeon.EncodeMembers(run.Members))
				}
				if err := store.SaveActiveExpedition(ctx, run); err != nil {
					t.Fatal(err)
				}
				svc, err := dungeon.NewService(newMockDungeonRepo(), chars, &corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store), dungeon.WithInventoryProvider(inv))
				if err != nil {
					t.Fatal(err)
				}
				inv.actors = nil
				got, err := svc.ObserveExpedition(ctx, "hero")
				radius := 1
				if ownsGoggles {
					radius++
				}
				if err != nil || got == nil || got.Map == nil || got.Map.Radius != radius {
					t.Fatalf("wrong actor radius: %+v %v want=%d", got, err, radius)
				}
				for _, actor := range inv.actors {
					if actor != "hero" {
						t.Fatalf("read another member inventory: %s", actor)
					}
				}
				failure := errors.New("inventory read failed")
				inv.err = failure
				if len(run.Members[0].ItemIDs) == 0 {
					if got, err := svc.ObserveExpedition(ctx, "hero"); got != nil || !errors.Is(err, failure) {
						t.Fatalf("partial map on failed enrichment: %+v %v", got, err)
					}
					noReader, err := dungeon.NewService(newMockDungeonRepo(), chars, &corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store))
					if err != nil {
						t.Fatal(err)
					}
					if got, err := noReader.ObserveExpedition(ctx, "hero"); got != nil || err == nil {
						t.Fatalf("missing reader fabricated map: %+v %v", got, err)
					}
				}
				saved, err := store.GetActiveExpedition(ctx, "hero")
				if err != nil || dungeon.EncodeMembers(saved.Members) != dungeon.EncodeMembers(run.Members) {
					t.Fatalf("observation rewrote saved inventory: %+v %v", saved, err)
				}
			})
		}
	}
}
