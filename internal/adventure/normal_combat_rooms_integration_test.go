package adventure_test

import (
	"errors"
	"slices"
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

// The stage24 EX boss's job405 table admits its whole pool (stage/24.cgi:96–117).
func TestRunCombatEXJob405BossAnswers(t *testing.T) {
	f, _ := lowRunCombat(t, 1)
	boss := runEnemy("boss", 25000)
	boss.Jobs = corebattle.BattleJobs{JobID: "job-405", SP: 1000, OldJobID: "starter"}
	boss.MP, boss.MaxMP, boss.Attack, boss.Defense, boss.Agility = 10000, 18000, 500, 500, 500
	cmd := combatRoom(t, f, f.actors[0], nil, boss)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "strike", "attack", "boss", "")); err != nil {
		t.Fatal(err)
	}
}

// EX-only NPC jobs without a table reject the whole command.
func TestRunCombatEXNPCJobResponseStaysGated(t *testing.T) {
	for _, job := range []string{"job-406", "job-407"} {
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

// During the NPC turn `%m` is still the requester, so job79's form entry reads
// the requester's `$m{sex}` (_skill.cgi:6242,6278–6290). MP 25 leaves job79.2
// (MP30) out of the pool, so the breath's gender gate is admitted alone.
func TestRunCombatNPCJob79BreathUsesRequesterGender(t *testing.T) {
	f := combatFixture(t, 1)
	f.cfg.RNG = &stockRunRNG{} // The last choice, the breath.
	f.restart(t)
	foe := runEnemy("foe", 900)
	foe.Jobs = corebattle.BattleJobs{JobID: "job-79", SP: 77, OldJobID: "starter"}
	foe.MP, foe.MaxMP = 25, 50
	cmd := combatRoom(t, f, f.actors[0], nil, foe)
	hp := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID).HP
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "defend", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	// The breath's MP21 is spent and its area hit lands on the working state
	// (#2630); the NPC's icon is not the requester's, so no form.
	if npc := runActorByID(r, foe.ID); npc.MP != 4 || npc.Icon != foe.Icon || runActorByID(r, cmd.ActorID).HP >= hp {
		t.Fatalf("NPC breath: NPC=%+v requester HP %d→%d", npc, hp, runActorByID(r, cmd.ActorID).HP)
	}
}

// npcRoom enters a run with the asset session, so every random-pool entry is
// bound as in production, and one NPC answering with the scripted draws.
func npcRoom(t *testing.T, npc corebattle.LifetimeActor, draws ...float64) (*normalFixture, adventure.RunCommand) {
	t.Helper()
	f, _ := lowRunCombat(t, 1)
	f.cfg.RNG = &scriptRNG{draws: draws}
	f.restart(t)
	return f, combatRoom(t, f, f.actors[0], nil, npc)
}

// Job79.2's NPC branch draws the general random helper twice
// (_skill.cgi:6252–6262). Draws 0–3 are the player's defend, 4 selects the
// last choice (job79.2), 5 starts the NPC action; each random then draws a
// job and a position (job3.2, DF up 40×0.4, twice).
func TestRunCombatNPCJob79TimeDrawsRandomTwice(t *testing.T) {
	job, position := (3-.5)/87, 2.5/8
	foe := runEnemy("foe", 900)
	foe.Jobs = corebattle.BattleJobs{JobID: "job-79", SP: 55, OldJobID: "starter"}
	foe.MP, foe.MaxMP, foe.Defense, foe.BaseStats.Defense = 50, 50, 40, 40
	f, cmd := npcRoom(t, foe, .5, .5, .5, .5, .99, .5, job, position, job, position)
	r := accepted(t, f, combatCommand(cmd, "defend", "defend", "", ""))
	if npc := runActorByID(r, foe.ID); npc.MP != 20 || npc.Defense != 72 {
		t.Fatalf("NPC time: MP %d (want 20), DF %d (want 72 after two draws)", npc.MP, npc.Defense)
	}
}

// An NPC's bypass jump keeps the encounter: npc_turn restores the members over
// set_monster's list (_npc_action.cgi:141–151), so only reset_status_all and
// the floor counter remain (_skill.cgi:6116–6133). Draw 4 selects job78.2 (the
// last choice) and draw 6 is its 1-in-3 roll.
func TestRunCombatNPCBypassJumpAdvancesFloorOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roll  float64
		floor int
	}{{"found", .1, 2}, {"not-found", .99, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			foe := runEnemy("foe", 900)
			foe.Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 30, OldJobID: "starter"}
			foe.MP, foe.MaxMP = 50, 50
			f, cmd := npcRoom(t, foe, .99, .99, .99, .99, .99, .99, tc.roll)
			r := accepted(t, f, combatCommand(cmd, "defend", "defend", "", ""))
			// The reset clears the requester's fresh defend (reset_status).
			defended := runActorByID(r, cmd.ActorID).Temporary != ""
			if r.Floor != tc.floor || r.EncounterFloor != 1 || !slices.Equal(r.Enemies, []string{foe.ID}) ||
				runActorByID(r, foe.ID).MP != 40 || defended != (tc.floor == 1) {
				t.Fatalf("floor %d encounter %d enemies %v NPC MP %d defended %v", r.Floor, r.EncounterFloor, r.Enemies, runActorByID(r, foe.ID).MP, defended)
			}
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
