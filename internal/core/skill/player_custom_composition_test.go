package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// The registered 94/28/74 sequence validates the later slots on actual state
// after its unknown numeric predecessor. Rejection restores the whole action.
func TestPlayerCustomRecoveryComposition(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requester bool
		draws     []float64
		wantErr   error
	}{
		{"sealed no-effect", false, []float64{.9, .9}, nil},
		{"cure with requester", true, []float64{0, 0, 0, 0}, nil},
		{"cure without requester", false, []float64{0, 0, 0, 0}, corebattle.ErrUnsupportedEffect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryGemFixture(false)
			c.Item195Activated = true
			if !tc.requester {
				c.RequesterLevel = nil
			}
			s.Actors[0].Status, s.Actors[0].Temporary, s.Actors[0].Tension = corebattle.StatusPhysicalSeal, corebattle.TemporaryCover, 8
			s.Actors[2].Temporary, s.Actors[2].Tension = corebattle.TemporaryCovering, 1.7
			s.Actors[3].Tension = 5
			s.Actors[0].Custom = &corebattle.CustomLoadout{Name: "custom", Gems: [3]string{"gem_debuff_def", "gem_buff_atk_1", "gem_inspire_sky"}, Cost: 43}
			var d BattleDefinitions
			d.RegisterRecoveryStatGems(&c)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			before, _ := json.Marshal(s)
			frameBefore := f
			if _, _, err := d.PlayerCustomAction(&s, &f, nil, noArithmeticDraws{t}); err != nil {
				t.Fatal(err)
			}
			rng := &damageGemDraws{t: t, draws: tc.draws}
			if err := d.ApplyPlayerCustomAction(&s, &f, nil, rng); !errors.Is(err, tc.wantErr) {
				t.Fatal(err)
			}
			if tc.wantErr != nil {
				after, _ := json.Marshal(s)
				if string(before) != string(after) || !reflect.DeepEqual(f, frameBefore) {
					t.Fatal("rejected custom action changed state, context or cost")
				}
			} else if s.Actors[0].CMP != 157 || s.Actors[3].Defense != 20 || s.Actors[0].Tension != 1 || s.Actors[2].Tension != 3 || c.TensionCalls != 1 || rng.used != len(tc.draws) {
				t.Fatal(s, c, rng.used)
			}
		})
	}
}

func TestPlayerCustomFallenEnemyAndRequester(t *testing.T) {
	for _, requester := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		s.Actors[0].Tension = 3
		dead := s.Actors[3].Clone()
		dead.ID, dead.HP = "dead-enemy", 0
		s.Actors = append(s.Actors, dead)
		c.EnemyIDs = append(c.EnemyIDs, dead.ID)
		if !requester {
			c.RequesterLevel = nil
		}
		s.Actors[0].Custom = &corebattle.CustomLoadout{Name: "custom", Gems: [3]string{"gem_mind_eye", "gem_calm_orb", "gem_divine_tension_sky"}, Cost: 93}
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{dead.ID}}
		if !requester {
			before, _ := json.Marshal(s)
			if err := d.ApplyPlayerCustomAction(&s, &f, nil, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, _ := json.Marshal(s)
			if string(before) != string(after) || c.TensionCalls != 0 {
				t.Fatal("rejected fallen-target branch changed state")
			}
			continue
		}
		rng := &damageGemDraws{t: t, draws: []float64{0}}
		if err := d.ApplyPlayerCustomAction(&s, &f, nil, rng); err != nil || s.Actors[0].CMP != 107 || s.Actors[0].Tension != 8 || s.Actors[0].Hit != 95 || rng.used != 1 {
			t.Fatal(err, s, rng.used)
		}
	}
}

func TestPlayerCustomSacrificeRevivalAndLateCost(t *testing.T) {
	for _, protect := range []bool{false, true} {
		s, c, d := revivalRegistryFixture()
		s.Actors[0].CMP = 50
		s.Actors[0].MP, s.Actors[0].MaxMP = 81, 100
		s.Actors[2].Status = corebattle.StatusSleep
		if protect {
			s.Actors[0].Abilities = []string{"pharaoh"}
		}
		s.Actors[0].Custom = &corebattle.CustomLoadout{Name: "custom", Gems: [3]string{"gem_sacrifice_soul", "gem_cure_sleep", ""}, Cost: 25}
		c.NativeDefeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}}
		d.RegisterRevivals(&c)
		status := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext}}
		d.RegisterStatuses(&status, nil)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen"}}
		if err := d.ApplyPlayerCustomAction(&s, &f, nil, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
		if s.Actors[0].CMP != 25 || s.Actors[0].MP != 0 || s.Actors[2].HP != 101 || (s.Actors[2].Status == "") != protect || (s.Actors[0].HP > 0) != protect {
			t.Fatal(protect, s)
		}
	}
}

func TestPlayerCustomCMPBurstChargesAfterInternalWrite(t *testing.T) {
	s, c, d := playerCustomFixture([3]string{"gem_cmp_burst", "", ""}, 7)
	d.RegisterResources(c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if err := d.ApplyPlayerCustomAction(&s, &f, nil, &damageGemDraws{t: t, draws: []float64{0}}); err != nil || s.Actors[0].CMP != 0 || s.Actors[1].HP != 717 {
		t.Fatal(err, s)
	}
}
