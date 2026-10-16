package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func weaponSealFixture(seal int) (corebattle.LifetimeState, corebattle.DamageContext) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 1000, MaxHP: 1000, MP: 100, Gear: &corebattle.BattleGearStats{WeaponID: "weapon-1"}}, Jobs: corebattle.BattleJobs{OldSP: 90}, WeaponSeal: seal, Tension: 1},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1},
		{Participant: corebattle.Participant{ID: "last", TeamID: "npc", HP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1},
	}}
	c := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy", "last"}}}, Defeat: func(string) error { return nil }}
	return s, c
}

func TestWeaponSealSharedDiscoverySubmissionBoundaries(t *testing.T) {
	want := []corebattle.CallbackEntry{
		{ID: "seal-7-breath", Name: "seal-7-breath", RequiredSP: 90, MPCost: 40},
		{ID: "seal-8-magic", Name: "seal-8-magic", RequiredSP: 90, MPCost: 27},
		{ID: "seal-9-magic", Name: "seal-9-magic", RequiredSP: 90, MPCost: 40},
	}
	if !reflect.DeepEqual(corebattle.WeaponSealEntries(), want) {
		t.Fatal(corebattle.WeaponSealEntries())
	}
	for i, entry := range corebattle.WeaponSealEntries() {
		for _, tc := range []struct {
			name string
			edit func(*corebattle.LifetimeState, *corebattle.DamageContext)
			want bool
		}{
			{"previous89 current999", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) {
				s.Actors[0].Jobs.OldSP, s.Actors[0].Jobs.SP = 89, 999
			}, false},
			{"previous89 current0", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].Jobs.OldSP = 89 }, false},
			{"previous90 current0", func(*corebattle.LifetimeState, *corebattle.DamageContext) {}, true},
			{"previous90 current999", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].Jobs.SP = 999 }, true},
			{"previous91 current0", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].Jobs.OldSP = 91 }, true},
			{"previous91 current999", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) {
				s.Actors[0].Jobs.OldSP, s.Actors[0].Jobs.SP = 91, 999
			}, true},
			{"MP below", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].MP = entry.MPCost - 1 }, false},
			{"MP exact", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].MP = entry.MPCost }, true},
			{"item249 below doubled", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) {
				s.Actors[0].ItemDefinitionIDs = []string{"item-249"}
				s.Actors[0].MP = entry.MPCost*2 - 1
			}, false},
			{"item249 exact doubled", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) {
				s.Actors[0].ItemDefinitionIDs = []string{"item-249"}
				s.Actors[0].MP = entry.MPCost * 2
			}, true},
			{"no weapon", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].Gear.WeaponID = "" }, false},
			{"no gear", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].Gear = nil }, false},
			{"seal0", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].WeaponSeal = 0 }, false},
			{"wrong seal", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].WeaponSeal = 10 }, false},
			{"no enemies", func(s *corebattle.LifetimeState, c *corebattle.DamageContext) {
				s.Actors = s.Actors[:1]
				c.EnemyIDs = nil
			}, false},
			{"cursed", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) {
				s.Actors[0].Abilities = []string{"cursed"}
			}, false},
			{"NPC", func(s *corebattle.LifetimeState, _ *corebattle.DamageContext) { s.Actors[0].NPC = true }, false},
			{"NPC context", func(_ *corebattle.LifetimeState, c *corebattle.DamageContext) { c.ActorNPC = true }, false},
		} {
			t.Run(entry.ID+"/"+tc.name, func(t *testing.T) {
				s, c := weaponSealFixture(i + 7)
				tc.edit(&s, &c)
				var d BattleDefinitions
				d.RegisterWeaponSeals(&c)
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				got, err := d.WeaponSealAction(&s, &c, entry.ID, noArithmeticDraws{t})
				if (err == nil) != tc.want {
					t.Fatal(got, err)
				}
				_, err = d.InvocationAction(&s, &frame, []string{entry.ID}, noArithmeticDraws{t})
				if (err == nil) != tc.want {
					t.Fatal(err)
				}
				if !tc.want {
					ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: entry.ID}}
					if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err == nil {
						t.Fatal("execution bypassed admission")
					}
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
					t.Fatal("admission mutated state or counters")
				}
			})
		}
	}
}

func TestWeaponSealRegistrationAndWholeEffectRejection(t *testing.T) {
	s, c := weaponSealFixture(7)
	var d BattleDefinitions
	if _, err := d.WeaponSealAction(&s, &c, "seal-7-breath", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.RegisterWeaponSeals(&c)
	entry := d.Invocations.Entries["seal-7-breath"]
	entry.Children = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "unsupported"}}
	d.Invocations.Entries["seal-7-breath"] = entry
	before, _ := json.Marshal(s)
	if _, err := d.WeaponSealAction(&s, &c, "seal-7-breath", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "seal-7-breath"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) || c.AreaDamageCalls != 0 {
		t.Fatal("rejected descendant changed state")
	}
	// A nontrivial earlier callback has no certified transition projection.
	d.RegisterWeaponSeals(&c)
	if _, err := d.InvocationAction(&s, &frame, []string{"seal-7-breath", "seal-7-breath"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestWeaponSealReloadAndOuterDebitAfterEffect(t *testing.T) {
	for i, entry := range corebattle.WeaponSealEntries() {
		s, c := weaponSealFixture(i + 7)
		s.Actors[0].MP = entry.MPCost * 2
		s.Actors[0].ItemDefinitionIDs = []string{"item-249"}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded corebattle.LifetimeState
		if err := json.Unmarshal(data, &reloaded); err != nil {
			t.Fatal(err)
		}
		var d BattleDefinitions
		d.RegisterWeaponSeals(&c)
		cost, err := d.WeaponSealAction(&reloaded, &c, entry.ID, noArithmeticDraws{t})
		if err != nil || cost.MPCost != entry.MPCost*2 {
			t.Fatal(cost, err)
		}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"last", "unused"}}
		ops, err := d.InvocationAction(&reloaded, &frame, []string{entry.ID}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		if err := reloaded.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(1440)); err != nil {
			t.Fatal(err)
		}
		if reloaded.Actors[0].MP != entry.MPCost*2 || c.DamageCalls != 2 || c.AreaDamageCalls != 1 || !reflect.DeepEqual(frame.Arguments, []string{"last", "unused"}) {
			t.Fatal(reloaded, c, frame)
		}
		// Receipt/version ownership remains in the consumer; the callback never debits MP.
		reloaded.Actors[0].MP = max(0, reloaded.Actors[0].MP-cost.MPCost)
		if reloaded.Actors[0].MP != 0 {
			t.Fatal(reloaded.Actors[0].MP)
		}
	}
}
