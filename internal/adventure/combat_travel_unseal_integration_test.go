package adventure_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type unsealSettlement struct {
	adventure.PostBattleSettler
	last                atomic.Bool
	manufacturedVictory atomic.Bool
}

func (s *unsealSettlement) ApplyPostBattleResult(ctx context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	s.last.Store(req.UnsealDemonKing)
	if req.UnsealDemonKing && req.BattleResult.Outcome == corebattle.OutcomeWin {
		s.manufacturedVictory.Store(true)
	}
	return s.PostBattleSettler.ApplyPostBattleResult(ctx, req)
}

func TestCombatTravelUnsealNotOnAssignmentOrExit(t *testing.T) {
	f := newNormalFixture(t, 1)
	installTravel(t, f)
	spy := &unsealSettlement{PostBattleSettler: f.cfg.Settler}
	f.cfg.Settler = spy
	f.restart(t)
	addTravelTool(t, f, 0, "item-150")
	run := f.enter(t, "normal").Command.RunID
	if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, `{"floor":10}`)); err != nil {
		t.Fatal(err)
	}
	f.kill(t, run, 0)
	addTravelTool(t, f, 0, "item-070")
	if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, `{}`)); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil || !r.StageCleared || r.Stage.ID != "stage-19" || r.Round != 1 {
		t.Fatal(r, err)
	}
	f.send(t, run, 0, "exit", "")
	c, err := f.chars.FindByID(context.Background(), f.actors[0].ID)
	if err != nil || c.MaoCount != 0 || spy.last.Load() {
		t.Fatal("coordinate travel/exit emitted an unseal event", c.MaoCount, spy.last.Load(), err)
	}
}

func TestCombatTravelUnsealTreasureReceiptSQL(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(map[int]string{1: "solo-runner", 2: "party-provider"}[count], func(t *testing.T) {
			f := newNormalFixture(t, count)
			installTravel(t, f)
			spy := &unsealSettlement{PostBattleSettler: f.cfg.Settler}
			f.cfg.Settler = spy
			var opened atomic.Int32
			f.cfg.Unsealed = func(context.Context, string, []corebattle.LifetimeActor) { opened.Add(1) }
			f.restart(t)
			run := ""
			if count == 1 {
				run = f.enter(t, "normal").Command.RunID
			} else {
				_, lobby := f.waitingParty(t, "normal")
				a := f.actors[0]
				entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
				if err != nil {
					t.Fatal(err)
				}
				run = entry.Command.RunID
				f.stage(t, run, fall(f.actors[1].ID)) // Fallen enrolled players also receive the source event.
			}
			for _, definition := range []string{"item-070", "item-107"} {
				addTravelTool(t, f, 0, definition)
				if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, `{}`)); err != nil {
					t.Fatal(err)
				}
				if spy.last.Load() {
					t.Fatal("coordinate tool emitted unseal")
				}
			}
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			if before.StageCleared || before.Round != 10 {
				t.Fatal(before)
			}
			f.clock.add(3 * time.Second)
			a := f.actors[0]
			cmd := runhost.Command{RunID: run, ID: "unseal-treasure", PlayerID: a.PlayerID, ActorID: a.ID, Version: before.Version, Action: "advance"}
			f.store.failWrite.Store(true)
			if _, err := f.svc.Command(context.Background(), cmd); err == nil {
				t.Fatal("SQL failure accepted")
			}
			after, err := f.store.FindRun(context.Background(), run)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("failed unseal leaked run changes", err)
			}
			for _, member := range f.actors {
				c, err := f.chars.FindByID(context.Background(), member.ID)
				if err != nil || c.MaoCount != 0 {
					t.Fatal("failed receipt leaked counter", c.MaoCount, err)
				}
			}
			f.store.failWrite.Store(false)
			f.store.loseCommit.Store(true)
			stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error {
				_, err := f.svc.Command(context.Background(), cmd)
				return err
			})
			if stats.Failures != 0 {
				t.Fatal(stats)
			}
			if !spy.last.Load() || spy.manufacturedVictory.Load() {
				t.Fatal("treasure event was absent or fabricated a battle victory")
			}
			first, err := f.svc.Command(context.Background(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			retry, err := f.svc.Command(context.Background(), cmd)
			if err != nil || !reflect.DeepEqual(first, retry) {
				t.Fatal("restart changed the unseal receipt", retry, err)
			}
			after, err = f.store.FindRun(context.Background(), run)
			if err != nil || after.Round != 11 || after.StageCleared || len(after.Boxes) == 0 {
				t.Fatal(after, err)
			}
			// Only the attempt that executed with a confirmed commit opens rooms; a
			// lost acknowledgement or a receipt replay never does.
			if opened.Load() > 1 {
				t.Fatal("concurrent retries opened sealing rooms more than once", opened.Load())
			}
			for _, member := range f.actors {
				c, err := f.chars.FindByID(context.Background(), member.ID)
				if err != nil || c.MaoCount != 1 {
					t.Fatal("treasure event not applied exactly once", c.MaoCount, err)
				}
			}
			f.send(t, run, 0, "chest", after.Boxes[0].ID)
			for i, member := range f.actors {
				f.send(t, run, i, "exit", "")
				c, err := f.chars.FindByID(context.Background(), member.ID)
				if err != nil || c.MaoCount != 1 || spy.last.Load() {
					t.Fatal("chest/exit repeated unseal", c.MaoCount, err)
				}
			}
		})
	}
}

type unsealCall struct {
	name      string
	unsealers []corebattle.LifetimeActor
}

func TestNormalRunUnsealOpensRoomsOnceAfterCommit(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(map[int]string{1: "solo", 2: "party"}[count], func(t *testing.T) {
			f := newNormalFixture(t, count)
			installTravel(t, f)
			var calls []unsealCall
			f.cfg.Unsealed = func(_ context.Context, name string, unsealers []corebattle.LifetimeActor) {
				calls = append(calls, unsealCall{name, unsealers})
			}
			f.restart(t)
			run, wantName := "", ""
			if count == 1 {
				run, wantName = f.enter(t, "normal").Command.RunID, "solo-quest"
			} else {
				_, lobby := f.waitingParty(t, "normal")
				a := f.actors[0]
				entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
				if err != nil {
					t.Fatal(err)
				}
				run, wantName = entry.Command.RunID, "normal-run"
			}
			for _, definition := range []string{"item-070", "item-107"} {
				addTravelTool(t, f, 0, definition)
				if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, `{}`)); err != nil {
					t.Fatal(err)
				}
			}
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			f.clock.add(3 * time.Second)
			a := f.actors[0]
			cmd := runhost.Command{RunID: run, ID: "unseal-once", PlayerID: a.PlayerID, ActorID: a.ID, Version: before.Version, Action: "advance"}
			if _, err := f.svc.Command(context.Background(), cmd); err != nil {
				t.Fatal(err)
			}
			if len(calls) != 1 || calls[0].name != wantName || len(calls[0].unsealers) != count || calls[0].unsealers[0].ID != a.ID {
				t.Fatalf("expected one post-commit unseal with the run name and the party, leader first: %+v", calls)
			}
			c, err := f.chars.FindByID(context.Background(), a.ID)
			if err != nil {
				t.Fatal(err)
			}
			if exp, gold := c.BattleRowRewards(); calls[0].unsealers[0].ExperienceReward != exp || calls[0].unsealers[0].GoldReward != gold {
				t.Fatalf("unsealer lacks its battle-row rewards: %+v", calls[0].unsealers[0].Participant)
			}
			if _, err := f.svc.Command(context.Background(), cmd); err != nil || len(calls) != 1 {
				t.Fatal("receipt replay reopened rooms", len(calls), err)
			}
			after, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			f.send(t, run, 0, "chest", after.Boxes[0].ID)
			for i := range f.actors {
				f.send(t, run, i, "exit", "")
			}
			if len(calls) != 1 {
				t.Fatal("chest or exit reopened rooms", len(calls))
			}
		})
	}
}
