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

func TestStatUpAdmissionNoEffectGuards(t *testing.T) {
	for _, item := range []string{"item-201", "item-205"} {
		stat := StatAgility
		if item == "item-205" {
			stat = StatDefense
		}
		for _, tc := range []struct {
			name, target, capItem string
			value                 int
			bound, zero, reject   bool
			additional            bool
		}{
			{name: "at 2.5 cap", target: "ally", value: 250},
			{name: "above 2.5 cap", target: "ally", value: 251},
			{name: "below 2.5 cap", target: "ally", value: 249, reject: true},
			{name: "at 3 cap", target: "self", capItem: "item-164", value: 300},
			{name: "below 3 cap", target: "self", capItem: "item-164", value: 299, reject: true},
			{name: "2.5 is below self 3 cap", target: "self", capItem: "item-164", value: 250, reject: true},
			{name: "cap item on actor does not raise ally cap", target: "ally", capItem: "item-164", value: 250},
			{name: "183 at 3 cap", target: "self", capItem: "item-183", value: 300},
			{name: "183 raises only AG cap", target: "self", capItem: "item-183", value: 299, reject: stat == StatAgility},
			{name: "bound", target: "ally", value: 150, bound: true},
			{name: "zero power", target: "ally", value: 150, zero: true},
			{name: "additional effect at cap", target: "ally", value: 250, additional: true},
		} {
			t.Run(item+"/"+tc.name, func(t *testing.T) {
				s, c := recoveryFixture()
				a := s.actor(tc.target)
				a.ItemDefinitionIDs = []string{item}
				*s.actor(tc.target).currentStat(stat) = tc.value
				if tc.capItem != "" {
					s.actor("self").ItemDefinitionIDs = append(s.actor("self").ItemDefinitionIDs, tc.capItem)
				}
				if tc.bound {
					a.Status = StatusSabaku
				}
				c.AdditionalEffect = tc.additional
				power := .25
				if tc.zero {
					power = 0
				}
				before := reloadLifetime(t, s)
				contextBefore := c
				rng := &targetRNG{t: t}
				r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: tc.target, Payload: CallbackPayload{Number: power}}, stat, rng)
				if tc.reject {
					if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(c, contextBefore) {
						t.Fatal(r, err, c)
					}
				} else if err != nil || r.Applied || r.Returned != (!tc.zero && !tc.additional) || c.StatUpCalls != map[bool]int{true: 0, false: 1}[tc.zero] {
					t.Fatal(r, err, c)
				}
				if !reflect.DeepEqual(s, before) {
					t.Fatal("no-effect/rejection changed state", r, s)
				}
				rng.done()
			})
		}
	}
}

func TestStatUpAdmissionMixedReachableCaps(t *testing.T) {
	for _, path := range []string{"fallback", "confusion", "activation", "reflection", "cover", "phantom"} {
		for _, item := range []string{"item-201", "item-205"} {
			for _, uncapped := range []bool{false, true} {
				s, c := recoveryFixture()
				stat := StatAgility
				if item == "item-205" {
					stat = StatDefense
				}
				for i := range s.Actors {
					s.Actors[i].ItemDefinitionIDs = []string{item}
					*s.Actors[i].currentStat(stat) = 250
				}
				e := TargetEffect{TargetID: "missing", Category: CategoryMagic, Payload: CallbackPayload{Number: .25}}
				rng := &targetRNG{t: t}
				switch path {
				case "fallback":
					rng.draws = []float64{.9}
				case "confusion":
					s.actor("self").Status = StatusConfusion
					e.TargetID = "ally"
					rng.draws = []float64{.4}
				case "activation":
					c.Item195Activated = true
					s.actor("self").Status = StatusMagicSeal
					e.TargetID = "ally"
					rng.draws = []float64{0, 0, .4}
				case "reflection":
					e.TargetID = "ally"
					s.actor("ally").Temporary = TemporaryReflectMagic
				case "cover":
					e.TargetID = "ally"
					s.actor("ally").Temporary = TemporaryCover
					s.actor("self").Temporary = TemporaryCovering
				case "phantom":
					e.TargetID = "ally"
					s.actor("ally").Temporary = TemporaryPhantom
					rng.draws = []float64{0, .9}
				}
				if uncapped {
					*s.actor("self").currentStat(stat) = 249
					rng.draws = nil
				}
				before := reloadLifetime(t, s)
				contextBefore := c
				r, err := s.ApplyStatUp(&c, e, stat, rng)
				if uncapped {
					if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
						t.Fatal(path, item, r, err, s, c)
					}
				} else if err != nil || r.Applied || !r.Returned || c.StatUpCalls != 1 {
					t.Fatal(path, item, r, err, c)
				}
				rng.done()
			}
		}
	}
}

func TestStatUpAdmissionZeroPathsPreserveModifierOrder(t *testing.T) {
	for _, item := range []string{"item-201", "item-205"} {
		for _, guard := range []string{"null magic", "darkness", "239 after 186", "fallen cover"} {
			t.Run(item+"/"+guard, func(t *testing.T) {
				s, c := recoveryFixture()
				stat := StatAgility
				if item == "item-205" {
					stat = StatDefense
				}
				s.actor("ally").ItemDefinitionIDs = []string{item}
				e := TargetEffect{TargetID: "ally", Category: CategoryMagic, Payload: CallbackPayload{Number: .25}}
				rng := &targetRNG{t: t}
				wantCount := 1
				switch guard {
				case "null magic":
					s.actor("ally").Temporary = TemporaryNullMagic
					wantCount = 0
				case "darkness":
					s.Fields.Default = "darkness"
				case "239 after 186":
					s.actor("self").ItemDefinitionIDs = []string{"item-186", "item-239"}
					c.DamageCalls = 1
					// Tool healing still runs before239 zeros the stat-up power.
					rng.draws = []float64{0}
				case "fallen cover":
					s.actor("ally").Temporary = TemporaryCover
					s.actor("fallen").Temporary = TemporaryCovering
					s.actor("fallen").ItemDefinitionIDs = []string{item}
					wantCount = 0
				}
				before := reloadLifetime(t, s)
				r, err := s.ApplyStatUp(&c, e, stat, rng)
				if err != nil || r.Applied || r.Returned || c.StatUpCalls != wantCount {
					t.Fatal(r, err, c)
				}
				if guard == "239 after 186" {
					if len(r.IncidentalHealing) != 1 || r.IncidentalHealing[0].Delta != 22 || s.actor("ally").HP != 122 || *s.actor("ally").currentStat(stat) != 150 {
						t.Fatal(r, s)
					}
				} else if !reflect.DeepEqual(s, before) {
					t.Fatal(r, s)
				}
				rng.done()
			})
		}
	}
}

func TestStatUpAdmissionRoundedZero(t *testing.T) {
	for _, item := range []string{"item-201", "item-205"} {
		for _, doubled := range []bool{false, true} {
			s, c := recoveryFixture()
			stat := StatAgility
			if item == "item-205" {
				stat = StatDefense
			}
			a := s.actor("ally")
			a.ItemDefinitionIDs = []string{item}
			a.BaseStats = BattleBaseStats{Attack: 1, Defense: 1, Agility: 1}
			a.Attack, a.Defense, a.Agility = 1, 1, 1
			if doubled {
				a.Temporary = TemporaryDouble
			}
			before := reloadLifetime(t, s)
			rng := &targetRNG{t: t}
			r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .5}}, stat, rng)
			if doubled {
				if !errors.Is(err, ErrUnsupportedEffect) || c.StatUpCalls != 0 {
					t.Fatal(r, err, c)
				}
			} else if err != nil || r.Returned || r.Applied || c.StatUpCalls != 1 {
				t.Fatal(r, err, c)
			}
			if !reflect.DeepEqual(s, before) {
				t.Fatal(r, s)
			}
			rng.done()
		}
	}
}

func TestCompositeStatAdmissionAccountsForEarlier205(t *testing.T) {
	for _, distributed := range []bool{false, true} {
		for _, prefix205 := range []bool{false, true} {
			s, c := recoveryFixture()
			level := 99
			c.RequesterLevel = &level
			s.actor("ally").BaseStats.Agility, s.actor("ally").Agility = 1, 1
			s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
			if distributed && prefix205 {
				s.actor("ally").ItemDefinitionIDs = append(s.actor("ally").ItemDefinitionIDs, "item-205")
			} else if prefix205 {
				s.actor("self").ItemDefinitionIDs = []string{"item-205"}
			}
			if distributed {
				s.actor("ally").Temporary = TemporaryStrongDefense
			}
			before := reloadLifetime(t, s)
			contextBefore := c
			rng := &targetRNG{t: t}
			if distributed && !prefix205 {
				rng.draws = []float64{.9, 0, 0, .9}
			}
			var err error
			if distributed {
				_, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, rng)
			} else {
				_, err = s.ApplyPartyStat(&c, .75, CategoryMagic, StatAgility, false, rng)
			}
			if prefix205 {
				if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
					t.Fatal(distributed, err, s, c)
				}
			} else if err != nil || s.actor("ally").Agility != 1 {
				t.Fatal(distributed, err, s, c)
			}
			rng.done()
		}
	}
}
