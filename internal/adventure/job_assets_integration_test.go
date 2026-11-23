package adventure_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

// assetCombat decorates the production RunCombat: it counts executed
// operations and can fail after their effects to prove rollback.
type assetCombat struct {
	adventure.RunCombat
	fail  atomic.Bool
	calls atomic.Int64
}

func (c *assetCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd runhost.Command) (adventure.RunActionEffects, error) {
	c.calls.Add(1)
	fx, err := c.RunCombat.ExecuteRunAction(ctx, r, cmd)
	if err == nil && c.fail.Load() {
		return fx, errors.New("asset operation failed after effects")
	}
	return fx, err
}

// assetRNG lands hits (zero draws evade) while job78's 1/5 scavenging and
// weighted purchase draws select their first entry.
type assetRNG struct{ normalRNG }

func (r *assetRNG) Float64() float64 { r.calls.Add(1); return .15 }
func (r *assetRNG) Intn(n int) int   { r.calls.Add(1); return int(.15 * float64(n)) }
func (r *assetRNG) IntN(n int) int   { return r.Intn(n) }

func installAssets(t *testing.T, f *normalFixture) *assetCombat {
	t.Helper()
	withAssetSession(t, f)
	c := &assetCombat{}
	f.cfg.Combat, f.routed, f.cfg.RNG = nil, true, &assetRNG{}
	f.decorate = func(base adventure.RunCombat) adventure.RunCombat { c.RunCombat = base; return c }
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
		r.Round, r.EncounterFloor, r.EncounterStageID = 1, 1, r.Stage.ID
		// The enemy cannot answer, so effects stay the operation's own.
		r.LastActions[npc.ID] = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
		r.Version++
		return f.store.SaveRun(ctx, *r, r.Version-1)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assetCommand(t *testing.T, f *normalFixture, run string, actor int, action, key string) runhost.Command {
	t.Helper()
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	c := f.actors[actor]
	cmd := runhost.Command{RunID: run, PlayerID: c.PlayerID, ActorID: c.ID, Version: r.Version, ID: key, Action: action, TargetID: "asset-npc"}
	// The requester knows the entry's job; item158 opens extended entries.
	r = editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
		a := runActorByID(r, c.ID)
		n, _ := strconv.Atoi(strings.Split(action, "-")[1])
		a.Jobs.JobID, a.Jobs.SP, a.MP = fmt.Sprintf("job-%02d", n), 250, max(a.MP, 200)
		a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-158")
	})
	f.clock.add(time.Hour)
	cmd.Version = r.Version
	return cmd
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
			calls, draws := c.calls.Load(), f.cfg.RNG.(*assetRNG).calls.Load()
			f.restart(t)
			again, err := f.svc.Command(context.Background(), cmd)
			if err != nil || !reflect.DeepEqual(first, again) || c.calls.Load() != calls || f.cfg.RNG.(*assetRNG).calls.Load() != draws {
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
			commands := []runhost.Command{assetCommand(t, f, run, 0, action, "requester-a"), assetCommand(t, f, run, 1, action, "requester-b")}
			commands[0].Version = commands[1].Version // Both race the same committed state.
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
	err = f.svc.WithJobAssets(context.Background(), r, cmd, func(*corebattle.JobAssets, *corecharacter.Character) error {
		t.Fatal("mutation outside Command")
		return nil
	})
	if !errors.Is(err, runhost.ErrForbidden) {
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
				grantHeld(t, f, "weapon-01")
				weapon := heldItems(t, f, 0)[0]
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
					assertAssets(t, f, cmd.ActorID, scenario.money, scenario.items+1)
					c.fail.Store(false)
					f.store.failWrite.Store(false)
					f.cfg.JobAssetRunner = originalRunner
					f.restart(t)
					first, err = f.svc.Command(context.Background(), cmd)
				}
				if err != nil {
					t.Fatal(err)
				}
				calls, draws := c.calls.Load(), f.cfg.RNG.(*assetRNG).calls.Load()
				f.restart(t)
				again, err := f.svc.Command(context.Background(), cmd)
				if err != nil || !reflect.DeepEqual(first, again) || c.calls.Load() != calls || f.cfg.RNG.(*assetRNG).calls.Load() != draws {
					t.Fatal("retry executed effects", first, again, err)
				}
				assertAssets(t, f, cmd.ActorID, scenario.wantMoney, scenario.wantN+1)
				if got := heldItems(t, f, 0); !slices.Contains(got, weapon) {
					t.Fatal("enhanced tool changed the weapon hand", got)
				}
				r, err := f.store.FindRun(context.Background(), run)
				if err != nil || *r.Battle.Actors[0].HeldToolID != scenario.wantHeld {
					t.Fatal(r, err)
				}
			})
		}
	}
}

// A stacked tool row leaves the hand unknown, so nothing may be charged or consumed.
func TestEnhancedToolSQLUnknownHandRejectsBeforeAssets(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		qty  int
	}{
		{"stacked-tool", "item-001", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNormalFixture(t, 1)
			installAssets(t, f)
			_, err := f.cfg.JobAssetRunner.ExecuteTransaction(context.Background(), economy.TransactionRequest{
				CharacterID: f.actors[0].ID,
				Grant:       economy.ResourceGrant{ItemDefinitionID: tc.id, ItemQuantity: tc.qty},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			entry := f.enter(t, "normal")
			run := entry.Command.RunID
			seedAssetEncounter(t, f, run)
			cmd := assetCommand(t, f, run, 0, "job-78-entry-0", "unknown-hand")
			if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			assertAssets(t, f, cmd.ActorID, 100, 1)
			inv, err := f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
			if err != nil || len(inv.Items) != 1 || inv.Items[0].Quantity != tc.qty {
				t.Fatal(inv, err)
			}
			if receipt, err := f.store.FindRunReceipt(context.Background(), run, cmd.ID); err != nil || receipt != nil {
				t.Fatal("rejection stored a receipt", receipt, err)
			}
		})
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
	if straight != reloaded || straight <= 5000 {
		t.Fatal(straight, reloaded)
	}
}
