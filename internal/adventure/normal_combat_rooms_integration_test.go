package adventure_test

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// lowRunCombat is assetRunCombat with the zero RNG, so job78's room draws
// succeed.
func lowRunCombat(t *testing.T, count int) (*normalFixture, *storedDepots) {
	t.Helper()
	f, depots := assetRunCombat(t, count)
	f.cfg.RNG = f.rng
	f.restart(t)
	return f, depots
}

// drawRNG lands midRNG's hits while every integer draw (royal bee, crystals) is 0.
type drawRNG struct{ midRNG }

func (r *drawRNG) Intn(int) int   { r.calls.Add(1); return 0 }
func (r *drawRNG) IntN(n int) int { return r.Intn(n) }

func drawRunCombat(t *testing.T) *normalFixture {
	t.Helper()
	f, _ := assetRunCombat(t, 1)
	f.cfg.RNG = &drawRNG{}
	f.restart(t)
	return f
}

// Item212's last strike can kill during an NPC response with the holder as
// killer, but the reward guard still uses the responding NPC (_battle.cgi:28–51).
func TestRunCombatItem212StrikeDuringResponseCreditsNothing(t *testing.T) {
	f := drawRunCombat(t)
	foe := runEnemy("foe", 1)
	foe.ExperienceReward, foe.GoldReward, foe.Attack = 50, 50, 300
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, f.actors[0].ID)
		a.HP, a.Defense = 1, 0
		a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-212")
	}, foe)
	before, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "brace", "tension", "", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	if foe := runActorByID(r, "foe"); foe != nil && foe.HP > 0 {
		t.Fatalf("last strike did not land: %+v", foe)
	}
	after, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil || after.Money != before.Money || after.Experience != before.Experience || r.Recipients[cmd.ActorID].Pending != (adventure.RunEarnings{}) {
		t.Fatalf("NPC-response defeat credited: %+v pending=%+v %v", after, r.Recipients[cmd.ActorID].Pending, err)
	}
}

// Stage22's royal bee defeat draws the EX unlock in the reward path.
func TestRunCombatRoyalBeeDefeatUnlocksEX(t *testing.T) {
	f := drawRunCombat(t)
	bee := runEnemy("bee", 5)
	bee.EncounterIdentity = "ordinary-22-monster-175"
	stage, err := f.cfg.Stages.FindByID("stage-22")
	if err != nil {
		t.Fatal(err)
	}
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.Stage, r.EncounterStageID = stage, stage.ID
	}, bee, runEnemy("guard", 900))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "sting", "attack", "bee", "")); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if r := readStockRoom(t, f, cmd.RunID); !r.EXUnlocked || runActorByID(r, "bee") != nil {
		t.Fatalf("royal bee defeat did not unlock EX: %v", r.EXUnlocked)
	}
}

// EX-only NPC jobs have no table: their response rejects the whole command.
func TestRunCombatEXNPCJobResponseStaysGated(t *testing.T) {
	for _, job := range []string{"job-405", "job-406", "job-407"} {
		t.Run(job, func(t *testing.T) {
			f, depots := lowRunCombat(t, 1)
			foe := runEnemy("foe", 900)
			foe.Jobs = corebattle.BattleJobs{JobID: job, OldJobID: job}
			cmd := combatRoom(t, f, f.actors[0], nil, foe)
			before := readStockRoom(t, f, cmd.RunID)
			strike := combatCommand(cmd, "strike", "attack", "foe", "")
			if _, err := f.svc.Command(t.Context(), strike); err == nil {
				t.Fatal("gated NPC job answered")
			}
			assertUnchanged(t, f, depots, strike, before, nil, nil)
		})
	}
}

// A stored travel tool or book withdrawal followed by a failed asset write
// commits no travel, combat, asset, depot or receipt change.
func TestRunCombatAssetFailureAfterTravelOrWithdrawalRollsBack(t *testing.T) {
	for _, tc := range []struct{ name, job, entry, selector, row string }{
		{"stored-travel", "job-50", "job-50-entry-7", "item-083", "item-083"},
		{"book-withdrawal", "job-86", "job-86-entry-0", "item-170", "item-170"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, depots := assetRunCombat(t, 1)
			if tc.job == "job-86" {
				grantHeldTo(t, f, 0, "item-169")
			}
			actor := f.actors[0].ID
			cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
				a := runActorByID(r, actor)
				a.Jobs, a.MP = corebattle.BattleJobs{JobID: tc.job, SP: 200}, 50
				a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-158")
			}, runEnemy("foe", 900))
			rows := seedDepot(t, depots, actor, tc.row)
			before := readStockRoom(t, f, cmd.RunID)
			held := heldItems(t, f, 0)
			depots.fail = true
			failed := combatCommand(cmd, "failed", tc.entry, tc.selector, "")
			if _, err := f.svc.Command(t.Context(), failed); err == nil || errors.Is(err, adventure.ErrRunUnavailable) {
				t.Fatalf("asset failure: %v", err)
			}
			assertUnchanged(t, f, depots, failed, before, held, rows)
			depots.fail = false
			if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "retry", tc.entry, tc.selector, "")); err != nil {
				t.Fatal(err)
			}
			if tc.name == "stored-travel" && readStockRoom(t, f, cmd.RunID).Floor != before.Floor+1 {
				t.Fatal("retry did not travel")
			}
		})
	}
}
