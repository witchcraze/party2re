package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Each passive identity goes through the registered discovery gate and native
// body. Formula boundaries are checked at core/battle, rather than repeated here.
func TestPlayerTailManifestDiscoveryAndExecution(t *testing.T) {
	for _, row := range []struct {
		id         string
		items      []string
		hp, mp, ag int
		draws      []float64
	}{
		{"item-189-normal-tail", []string{"item-189"}, 100, 235, 100, []float64{0, 0, 0}},
		{"item-210-revival-tail", nil, 370, 100, 100, []float64{0}},
		{"item-183-combat-tail", []string{"item-183"}, 100, 100, 120, []float64{0}},
		{"item-194-combat-tail", []string{"item-194"}, 100, 100, 100, nil},
		{"item-202-combat-tail", []string{"item-202"}, 136, 100, 100, []float64{0}},
		{"item-198-combat-tail", []string{"item-198"}, 100, 118, 100, []float64{0}},
		{"weapon-35-armor-37-tail", []string{"weapon-35", "armor-37"}, 190, 100, 100, []float64{0}},
		{"weapon-69-armor-54-tail", []string{"weapon-69", "armor-54"}, 100, 100, 100, []float64{0}},
		{"weapon-70-armor-55-tail", []string{"weapon-70", "armor-55"}, 100, 100, 100, []float64{0}},
		{"weapon-30-tail", []string{"weapon-30"}, 118, 100, 100, []float64{0, 0}},
		{"weapon-54-tail", []string{"weapon-54"}, 118, 100, 100, []float64{0, 0}},
		{"cmp-recharge-tail", nil, 100, 100, 100, []float64{0, 0}},
		{"frenzy-tail", nil, 55, 100, 100, []float64{0, 0}},
		{"crown-tail", nil, 172, 100, 100, []float64{0, 0, 0}},
	} {
		t.Run(row.id, func(t *testing.T) {
			s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
				{Participant: corebattle.Participant{ID: "actor", TeamID: "party", HP: 100, MaxHP: 1000, MP: 100, MaxMP: 1000, MaxCMP: 100, Attack: 100, Defense: 100, Agility: 100, BaseStats: corebattle.BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}, Gear: &corebattle.BattleGearStats{}, ItemDefinitionIDs: row.items}, Tension: 1, CMPCharge: 3, CMPNeedCharge: 3},
				{Participant: corebattle.Participant{ID: "requester", TeamID: "party", HP: 1000, MaxHP: 1000, Gear: &corebattle.BattleGearStats{}}},
				{Participant: corebattle.Participant{ID: "enemy", TeamID: "other", HP: 1000, MaxHP: 1000, Gear: &corebattle.BattleGearStats{}}},
			}}
			level := 50
			c := corebattle.PlayerTailContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{
				TargetContext: corebattle.TargetContext{ActorID: "actor", PartyIDs: []string{"actor", "requester"}, EnemyIDs: []string{"enemy"}}, DamageCalls: 1,
			}}, Completion: corebattle.LifetimeCompletion{Accepted: true, ActorID: "actor", EnemiesRemain: true}, NormalAction: true}
			c.RequesterLevel = &level
			if row.id == "item-210-revival-tail" {
				c.Item210Fallen = []string{"actor"}
			}
			if row.id == "item-194-combat-tail" {
				c.DefeatContext = &corebattle.DefeatContext{}
				s.Actors[0].Gear.AccessoryID = "item-194"
			}
			if row.id == "frenzy-tail" {
				s.Actors[0].Extended = corebattle.ExtendedFrenzy
			}
			if row.id == "crown-tail" {
				s.Actors[0].Extended = corebattle.ExtendedCrown
			}
			var d BattleDefinitions
			if err := d.PlayerTailAction(&s, &c, row.id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			d.RegisterPlayerTails()
			if len(d.PlayerTails) != 14 || len(d.Invocations.Entries) != 0 || len(d.Invocations.JobTables) != 0 {
				t.Fatal(d)
			}
			before, context := s.Actors[0], c
			if err := d.PlayerTailAction(&s, &c, row.id, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(before, s.Actors[0]) || !reflect.DeepEqual(context, c) {
				t.Fatal(err, s, c)
			}
			rng := &damageGemDraws{t: t, draws: row.draws}
			if err := s.ApplyPlayerTail(&c, row.id, rng); err != nil {
				t.Fatal(err)
			}
			a := s.Actors[0]
			if a.HP != row.hp || a.MP != row.mp || a.Agility != row.ag || rng.used != len(row.draws) {
				t.Fatal(a, rng.used)
			}
			if (row.id == "weapon-69-armor-54-tail" || row.id == "weapon-70-armor-55-tail") && a.Tension != 1.7 {
				t.Fatal(a)
			}
			if row.id == "cmp-recharge-tail" && (c.Completion.CMPAmount != 1 || c.Completion.NextCMPThreshold != 3) {
				t.Fatal(c)
			}
		})
	}
}

func TestPlayerTailDiscoveryPreservesUnsupportedAndIneligible(t *testing.T) {
	s, base, _ := dualAgilityFixture()
	c := corebattle.PlayerTailContext{DamageContext: base.DamageContext, Completion: corebattle.LifetimeCompletion{Accepted: true, ActorID: "self", EnemiesRemain: true}}
	s.Actors[0].ItemDefinitionIDs, s.Actors[0].Attack, s.Actors[0].HP = []string{"item-194"}, 0, 100
	var d BattleDefinitions
	d.RegisterPlayerTails()
	if err := d.PlayerTailAction(&s, &c, "item-194-combat-tail", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	c.Completion.Suppressed = true
	if err := d.PlayerTailAction(&s, &c, "item-194-combat-tail", noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"item-110", "gem-101", "gem-102"} {
		d.PlayerTails[id] = true
		if err := d.PlayerTailAction(&s, &c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
}
