package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func npc405Fixture() (corebattle.LifetimeState, *corebattle.NPCJob405Context, BattleDefinitions) {
	s, base, _ := npcOrderedFixture()
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-405", SP: 1000, OldJobID: "job-405"}
	s.Actors[1].Defense = 30
	s.Actors[1].CMP, s.Actors[1].MaxCMP = 100, 100
	c := &corebattle.NPCJob405Context{DamageContext: base.DamageContext, Death: &corebattle.DefeatContext{
		Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		},
	}}
	var d BattleDefinitions
	d.RegisterNPCJob405(c)
	return s, c, d
}

// executeNPC405 resolves id through the HP-selected table, as discovery does.
func executeNPC405(t *testing.T, s *corebattle.LifetimeState, c *corebattle.NPCJob405Context, d BattleDefinitions, id string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	bindings, err := s.CallbackBindings(&f, d.Invocations, 405)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range bindings {
		found = found || b.ID == id
	}
	if !found {
		t.Fatal("not in the selected variant:", id, bindings)
	}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(err, rng.used, draws)
	}
}

// Independent vectors from ff97fc5:_skill.cgi:7888–8076. Self AT100/DF80/AG40,
// enemy DF30 (tool defense 9); three members, so two others.
func TestNPC405PositionVectors(t *testing.T) {
	for _, row := range []struct {
		name, id     string
		selfHP       int
		args         []string
		draws        []float64
		enemyHP      int
		enemyStatus  corebattle.StatusEffect
		calls, areas int
	}{
		// 4 × (power 80 → 72) then one seal draw per enemy each round.
		{"holy", "npc-job-405-position-0", 1000, nil, []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 712, corebattle.StatusMagicSeal, 4, 4},
		// The wounded name runs five rounds; a .25 draw does not seal.
		{"holy-wounded", "npc-job-405-position-0-wounded", 500, nil, []float64{0, 0, .25, 0, 0, .25, 0, 0, .25, 0, 0, .25, 0, 0, .25}, 640, "", 5, 5},
		// DF×.6=48 → 43; AT×1.3=130 → (65−9)×.9=50; status draw, then the 1.5 draw.
		{"baptism", "npc-job-405-position-2", 1000, []string{"enemy", "enemy"}, []float64{0, 0, .75, 0}, 907, corebattle.StatusDofuu, 2, 0},
		{"baptism-second-skipped", "npc-job-405-position-2", 1000, []string{"enemy", "enemy"}, []float64{0, 0, .5, .9}, 907, corebattle.StatusDanceSeal, 2, 0},
		// Area 300 untyped direct → 270.
		{"barrier", "npc-job-405-position-4", 1000, nil, []float64{0}, 730, "", 1, 1},
		// Per-enemy int(500+rand300): draw .5 → 650 → 585.
		{"barrier-wounded", "npc-job-405-position-4-wounded", 500, nil, []float64{.5, 0}, 415, "", 1, 0},
		// (220)×.7+2×200=554 → (277−9)×.9=241.
		{"triad", "npc-job-405-position-5", 1000, []string{"enemy"}, []float64{0}, 759, "", 1, 0},
		// 220+2×300=820 → (410−9)×.9=360; one enemy ends the chain.
		{"triad-wounded", "npc-job-405-position-5-wounded", 500, nil, []float64{0, 0}, 640, "", 1, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := npc405Fixture()
			s.Actors[0].HP = row.selfHP
			if row.id == "npc-job-405-position-4" || row.id == "npc-job-405-position-4-wounded" {
				// Cleaved before the hit: reflection gone, defense back to base.
				s.Actors[1].Temporary, s.Actors[1].Defense = corebattle.TemporaryReflectMagic, 60
			}
			executeNPC405(t, &s, c, d, row.id, row.args, row.draws...)
			e := s.Actors[1]
			if e.HP != row.enemyHP || e.Status != row.enemyStatus || c.DamageCalls != row.calls || c.AreaDamageCalls != row.areas || len(c.Arguments) != 0 {
				t.Fatal(e, c.DamageCalls, c.AreaDamageCalls, c.Arguments)
			}
			switch row.id {
			case "npc-job-405-position-4", "npc-job-405-position-4-wounded":
				if e.Temporary != "" || e.Defense != 30 {
					t.Fatal("barrier was not cleaved", e)
				}
			case "npc-job-405-position-5", "npc-job-405-position-5-wounded":
				cmp := map[bool]int{false: 70, true: 50}[row.id == "npc-job-405-position-5-wounded"]
				if e.CMP != cmp || e.CMPCharge != 0 || e.CMPNeedCharge != 10 {
					t.Fatal(e)
				}
			}
		})
	}
}

func TestNPC405BlessingSealsSelfAndPicksCounter(t *testing.T) {
	for draw, want := range map[float64]corebattle.TemporaryEffect{0: corebattle.TemporaryReflectAttack, .5: corebattle.TemporaryReflectBreath, .99: corebattle.TemporaryReflectDance} {
		s, c, d := npc405Fixture()
		executeNPC405(t, &s, c, d, "npc-job-405-position-1", nil, draw)
		// The fallen member's helper falls back to the caster: two ×.6 increments.
		a := s.Actors[0]
		if a.Attack != 220 || a.Defense != 176 || a.Agility != 88 || a.Status != corebattle.StatusDofuu || a.Temporary != want {
			t.Fatal(draw, a)
		}
	}
}

func TestNPC405RenewalHealsOpponentsSacrificesAlliesAndBendsField(t *testing.T) {
	for _, row := range []struct {
		name               string
		temporary          string
		turns, fieldDraw   int
		wantTemp           string
		wantTurns, wantAnt int
	}{
		{"no-field", "", 0, 0, "", 0, 6},
		{"short-field", "darkness", 3, 0, "", 0, 3},
		{"equal-field-ticks-once", "darkness", 6, 0, "darkness", 5, 0},
		{"long-field", "darkness", 12, 4, "darkness", 2, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := npc405Fixture()
			s.Fields = corebattle.LifetimeFields{Temporary: row.temporary, Turns: row.turns, Default: "snow"}
			s.Actors[0].HP = 400
			s.Actors[1].HP, s.Actors[1].MP, s.Actors[1].MaxMP = 500, 0, 200
			s.Actors[2].HP, s.Actors[2].MaxHP = 100, 100
			// Self heal 900; opponent heal: target draw + 135; MP: target draw + 91;
			// tension draw; field draw; sacrifice 150×1.7 tension → 229.
			executeNPC405(t, &s, c, d, "npc-job-405-position-3", nil, 0, 0, 0, 0, 0, 0, float64(row.fieldDraw)/8, 0)
			self, enemy, ally := s.Actors[0], s.Actors[1], s.Actors[2]
			if self.HP != 1000 || enemy.MP != 91 || enemy.HP != 635-229 || ally.HP != 0 || self.Tension != 1 ||
				self.Extended != corebattle.ExtendedPurification || self.ExtendedTurns != 7 {
				t.Fatal(self, enemy, ally)
			}
			if f := s.Fields; f.Temporary != row.wantTemp || f.Turns != row.wantTurns || f.AntiTurns != row.wantAnt || f.Default != "snow" {
				t.Fatal(f)
			}
		})
	}
}

// Each later hit carries the previous hit's tension-scaled power (×.8).
func TestNPC405WoundedChainCarriesTension(t *testing.T) {
	s, c, d := npc405Fixture()
	other := s.Actors[1]
	other.ID = "enemy2"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "enemy2")
	s.Actors[0].HP, s.Actors[0].Tension = 500, 3
	s.Actors[1].HP, s.Actors[3].HP = 5000, 5000
	// 220+3×300=1120; ×3 tension → (1680−9)×.9=1503; int(3360×.8)=2688 → 1201.
	executeNPC405(t, &s, c, d, "npc-job-405-position-5-wounded", nil, 0, 0, 0, 0)
	if s.Actors[1].HP != 5000-1503-1201 || s.Actors[3].HP != 5000 || c.DamageCalls != 2 {
		t.Fatal(s.Actors, c.DamageCalls)
	}
}

func TestNPC405ChoicesFilterByResourcesAndHP(t *testing.T) {
	for _, row := range []struct {
		hp, mp int
		want   []string
	}{
		{1000, 85, []string{"npc-normal-attack", "npc-job-405-position-0", "npc-job-405-position-2", "npc-job-405-position-3", "npc-job-405-position-4"}},
		{500, 85, []string{"npc-normal-attack", "npc-job-405-position-0-wounded", "npc-job-405-position-2", "npc-job-405-position-3", "npc-job-405-position-4-wounded"}},
		{500, 90, []string{"npc-normal-attack", "npc-job-405-position-0-wounded", "npc-job-405-position-1", "npc-job-405-position-2", "npc-job-405-position-3", "npc-job-405-position-4-wounded", "npc-job-405-position-5-wounded"}},
	} {
		s, c, d := npc405Fixture()
		s.Actors[0].NPC, c.ActorNPC = true, true
		s.Actors[0].Jobs.OldJobID = "job-405" // OldSP 0 re-adds only SP0 names.
		s.Actors[0].HP, s.Actors[0].MP = row.hp, row.mp
		plan, err := d.npcChoicePool(&s, &corebattle.CallbackContext{TargetContext: c.TargetContext, Damage: &c.DamageContext}, NPCDefaultSeed())
		if err != nil {
			t.Fatal(err)
		}
		// SP0 names repeat from the previous job and keep their weight.
		var got []string
		for _, choice := range plan.Choices {
			if !slices.Contains(got, choice.ID) {
				got = append(got, choice.ID)
			}
		}
		if !reflect.DeepEqual(got, row.want) {
			t.Fatal(row.hp, row.mp, got)
		}
	}
}

func TestNPC405AdmissionRejectsBeforeAnyMutation(t *testing.T) {
	for _, row := range []struct {
		name, id string
		selfHP   int
	}{
		{"later-holy-round", "npc-job-405-position-0", 1000},
		{"agility-hook", "npc-job-405-position-1", 1000},
		{"lethal-baptism", "npc-job-405-position-2", 1000},
		{"sacrifice", "npc-job-405-position-3", 1000},
		{"cleave", "npc-job-405-position-4", 1000},
		{"wounded-cleave", "npc-job-405-position-4-wounded", 500},
		{"lethal-triad", "npc-job-405-position-5", 1000},
		{"chain", "npc-job-405-position-5-wounded", 500},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := npc405Fixture()
			args := []string{"enemy", "enemy"}
			switch row.name {
			case "chain":
				other := s.Actors[1]
				other.ID = "enemy2"
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, "enemy2")
				s.Actors[1].Temporary, c.ToolCategory.Source = corebattle.TemporaryOneHitDefense, "unknown"
			case "agility-hook":
				s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
			case "later-holy-round", "cleave", "wounded-cleave":
				// A one-hit defense is consumed by the first hit; the next reaches
				// an unknown tool category.
				s.Actors[1].Temporary, c.ToolCategory.Source = corebattle.TemporaryOneHitDefense, "unknown"
			case "sacrifice":
				s.Actors[2].HP, c.ToolCategory.Source = 100, "unknown"
			default:
				s.Actors[1].HP, c.Death = 1, nil
			}
			s.Actors[0].HP = row.selfHP
			c.Arguments = append([]string(nil), args...)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			for _, validate := range []func() error{
				func() error {
					_, err := d.InvocationAction(&s, &f, []string{row.id}, noArithmeticDraws{t})
					return err
				},
				func() error { return s.ApplyNPCJob405(c, row.id, noArithmeticDraws{t}) },
			} {
				if err := validate(); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(args, c.Arguments) || c.DamageCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

// Reload rebuilds definitions from serialized facts; the same draws then
// produce the same table and outcome.
func TestNPC405ReloadedActorChoosesAndActsIdentically(t *testing.T) {
	s, c, d := npc405Fixture()
	s.Actors[0].HP = 500
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.NPCJob405Context{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext}, GuaranteedHit: true,
	}, Death: &corebattle.DefeatContext{Rewards: c.Death.Rewards}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterNPCJob405(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCJob405Context
		d BattleDefinitions
	}{{&s, c, d}, {&loaded, resumed, rebuilt}} {
		executeNPC405(t, run.s, run.c, run.d, "npc-job-405-position-4-wounded", nil, .5, 0)
	}
	if !reflect.DeepEqual(s, loaded) || loaded.Actors[1].HP != 415 {
		t.Fatal(s, loaded)
	}
}

// A reachable defeat must be admitted before the body runs, whether the port
// is absent or its reward rejects; a nonlethal body needs no port.
func TestNPC405AdmissionCoversReachableDefeat(t *testing.T) {
	ids := []string{"npc-job-405-position-0", "npc-job-405-position-3", "npc-job-405-position-4",
		"npc-job-405-position-4-wounded", "npc-job-405-position-5", "npc-job-405-position-5-wounded"}
	for _, mode := range []string{"no-port", "reward-rejects"} {
		for _, id := range ids {
			s, c, _ := npc405Fixture()
			s.Actors[0].HP, s.Actors[1].HP, s.Actors[2].HP = 1000, 1, 100
			if id == "npc-job-405-position-4-wounded" || id == "npc-job-405-position-5-wounded" {
				s.Actors[0].HP = 500
			}
			if mode == "no-port" {
				c.Death = nil
			} else {
				c.Death.Rewards.Validate = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
					return corebattle.ErrUnsupportedEffect
				}
			}
			c.Arguments = []string{"enemy"}
			before, _ := json.Marshal(s)
			if err := s.ApplyNPCJob405(c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(mode, id, err)
			}
			if after, _ := json.Marshal(s); string(after) != string(before) {
				t.Fatal(mode, id, "mutated before rejection")
			}
		}
	}
	s, c, d := npc405Fixture()
	s.Actors[0].HP, c.Death = 500, nil
	executeNPC405(t, &s, c, d, "npc-job-405-position-4-wounded", nil, .5, 0)
	if s.Actors[1].HP != 415 {
		t.Fatal(s.Actors[1])
	}
}

// Item149 skips the tension multiplier but keeps tension, so the carry grows:
// hit1 1120×1.5 → ≤997, hit2 int(1120×5×.8)×1.5 → ≤4021 (_skill.cgi:8049,8067,
// 8887,8923,9954). Only the second hit can defeat a 3000-HP enemy; 8000 HP
// survives both, so a rejecting defeat port must not block it.
func TestNPC405ChainCarryWithItem149(t *testing.T) {
	for _, row := range []struct {
		hp     int
		reject bool
	}{{3000, true}, {8000, false}} {
		s, c, d := npc405Fixture()
		other := s.Actors[1]
		other.ID = "enemy2"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "enemy2")
		s.Actors[0].HP, s.Actors[0].Tension, s.Actors[0].ItemDefinitionIDs = 500, 5, []string{"item-149"}
		s.Actors[1].HP, s.Actors[3].HP = row.hp, row.hp
		c.Death = nil
		err := s.ValidateNPCJob405(c, "npc-job-405-position-5-wounded", noArithmeticDraws{t})
		if row.reject != errors.Is(err, corebattle.ErrUnsupportedEffect) || (!row.reject && err != nil) {
			t.Fatal(row.hp, err)
		}
		if !row.reject {
			// Near-maximal rolls on one enemy: 996 + 4020. The CMP stat-down
			// resets tension even with item149 (_skill.cgi:9284).
			executeNPC405(t, &s, c, d, "npc-job-405-position-5-wounded", nil, 0, .999, 0, .999)
			if s.Actors[1].HP != 8000-996-4020 || s.Actors[0].Tension != 1 {
				t.Fatal(s.Actors[1].HP, s.Actors[0].Tension)
			}
		}
	}
}

// Four holy rounds (≤132 each) cannot defeat a 300-HP enemy alone but can
// together; 600 HP survives all four, so no defeat port is required.
func TestNPC405AdmissionSeesCumulativeDamage(t *testing.T) {
	for _, row := range []struct {
		hp     int
		reject bool
	}{{300, true}, {600, false}} {
		s, c, _ := npc405Fixture()
		s.Actors[1].HP, c.Death = row.hp, nil
		err := s.ValidateNPCJob405(c, "npc-job-405-position-0", noArithmeticDraws{t})
		if row.reject != errors.Is(err, corebattle.ErrUnsupportedEffect) || (!row.reject && err != nil) {
			t.Fatal(row.hp, err)
		}
	}
}

// Two sacrificed allies (HP100 → power 150): the first hit carries the drawn
// tension 1.7 (≤306), the second does not (≤180). Only together can they
// defeat 450 HP; 500 HP survives both.
func TestNPC405RenewalAdmitsCumulativeSacrifice(t *testing.T) {
	for _, row := range []struct {
		hp     int
		reject bool
	}{{450, true}, {500, false}} {
		s, c, _ := npc405Fixture()
		ally := s.Actors[2]
		ally.ID = "ally2"
		s.Actors = append(s.Actors, ally)
		c.PartyIDs = append(c.PartyIDs, "ally2")
		s.Actors[2].HP, s.Actors[2].MaxHP, s.Actors[3].HP, s.Actors[3].MaxHP = 100, 100, 100, 100
		// Purification resets each hit's target, so keep the reset port and
		// reject only the defeat.
		s.Actors[1].HP = row.hp
		c.Death.Rewards.Validate = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
			return corebattle.ErrUnsupportedEffect
		}
		err := s.ValidateNPCJob405(c, "npc-job-405-position-3", noArithmeticDraws{t})
		if row.reject != errors.Is(err, corebattle.ErrUnsupportedEffect) || (!row.reject && err != nil) {
			t.Fatal(row.hp, err)
		}
	}
}

// A first hit that defeats its target leaves no living recipient for the
// second hit or either status; the 1.5 draw is still consumed.
func TestNPC405BaptismSkipsFallenTargets(t *testing.T) {
	s, c, d := npc405Fixture()
	s.Actors[1].HP = 40
	executeNPC405(t, &s, c, d, "npc-job-405-position-2", []string{"enemy", "enemy"}, 0, 0)
	if e := s.Actors[1]; e.HP != 0 || e.Status != "" || c.DamageCalls != 1 {
		t.Fatal(e, c.DamageCalls)
	}
}
