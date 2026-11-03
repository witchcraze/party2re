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
	coreitem "github.com/witchcraze/party2re/internal/core/item"
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
	book := addTravelTool(t, f, 0, "item-150")
	orb := addTravelTool(t, f, 0, "item-070")
	run := f.enter(t, "normal").Command.RunID
	if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, book, `{"floor":10}`)); err != nil {
		t.Fatal(err)
	}
	f.send(t, run, 0, "kill", "")
	if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, orb, `{}`)); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil || !r.StageCleared || r.Stage.ID != "stage-19" || r.Floor != 1 {
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
			f.restart(t)
			orb := addTravelTool(t, f, 0, "item-070")
			key := addTravelTool(t, f, 0, "item-107")
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
				f.send(t, run, 1, "fall", "") // Fallen enrolled players also receive the source event.
			}
			for _, instance := range []coreitem.Instance{orb, key} {
				if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, instance, `{}`)); err != nil {
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
			if before.StageCleared || before.Floor != 10 {
				t.Fatal(before)
			}
			f.clock.add(3 * time.Second)
			a := f.actors[0]
			cmd := adventure.RunCommand{RunID: run, ID: "unseal-treasure", PlayerID: a.PlayerID, ActorID: a.ID, Version: before.Version, Action: "advance"}
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
			if err != nil || after.Floor != 11 || after.StageCleared || len(after.Boxes) == 0 {
				t.Fatal(after, err)
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
