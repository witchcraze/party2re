package adventure

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type ordinaryCombat struct{ fieldCalls int }

func (*ordinaryCombat) PrepareAdvance(context.Context, *NormalRun, string) error { return nil }
func (c *ordinaryCombat) PrepareBossField(_ context.Context, r *NormalRun, id string) error {
	c.fieldCalls++
	frame := corebattle.TargetContext{ActorID: id, EnemyIDs: r.Enemies}
	for _, a := range r.Battle.Actors {
		if a.TeamID == r.actor(id).TeamID {
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		}
	}
	return r.Battle.ApplySpecialBossField(&corebattle.EquipmentResetContext{TargetContext: frame}, &ordinaryRNG{})
}
func (*ordinaryCombat) ExecuteRunAction(context.Context, *NormalRun, RunCommand) (RunActionEffects, error) {
	return RunActionEffects{}, ErrRunUnavailable
}

func TestOrdinaryAdvanceFreezesCompleteEncounterAndCaps(t *testing.T) {
	stages, err := InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stages.FindByID("stage-24")
	if err != nil {
		t.Fatal(err)
	}
	party := ordinaryParty()
	for i := range party {
		party[i].Gear = &corebattle.BattleGearStats{}
	}
	helper := party[0].Clone()
	helper.ID = "helper"
	helper.NPC = true
	helper.MaxHP = 3000
	helper.BaseStats.Attack = 600
	party = append(party, helper)
	r := NormalRun{ID: "run", Stage: stage, Floor: 9, Roster: []string{"a", "b"}, Battle: corebattle.LifetimeState{Actors: party}}
	combat := &ordinaryCombat{}
	s := NormalRunService{cfg: NormalRunConfig{Monsters: monsters, Combat: combat, RNG: &ordinaryRNG{}}}
	if err := s.advance(context.Background(), &r, "a"); err != nil {
		t.Fatal(err)
	}
	if r.Floor != 10 || r.EncounterFloor != 10 || len(r.Enemies) != 3 || len(r.EncounterPool) != 6 || !r.Battle.SpecialBossField || combat.fieldCalls != 1 {
		t.Fatalf("%+v field calls%d", r, combat.fieldCalls)
	}
	if r.actor("helper").MaxHP != 999 || r.actor("helper").Attack != 255 || r.actor(r.Enemies[0]).HP != 5000 {
		t.Fatal("helper omitted from party scaling/caps")
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var loaded NormalRun
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(again) {
		t.Fatal("reload changed durable encounter facts")
	}
}

type ordinaryRNG struct {
	value float64
	calls int
}

func (r *ordinaryRNG) Float64() float64 { r.calls++; return r.value }
func (r *ordinaryRNG) Intn(n int) int   { return int(r.Float64() * float64(n)) }
func (r *ordinaryRNG) IntN(n int) int   { return r.Intn(n) }

func ordinaryParty() []corebattle.LifetimeActor {
	return []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "a", TeamID: "party", HP: 500, MaxHP: 1000, MP: 10, MaxMP: 200, Attack: 80, Defense: 60, Agility: 40, BaseStats: corebattle.BattleBaseStats{Attack: 50, Defense: 40, Agility: 30}}, Jobs: corebattle.BattleJobs{JobID: "job-01", SP: 80, OldJobID: "job-02", OldSP: 5}},
		{Participant: corebattle.Participant{ID: "b", TeamID: "party", HP: 0, MaxHP: 200, MaxMP: 50, Attack: 25, Defense: 15, Agility: 10}, Jobs: corebattle.BattleJobs{JobID: "job-03", SP: 20, OldJobID: "starter", OldSP: 9}}}
}

func TestOrdinaryCatalogAllSourcePositions(t *testing.T) {
	stages, err := InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	bosses, normal := 0, 0
	seen := map[string]bool{}
	for n := 0; n < 28; n++ {
		stage, err := stages.FindByID(fmt.Sprintf("stage-%02d", n))
		if err != nil {
			t.Fatal(err)
		}
		variants := []Stage{stage}
		if n == 26 {
			variants = nil
			for _, season := range []string{"spring", "summer", "autumn", "winter"} {
				variants = append(variants, stage.OrdinarySeason(season))
			}
		}
		for _, stage := range variants {
			if stage.Ordinary == nil {
				t.Fatalf("missing %s %s", stage.ID, stage.Season)
			}
			if n == 15 {
				bosses++
				normal++
				continue
			}
			groups := append([][]string{stage.Ordinary.MonsterIDs}, stage.Ordinary.BossRosters...)
			for group, ids := range groups {
				if group == 0 {
					normal += len(ids)
				} else {
					bosses += len(ids)
				}
				for _, id := range ids {
					m, err := monsters.FindByID(id)
					if err != nil {
						t.Fatal(err)
					}
					if m.Combat == nil || m.Combat.NPCLoadout == nil || seen[id] {
						t.Fatalf("invalid source position %s", id)
					}
					seen[id] = true
					if m.Combat.Jobs.JobID == "job-406" {
						t.Fatal("EX body imported into ordinary stage23")
					}
				}
			}
		}
	}
	if bosses != 74 || normal != 241 {
		t.Fatalf("source positions bosses=%d monsters=%d", bosses, normal)
	}
}

func TestOrdinaryBossPayloadAndReload(t *testing.T) {
	stages, _ := InitialStageCatalog()
	catalog, _ := InitialMonsterCatalog()
	stage, _ := stages.FindByID("stage-24")
	party := ordinaryParty()
	rng := &ordinaryRNG{}
	encounter, err := stage.BuildOrdinaryEncounter(catalog, party, party[0], 10, CrawlModeNormal, "encounter", rng)
	actors := encounter.Actors
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 3 || actors[0].MaxMP != 5000 || actors[1].MaxMP != 18000 || actors[2].MaxMP != 5000 || actors[0].MP != 1800 {
		t.Fatalf("maxima/ordering: %+v", actors)
	}
	if actors[0].Jobs.OldJobID != "job-53" || actors[2].Jobs.OldJobID != "job-54" || actors[0].Temporary != corebattle.TemporaryNullMagic || actors[1].Temporary != corebattle.TemporaryNullAttack {
		t.Fatal("full payload lost")
	}
	if actors[0].NPCLoadout.CustomCost != 41 || actors[0].Gear.AccessoryID != "item-186" {
		t.Fatal("gem cost/equipment lost")
	}
	before := rng.calls
	run := NormalRun{Battle: corebattle.LifetimeState{Actors: actors}}
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var loaded NormalRun
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(run, loaded) || rng.calls != before {
		t.Fatal("reload altered frozen payload")
	}
	actors[0].NPCLoadout.Gems[0] = "changed"
	m, _ := catalog.FindByID(stage.Ordinary.BossRosters[0][0])
	if m.Combat.NPCLoadout.Gems[0] == "changed" {
		t.Fatal("template mutated")
	}
}

func TestOrdinaryConditionalStages(t *testing.T) {
	stages, _ := InitialStageCatalog()
	catalog, _ := InitialMonsterCatalog()
	party := ordinaryParty()
	stage, _ := stages.FindByID("stage-25")
	for branch, draw := range []float64{0, .4, .8} {
		rng := &ordinaryRNG{value: draw}
		encounter, err := stage.BuildOrdinaryEncounter(catalog, party, party[0], 10, CrawlModeNormal, "e", rng)
		actors := encounter.Actors
		if err != nil || len(actors) != 3 {
			t.Fatalf("branch%d: %v %+v", branch, err, actors)
		}
		want := []string{"job-55", "job-54", "job-403"}[branch]
		if actors[0].Jobs.JobID != want || actors[2].Jobs.JobID != want {
			t.Fatalf("branch%d jobs: %+v", branch, actors)
		}
	}
	stage, _ = stages.FindByID("stage-26")
	if stage.OrdinarySeason().Season != "spring" || stage.OrdinarySeason("winter", "summer").Season != "summer" {
		t.Fatal("room marker priority/default")
	}
	for _, season := range []string{"spring", "summer", "autumn", "winter"} {
		variant := stage.OrdinarySeason(season)
		encounter, err := variant.BuildOrdinaryEncounter(catalog, party, party[0], 10, CrawlModeNormal, "e", &ordinaryRNG{})
		actors := encounter.Actors
		if err != nil || len(actors) != 1 || actors[0].Hit != 150 {
			t.Fatalf("%s: %+v %v", season, actors, err)
		}
		if season == "summer" && actors[0].NPCLoadout.CustomName != "wild-claw" {
			t.Fatal("custom/source job88 name identity diverged")
		}
	}
	stage, _ = stages.FindByID("stage-23")
	encounter, err := stage.BuildOrdinaryEncounter(catalog, party, party[0], 10, CrawlModeNormal, "e", &ordinaryRNG{})
	actors := encounter.Actors
	if err != nil || len(actors) != 1 || actors[0].Jobs.JobID != "job-21" || actors[0].Jobs.SP != 10 || actors[0].MP != 27 {
		t.Fatalf("stage23: %+v %v", actors, err)
	}
}

func TestOrdinaryStage15SnapshotAndGeneratedSlots(t *testing.T) {
	stages, _ := InitialStageCatalog()
	catalog, _ := InitialMonsterCatalog()
	stage, _ := stages.FindByID("stage-15")
	party := ordinaryParty()
	encounter, err := stage.BuildOrdinaryEncounter(catalog, party, party[0], 10, CrawlModeNormal, "e", &ordinaryRNG{})
	actors := encounter.Actors
	if err != nil || len(actors) != 2 {
		t.Fatalf("%v %+v", err, actors)
	}
	if actors[0].HP != 900 || actors[0].Attack != 72 || actors[1].HP != 180 || actors[0].Temporary != corebattle.TemporaryReflectMagic || actors[0].Jobs != party[0].Jobs {
		t.Fatal("party copy did not retain current stats and max resources")
	}
	if len(encounter.Pool) != 10 || encounter.Pool[0].Attack != 50 || encounter.Pool[0].MaxHP != 0 {
		t.Fatal("generated summon table was lost or rolled twice")
	}
	data, err := json.Marshal(encounter)
	if err != nil {
		t.Fatal(err)
	}
	var loaded OrdinaryEncounter
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(encounter, loaded) {
		t.Fatal("generated source table changed after reload")
	}
	for _, draw := range []float64{0, .999} {
		encounter, err := stage.BuildOrdinaryEncounter(catalog, party, party[0], 1, CrawlModeNormal, "e", &ordinaryRNG{value: draw})
		actors := encounter.Actors
		if err != nil {
			t.Fatal(err)
		}
		want := "starter"
		if draw > 0 {
			want = "job-87"
		}
		if actors[0].Jobs.JobID != want || actors[0].Jobs.OldJobID != "job-36" || actors[0].Jobs.OldSP != 999 {
			t.Fatalf("generated jobs: %+v", actors[0].Jobs)
		}
	}
}

func TestOrdinarySuppliedStateAndHardScaling(t *testing.T) {
	m := Monster{ID: "m", Name: "native", HP: 100, MP: 80, Attack: 30, Defense: 20, Agility: 10, CMP: 100, ExperienceReward: 10, GoldReward: 20,
		Combat: &corebattle.LifetimeActor{Participant: corebattle.Participant{MaxHP: 500, MaxMP: 400, MaxCMP: 300, BaseStats: corebattle.BattleBaseStats{Attack: 70}, Abilities: []string{"poison1"}, Gear: &corebattle.BattleGearStats{AccessoryID: "item-159"}}, Jobs: corebattle.BattleJobs{JobID: "job-01", SP: 9, OldJobID: "job-02", OldSP: 5}, Hit: 200, Tension: 3, Temporary: corebattle.TemporaryRegeneration, Extended: corebattle.ExtendedPurification, ExtendedTurns: 4, CrystalReward: 3, NPCLoadout: &corebattle.NPCLoadout{CustomName: "custom", Gems: [3]string{"a", "a", "b"}, GemCosts: [3]int{27, 27, 16}}}}
	s := CrawlSession{Mode: CrawlModeHard, RngFloat: func() float64 { return 0 }}
	a := s.buildMonsterActor("e", m, 0, false)
	if a.HP != 135 || a.MaxHP != 500 || a.MaxMP != 400 || a.MaxCMP != 300 || a.Jobs.SP != 13 || a.Jobs.OldSP != 7 || a.CrystalReward != 4 || a.NPCLoadout.CustomCost != 84 || a.Hit != 200 || a.ExtendedTurns != 4 {
		t.Fatalf("supplied/hard payload: %+v", a)
	}
}

func TestOrdinaryNPCSkippedHeadRotationAndReload(t *testing.T) {
	now := time.Unix(1000, 0)
	r := NormalRun{Admission: NormalAdmission{Speed: 18}, Enemies: []string{"a", "b", "c"}, LastActions: map[string]time.Time{"a": now, "b": now.Add(-18 * time.Second)}, Battle: corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "a", HP: 10}, NPC: true}, {Participant: corebattle.Participant{ID: "b", HP: 10}, NPC: true}, {Participant: corebattle.Participant{ID: "c", HP: 0}, NPC: true}}}}
	if r.NextNPC(now) != "b" {
		t.Fatal("wrong eligible member")
	}
	r.actor("b").HP = 0 // The selected actor can fall during its own action/tails.
	if err := r.RecordNPCResponse("b", now); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Enemies, []string{"b", "c", "a"}) || r.LastActions["a"] != now || r.NextNPC(now) != "" {
		t.Fatal("rotation/timestamps")
	}
	data, _ := json.Marshal(r)
	var loaded NormalRun
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	before := append([]string(nil), loaded.Enemies...)
	if loaded.NextNPC(now) != "" || !reflect.DeepEqual(before, loaded.Enemies) {
		t.Fatal("no eligible NPC changed order")
	}
}
