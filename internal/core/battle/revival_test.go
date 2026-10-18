package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func revivalFixture() (LifetimeState, RevivalContext) {
	s, target := targetFixture()
	for i := range s.Actors {
		s.Actors[i].MaxHP = 101
		s.Actors[i].MaxMP = 100
		s.Actors[i].Gear = &BattleGearStats{}
	}
	return s, RevivalContext{TargetContext: target}
}

func TestRevivalSingleFormulasAndChances(t *testing.T) {
	for _, tc := range []struct {
		id    string
		hp    int
		draws []float64
	}{
		{"tool-6", 101, nil}, {"book-173", 50, []float64{.499}}, {"book-173", 0, []float64{.5}},
		{"book-196", 101, nil}, {"gem-63", 50, nil}, {"gem-64", 101, nil},
		{"job-5-entry-7", 50, []float64{0}}, {"job-39-entry-5", 0, []float64{.9}},
		{"job-16-entry-6", 25, nil}, {"job-17-entry-6", 101, nil}, {"job-18-entry-6", 101, nil},
		{"job-33-entry-6", 101, nil}, {"job-43-entry-6", 101, nil},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := revivalFixture()
			s.actor("fallen").Status, s.actor("fallen").Temporary = StatusPoison, TemporaryDefense
			rng := &targetRNG{t: t, draws: tc.draws}
			if _, err := s.ApplyRevival(&c, tc.id, "fallen", rng); err != nil || s.actor("fallen").HP != tc.hp {
				t.Fatal(s, err)
			}
			if s.actor("fallen").Status != StatusPoison || s.actor("fallen").Temporary != TemporaryDefense {
				t.Fatal("direct revival reset effects")
			}
			rng.done()
			s.actor("self").Status = ""
			if _, err := s.ApplyRevival(&c, tc.id, "ally", &targetRNG{t: t}); err != nil || s.actor("ally").HP != 100 {
				t.Fatal(err)
			}
		})
	}
}

func TestRevivalPartyGuardsAndIndependentDraws(t *testing.T) {
	for _, id := range []string{"gem-65", "job-18-entry-4", "job-70-entry-3", "job-51-entry-5", "extension-261", "job-32-entry-4"} {
		t.Run(id, func(t *testing.T) {
			s, c := revivalFixture()
			s.actor("self").Status = StatusMagicSeal
			s.actor("ally").HP = 0
			var draws []float64
			want1, want2 := 50, 0
			switch id {
			case "extension-261":
				want1, want2 = 101, 0
				s.actor("ally").Abilities = []string{"cursed"}
			case "job-32-entry-4":
				want1, want2, draws = 15, 15, []float64{0}
			case "job-51-entry-5":
				want1, draws = 101, []float64{.199, .2}
			default:
				draws = []float64{.499, .5}
			}
			rng := &targetRNG{t: t, draws: draws}
			if _, err := s.ApplyRevival(&c, id, "", rng); err != nil || s.actor("fallen").HP != want1 || s.actor("ally").HP != want2 {
				t.Fatal(s, err)
			}
			rng.done()
			if id == "extension-261" && !hasAbility(s.actor("fallen").Abilities, "cursed") {
				t.Fatal("missing curse")
			}
			if id != "job-32-entry-4" && s.actor("self").Status != StatusMagicSeal {
				t.Fatal("invented caster seal check")
			}
		})
	}
}

func TestRevivalZeroFloorLibraryAndTargetMP(t *testing.T) {
	for _, hp := range []int{0, 1, 3, 4} {
		s, c := revivalFixture()
		s.actor("fallen").MaxHP = hp
		if _, err := s.ApplyRevival(&c, "job-16-entry-6", "fallen", &targetRNG{t: t}); err != nil || s.actor("fallen").HP != hp/4 {
			t.Fatal(s, err)
		}
	}
	for _, field := range []string{"", "library"} {
		s, c := revivalFixture()
		s.Fields.Default = field
		if _, err := s.ApplyRevival(&c, "book-196", "fallen", &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		if (s.actor("self").Status == StatusMagicSeal) != (field != "library") {
			t.Fatal(s)
		}
	}
	for _, mp := range []int{0, 1, 40, 41, 201} {
		s, c := revivalFixture()
		s.actor("fallen").MP = mp
		if _, err := s.ApplyRevival(&c, "job-75-entry-5", "fallen", &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		want := mp
		if mp < 1 {
			want = 0
		}
		if s.actor("fallen").HP != want || s.actor("fallen").MP != mp/2 {
			t.Fatal(s)
		}
	}
}

func TestHelperRevivalBranchesBanAndReload(t *testing.T) {
	for _, hp := range []int{0, 100} {
		for _, mp := range []int{0, 1, 40, 41} {
			for _, helperHP := range []int{-1, 0, 1, 8} {
				s, c := revivalFixture()
				s.actor("self").HP, s.actor("self").MP = hp, mp
				if helperHP >= 0 {
					s.actor("ally").RosterIdentity, s.actor("ally").HP = "calling-66", helperHP
				}
				r, err := s.ApplyRevival(&c, "job-66-revive", "fallen", &targetRNG{t: t})
				if err != nil {
					t.Fatal(err)
				}
				applied := helperHP > 0 && (hp <= 0 || mp > 40)
				if r.Applied != applied {
					t.Fatal(hp, mp, helperHP, r)
				}
				if applied {
					id, wantHP, wantMP := "fallen", 1, mp-40
					if hp <= 0 {
						id, wantHP, wantMP = "self", 50, mp
					}
					if s.actor(id).HP != wantHP || s.actor(id).Status != StatusDofuu || s.actor("self").MP != wantMP || s.actor("ally").HP != helperHP/2 || s.SkillBans["job-66-revive"] != 3 {
						t.Fatal(s)
					}
					s = reloadLifetime(t, s)
					if err := s.ValidateRevivalAction(&c, "job-66-revive", "fallen", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
						t.Fatal("ban bypass", err)
					}
					for range 3 {
						s.TickSkillBans()
					}
					if err := s.ValidateRevivalAction(&c, "job-66-revive", "fallen", &targetRNG{t: t}); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestSacrificeAdmissionAndOrder(t *testing.T) {
	for _, id := range []string{"gem-66", "job-3-entry-6", "job-14-entry-4", "job-17-entry-4", "job-76-entry-3", "job-50-entry-6", "job-7-entry-8"} {
		t.Run(id, func(t *testing.T) {
			s, c := revivalFixture()
			before := reloadLifetime(t, s)
			if _, err := s.ApplyRevival(&c, id, "ally", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
				t.Fatal("missing closure admitted", err)
			}
			var order []string
			c.Defeat = func(target string) error {
				order = append(order, "defeat")
				if target != "self" || s.actor(target).HP != 100 {
					t.Fatal("sacrifice pre-cleared HP", s)
				}
				if id != "job-7-entry-8" && s.actor("ally").HP != 101 {
					t.Fatal("defeat before restoration", s)
				}
				if id == "job-50-entry-6" && s.actor("ally").MP != 100 {
					t.Fatal(s)
				}
				s.actor(target).HP, s.actor(target).MP = 20, 50 // automatic revival protection
				return nil
			}
			c.Deaths = &RevivalDeathCallbacks{
				Validate: func(_ *LifetimeState, _ TargetContext, category CallbackCategory, chance float64) error {
					if category != CategoryUntyped || chance != 60 {
						t.Fatal(category, chance)
					}
					return nil
				},
				Execute: func(_ *LifetimeState, _ TargetContext, _ CallbackCategory, _ float64, _ random.Generator) error {
					order = append(order, "deaths")
					return nil
				},
			}
			if _, err := s.ApplyRevival(&c, id, "ally", &targetRNG{t: t}); err != nil {
				t.Fatal(err)
			}
			wantMP := 1
			if id == "gem-66" {
				wantMP = 0
			}
			if s.actor("self").HP != 20 || s.actor("self").MP != wantMP || order[len(order)-1] != "defeat" {
				t.Fatal(s, order)
			}
			if id == "job-7-entry-8" && !reflect.DeepEqual(order, []string{"deaths", "defeat"}) {
				t.Fatal(order)
			}
		})
	}
}
