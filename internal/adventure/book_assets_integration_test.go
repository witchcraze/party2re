package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

func grantHeldTo(t *testing.T, f *normalFixture, actor int, id string) {
	t.Helper()
	_, err := f.cfg.JobAssetRunner.ExecuteTransaction(context.Background(), economy.TransactionRequest{CharacterID: f.actors[actor].ID, Grant: economy.ResourceGrant{ItemDefinitionID: id, ItemQuantity: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func heldItems(t *testing.T, f *normalFixture, actor int) []item.Instance {
	t.Helper()
	inv, err := f.invs.FindByCharacterID(context.Background(), f.actors[actor].ID)
	if err != nil {
		t.Fatal(err)
	}
	return inv.Items
}

// A full depot swaps the requested first book row in place; no slot is added.
func TestBookSQLWithdrawalSwapsInPlaceAtCapacity(t *testing.T) {
	f := newNormalFixture(t, 1)
	_, depots := installStored(t, f)
	grantHeldTo(t, f, 0, "item-169")
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	owner := f.actors[0].ID
	rows := seedDepot(t, depots, owner, "item-001", "item-170", "item-170", "item-002", "item-003")
	held := heldItems(t, f, 0)
	cmd := assetCommand(t, f, run, 0, "job-86-entry-0", "book-withdraw")
	cmd.TargetID = "item-170"
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	want := []item.Instance{rows[0], held[0], rows[2], rows[3], rows[4]}
	if got := depotItems(t, depots, owner); !reflect.DeepEqual(got, want) {
		t.Fatal("held book not swapped into the first match", got, want)
	}
	r, err := f.store.FindRun(context.Background(), run)
	if got := heldItems(t, f, 0); err != nil || len(got) != 1 || got[0] != rows[1] || *r.Battle.Actors[0].HeldToolID != "item-170" {
		t.Fatal("withdrawn instance not held", got, err)
	}
}

// Return appends the held instance over capacity; a depot write failure rolls
// everything back, and the committed receipt never returns twice.
func TestBookSQLReturnOverCapacityRollbackAndRetry(t *testing.T) {
	f := newNormalFixture(t, 1)
	c, depots := installStored(t, f)
	grantHeldTo(t, f, 0, "item-170")
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	owner := f.actors[0].ID
	rows := seedDepot(t, depots, owner, "item-001", "item-002", "item-003", "item-004", "item-005")
	held := heldItems(t, f, 0)
	cmd := assetCommand(t, f, run, 0, "job-86-entry-2", "book-return")
	before, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}

	depots.fail = true
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("depot failure committed")
	}
	after, err := f.store.FindRun(context.Background(), run)
	if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(depotItems(t, depots, owner), rows) || !reflect.DeepEqual(heldItems(t, f, 0), held) {
		t.Fatal("storage fault escaped rollback", err)
	}

	depots.fail = false
	first, err := f.svc.Command(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]item.Instance{}, rows...), held[0])
	if got := depotItems(t, depots, owner); !reflect.DeepEqual(got, want) || len(heldItems(t, f, 0)) != 0 {
		t.Fatal("return not delivered over capacity", got)
	}
	calls := c.calls.Load()
	f.restart(t)
	again, err := f.svc.Command(context.Background(), cmd)
	if err != nil || !reflect.DeepEqual(first, again) || c.calls.Load() != calls || len(depotItems(t, depots, owner)) != len(want) {
		t.Fatal("retry replayed the return", err, c.calls.Load(), calls)
	}
}

// Two requesters race on one run version; exactly one returns its own book and
// every instance stays with its owner.
func TestBookSQLPairedReturnsConserveInstances(t *testing.T) {
	f := newNormalFixture(t, 2)
	c, depots := installStored(t, f)
	for actor := range f.actors {
		grantHeldTo(t, f, actor, "item-170")
	}
	_, lobby := f.waitingParty(t, "normal")
	leader := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	for _, actor := range f.actors {
		seedDepot(t, depots, actor.ID)
	}
	commands := []runhost.Command{assetCommand(t, f, run, 0, "job-86-entry-2", "return-a"), assetCommand(t, f, run, 1, "job-86-entry-2", "return-b")}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		_, err := f.svc.Command(context.Background(), commands[(worker+op)%2])
		if errors.Is(err, runhost.ErrConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	returned := 0
	for actor := range f.actors {
		inDepot, inHand := len(depotItems(t, depots, f.actors[actor].ID)), len(heldItems(t, f, actor))
		if inDepot+inHand != 1 {
			t.Fatal("instance lost or duplicated", actor, inDepot, inHand)
		}
		returned += inDepot
	}
	if returned != 1 || c.calls.Load() != 1 {
		t.Fatal("paired commands duplicated a return", returned, c.calls.Load())
	}
}

// Execution-time rejection after job-86-entry-0 restores retained in-memory
// hands/assets and rolls back SQL inventory, depot, run and receipt.
func TestBookSQLWithdrawalRejectionRestoresCallerReferentsAndStorage(t *testing.T) {
	f := newNormalFixture(t, 1)
	_, depots := installStored(t, f)
	grantHeldTo(t, f, 0, "item-169")
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	owner := f.actors[0].ID
	rows := seedDepot(t, depots, owner, "item-001", "item-170")
	held := heldItems(t, f, 0)

	var retainedAssets *corebattle.JobAssets
	var retainedAssetsHeld, retainedActorHeld *string
	// A synthetic rejecting slot: no reachable production pair rejects after a withdrawal.
	f.routed, f.decorate = false, nil
	f.cfg.Combat = &bookRejectCombat{normalCombat: f.combat, f: f, onCapture: func(assets *corebattle.JobAssets, actorHeld *string) {
		retainedAssets, retainedAssetsHeld, retainedActorHeld = assets, assets.HeldToolID, actorHeld
	}}
	f.restart(t)

	cmd := assetCommand(t, f, run, 0, "job-86-entry-0", "book-withdraw-reject")
	cmd.TargetID = "item-170"
	beforeRun, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatalf("err = %v, want ErrUnsupportedEffect", err)
	}
	if retainedAssets == nil || *retainedAssetsHeld != "item-169" || *retainedActorHeld != "item-169" || len(retainedAssets.StoredOutcomes) != 0 {
		t.Fatalf("caller referents leaked: assets=%+v assetsHeld=%q actorHeld=%q", retainedAssets, *retainedAssetsHeld, *retainedActorHeld)
	}
	afterRun, err := f.store.FindRun(context.Background(), run)
	if err != nil || !reflect.DeepEqual(beforeRun, afterRun) || !reflect.DeepEqual(depotItems(t, depots, owner), rows) || !reflect.DeepEqual(heldItems(t, f, 0), held) {
		t.Fatalf("SQL state mutated on rejection: err=%v depot=%+v held=%+v", err, depotItems(t, depots, owner), heldItems(t, f, 0))
	}
	if receipt, err := f.store.FindRunReceipt(context.Background(), run, cmd.ID); err != nil || receipt != nil {
		t.Fatalf("unexpected receipt %+v err=%v", receipt, err)
	}
}

type bookRejectCombat struct {
	*normalCombat
	f         *normalFixture
	onCapture func(*corebattle.JobAssets, *string)
}

func (c *bookRejectCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd runhost.Command) (adventure.RunActionEffects, error) {
	if cmd.Action != "job-86-entry-0" {
		return c.normalCombat.ExecuteRunAction(ctx, r, cmd)
	}
	err := c.f.svc.WithJobAssets(ctx, r, cmd, func(assets *corebattle.JobAssets, _ *corecharacter.Character) error {
		c.onCapture(assets, r.Battle.Actors[0].HeldToolID)
		frame := corebattle.CallbackContext{
			TargetContext: corebattle.TargetContext{ActorID: cmd.ActorID, PartyIDs: slices.Clone(r.Roster), EnemyIDs: slices.Clone(r.Enemies)},
			Arguments:     []string{cmd.TargetID},
			Assets:        assets,
		}
		operation := &corebattle.JobAssetContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame.TargetContext}, GuaranteedHit: true}}
		var d skill.BattleDefinitions
		d.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{JobAssets: operation}, len(r.Roster))
		d.Invocations.Entries["reject-if-item-170"] = corebattle.CallbackDefinition{
			Validate: func(state *corebattle.LifetimeState, _ *corebattle.CallbackContext) error {
				if state.Actors[0].HeldToolID != nil && *state.Actors[0].HeldToolID == "item-170" {
					return corebattle.ErrUnsupportedEffect
				}
				return nil
			},
			Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil },
		}
		ops, err := d.InvocationAction(&r.Battle, &frame, []string{"job-86-entry-0", "reject-if-item-170"}, c.f.rng)
		if err != nil {
			return err
		}
		return r.Battle.ApplyInvocations(&frame, d.Invocations, ops, c.f.rng)
	})
	return adventure.RunActionEffects{RoomActivity: true}, err
}
