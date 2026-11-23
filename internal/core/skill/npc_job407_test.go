package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// npc407Fixture is a stage25-shaped EX room: the statue, two encounter ice
// objects and one unrelated ally against one hero. Pool position i has HP/MP
// 1000+i so successors reveal their source position.
func npc407Fixture(iceMaxHP ...int) (corebattle.LifetimeState, *corebattle.RosterContext, BattleDefinitions) {
	actor := func(id, team string, hp int) corebattle.LifetimeActor {
		return corebattle.LifetimeActor{Participant: corebattle.Participant{ID: id, TeamID: team, HP: hp, MaxHP: hp, Attack: 100, Defense: 100, Agility: 100, Gear: &corebattle.BattleGearStats{}}, Hit: 95, Tension: 1}
	}
	statue := actor("statue", "npc", 12000)
	statue.HP, statue.MP, statue.MaxMP, statue.NPC, statue.EncounterIdentity = 5000, 1500, 1500, true, "statue"
	statue.Jobs = corebattle.BattleJobs{JobID: "job-407", SP: 999, OldJobID: "starter"}
	other := actor("other", "npc", 900)
	other.NPC, other.EncounterIdentity = true, "iceAB" // Not the family plus one letter.
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{statue, actor("hero", "party", 2000), other}}
	party := []string{"statue", "other"}
	for i, hp := range iceMaxHP {
		ice := actor(fmt.Sprintf("ice-%d", i), "npc", hp)
		ice.NPC, ice.EncounterIdentity = true, "ice"
		if i == 0 {
			ice.EncounterIdentity = "iceA"
		}
		s.Actors = append(s.Actors, ice)
		party = append(party, ice.ID)
	}
	var pool []corebattle.LifetimeActor
	for i := range 12 {
		p := actor(fmt.Sprintf("pool-%d", i), "", 1000+i)
		p.MP, p.Name, p.EncounterIdentity, p.Icon = 1000+i, "ice-obj", "ice", fmt.Sprintf("icon-%d", i)
		p.Jobs = corebattle.BattleJobs{JobID: fmt.Sprintf("job-%d", i), SP: i, OldJobID: "starter"}
		pool = append(pool, p)
	}
	statuePayload := actor("ex-statue", "", 12000)
	statuePayload.MP, statuePayload.Name, statuePayload.EncounterIdentity, statuePayload.Icon = 1500, "statue", "statue", "icon-statue"
	statuePayload.Jobs = corebattle.BattleJobs{JobID: "job-407", SP: 999, OldJobID: "starter"}
	goddess := actor("exx-goddess", "", 50000)
	goddess.MP, goddess.Attack, goddess.Defense, goddess.Agility = 15000, 500, 700, 500
	goddess.Name, goddess.EncounterIdentity, goddess.Icon = "goddess", "goddess", "icon-goddess"
	goddess.ExperienceReward, goddess.CrystalReward, goddess.Gear = 15000, 999, &corebattle.BattleGearStats{AccessoryID: "item-246"}
	goddess.Jobs = corebattle.BattleJobs{JobID: "job-408", SP: 999, OldJobID: "job-9", OldSP: 9}
	level := 50
	c := &corebattle.RosterContext{
		DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "statue", PartyIDs: party, EnemyIDs: []string{"hero"}}, RequesterLevel: &level, ActorNPC: true}, Defeat: func(string) error { return nil }},
		EnemyPool:     pool, EXPool: []corebattle.LifetimeActor{pool[0], statuePayload, pool[0]}, EXXPool: []corebattle.LifetimeActor{goddess}, ReloadDraws: 1,
		Death: &corebattle.DefeatContext{Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		}},
	}
	var d BattleDefinitions
	d.RegisterNPCJob407(c)
	return s, c, d
}

func executeNPC407(t *testing.T, s *corebattle.LifetimeState, c *corebattle.RosterContext, d BattleDefinitions, id string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(err, rng.used, len(draws))
	}
	c.TargetContext = f.TargetContext
}

func npc407Actor(s *corebattle.LifetimeState, identity string) *corebattle.LifetimeActor {
	for i := range s.Actors {
		if s.Actors[i].RosterIdentity == identity {
			return &s.Actors[i]
		}
	}
	return nil
}

// Source table: ff97fc5 _skill.cgi:8284–8317, all SP0, duplicate position kept.
func TestNPC407TableKeepsSourcePositions(t *testing.T) {
	s, c, d := npc407Fixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	bindings, err := s.CallbackBindings(&f, d.Invocations, 407)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	var keys []string
	for _, b := range bindings {
		got = append(got, fmt.Sprint(b.ID, "/", b.MPCost))
		keys = append(keys, BindingSourceKey(b, false))
	}
	want := []string{"npc-job-407-position-0/24", "npc-job-407-position-1/24", "npc-job-407-position-2/55", "npc-job-407-position-3/66"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if want := []string{"e697cff309d2c3ab", "e697cff309d2c3ab", "16e8bf65478c95a1", "6535a05fa2f4b597"}; !slices.Equal(keys, want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
}

func TestNPC407PoolKeepsDuplicateTransformWeight(t *testing.T) {
	s, c, d := npc407Fixture()
	d.RegisterBaseGuard(&corebattle.BaseGuardContext{DamageContext: c.DamageContext})
	s.Actors[0].Jobs.SP, s.Actors[0].MP = 0, 24
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Damage: &c.DamageContext}
	pool, err := d.npcChoicePool(&s, &f, NPCDefaultSeed())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, choice := range pool.Choices {
		ids = append(ids, choice.ID)
	}
	if want := []string{"npc-normal-attack", "npc-job-407-position-1", "npc-job-407-position-1"}; !slices.Equal(ids, want) {
		t.Fatalf("pool %v, want %v", ids, want)
	}
}

// Independent vectors from _obgene (_skill.cgi:10653–10829) and _add_party
// (9793–9829). Draws: stage reread, extra small int(100+300r), then per
// successor its position draw, its HP multiplier draw and five ×(.9+.3r) rolls;
// zero rolls give ×.9. Thresholds sit on each class boundary.
func TestNPC407ConsolidationClassesBoundariesAndLetters(t *testing.T) {
	s, c, d := npc407Fixture(499, 500, 999, 1000, 2499, 2500)
	roll := []float64{0, 0, 0, 0, 0}
	draws := []float64{0, .5}                       // reread, extra small 250
	draws = append(append(draws, 0, 0), roll...)    // 499 → pos0, ×1.4
	draws = append(append(draws, .25, .5), roll...) // 250 → pos6, ×1.75
	draws = append(append(draws, .5, 0), roll...)   // 500 → pos4, ×1.3
	draws = append(append(draws, .75, .5), roll...) // 999 → pos10, ×1.55
	draws = append(append(draws, 0, 0), roll...)    // 1000 → pos2, ×1.2
	draws = append(append(draws, .75, .5), roll...) // 2499 → pos11, ×1.35
	draws = append(append(draws, .5, .5), roll...)  // single 2500 ≤ 4000 → pos5, ×1.35
	draws = append(draws, 0)                        // heal roll
	executeNPC407(t, &s, c, d, "npc-job-407-position-1", draws...)
	for _, row := range []struct {
		letter  string
		pos, hp int
	}{{"I", 0, 628}, {"J", 6, 393}, {"K", 4, 585}, {"L", 10, 1393}, {"M", 2, 1080}, {"N", 11, 3036}, {"O", 5, 3037}} {
		a := npc407Actor(&s, "ice-"+row.letter)
		if a == nil || a.Icon != fmt.Sprintf("icon-%d", row.pos) || a.HP != row.hp || a.MaxHP != row.hp || a.MP != int(float64(1000+row.pos)*.9) ||
			a.Name != "ice-obj "+row.letter || a.EncounterIdentity != "ice" || a.Jobs.JobID != fmt.Sprintf("job-%d", row.pos) || a.TeamID != "npc" || !a.NPC ||
			a.ExperienceReward != 25 || a.GoldReward != 12 || !slices.Contains(c.PartyIDs, a.ID) {
			t.Fatalf("%+v: %+v", row, a)
		}
	}
	for i := range 6 {
		a := s.Actors[3+i]
		if a.HP != 0 || slices.Contains(c.PartyIDs, a.ID) {
			t.Fatal("ice object stayed", a)
		}
	}
	// The unrelated ally stays; heal = 2×150 + 2×300 + 2×600 + 1000 = 3100 → ×.9.
	if s.Actors[2].HP != 900 || !slices.Contains(c.PartyIDs, "other") || s.Actors[0].HP != 5000+2790 {
		t.Fatal(s.Actors[0].HP, s.Actors[2])
	}
}

// Two ex-class objects, or one above 4000, become one statue copy; exactly
// 4000 stays an ex successor.
func TestNPC407ExClassBecomesStatueCopy(t *testing.T) {
	roll := []float64{0, 0, 0, 0, 0}
	for _, row := range []struct {
		name   string
		ice    []int
		copied bool
	}{{"two", []int{2500, 3000}, true}, {"above-4000", []int{4001}, true}, {"4000", []int{4000}, false}} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := npc407Fixture(row.ice...)
			draws := append([]float64{0, 0, 0, 0}, roll...) // reread, extra 100, small pos0 ×1.4
			if row.copied {
				draws = append(draws, roll...)
			} else {
				draws = append(append(draws, 0, 0), roll...) // pos2 ×1.2
			}
			executeNPC407(t, &s, c, d, "npc-job-407-position-0", append(draws, 0)...)
			copy := npc407Actor(&s, "statue-copy-J")
			if row.copied != (copy != nil) {
				t.Fatal(copy)
			}
			heal := 150 + 1000*len(row.ice)
			if s.Actors[0].HP != 5000+int(float64(heal)*.9) {
				t.Fatal(s.Actors[0].HP)
			}
			if copy == nil {
				if a := npc407Actor(&s, "ice-J"); a == nil || a.HP != int(4000*1.2*.9) || a.Icon != "icon-2" {
					t.Fatal(a)
				}
				return
			}
			if copy.HP != 10800 || copy.MP != 1350 || copy.Jobs.JobID != "job-407" || copy.Name != "氷の女神模像 J" || copy.EncounterIdentity != "statue" || copy.Icon != "icon-statue" {
				t.Fatal(copy)
			}
		})
	}
}

// With no ice objects only the extra small successor appears.
func TestNPC407EmptyRosterAddsOneSmall(t *testing.T) {
	s, c, d := npc407Fixture()
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if a := npc407Actor(&s, "ice-I"); a == nil || a.HP != 126 || len(c.PartyIDs) != 3 || s.Actors[0].HP != 5135 {
		t.Fatal(a, c.PartyIDs, s.Actors[0].HP)
	}
}

// A living copy other than the actor summons the goddess in place of the whole
// living side, the actor included. _add_party keeps only nine payload fields:
// rewards follow the requester level, and item/crystal/old job are reset.
func TestNPC407LivingCopySummonsGoddess(t *testing.T) {
	s, c, d := npc407Fixture(800)
	copy := s.Actors[3]
	copy.ID, copy.EncounterIdentity, copy.RosterIdentity = "copy", "statue", "statue-copy-K"
	s.Actors = append(s.Actors, copy)
	c.PartyIDs = append(c.PartyIDs, "copy")
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", 0, 0, 0, 0, 0, 0)
	for _, id := range []string{"statue", "other", "ice-0", "copy"} {
		for _, a := range s.Actors {
			if a.ID == id && a.HP != 0 {
				t.Fatal("side member stayed", a)
			}
		}
	}
	g := npc407Actor(&s, "goddess")
	if g == nil || g.HP != 45000 || g.MaxMP != 13500 || g.Defense != 630 || g.Jobs != (corebattle.BattleJobs{JobID: "job-408", SP: 999, OldJobID: "starter"}) ||
		g.ExperienceReward != 25 || g.CrystalReward != 0 || g.Gear.AccessoryID != "" || g.Icon != "icon-goddess" || g.EncounterIdentity != "goddess" ||
		!reflect.DeepEqual(c.PartyIDs, []string{"statue", g.ID}) {
		t.Fatal(g, c.PartyIDs)
	}
}

// The actor as the only copy does not summon. An acting copy whose letter is
// reused is overwritten in place, keeping one slot.
func TestNPC407CopyPresenceAndInPlaceOverwrite(t *testing.T) {
	roll := []float64{0, 0, 0, 0, 0}
	s, c, d := npc407Fixture(2500, 2600)
	s.Actors[0].RosterIdentity = "statue-copy-J"
	draws := append(append([]float64{0, 0, 0, 0}, roll...), roll...)
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", append(draws, 0)...)
	a := s.Actors[0]
	// Overwritten to the payload roll, then healed 150+2000 → ×.9.
	if a.ID != "statue" || a.RosterIdentity != "statue-copy-J" || a.MaxHP != 10800 || a.HP != 10800 || npc407Actor(&s, "goddess") != nil {
		t.Fatal(a)
	}
	n := 0
	for _, id := range c.PartyIDs {
		if id == "statue" {
			n++
		}
	}
	if n != 1 {
		t.Fatal(c.PartyIDs)
	}
}

// Members felled earlier in the request stay in the source's NPC-turn party:
// a fallen copy still summons the goddess, and a fallen ice object is still
// consolidated and counted for the heal. A helper the requester dismissed this
// request is not on that side.
func TestNPC407FallenThisRequestStillCounts(t *testing.T) {
	s, c, d := npc407Fixture()
	copy := s.Actors[2]
	copy.ID, copy.HP, copy.EncounterIdentity, copy.RosterIdentity = "fallen-copy", 0, "statue", "statue-copy-Z"
	s.Actors = append(s.Actors, copy)
	c.PartyIDs = append(c.PartyIDs, "fallen-copy")
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", 0, 0, 0, 0, 0, 0)
	if g := npc407Actor(&s, "goddess"); g == nil || s.Actors[0].HP != 0 {
		t.Fatal("fallen copy did not summon", g)
	}

	roll := []float64{0, 0, 0, 0, 0}
	s, c, d = npc407Fixture(1000)
	s.Actors[3].HP = 0
	dismissed := s.Actors[2]
	dismissed.ID, dismissed.HP, dismissed.EncounterIdentity, dismissed.RosterIdentity, dismissed.RosterDetached = "dismissed", 0, "ice", "calling-63", true
	s.Actors = append(s.Actors, dismissed)
	c.PartyIDs = append(c.PartyIDs, "dismissed")
	// Extra small 100 → pos0; fallen 1000 → pos2 ×1.2; heal 150+600 → ×.9.
	draws := append(append(append([]float64{0, 0, 0, 0}, roll...), 0, 0), roll...)
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", append(draws, 0)...)
	if a := npc407Actor(&s, "ice-J"); a == nil || a.HP != 1080 || slices.Contains(c.PartyIDs, "ice-0") || s.Actors[0].HP != 5675 {
		t.Fatal(a, c.PartyIDs, s.Actors[0].HP)
	}
	if d := s.Actors[len(s.Actors)-3]; d.ID != "dismissed" || d.RosterIdentity != "calling-63" || !slices.Contains(c.PartyIDs, "dismissed") {
		t.Fatal("dismissed helper was consumed", d)
	}
}

// Breath hits int(2+2r) times and stops once the actor falls; the wave
// paralyzes on r×4<1 and lowers AT/DF/AG by .2 (_skill.cgi:8287–8315).
func TestNPC407BreathCountAndParalysisDraws(t *testing.T) {
	hp := func(draw float64) int {
		s, c, d := npc407Fixture()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		ops, err := d.InvocationAction(&s, &f, []string{"npc-job-407-position-2"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, &npc407Draws{first: draw}); err != nil {
			t.Fatal(err)
		}
		return 2000 - s.Actors[1].HP
	}
	two, three := hp(.49), hp(.5)
	if two <= 0 || three*2 != two*3 {
		t.Fatal(two, three)
	}
	s, c, d := npc407Fixture()
	s.Actors[0].HP = 1
	s.Actors[1].Temporary = corebattle.TemporaryReflectBreath
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(&s, &f, []string{"npc-job-407-position-2"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &npc407Draws{first: .99}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || s.Actors[0].HP != 0 || s.Actors[1].HP != 2000 || c.AreaDamageCalls > 1 {
		t.Fatal(err, s.Actors[0].HP, s.Actors[1].HP)
	}

	for _, row := range []struct {
		draw float64
		hit  bool
	}{{.24, true}, {.25, false}} {
		s, c, d := npc407Fixture()
		// HP above 999 would make AT immune (_skill.cgi:9263–9264).
		s.Actors[1].HP, s.Actors[1].Status = 900, corebattle.StatusPoison
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		ops, err := d.InvocationAction(&s, &f, []string{"npc-job-407-position-3"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, &npc407Draws{first: row.draw}); err != nil {
			t.Fatal(err)
		}
		h := s.Actors[1]
		if row.hit != (h.Status == corebattle.StatusParalyze) || row.hit != (h.Attack < 100 && h.Defense < 100 && h.Agility < 100) {
			t.Fatal(row, h)
		}
	}
}

// npc407Draws returns first, then zeros.
type npc407Draws struct {
	first float64
	used  int
}

func (r *npc407Draws) Float64() float64 {
	r.used++
	if r.used == 1 {
		return r.first
	}
	return 0
}
func (r *npc407Draws) Intn(int) int { return 0 }
func (r *npc407Draws) IntN(int) int { return 0 }

// Missing stage payloads or requester level reject the transform before any
// draw or mutation; the other positions stay available.
func TestNPC407RosterGatesBeforeMutation(t *testing.T) {
	for name, edit := range map[string]func(*corebattle.RosterContext){
		"no-exx":   func(c *corebattle.RosterContext) { c.EXXPool = nil },
		"no-ex":    func(c *corebattle.RosterContext) { c.EXPool = c.EXPool[:1] },
		"pool":     func(c *corebattle.RosterContext) { c.EnemyPool = c.EnemyPool[:11] },
		"no-level": func(c *corebattle.RosterContext) { c.RequesterLevel = nil },
	} {
		t.Run(name, func(t *testing.T) {
			s, c, _ := npc407Fixture(700)
			edit(c)
			before, _ := json.Marshal(s)
			rng := &npc407Draws{}
			if err := s.ApplyNPCJob407(c, "npc-job-407-position-0", rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if after, _ := json.Marshal(s); string(after) != string(before) || rng.used != 0 {
				t.Fatal("mutated or drew")
			}
			if err := s.ValidateNPCJob407(c, "npc-job-407-position-3", rng); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Roster changes persist through the battle JSON the run stores.
func TestNPC407RosterSurvivesReload(t *testing.T) {
	s, c, d := npc407Fixture(600)
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	frame := corebattle.TargetContext{ActorID: "statue"}
	loaded.PruneRoster(&frame)
	var ids []string
	for _, a := range loaded.Actors {
		ids = append(ids, a.RosterIdentity+"@"+a.EncounterIdentity)
	}
	if !reflect.DeepEqual(ids, []string{"@statue", "@", "@iceAB", "ice-I@ice", "ice-J@ice"}) || !reflect.DeepEqual(frame.PartyIDs, []string{"statue", "other", "summoned-1", "summoned-2"}) {
		t.Fatal(ids, frame.PartyIDs)
	}
}

// Enemy summon and job407 share one request-scoped StageLoaded flag so only the
// first loader consumes ReloadDraws, and execution rollback restores it.
func TestNPC407AndEnemySummonShareStageLoadedFlagAndRollback(t *testing.T) {
	s, c, d := npc407Fixture(600)
	c.EnemyPool[0].EncounterIdentity = "other-species"
	var loaded bool
	c.StageLoaded = &loaded
	c.EnemyTeamID = "party"
	d.RegisterSummonAppearanceGems(c, nil)
	d.Invocations.Entries["fail-at-exec"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			return corebattle.ErrUnsupportedEffect
		},
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_summon_sky"}, {Kind: "entry", EntryID: "fail-at-exec"}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: []float64{0, 0, 0}}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || loaded {
		t.Fatalf("failed sequence did not roll back StageLoaded: err=%v loaded=%v", err, loaded)
	}

	// First loader (gem_summon_sky) consumes the 1 ReloadDraws + suffix + position.
	executeNPC407(t, &s, c, d, "gem_summon_sky", 0, 0, 0)
	if !loaded || len(s.Actors) != 5 {
		t.Fatalf("gem_summon_sky: loaded=%v actors=%d", loaded, len(s.Actors))
	}

	// Subsequent job407 in the same request consumes 16 draws (skips the 1 reread draw).
	executeNPC407(t, &s, c, d, "npc-job-407-position-0", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
}
