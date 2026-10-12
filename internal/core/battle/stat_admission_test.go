package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestStatUpAdmissionFollowsReachableTargets(t *testing.T) {
	for _, item := range []string{"item-201", "item-205"} {
		stat := StatAgility
		if item == "item-205" {
			stat = StatDefense
		}
		for _, tc := range []struct {
			name, target, holder, want string
			tmp                        TemporaryEffect
			status                     string
			activation, boss, reject   bool
			draws                      []float64
		}{
			{name: "unrelated enemy", target: "ally", holder: "enemy", want: "ally"},
			{name: "unrelated actor", target: "ally", holder: "self", want: "ally"},
			{name: "fallen falls back", target: "fallen", holder: "fallen", want: "self"},
			{name: "enemy falls back to party", target: "enemy", holder: "enemy", want: "ally", draws: []float64{.9}},
			{name: "missing falls back to party", target: "missing", holder: "enemy", want: "ally", draws: []float64{.9}},
			{name: "fallback can reach holder", target: "missing", holder: "ally", reject: true},
			{name: "direct holder", target: "ally", holder: "ally", reject: true},
			{name: "reflection leaves holder", target: "ally", holder: "ally", tmp: TemporaryReflectMagic, want: "self"},
			{name: "reflection reaches holder", target: "ally", holder: "self", tmp: TemporaryReflectMagic, reject: true},
			{name: "other category cannot reflect", target: "ally", holder: "self", tmp: TemporaryReflectAttack, want: "ally"},
			{name: "cover leaves holder", target: "ally", holder: "ally", tmp: TemporaryCover, want: "self"},
			{name: "cover reaches holder", target: "ally", holder: "self", tmp: TemporaryCover, reject: true},
			{name: "cover without covering actor", target: "ally", holder: "enemy", tmp: TemporaryCover, want: "ally"},
			{name: "phantom stays in party", target: "ally", holder: "enemy", tmp: TemporaryPhantom, want: "self", draws: []float64{0, .9}},
			{name: "phantom can reach holder", target: "ally", holder: "self", tmp: TemporaryPhantom, reject: true},
			{name: "confusion can reach enemy", target: "ally", holder: "enemy", status: StatusConfusion, reject: true},
			{name: "boss confusion can reach enemy", target: "ally", holder: "enemy", status: StatusConfusion, boss: true, reject: true},
			{name: "activation can reach enemy", target: "ally", holder: "enemy", activation: true, reject: true},
			{name: "cure then activation can reach enemy", target: "ally", holder: "enemy", status: StatusMagicSeal, activation: true, reject: true},
			{name: "other status blocks activation", target: "ally", holder: "enemy", status: StatusPoison, activation: true, want: "ally"},
			{name: "cure without activation", target: "ally", holder: "enemy", status: StatusMagicSeal, want: "ally", draws: []float64{0}},
		} {
			t.Run(item+"/"+tc.name, func(t *testing.T) {
				s, c := recoveryFixture()
				s.actor(tc.holder).ItemDefinitionIDs = []string{item}
				s.actor("self").Status = tc.status
				s.actor("ally").Temporary = tc.tmp
				if tc.tmp == TemporaryCover && tc.holder != "enemy" {
					s.actor("self").Temporary = TemporaryCovering
				}
				c.Item195Activated, s.SpecialBossField = tc.activation, tc.boss
				before := reloadLifetime(t, s)
				rng := &targetRNG{t: t, draws: tc.draws}
				r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: tc.target, Category: CategoryMagic, Payload: CallbackPayload{Number: .25}}, stat, rng)
				if tc.reject {
					if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.StatUpCalls != 0 {
						t.Fatal(r, err, s, c)
					}
				} else if err != nil || !r.Applied || r.TargetID != tc.want || *s.actor(tc.want).currentStat(stat) != 175 {
					t.Fatal(r, err, s)
				}
				rng.done()
			})
		}
	}
}

func TestStatUpDeflectionAdmissionAndDraws(t *testing.T) {
	for _, reject := range []bool{false, true} {
		s, c := recoveryFixture()
		s.actor("ally").Temporary = TemporaryDeflect
		if reject {
			s.actor("enemy").ItemDefinitionIDs = []string{"item-201"}
		}
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		if !reject {
			rng.draws = []float64{.9, 0}
		}
		r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Category: CategoryPhysical, Payload: CallbackPayload{Number: .25}}, StatAgility, rng)
		if reject {
			if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.StatUpCalls != 0 {
				t.Fatal(r, err, s, c)
			}
		} else if err != nil || !r.Applied || r.TargetID != "enemy" {
			t.Fatal(r, err)
		}
		rng.done()
	}
}

func TestCompositeStatAdmissionBeforeAnyEffects(t *testing.T) {
	for _, distributed := range []bool{false, true} {
		for _, item := range []string{"item-201", "item-205"} {
			for _, reachable := range []bool{false, true} {
				s, c := recoveryFixture()
				holder := "enemy"
				if reachable {
					holder = "ally"
				}
				s.actor(holder).ItemDefinitionIDs = []string{item}
				s.actor("self").Attack = 200
				before := reloadLifetime(t, s)
				rng := &targetRNG{t: t}
				var err error
				if distributed {
					if !reachable {
						rng.draws = []float64{.9, .9, .9, .9}
					}
					_, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, rng)
				} else {
					stat := StatAgility
					if item == "item-205" {
						stat = StatDefense
					}
					_, err = s.ApplyPartyStat(&c, .25, CategoryMagic, stat, false, rng)
				}
				if reachable {
					if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.PartyStatUpCalls != 0 || c.StatUpCalls != 0 {
						t.Fatal(distributed, item, err, s, c)
					}
				} else if err != nil || reflect.DeepEqual(s, before) {
					t.Fatal(distributed, item, err, s)
				}
				rng.done()
			}
		}
	}
}

func TestItem201DamageCounterControlsFollowingEffects(t *testing.T) {
	for _, item := range []string{"item-239", "item-256"} {
		for _, counted := range []bool{false, true} {
			t.Run(item+map[bool]string{false: "/early return", true: "/counted"}[counted], func(t *testing.T) {
				s, c := recoveryFixture()
				s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
				s.actor("self").ItemDefinitionIDs = []string{item}
				called := 0
				c.Damage = func(ctx TargetContext, hit TargetEffect, rng random.Generator) error {
					called++
					if c.DamageCalls != 0 || s.actor("ally").Agility != 175 || hit.Payload.Number != 87.5 {
						t.Fatal(c, hit, s)
					}
					// The damage owner counts after target/zero gates, even for a miss.
					if counted {
						c.DamageCalls++
					}
					return nil
				}
				rng := &targetRNG{t: t}
				r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .25}}, StatAgility, rng)
				wantCount := 0
				if counted {
					wantCount = 1
				}
				if err != nil || !r.Applied || called != 1 || c.DamageCalls != wantCount {
					t.Fatal(r, err, called, c)
				}
				if item != "item-239" || !counted {
					rng.draws = []float64{0}
				}
				heal, err := s.ApplyHealing(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 100}}, rng)
				if err != nil || heal.Applied != (item != "item-239" || !counted) {
					t.Fatal(heal, err)
				}
				wantSplash := item == "item-256" && !counted
				if (len(heal.IncidentalHealing) > 0) != wantSplash || (c.PartyHealCalls > 0) != wantSplash || c.DamageCalls != wantCount {
					t.Fatal(heal, c)
				}
				rng.done()
			})
		}
	}
}
