package adventure_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
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
	if err != nil || after.Money != before.Money || after.Experience != before.Experience || r.Recipients[cmd.ActorID].Pending != (runhost.Earnings{}) {
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

// The stage25 statue's roster transform (_skill.cgi:10653–10829) in a response
// where every draw is .99: the last choice (MP54 excludes the breath), the
// stage reread, then the extra small int(100+297)=397 → position 9 ×2.093 and
// both encounter ice objects (MaxHP 1100, class ≥1000) → position 11 ×1.497,
// each rolled ×1.197; heal 150+2×600 → ×1.197. Item165 is the requester's,
// never the NPC's (`%m` stays the requester in npc_turn). The saved order
// rotates the post-action head.
func TestRunCombatEXJob407RosterTransformPersists(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requester, npc string
		small, large   int
	}{{"plain", "", "item-165", 994, 1971}, {"requester-item165", "item-165", "", 1192, 2365}} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := lowRunCombat(t, 1)
			f.cfg.RNG = &stockRunRNG{}
			f.restart(t)
			stages, err := adventure.InitialStageCatalog()
			if err != nil {
				t.Fatal(err)
			}
			monsters, err := adventure.InitialMonsterCatalog()
			if err != nil {
				t.Fatal(err)
			}
			stage, err := stages.FindByID("stage-25")
			if err != nil {
				t.Fatal(err)
			}
			payload := func(id, actorID string) corebattle.LifetimeActor {
				m, err := monsters.FindByID(id)
				if err != nil {
					t.Fatal(err)
				}
				a := m.NPCPayload()
				a.ID, a.TeamID, a.MaxHP, a.MaxMP = actorID, "enemies", a.HP, a.MP
				return a
			}
			statue := payload("ex-25-boss-224", "statue")
			statue.HP, statue.MP = 5000, 54 // Below the breath's MP55: the transform is last.
			statue.Gear.AccessoryID = tc.npc
			cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
				r.Stage = stage.OrdinarySeason("stage-25")
				r.EncounterPool = nil
				for _, id := range r.Stage.Ordinary.MonsterIDs {
					r.EncounterPool = append(r.EncounterPool, payload(id, id))
				}
				runActorByID(r, f.actors[0].ID).HeldToolID = new(tc.requester)
			}, statue, payload("ex-25-boss-206", "ice-a"), payload("ex-25-boss-243", "ice-b"))
			if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "guard", "defend", "", "")); err != nil {
				t.Fatal(err)
			}
			r := readStockRoom(t, f, cmd.RunID)
			var got []string
			for _, id := range r.Enemies {
				a := runActorByID(r, id)
				got = append(got, fmt.Sprint(a.RosterIdentity, ":", a.HP))
			}
			want := []string{fmt.Sprint("ordinary-25-boss-22-I:", tc.small), fmt.Sprint("ordinary-25-boss-22-J:", tc.large), fmt.Sprint("ordinary-25-boss-22-K:", tc.large), ":6615"}
			if !slices.Equal(got, want) || runActorByID(r, "ice-a") != nil || runActorByID(r, "ice-b") != nil {
				t.Fatal(got)
			}
		})
	}
}

func TestRunCombatEXJob408AnswersAfterJob407Summons(t *testing.T) {
	f, _ := lowRunCombat(t, 1)
	f.cfg.RNG = &stockRunRNG{}
	f.restart(t)
	stage, err := f.cfg.Stages.FindByID("stage-25")
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	statueTemplate, err := monsters.FindByID("ex-25-boss-224")
	if err != nil {
		t.Fatal(err)
	}
	statue := statueTemplate.NPCPayload()
	statue.ID, statue.TeamID = "statue", "enemies"
	statue.MaxHP, statue.MaxMP, statue.MP = statue.HP, statue.MP, 54
	copy := statue.Clone()
	copy.ID, copy.RosterIdentity = "copy", statue.EncounterIdentity+"-copy-I"
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.Stage, r.EncounterStageID = stage.OrdinarySeason("stage-25"), stage.ID
		r.EncounterPool = nil
		for _, id := range r.Stage.Ordinary.MonsterIDs {
			m, err := monsters.FindByID(id)
			if err != nil {
				t.Fatal(err)
			}
			r.EncounterPool = append(r.EncounterPool, m.NPCPayload())
		}
	}, statue, copy)
	r := accepted(t, f, combatCommand(cmd, "summon", "defend", "", ""))
	if len(r.Enemies) != 1 {
		t.Fatalf("summon roster %v", r.Enemies)
	}
	goddess := runActorByID(r, r.Enemies[0])
	if goddess.Jobs.JobID != "job-408" || goddess.EncounterIdentity != "exx-25-boss-264" {
		t.Fatalf("summoned actor %+v", goddess)
	}
	id, mp := goddess.ID, goddess.MP
	cmd.Version = r.Version
	f.clock.add(3 * time.Second)
	r = accepted(t, f, combatCommand(cmd, "answer", "defend", "", ""))
	goddess = runActorByID(r, id)
	if goddess.MP != mp-87 || runActorByID(r, cmd.ActorID).Temporary != "" {
		t.Fatalf("freezing wave left NPC MP %d (want %d) and requester temporary %q", goddess.MP, mp-87, runActorByID(r, cmd.ActorID).Temporary)
	}
}

func TestRunCombatEXJob406ResponseStaysGated(t *testing.T) {
	f, depots := lowRunCombat(t, 1)
	foe := runEnemy("foe", 900)
	foe.Jobs = corebattle.BattleJobs{JobID: "job-406", OldJobID: "job-406"}
	cmd := combatRoom(t, f, f.actors[0], nil, foe)
	before := readStockRoom(t, f, cmd.RunID)
	strike := combatCommand(cmd, "strike", "attack", "foe", "")
	if _, err := f.svc.Command(t.Context(), strike); err == nil {
		t.Fatal("gated NPC job answered")
	}
	assertUnchanged(t, f, depots, strike, before, nil, nil)
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
func npcRoom(t *testing.T, npc corebattle.LifetimeActor, draws ...float64) (*normalFixture, runhost.Command) {
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
			if r.Round != tc.floor || r.EncounterFloor != 1 || !slices.Equal(r.Enemies, []string{foe.ID}) ||
				runActorByID(r, foe.ID).MP != 40 || defended != (tc.floor == 1) {
				t.Fatalf("floor %d encounter %d enemies %v NPC MP %d defended %v", r.Round, r.EncounterFloor, r.Enemies, runActorByID(r, foe.ID).MP, defended)
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
			if _, err := f.svc.Command(t.Context(), failed); err == nil || errors.Is(err, runhost.ErrUnavailable) {
				t.Fatalf("asset failure: %v", err)
			}
			assertUnchanged(t, f, depots, failed, before, held, rows)
			depots.fail = false
			if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "retry", tc.entry, tc.selector, "")); err != nil {
				t.Fatal(err)
			}
			if tc.name == "stored-travel" && readStockRoom(t, f, cmd.RunID).Round != before.Round+1 {
				t.Fatal("retry did not travel")
			}
		})
	}
}

// Stage25's first summon-path load in a request consumes the top-level branch
// draw before the letter draw, and a subsequent job407 transform in the same
// request does not repeat it.
func TestRunCombatStage25SummonAndJob407ShareStageReload(t *testing.T) {
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stages.FindByID("stage-25")
	if err != nil {
		t.Fatal(err)
	}
	payload := func(id, actorID string) corebattle.LifetimeActor {
		m, err := monsters.FindByID(id)
		if err != nil {
			t.Fatal(err)
		}
		a := m.NPCPayload()
		a.ID, a.TeamID, a.MaxHP, a.MaxMP = actorID, "enemies", a.HP, a.MP
		return a
	}

	// Player uses held tool-78 (consumes 1 reload draw, then suffix .0 -> 'I',
	// then position .0 -> pool[0]), followed in the same request by statue's
	// job407 transform (which skips the reload draw and consumes 23 draws across
	// 3 successors, plus statue's item-246 tail draw).
	f, _ := lowRunCombat(t, 1)
	// Draws: [seal .5] [player tmp .5] [reload .5] [suffix .0] [pos .0] [npc tmp .5] [npc sel .99] [job407: 23 draws] [item-246 .5]
	draws := []float64{.5, .5, .5, 0, 0, .5, .99}
	for range 23 {
		draws = append(draws, .99)
	}
	draws = append(draws, .5)
	rng := &scriptRNG{draws: draws}
	f.cfg.RNG = rng
	f.restart(t)
	grantHeldTo(t, f, 0, "item-078")
	statue := payload("ex-25-boss-224", "statue")
	statue.HP, statue.MP = 5000, 54
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.Stage = stage.OrdinarySeason("stage-25")
		r.EncounterPool = nil
		for _, id := range r.Stage.Ordinary.MonsterIDs {
			r.EncounterPool = append(r.EncounterPool, payload(id, id))
		}
		r.AppearancePositions = append([]int(nil), r.Stage.Ordinary.Appears...)
		a := runActorByID(r, f.actors[0].ID)
		a.CMPCharge, a.CMPNeedCharge = 0, 9
		a.HeldToolID = new("item-078")
	}, statue, payload("ex-25-boss-206", "ice-a"))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "harp", "tool", "", "")); err != nil {
		t.Fatal(err)
	}
	if rng.calls.Load() != 31 || len(rng.draws) != 0 {
		t.Fatalf("unexpected draws: calls=%d remaining=%v", rng.calls.Load(), rng.draws)
	}
	r := readStockRoom(t, f, cmd.RunID)
	// The summoned ice object was consolidated by job407 into a third successor (3 successors + statue = 4).
	if len(r.Enemies) != 4 || r.Enemies[3] != "statue" {
		t.Fatalf("expected 3 consolidated successors + statue, got %v", r.Enemies)
	}
}

// Stage15's first summon in a request rebuilds ten pool entries from the
// summoner's stats (20 draws), which a second summon in the same request
// reuses; a new request whose first loader is an NPC rebuilds from that NPC.
func TestRunCombatStage15RebuildsPoolFromFirstSummonerPerRequest(t *testing.T) {
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stages.FindByID("stage-15")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := lowRunCombat(t, 1)
	// Request 1: player uses item-078 (rebuilds from player SP=17, AT=77),
	// then npc-1 responds with job-95 (reuses player-rebuilt pool without reload).
	// Draws: [seal .5] [tmp .5] [20 rebuild draws: 10×(0, 0 -> starter)] [suffix 0 -> I] [pos 0]
	//        [npc tmp .5] [npc sel .99] [npc suffix .5 -> R] [npc pos 0]
	draws := []float64{.5, .5}
	for range 20 {
		draws = append(draws, 0)
	}
	draws = append(draws, 0, 0, .5, .99, .5, 0)
	rng := &scriptRNG{draws: draws}
	f.cfg.RNG = rng
	f.restart(t)
	grantHeldTo(t, f, 0, "item-078")
	npc1 := runEnemy("npc-1", 888)
	npc1.MaxHP, npc1.MaxMP, npc1.Attack, npc1.BaseStats.Attack, npc1.MP = 888, 222, 123, 123, 100
	npc1.Jobs = corebattle.BattleJobs{JobID: "job-95", SP: 67, OldJobID: "starter"}
	npc2 := npc1
	npc2.ID = "npc-2"
	stale := runEnemy("stale", 99)
	stale.Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 1}
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.Stage = stage
		r.EncounterPool = []corebattle.LifetimeActor{stale}
		a := runActorByID(r, f.actors[0].ID)
		a.MaxHP, a.Attack, a.BaseStats.Attack = 432, 77, 77
		a.Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 17, OldJobID: "starter"}
		a.CMPCharge, a.CMPNeedCharge = 0, 9
		a.HeldToolID = new("item-078")
	}, npc1, npc2)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "req1-tool", "tool", "", "")); err != nil {
		t.Fatal(err)
	}
	if rng.calls.Load() != 28 || len(rng.draws) != 0 {
		t.Fatalf("req1 draws: calls=%d remaining=%v", rng.calls.Load(), rng.draws)
	}
	r := readStockRoom(t, f, cmd.RunID)
	var playerSummon, npcReuse *corebattle.LifetimeActor
	for _, id := range r.Enemies {
		a := runActorByID(r, id)
		if a == nil {
			continue
		}
		switch a.RosterIdentity {
		case "generated-job:starter-I":
			playerSummon = a
		case "generated-job:starter-R":
			npcReuse = a
		}
	}
	if playerSummon == nil || npcReuse == nil || playerSummon.Jobs.SP != 17 || npcReuse.Jobs.SP != 17 || npcReuse.Jobs.JobID != "starter" || npcReuse.TeamID != "enemies" {
		t.Fatalf("req1 stage15 summons: enemies=%v player=%+v npc=%+v", r.Enemies, playerSummon, npcReuse)
	}

	// Request 2: player defends, then npc-2 responds first with job-95 (rebuilds
	// 10 entries from npc-2 SP=67, job draw .99 -> job-87).
	f.clock.add(3 * time.Second)
	cmd.Version = r.Version
	draws2 := []float64{.5, .5}      // seal, player defend tmp
	draws2 = append(draws2, .5, .99) // npc tmp, npc sel
	for range 10 {
		draws2 = append(draws2, 0, .99) // sex, job-87
	}
	draws2 = append(draws2, .95, 0) // suffix -> Z, pos -> 0
	rng2 := &scriptRNG{draws: draws2}
	f.cfg.RNG = rng2
	f.restart(t)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "req2-defend", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	if rng2.calls.Load() != 26 || len(rng2.draws) != 0 {
		t.Fatalf("req2 draws: calls=%d remaining=%v", rng2.calls.Load(), rng2.draws)
	}
	r2 := readStockRoom(t, f, cmd.RunID)
	var npcRebuilt *corebattle.LifetimeActor
	for _, id := range r2.Enemies {
		if a := runActorByID(r2, id); a != nil && a.RosterIdentity == "generated-job:job-87-Z" {
			npcRebuilt = a
		}
	}
	if npcRebuilt == nil || npcRebuilt.Jobs.SP != 67 || npcRebuilt.Jobs.JobID != "job-87" || npcRebuilt.TeamID != "enemies" {
		t.Fatalf("req2 npc-rebuilt summon: enemies=%v npcRebuilt=%+v", r2.Enemies, npcRebuilt)
	}
}

// Stage25's twelve ice-object pool entries share ordinary-25-boss-22, so
// summons from different pool positions collide on the same letter across
// producers and persisted reload boundaries, retry the previous letter, and
// skip when both are occupied.
func TestRunCombatStage25SummonCollisionAndReload(t *testing.T) {
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stages.FindByID("stage-25")
	if err != nil {
		t.Fatal(err)
	}
	payload := func(id, actorID string) corebattle.LifetimeActor {
		m, err := monsters.FindByID(id)
		if err != nil {
			t.Fatal(err)
		}
		a := m.NPCPayload()
		a.ID, a.TeamID, a.MaxHP, a.MaxMP = actorID, "enemies", a.HP, a.MP
		return a
	}

	f, _ := lowRunCombat(t, 1)
	rng := &scriptRNG{}
	f.cfg.RNG = rng
	f.restart(t)
	grantHeldTo(t, f, 0, "item-078")
	foe := runEnemy("foe", 5000)
	foe.Jobs = corebattle.BattleJobs{JobID: "job-95", SP: 99, OldJobID: "starter"}
	foe.MP, foe.MaxMP = 200, 200
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.Stage = stage.OrdinarySeason("stage-25")
		r.EncounterPool = nil
		for _, id := range r.Stage.Ordinary.MonsterIDs {
			r.EncounterPool = append(r.EncounterPool, payload(id, id))
		}
		a := runActorByID(r, f.actors[0].ID)
		a.CMPCharge, a.CMPNeedCharge = 0, 9
		a.HeldToolID = new("item-078")
	}, foe)
	// Req 1: player defends ([seal .5] [player tmp .5]), foe responds with
	// job-95 ([npc sel .99] [npc tmp .5] [reload .5] [suffix 0->I] [pos 0/12]).
	// Enemies rotates to [summoned-1, foe].
	rng.calls.Store(0)
	rng.draws = []float64{.5, .5, .99, .5, .5, 0, 0}
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "req1", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	if rng.calls.Load() != 7 || len(rng.draws) != 0 {
		t.Fatalf("req1 draws: calls=%d rem=%v", rng.calls.Load(), rng.draws)
	}
	f.restart(t)
	r1 := readStockRoom(t, f, cmd.RunID)
	if len(r1.Enemies) != 2 || runActorByID(r1, r1.Enemies[0]).RosterIdentity != "ordinary-25-boss-22-I" {
		t.Fatalf("req1 enemies=%v actor0=%+v", r1.Enemies, runActorByID(r1, r1.Enemies[0]))
	}

	// Req 2 (after reload): player uses held tool-78 on pool pos 1 with suffix 0 ->
	// collides on I, retries Z; summoned-1 responds with attack. Enemies rotates to
	// [foe, summoned-2, summoned-1].
	f.clock.add(3 * time.Second)
	rng2 := &scriptRNG{draws: []float64{.5, .5, 0, 1.0 / 12.0, 0, 0, .5, .5, .5, .5, .5}}
	f.cfg.RNG = rng2
	f.restart(t)
	cmd.Version = r1.Version
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "req2", "tool", "", "")); err != nil {
		t.Fatal(err)
	}
	if rng2.calls.Load() != 11 || len(rng2.draws) != 0 {
		t.Fatalf("req2 draws: calls=%d rem=%v", rng2.calls.Load(), rng2.draws)
	}
	f.restart(t)
	r2 := readStockRoom(t, f, cmd.RunID)
	if len(r2.Enemies) != 3 || runActorByID(r2, r2.Enemies[2]).RosterIdentity != "ordinary-25-boss-22-I" || runActorByID(r2, r2.Enemies[1]).RosterIdentity != "ordinary-25-boss-22-Z" {
		t.Fatalf("req2 enemies=%v id1=%q id2=%q", r2.Enemies, runActorByID(r2, r2.Enemies[1]).RosterIdentity, runActorByID(r2, r2.Enemies[2]).RosterIdentity)
	}

	// Req 3 (after reload): player defends, foe responds with job-95 on pool pos 2
	// with suffix 0 -> both I and Z occupied, accepted no-op.
	f.clock.add(3 * time.Second)
	rng3 := &scriptRNG{draws: []float64{.5, .5, .99, .5, .5, 0, 2.0 / 12.0}}
	f.cfg.RNG = rng3
	f.restart(t)
	cmd.Version = r2.Version
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "req3", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	if rng3.calls.Load() != 7 || len(rng3.draws) != 0 {
		t.Fatalf("req3 draws: calls=%d rem=%v", rng3.calls.Load(), rng3.draws)
	}
	r3 := readStockRoom(t, f, cmd.RunID)
	want := []string{r2.Enemies[1], r2.Enemies[2], r2.Enemies[0]}
	if !slices.Equal(r3.Enemies, want) {
		t.Fatalf("req3 should not add another enemy: got=%v want=%v", r3.Enemies, want)
	}
}
