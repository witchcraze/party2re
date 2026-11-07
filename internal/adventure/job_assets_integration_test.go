package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/testutil"
)

// This bounded consumer tests real intrinsic effects plus SQL/receipt atomicity;
// #1329 retains production action/tail composition and #1330 transport.
type assetCombat struct {
	*normalCombat
	f     *normalFixture
	npc   bool
	fail  atomic.Bool
	calls atomic.Int64
}

func (c *assetCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	c.calls.Add(1)
	err := c.f.svc.WithJobAssets(ctx, r, cmd, func(assets *corebattle.JobAssets) error {
		actor := cmd.ActorID
		if c.npc {
			actor = "asset-npc"
		}
		frame := corebattle.CallbackContext{TargetContext: corebattle.TargetContext{ActorID: actor, PartyIDs: slices.Clone(r.Roster), EnemyIDs: slices.Clone(r.Enemies)}, Arguments: []string{cmd.TargetID}, Assets: assets}
		if c.npc {
			frame.PartyIDs, frame.EnemyIDs = frame.EnemyIDs, frame.PartyIDs
		}
		operation := &corebattle.JobAssetContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame.TargetContext}, GuaranteedHit: true, Defeat: func(string) error { return corebattle.ErrUnsupportedEffect }}}
		var d skill.BattleDefinitions
		d.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{JobAssets: operation}, len(r.Roster))
		ops, err := d.InvocationAction(&r.Battle, &frame, []string{cmd.Action}, c.f.rng)
		if err != nil {
			return err
		}
		if err := r.Battle.ApplyInvocations(&frame, d.Invocations, ops, c.f.rng); err != nil {
			return err
		}
		if c.fail.Load() {
			return errors.New("asset operation failed after effects")
		}
		return nil
	})
	return adventure.RunActionEffects{RoomActivity: true}, err
}

func installAssets(t *testing.T, f *normalFixture) *assetCombat {
	t.Helper()
	runner, err := economy.NewService(f.chars, f.invs, economy.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}
	c := &assetCombat{normalCombat: f.combat, f: f}
	f.cfg.JobAssetRunner = runner
	f.cfg.JobAssetHelpers = func(context.Context) ([]int, error) { return []int{}, nil }
	f.cfg.Combat = c
	f.restart(t)
	return c
}

func seedAssetEncounter(t *testing.T, f *normalFixture, run string) {
	t.Helper()
	err := f.store.WithRunForUpdate(context.Background(), run, false, func(ctx context.Context) error {
		r, err := f.store.FindRun(ctx, run)
		if err != nil {
			return err
		}
		for i := range r.Battle.Actors {
			a := &r.Battle.Actors[i]
			a.HP, a.MaxHP, a.Attack, a.Agility = 5000, 10000, 100, 40
			a.Defense, a.BaseStats.Defense = 0, 0
			a.BaseStats.Agility = 40
		}
		npc := r.Battle.Actors[0].Clone()
		npc.ID, npc.TeamID, npc.NPC = "asset-npc", "enemy", true
		r.Battle.Actors = append(r.Battle.Actors, npc)
		r.Enemies = []string{npc.ID}
		r.Version++
		return f.store.SaveRun(ctx, *r, r.Version-1)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assetCommand(t *testing.T, f *normalFixture, run string, actor int, action, key string) adventure.RunCommand {
	t.Helper()
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	c := f.actors[actor]
	return adventure.RunCommand{RunID: run, PlayerID: c.PlayerID, ActorID: c.ID, Version: r.Version, ID: key, Action: action, TargetID: "asset-npc"}
}

func TestJobAssetsSQLRollbackReceiptAndRestart(t *testing.T) {
	for _, failure := range []string{"effect", "receipt", "receipt-item", "commit-unknown", "inventory"} {
		t.Run(failure, func(t *testing.T) {
			f := newNormalFixture(t, 1)
			c := installAssets(t, f)
			entry := f.enter(t, "normal")
			run := entry.Command.RunID
			seedAssetEncounter(t, f, run)
			action := "job-7-entry-2"
			if failure == "inventory" || failure == "receipt-item" {
				action = "job-78-entry-5"
			}
			cmd := assetCommand(t, f, run, 0, action, "asset-command")
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			originalRunner := f.cfg.JobAssetRunner
			switch failure {
			case "effect":
				c.fail.Store(true)
			case "receipt", "receipt-item":
				f.store.failWrite.Store(true)
			case "commit-unknown":
				f.store.loseCommit.Store(true)
			case "inventory":
				f.cfg.JobAssetRunner, err = economy.NewService(f.chars, &assetInventoryFailure{InventoryRepository: f.invs}, economy.WithTransactionProvider(database.NewTransactionProvider(f.db)))
				if err != nil {
					t.Fatal(err)
				}
				f.restart(t)
			}
			first, err := f.svc.Command(context.Background(), cmd)
			if failure != "commit-unknown" {
				if err == nil {
					t.Fatal("injected failure committed")
				}
				after, e := f.store.FindRun(context.Background(), run)
				if e != nil || !reflect.DeepEqual(before, after) {
					t.Fatal("run escaped rollback", after, e)
				}
				char, e := f.chars.FindByID(context.Background(), cmd.ActorID)
				if e != nil || char.Money != 100 {
					t.Fatal("wallet escaped rollback", char, e)
				}
				inv, e := f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
				if e != nil || len(inv.Items) != 0 {
					t.Fatal("item escaped rollback", inv, e)
				}
				c.fail.Store(false)
				f.store.failWrite.Store(false)
				f.cfg.JobAssetRunner = originalRunner
				f.restart(t)
				first, err = f.svc.Command(context.Background(), cmd)
			}
			if err != nil {
				t.Fatal(err)
			}
			calls, draws := c.calls.Load(), f.rng.calls.Load()
			f.restart(t)
			again, err := f.svc.Command(context.Background(), cmd)
			if err != nil || !reflect.DeepEqual(first, again) || c.calls.Load() != calls || f.rng.calls.Load() != draws {
				t.Fatal("retry executed effects", first, again, err)
			}
			char, err := f.chars.FindByID(context.Background(), cmd.ActorID)
			want := 108
			if action == "job-78-entry-5" {
				want = 100
			}
			if err != nil || char.Money != want {
				t.Fatal(char, err)
			}
			if action == "job-78-entry-5" {
				inv, err := f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
				r, e := f.store.FindRun(context.Background(), run)
				if err != nil || e != nil || len(inv.Items) != 1 || inv.Items[0].DefinitionID != "item-001" || *r.Battle.Actors[0].HeldToolID != "item-001" {
					t.Fatal(inv, r, err, e)
				}
			}
		})
	}
}

type assetInventoryFailure struct{ economy.InventoryRepository }

func (r *assetInventoryFailure) Save(ctx context.Context, inv coreinventory.Inventory) error {
	if err := r.InventoryRepository.Save(ctx, inv); err != nil {
		return err
	}
	return errors.New("inventory persistence acknowledgement unavailable")
}

func TestJobAssetsSQLPairedRetry(t *testing.T) {
	for _, action := range []string{"job-28-entry-3", "job-78-entry-0", "job-78-entry-5"} {
		t.Run(action, func(t *testing.T) {
			f := newNormalFixture(t, 2)
			c := installAssets(t, f)
			_, lobby := f.waitingParty(t, "normal")
			leader := f.actors[0]
			entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
			if err != nil {
				t.Fatal(err)
			}
			run := entry.Command.RunID
			seedAssetEncounter(t, f, run)
			c.npc = action == "job-28-entry-3"
			commands := []adventure.RunCommand{assetCommand(t, f, run, 0, action, "requester-a"), assetCommand(t, f, run, 1, action, "requester-b")}
			stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				_, err := f.svc.Command(context.Background(), commands[(worker+op)%2])
				if errors.Is(err, adventure.ErrRunConflict) {
					return nil
				}
				return err
			})
			if stats.Failures != 0 {
				t.Fatal(stats)
			}
			total, items := 0, 0
			for _, actor := range f.actors {
				char, err := f.chars.FindByID(context.Background(), actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				total += char.Money
				inv, err := f.invs.FindByCharacterID(context.Background(), actor.ID)
				if err != nil {
					t.Fatal(inv, err)
				}
				items += len(inv.Items)
			}
			r, err := f.store.FindRun(context.Background(), run)
			wantGold, wantItems := 100, 0
			if action == "job-78-entry-5" {
				wantGold, wantItems = 200, 1
			}
			if action == "job-78-entry-0" {
				wantGold, wantItems = 125, 1 // One winner pays floor(30×2.5).
			}
			if err != nil || r.Version != commands[0].Version+1 || total != wantGold || items != wantItems || c.calls.Load() != 1 {
				t.Fatal("paired commands duplicated assets", r, total, items, c.calls.Load(), err)
			}
		})
	}
}

func TestJobAssetsSQLCapAndCommandAuthority(t *testing.T) {
	f := newNormalFixture(t, 1)
	installAssets(t, f)
	_, err := f.cfg.JobAssetRunner.ExecuteTransaction(context.Background(), economy.TransactionRequest{CharacterID: f.actors[0].ID, Grant: economy.ResourceGrant{Gold: corecharacter.MaxMoney - 102}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	cmd := assetCommand(t, f, run, 0, "job-7-entry-2", "wallet-cap")
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	err = f.svc.WithJobAssets(context.Background(), r, cmd, func(*corebattle.JobAssets) error { t.Fatal("mutation outside Command"); return nil })
	if !errors.Is(err, adventure.ErrRunForbidden) {
		t.Fatal(err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	char, err := f.chars.FindByID(context.Background(), cmd.ActorID)
	if err != nil || char.Money != corecharacter.MaxMoney {
		t.Fatal(char, err)
	}
}

func TestJobAssetsSQLNPCScavengingPreservesDurableHand(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := installAssets(t, f)
	c.npc = true
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	cmd := assetCommand(t, f, run, 0, "job-78-entry-5", "npc-scavenging")
	cmd.TargetID = cmd.ActorID
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	npc := r.Battle.Actors[len(r.Battle.Actors)-1]
	if *npc.HeldToolID != "item-001" || npc.Agility != 50 || *r.Battle.Actors[0].HeldToolID != "" {
		t.Fatal(r)
	}
	inv, err := f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
	if err != nil || len(inv.Items) != 0 {
		t.Fatal(inv, err)
	}
}

func grantHeld(t *testing.T, f *normalFixture, id string) {
	t.Helper()
	_, err := f.cfg.JobAssetRunner.ExecuteTransaction(context.Background(), economy.TransactionRequest{CharacterID: f.actors[0].ID, Grant: economy.ResourceGrant{ItemDefinitionID: id, ItemQuantity: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func assertAssets(t *testing.T, f *normalFixture, characterID string, money, items int) {
	t.Helper()
	char, err := f.chars.FindByID(context.Background(), characterID)
	if err != nil || char.Money != money {
		t.Fatal("wallet", char.Money, money, err)
	}
	inv, err := f.invs.FindByCharacterID(context.Background(), characterID)
	if err != nil || len(inv.Items) != items {
		t.Fatal("inventory", inv, items, err)
	}
}

func TestEnhancedToolSQLRollbackReceiptAndRestart(t *testing.T) {
	const action = "job-78-entry-0"
	for _, scenario := range []struct {
		name             string
		held             string
		money, items     int
		wantHeld         string
		wantMoney, wantN int
	}{
		{"purchase", "", 100, 0, "item-001", 25, 1},
		{"use", "item-001", 100, 1, "", 100, 0},
	} {
		for _, failure := range []string{"effect", "receipt", "commit-unknown", "inventory"} {
			t.Run(scenario.name+"/"+failure, func(t *testing.T) {
				f := newNormalFixture(t, 1)
				c := installAssets(t, f)
				if scenario.held != "" {
					grantHeld(t, f, scenario.held)
				}
				entry := f.enter(t, "normal")
				run := entry.Command.RunID
				seedAssetEncounter(t, f, run)
				cmd := assetCommand(t, f, run, 0, action, "enhanced-command")
				before, err := f.store.FindRun(context.Background(), run)
				if err != nil {
					t.Fatal(err)
				}
				originalRunner := f.cfg.JobAssetRunner
				switch failure {
				case "effect":
					c.fail.Store(true)
				case "receipt":
					f.store.failWrite.Store(true)
				case "commit-unknown":
					f.store.loseCommit.Store(true)
				case "inventory":
					f.cfg.JobAssetRunner, err = economy.NewService(f.chars, &assetInventoryFailure{InventoryRepository: f.invs}, economy.WithTransactionProvider(database.NewTransactionProvider(f.db)))
					if err != nil {
						t.Fatal(err)
					}
					f.restart(t)
				}
				first, err := f.svc.Command(context.Background(), cmd)
				if failure != "commit-unknown" {
					if err == nil {
						t.Fatal("injected failure committed")
					}
					after, e := f.store.FindRun(context.Background(), run)
					if e != nil || !reflect.DeepEqual(before, after) {
						t.Fatal("run escaped rollback", after, e)
					}
					assertAssets(t, f, cmd.ActorID, scenario.money, scenario.items)
					c.fail.Store(false)
					f.store.failWrite.Store(false)
					f.cfg.JobAssetRunner = originalRunner
					f.restart(t)
					first, err = f.svc.Command(context.Background(), cmd)
				}
				if err != nil {
					t.Fatal(err)
				}
				calls, draws := c.calls.Load(), f.rng.calls.Load()
				f.restart(t)
				again, err := f.svc.Command(context.Background(), cmd)
				if err != nil || !reflect.DeepEqual(first, again) || c.calls.Load() != calls || f.rng.calls.Load() != draws {
					t.Fatal("retry executed effects", first, again, err)
				}
				assertAssets(t, f, cmd.ActorID, scenario.wantMoney, scenario.wantN)
				r, err := f.store.FindRun(context.Background(), run)
				if err != nil || *r.Battle.Actors[0].HeldToolID != scenario.wantHeld {
					t.Fatal(r, err)
				}
			})
		}
	}
}

// An NPC's tool lives only in combat; the commanding requester still pays.
func TestEnhancedToolSQLNPCActorChargesRequesterOnly(t *testing.T) {
	for _, tc := range []struct {
		name                string
		held                string
		wantNPC             string
		wantMoney, wantRows int
	}{
		{"purchase", "", "item-001", 25, 0},
		{"use", "item-001", "", 100, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNormalFixture(t, 1)
			c := installAssets(t, f)
			c.npc = true
			if tc.held != "" {
				grantHeld(t, f, tc.held)
			}
			entry := f.enter(t, "normal")
			run := entry.Command.RunID
			seedAssetEncounter(t, f, run)
			cmd := assetCommand(t, f, run, 0, "job-78-entry-0", "npc-enhanced")
			cmd.TargetID = cmd.ActorID
			if _, err := f.svc.Command(context.Background(), cmd); err != nil {
				t.Fatal(err)
			}
			assertAssets(t, f, cmd.ActorID, tc.wantMoney, tc.wantRows)
			r, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			if npc := r.Battle.Actors[len(r.Battle.Actors)-1]; *npc.HeldToolID != tc.wantNPC || *r.Battle.Actors[0].HeldToolID != tc.held {
				t.Fatal(*npc.HeldToolID, *r.Battle.Actors[0].HeldToolID)
			}
		})
	}
}

// A weapon row cannot certify the hand, so nothing may be charged or consumed.
func TestEnhancedToolSQLUnknownHandRejectsBeforeAssets(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := installAssets(t, f)
	grantHeld(t, f, "weapon-01")
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	cmd := assetCommand(t, f, run, 0, "job-78-entry-0", "unknown-hand")
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	assertAssets(t, f, cmd.ActorID, 100, 1)
	if c.fail.Load() || f.rng.calls.Load() != 0 {
		t.Fatal("rejection drew", f.rng.calls.Load())
	}
}

func TestEnhancedToolSQLHelperExclusionAndReload(t *testing.T) {
	t.Run("fully excluded pool charges nothing", func(t *testing.T) {
		f := newNormalFixture(t, 1)
		installAssets(t, f)
		f.cfg.JobAssetHelpers = func(context.Context) ([]int, error) {
			return []int{1, 2, 9, 11, 14, 15, 41, 80, 81, 84}, nil
		}
		f.restart(t)
		entry := f.enter(t, "normal")
		run := entry.Command.RunID
		seedAssetEncounter(t, f, run)
		cmd := assetCommand(t, f, run, 0, "job-78-entry-0", "excluded")
		if _, err := f.svc.Command(context.Background(), cmd); err != nil {
			t.Fatal(err)
		}
		assertAssets(t, f, cmd.ActorID, 100, 0)
	})
	// Buy then use across a process restart must match the same two commands
	// without one: the bought tool is durable and then consumed exactly once.
	sequence := func(t *testing.T, restart bool) int {
		f := newNormalFixture(t, 1)
		installAssets(t, f)
		entry := f.enter(t, "normal")
		run := entry.Command.RunID
		seedAssetEncounter(t, f, run)
		if _, err := f.svc.Command(context.Background(), assetCommand(t, f, run, 0, "job-78-entry-0", "buy")); err != nil {
			t.Fatal(err)
		}
		if restart {
			f.restart(t)
		}
		cmd := assetCommand(t, f, run, 0, "job-78-entry-0", "use")
		if _, err := f.svc.Command(context.Background(), cmd); err != nil {
			t.Fatal(err)
		}
		assertAssets(t, f, cmd.ActorID, 25, 0)
		r, err := f.store.FindRun(context.Background(), run)
		if err != nil || *r.Battle.Actors[0].HeldToolID != "" {
			t.Fatal(r, err)
		}
		return r.Battle.Actors[0].HP
	}
	straight, reloaded := sequence(t, false), sequence(t, true)
	if straight != reloaded || straight != 5108 {
		t.Fatal(straight, reloaded)
	}
}
