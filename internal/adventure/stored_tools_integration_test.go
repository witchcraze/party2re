package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

const storedAction = "job-50-entry-7"

type storedDepots struct {
	*database.DepotRepository
	fail bool
}

func (d *storedDepots) Save(ctx context.Context, value depot.Depot) error {
	if d.fail {
		return errors.New("depot write unavailable")
	}
	return d.DepotRepository.Save(ctx, value)
}

func installStored(t *testing.T, f *normalFixture) (*assetCombat, *storedDepots) {
	t.Helper()
	c := installAssets(t, f)
	repo, err := database.NewDepotRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	depots := &storedDepots{DepotRepository: repo}
	c.extra = func(d *skill.BattleDefinitions, op *corebattle.JobAssetContext) {
		d.RegisterOrdinaryTools(&corebattle.OrdinaryToolContext{DamageContext: op.DamageContext})
	}
	f.cfg.JobAssetDepots = depots
	f.restart(t)
	return c, depots
}

func seedDepot(t *testing.T, depots *storedDepots, owner string, definitions ...string) []item.Instance {
	t.Helper()
	value := depot.Depot{CharacterID: owner, Capacity: depot.MinDepotCapacity}
	for _, definition := range definitions {
		value.Items = append(value.Items, item.Instance{ID: id.New(), DefinitionID: definition, Quantity: 1})
	}
	if err := depots.DepotRepository.Save(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	return value.Items
}

func depotItems(t *testing.T, depots *storedDepots, owner string) []item.Instance {
	t.Helper()
	value, err := depots.FindByCharacterID(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	return value.Items
}

func storedCommand(t *testing.T, f *normalFixture, run string, actor int, selector, key string) adventure.RunCommand {
	cmd := assetCommand(t, f, run, actor, storedAction, key)
	cmd.TargetID, cmd.Input = selector, " >"+cmd.ActorID+" "
	return cmd
}

func storedHistory(t *testing.T, f *normalFixture, run string) map[string]map[string]bool {
	t.Helper()
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	return r.Battle.StoredToolHistory
}

func TestStoredToolSQLConsumptionRollbackAndRetry(t *testing.T) {
	f := newNormalFixture(t, 1)
	c, depots := installStored(t, f)
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	owner := f.actors[0].ID
	rows := seedDepot(t, depots, owner, "item-001", "item-001")
	cmd := storedCommand(t, f, run, 0, "item-001", "stored-herb")
	before, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}

	depots.fail = true
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("depot failure committed")
	}
	after, err := f.store.FindRun(context.Background(), run)
	if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(depotItems(t, depots, owner), rows) {
		t.Fatal("storage fault escaped rollback", after.Battle.StoredToolHistory, err)
	}

	depots.fail = false
	first, err := f.svc.Command(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	color := before.Battle.Actors[0].TeamID
	if got := depotItems(t, depots, owner); !reflect.DeepEqual(got, rows[1:]) || !storedHistory(t, f, run)[color]["item-001"] {
		t.Fatal("first copy not consumed", got, storedHistory(t, f, run))
	}
	calls := c.calls.Load()
	f.restart(t)
	again, err := f.svc.Command(context.Background(), cmd)
	if err != nil || !reflect.DeepEqual(first, again) || c.calls.Load() != calls || len(depotItems(t, depots, owner)) != 1 {
		t.Fatal("retry replayed the child", err, c.calls.Load(), calls)
	}

	// The remaining copy stays excluded for this color: no match, no asset write.
	next := storedCommand(t, f, run, 0, "item-001", "stored-herb-again")
	if _, err := f.svc.Command(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if got := depotItems(t, depots, owner); !reflect.DeepEqual(got, rows[1:]) {
		t.Fatal("used definition reselected", got)
	}
}

func TestStoredToolSQLExchangeKeepsInstances(t *testing.T) {
	definition, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	book, err := definition.FindByID("item-018")
	if err != nil || !book.IsTool() || book.UsageCategory == item.UsageCategoryCombatOnly {
		t.Fatal("fixture needs a non-category1 tool", book, err)
	}
	for _, held := range []string{"", "item-003"} {
		t.Run("held="+held, func(t *testing.T) {
			f := newNormalFixture(t, 1)
			_, depots := installStored(t, f)
			if held != "" {
				grantHeld(t, f, held)
			}
			entry := f.enter(t, "normal")
			run := entry.Command.RunID
			seedAssetEncounter(t, f, run)
			owner := f.actors[0].ID
			rows := seedDepot(t, depots, owner, "item-001", "item-018", "item-002")
			inv, err := f.invs.FindByCharacterID(context.Background(), owner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.svc.Command(context.Background(), storedCommand(t, f, run, 0, "item-018", "stored-exchange")); err != nil {
				t.Fatal(err)
			}
			got, err := f.invs.FindByCharacterID(context.Background(), owner)
			if err != nil || len(got.Items) != 1 || got.Items[0] != rows[1] {
				t.Fatal("selected instance not equipped", got, err)
			}
			want := []item.Instance{rows[0], rows[2]}
			if held != "" {
				want = []item.Instance{rows[0], inv.Items[0], rows[2]}
			}
			r, err := f.store.FindRun(context.Background(), run)
			if err != nil || !reflect.DeepEqual(depotItems(t, depots, owner), want) || *r.Battle.Actors[0].HeldToolID != "item-018" {
				t.Fatal("held tool not stored at the source row", depotItems(t, depots, owner), want, err)
			}
		})
	}
}

func TestStoredToolSQLNoMatchWritesNothing(t *testing.T) {
	f := newNormalFixture(t, 1)
	_, depots := installStored(t, f)
	entry := f.enter(t, "normal")
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	owner := f.actors[0].ID
	rows := seedDepot(t, depots, owner, "item-001")
	if _, err := f.svc.Command(context.Background(), storedCommand(t, f, run, 0, "item-002", "stored-miss")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(depotItems(t, depots, owner), rows) || len(storedHistory(t, f, run)) != 0 {
		t.Fatal("no match changed depot or history")
	}
	assertAssets(t, f, owner, 100, 0)
}

// Two requesters race on the same run version; exactly one consumes its own
// stored instance and the loser's depot is unchanged.
func TestStoredToolSQLPairedCommandsConserveItems(t *testing.T) {
	f := newNormalFixture(t, 2)
	c, depots := installStored(t, f)
	_, lobby := f.waitingParty(t, "normal")
	leader := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	run := entry.Command.RunID
	seedAssetEncounter(t, f, run)
	for _, actor := range f.actors {
		seedDepot(t, depots, actor.ID, "item-001")
	}
	commands := []adventure.RunCommand{storedCommand(t, f, run, 0, "item-001", "stored-a"), storedCommand(t, f, run, 1, "item-001", "stored-b")}
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
	remaining := 0
	for _, actor := range f.actors {
		remaining += len(depotItems(t, depots, actor.ID))
	}
	history := storedHistory(t, f, run)
	if remaining != 1 || c.calls.Load() != 1 || len(history) != 1 {
		t.Fatal("paired commands duplicated stored use", remaining, c.calls.Load(), history)
	}
}
