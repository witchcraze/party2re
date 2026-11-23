package adventure_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestNormalRunPairedChestClaim(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	a := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.stage(t, runID, travel10)
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) == 0 {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		c := f.actors[(worker+op)%2]
		_, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "claim-" + c.ID, Version: r.Version, Action: "chest", TargetID: r.Boxes[0].ID})
		if errors.Is(err, runhost.ErrConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	total, inspections := 0, 0
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range f.actors {
		inv, err := f.invs.FindByCharacterID(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := f.depots.FindByCharacterID(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range append(inv.Items, dep.Items...) {
			total += item.Quantity
		}
		inspections += r.Recipients[c.ID].Inspected
	}
	if total != 1 || inspections != 1 || r.Boxes[0].OpenedBy == "" {
		t.Fatal("competing claims duplicated a chest", total, inspections)
	}
}

func TestNormalRunTreasureCombatInspectionClaimAndReload(t *testing.T) {
	for _, lethal := range []bool{false, true} {
		name := "damaged-then-claim"
		if lethal {
			name = "defeated-no-grant"
		}
		t.Run(name, func(t *testing.T) {
			f := newNormalFixture(t, 2)
			_, lobby := f.waitingParty(t, "normal")
			a := f.actors[0]
			entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
			if err != nil {
				t.Fatal(err)
			}
			runID := entry.Command.RunID
			f.stage(t, runID, travel10)
			f.send(t, runID, 0, "advance", "")

			var unopenedID, unopenedName, openedID string
			f.stage(t, runID, func(r *adventure.NormalRun) {
				if len(r.Boxes) < 2 {
					t.Fatal(r.Boxes)
				}
				r.Boxes[0].ItemID = "item-001"
				r.Boxes[1].OpenedBy = f.actors[1].ID
				r.Boxes[1].ItemID = "item-002"
				runActorByID(r, a.ID).Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 50}
				unopenedID, unopenedName, openedID = r.Boxes[0].ID, r.Boxes[0].Name, r.Boxes[1].ID
			})

			// Valuation by legacy box name runs as an explicit operation and opens nothing.
			value := exCommand(t, f, runID, id.New(), "")
			value.Input = "＠トレハンのかん>" + unopenedName
			if _, err := f.svc.Command(context.Background(), value); err != nil {
				t.Fatal("valuation", err)
			}
			if r := readStockRoom(t, f, runID); !r.Boxes[0].IsUnopened() || r.Boxes[0].Combat != nil || r.Version != value.Version+1 {
				t.Fatalf("valuation changed the box: %+v", r.Boxes[0])
			}

			if !lethal {
				// A box an earlier strike damaged stays claimable after reload.
				f.stage(t, runID, func(r *adventure.NormalRun) {
					damaged := runActorByID(r, a.ID).Clone()
					damaged.ID, damaged.Name, damaged.TeamID, damaged.NPC, damaged.HP, damaged.MaxHP = unopenedID, unopenedName, "enemies", true, 700, 1000
					r.Boxes[0].Combat = &damaged
				})
				f.restart(t)
				reloaded := readStockRoom(t, f, runID)
				state, frame, _, err := reloaded.ProjectCombat(a.ID)
				if err != nil || !slices.Equal(frame.EnemyIDs, []string{unopenedID}) || len(state.Actors) != 3 ||
					reloaded.Boxes[0].Combat == nil || reloaded.Boxes[0].Combat.HP >= 1000 {
					t.Fatalf("reloaded projected chest wrong: frame=%+v box=%+v err=%v", frame, reloaded.Boxes[0], err)
				}
				f.clock.add(3 * time.Second)
				opened := runhost.Command{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: reloaded.Version, Action: "chest", TargetID: openedID}
				if _, err := f.svc.Command(context.Background(), opened); !errors.Is(err, adventure.ErrNoUnopenedTreasure) {
					t.Fatalf("opened chest admitted: %v", err)
				}
				claimCmd := runhost.Command{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: reloaded.Version, Action: "chest", TargetID: unopenedID}
				receipt, err := f.svc.Command(context.Background(), claimCmd)
				if err != nil || receipt.Box == nil || receipt.Box.OpenedBy != a.ID {
					t.Fatalf("claim failed: %+v err=%v", receipt, err)
				}
				// Replay preserves single delivery.
				again, err := f.svc.Command(context.Background(), claimCmd)
				if err != nil || again.Box.ID != receipt.Box.ID {
					t.Fatalf("replay failed: %+v err=%v", again, err)
				}
				if got := countItem(t, f, a.ID, "item-001"); got != 1 {
					t.Fatalf("expected single delivery of item-001, got %d", got)
				}
				afterClaim, err := f.store.FindRun(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if _, nextFrame, _, err := afterClaim.ProjectCombat(a.ID); err != nil || len(nextFrame.EnemyIDs) != 0 {
					t.Fatalf("claimed box stayed in opposing pool: %+v err=%v", nextFrame, err)
				}
			} else {
				// A box an earlier strike defeated grants nothing and cannot be claimed.
				f.stage(t, runID, func(r *adventure.NormalRun) { r.Boxes[0].Defeated = true })
				f.restart(t)
				reloaded := readStockRoom(t, f, runID)
				_, frame, _, err := reloaded.ProjectCombat(a.ID)
				if err != nil || len(frame.EnemyIDs) != 0 {
					t.Fatalf("defeated box still in opposing pool: %+v err=%v", frame, err)
				}
				f.clock.add(3 * time.Second)
				if _, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: reloaded.Version, Action: "chest", TargetID: unopenedID}); !errors.Is(err, adventure.ErrNoUnopenedTreasure) {
					t.Fatalf("defeated box claimable: %v", err)
				}
				if got := countItem(t, f, a.ID, "item-001"); got != 0 {
					t.Fatalf("defeated box leaked treasure item: %d", got)
				}
			}
		})
	}
}
