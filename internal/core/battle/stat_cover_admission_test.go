package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestCompositeStatAdmissionTracks205CoverChanges(t *testing.T) {
	for _, distributed := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			reject bool
		}{
			{"covering removed", true},
			{"covering removed with seal", true},
			{"205 at cap", false},
			{"205 bound", false},
			{"zero power", false},
			{"201 at cap", false},
			{"second stable covering", false},
			{"unrelated enemy 201", false},
		} {
			t.Run(map[bool]string{false: "party/", true: "distributed/"}[distributed]+tc.name, func(t *testing.T) {
				s, c := recoveryFixture()
				level := 99
				c.RequesterLevel = &level
				self, ally := s.actor("self"), s.actor("ally")
				self.ItemDefinitionIDs, self.Temporary = []string{"item-205"}, TemporaryCovering
				ally.ItemDefinitionIDs, ally.Temporary = []string{"item-201"}, TemporaryCover
				power := .25
				switch tc.name {
				case "covering removed with seal":
					self.Status = StatusMagicSeal
					if distributed {
						self.Status = StatusDanceSeal
					}
				case "205 at cap":
					self.Defense, self.Agility = 250, 250
				case "205 bound":
					self.Status = StatusSabaku
				case "zero power":
					power = 0
					if distributed {
						s.Fields.Default = "darkness"
					}
				case "201 at cap":
					ally.Agility = 250
				case "second stable covering":
					// Fallen covering still wins before the original target's HP guard.
					s.actor("fallen").Temporary = TemporaryCovering
				case "unrelated enemy 201":
					ally.ItemDefinitionIDs = nil
					s.actor("enemy").ItemDefinitionIDs = []string{"item-201"}
				}
				before := reloadLifetime(t, s)
				contextBefore := c
				rng := &targetRNG{t: t}
				if distributed && !tc.reject {
					// Take DF and AG tails; the outer operation must admit them first.
					rng.draws = []float64{.9, 0, 0, .9}
				}
				var err error
				if distributed {
					_, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, rng)
				} else {
					_, err = s.ApplyPartyStat(&c, power, CategoryMagic, StatAgility, false, rng)
				}
				if tc.reject {
					if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
						t.Fatal(err, s, c)
					}
				} else if err != nil {
					t.Fatal(err, s, c)
				}
				rng.done()
			})
		}
	}
}
