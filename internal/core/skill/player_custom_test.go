package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func playerCustomFixture(gems [3]string, cost int) (corebattle.LifetimeState, *corebattle.DamageContext, BattleDefinitions) {
	s, c, d := damageGemRegistryFixture()
	d.RegisterDamageGems(&c)
	s.Actors[0].Custom = &corebattle.CustomLoadout{Name: "custom", Gems: gems, Cost: cost}
	return s, &c, d
}

// _data.cgi:2171 and system.cgi:1599–1608: three gem83 slots cost 18.
// Each redraw excludes the extended entry in the same complete job table.
func TestPlayerCustomRepeatedRandomSlots(t *testing.T) {
	s, c, d := playerDispatchFixture()
	s.Actors[0].MP, s.Actors[0].CMP, s.Actors[0].Jobs.SP = 1, 30, 0
	s.Actors[0].Custom = &corebattle.CustomLoadout{Name: "custom", Gems: [3]string{"gem_metronome", "gem_metronome", "gem_metronome"}, Cost: 18}
	d.Invocations.Entries["job-1-entry-2"] = baseGuardInvocation(c, "job-1-entry-2")
	d.RegisterNonJobInvocations()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen"}}
	rng := &damageGemDraws{t: t, draws: []float64{.5 / 87, 0, 2.5 / 7, .5 / 87, 0, 2.5 / 7, .5 / 87, 0, 2.5 / 7}}
	if err := d.ApplyPlayerCustomAction(&s, &f, nil, rng); err != nil {
		t.Fatal(err)
	}
	if rng.used != 9 || s.Actors[0].CMP != 12 || s.Actors[0].MP != 1 || s.Actors[0].Temporary != corebattle.TemporaryCovering || s.Actors[2].Temporary != corebattle.TemporaryCover {
		t.Fatal(s.Actors, rng.used)
	}
}

// _skill.cgi:298–307 and _battle.cgi:787–789: repeated slots run in order with
// the same target; the outer CMP charge follows the callbacks.
func TestPlayerCustomRepeatedSlotsChargeLateCMP(t *testing.T) {
	s, c, d := playerCustomFixture([3]string{"gem_dance_combo", "", "gem_dance_combo"}, 30)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	before, _ := json.Marshal(s)
	frame, ops, err := d.PlayerCustomAction(&s, &f, []string{"attack"}, noArithmeticDraws{t})
	after, _ := json.Marshal(s)
	if err != nil || len(ops) != 2 || !reflect.DeepEqual(frame.Arguments, []string{"enemy"}) || !frame.Custom || string(before) != string(after) {
		t.Fatal(ops, frame, err)
	}
	rng := &damageGemDraws{t: t, draws: make([]float64, 14)}
	if err := d.ApplyPlayerCustomAction(&s, &f, []string{"attack"}, rng); err != nil {
		t.Fatal(err)
	}
	if s.Actors[1].HP != 1000-27*4 || c.DamageCalls != 4 || rng.used != 14 || s.Actors[0].CMP != 60 || s.Actors[0].MP != 80 {
		t.Fatal(s.Actors[:2], c.DamageCalls, rng.used)
	}
}

func TestPlayerCustomUnsupportedSlotRejectsBeforeEffectOrCost(t *testing.T) {
	for _, gems := range [][3]string{{"gem_dance_combo", "gem_dance_combo", "gem-101"}, {"gem-102", "gem_atk_1", ""}, {"gem_atk_1", "missing", ""}} {
		s, c, d := playerCustomFixture(gems, 30)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		before, _ := json.Marshal(s)
		if err := d.ApplyPlayerCustomAction(&s, &f, nil, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(gems, err)
		}
		after, _ := json.Marshal(s)
		if string(before) != string(after) || c.DamageCalls != 0 || f.Custom {
			t.Fatal(gems, "rejection changed state")
		}
	}
}

// The actor's death stops later slots; the charge is still applied.
func TestPlayerCustomDeathStopsLaterSlotsButCharges(t *testing.T) {
	s, c, d := playerCustomFixture([3]string{"gem_kamikaze_sky", "gem_atk_1", "gem_atk_1"}, 30)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if err := d.ApplyPlayerCustomAction(&s, &f, nil, &damageGemDraws{t: t, draws: []float64{0}}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[0].HP != 0 || s.Actors[1].HP != 910 || c.DamageCalls != 1 || s.Actors[0].CMP != 60 {
		t.Fatal(s.Actors[:2], c.DamageCalls)
	}
}

func TestPlayerCustomResolvesUnknownTargetToActorOnce(t *testing.T) {
	s, c, d := playerCustomFixture([3]string{"gem_atk_1", "", ""}, 0)
	for _, args := range [][]string{nil, {"nobody", "enemy"}} {
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
		frame, _, err := d.PlayerCustomAction(&s, &f, nil, noArithmeticDraws{t})
		if err != nil || !reflect.DeepEqual(frame.Arguments, []string{"self"}) {
			t.Fatal(args, frame.Arguments, err)
		}
	}
}

func TestPlayerCustomListingGates(t *testing.T) {
	cases := map[string]func(*corebattle.LifetimeState, *corebattle.CallbackContext, *[]string){
		"no loadout": func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ *[]string) {
			s.Actors[0].Custom = nil
		},
		"no name": func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ *[]string) {
			s.Actors[0].Custom.Name = ""
		},
		"no gems": func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ *[]string) {
			s.Actors[0].Custom.Gems = [3]string{}
		},
		"no enemies":    func(_ *corebattle.LifetimeState, c *corebattle.CallbackContext, _ *[]string) { c.EnemyIDs = nil },
		"name collides": func(_ *corebattle.LifetimeState, _ *corebattle.CallbackContext, n *[]string) { *n = []string{"custom"} },
		"short CMP":     func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ *[]string) { s.Actors[0].CMP = 29 },
		"cursed": func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ *[]string) {
			s.Actors[0].Abilities = []string{"cursed"}
		},
	}
	for name, mutate := range cases {
		s, c, d := playerCustomFixture([3]string{"gem_atk_1", "", ""}, 30)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		names := []string{"attack"}
		mutate(&s, &f, &names)
		if err := d.ApplyPlayerCustomAction(&s, &f, names, noArithmeticDraws{t}); !errors.Is(err, ErrUnavailable) {
			t.Fatal(name, err)
		}
	}
	s, c, d := playerCustomFixture([3]string{"gem_atk_1", "", ""}, 30)
	s.Actors[0].NPC = true
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, _, err := d.PlayerCustomAction(&s, &f, nil, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrInvalidParticipant) {
		t.Fatal(err)
	}
}

// The loadout is frozen run input: a reloaded state repeats the same result.
func TestPlayerCustomLoadoutSurvivesReload(t *testing.T) {
	s, c, d := playerCustomFixture([3]string{"gem_atk_1", "gem_atk_1", ""}, 12)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil || !reflect.DeepEqual(loaded.Actors[0].Custom, s.Actors[0].Custom) {
		t.Fatal(loaded.Actors[0].Custom, err)
	}
	for _, state := range []*corebattle.LifetimeState{&s, &loaded} {
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		if err := d.ApplyPlayerCustomAction(state, &f, nil, &damageGemDraws{t: t, draws: []float64{0, 0}}); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(s, loaded) || s.Actors[0].CMP != 78 {
		t.Fatal(s.Actors[0], loaded.Actors[0])
	}
}
