package challenge_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

// receiptFailure fails the command's last write, rolling back its transaction.
type receiptFailure struct {
	runhost.Store[challenge.Run]
	fail bool
}

func (s *receiptFailure) SaveRunReceipt(ctx context.Context, r runhost.Receipt) error {
	if s.fail {
		return errors.New("receipt write failure")
	}
	return s.Store.SaveRunReceipt(ctx, r)
}

// owned counts a definition across a character's hand and depot.
func (f *runFixture) owned(t *testing.T, i int, def string) int {
	t.Helper()
	var n int
	err := f.db.QueryRowContext(context.Background(), `SELECT
		(SELECT COALESCE(SUM(quantity),0) FROM inventory_items WHERE character_id = ? AND definition_id = ?) +
		(SELECT COALESCE(SUM(quantity),0) FROM depot_items WHERE character_id = ? AND definition_id = ?)`,
		f.actors[i].ID, def, f.actors[i].ID, def).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// shiraberu on a Challenge treasure wave (vs_challenge.cgi:20–30,64–66;
// _npc_action.cgi:6–84): one box per member, an extra one per consumed
// item 215, empty boxes yield nothing, and the taken-treasure flag stops
// later treasure for the member who advances.
func TestRunTreasureWaveIntegration(t *testing.T) {
	f := newRunFixture(t, 2)
	ctx := context.Background()
	tier := f.cfg.Tiers[f.tier]
	tier.TreasureRound, tier.TreasureItemPool = 1, []string{"item-004"}
	f.cfg.Tiers[f.tier] = tier
	store := &receiptFailure{Store: f.store}
	f.cfg.Store = store
	f.restart(t)

	runID := f.enterParty(t)
	f.send(t, runID, 0, "advance", "")
	r := f.run(t, runID)
	if r.Round != 1 || len(r.Enemies) != 0 || len(r.Boxes) != 2 {
		t.Fatalf("treasure wave: round %d enemies %v boxes %+v", r.Round, r.Enemies, r.Boxes)
	}
	f.stage(t, runID, func(r *challenge.Run) {
		r.Boxes = append(r.Boxes, runhost.TreasureBox{ID: "box-3", Name: "丸い宝箱C"})
	})
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 0, "advance", ""); !errors.Is(err, challenge.ErrEnemiesRemain) {
		t.Fatal("advanced past unopened boxes", err)
	}

	opened := f.send(t, runID, 0, "chest", "box-1")
	f.restart(t)
	if receipt, err := f.svc.Command(ctx, opened); err != nil || receipt.Box == nil || receipt.Box.ItemID != "item-004" || f.owned(t, 0, "item-004") != 1 {
		t.Fatal("replayed chest", receipt, err, f.owned(t, 0, "item-004"))
	}
	f.clock.add(3 * time.Second)
	before := *f.run(t, runID)
	if _, err := f.command(t, runID, 0, "chest", "box-2"); !errors.Is(err, runhost.ErrTreasureLimitReached) {
		t.Fatal("second box without item 215", err)
	}
	if after := f.run(t, runID); !reflect.DeepEqual(before, *after) {
		t.Fatal("rejected chest changed the run")
	}

	key := id.New()
	if _, err := f.db.ExecContext(ctx, `INSERT INTO inventory_items (id,character_id,definition_id,quantity,enhancement_level) VALUES (?,?,?,1,0)`, key, f.actors[0].ID, runhost.ItemTreasurePocket); err != nil {
		t.Fatal(err)
	}
	store.fail = true
	if _, err := f.command(t, runID, 0, "chest", "box-2"); err == nil {
		t.Fatal("receipt failure accepted")
	}
	if f.owned(t, 0, runhost.ItemTreasurePocket) != 1 || f.owned(t, 0, "item-004") != 1 || !reflect.DeepEqual(before, *f.run(t, runID)) {
		t.Fatal("failed chest committed")
	}
	store.fail = false
	extra, err := f.command(t, runID, 0, "chest", "box-2")
	if err != nil || f.owned(t, 0, runhost.ItemTreasurePocket) != 0 || f.owned(t, 0, "item-004") != 2 {
		t.Fatal("extra box", err, f.owned(t, 0, runhost.ItemTreasurePocket), f.owned(t, 0, "item-004"))
	}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error { _, err := f.svc.Command(ctx, extra); return err })
	if stats.Failures != 0 || f.owned(t, 0, "item-004") != 2 {
		t.Fatal("replayed extra box", stats, f.owned(t, 0, "item-004"))
	}

	empty := f.send(t, runID, 1, "chest", "box-3")
	if receipt, err := f.svc.Command(ctx, empty); err != nil || receipt.Box == nil || receipt.Box.ItemID != "" || f.owned(t, 1, "item-004") != 0 {
		t.Fatal("empty box", receipt, err)
	}

	// Both members hold the flag, so the zero RNG's treasure roll is never drawn.
	f.send(t, runID, 1, "advance", "")
	if r := f.run(t, runID); r.Round != 2 || len(r.Boxes) != 0 || len(r.Enemies) != 1 {
		t.Fatalf("flagged advance: round %d boxes %+v enemies %v", r.Round, r.Boxes, r.Enemies)
	}
}

// Members racing for the same box: exactly one opens it, without deadlock.
func TestRunTreasureBoxRaceIntegration(t *testing.T) {
	f := newRunFixture(t, 2)
	tier := f.cfg.Tiers[f.tier]
	tier.TreasureRound, tier.TreasureItemPool = 1, []string{"item-004"}
	f.cfg.Tiers[f.tier] = tier
	f.restart(t)
	runID := f.enterParty(t)
	f.send(t, runID, 0, "advance", "")
	f.clock.add(3 * time.Second)
	version := f.run(t, runID).Version
	open := func(i int) func() error {
		c := f.actors[i]
		cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: version, Action: "chest", TargetID: "box-1"}
		return func() error { _, err := f.svc.Command(context.Background(), cmd); return err }
	}
	err0, err1 := testutil.RunRace2(open(0), open(1))
	if (err0 == nil) == (err1 == nil) || f.owned(t, 0, "item-004")+f.owned(t, 1, "item-004") != 1 {
		t.Fatal("box race", err0, err1)
	}
}
