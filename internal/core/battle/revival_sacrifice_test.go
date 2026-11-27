package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestSacrificeGemAdmitsMissingContextOnlyWhenUnreachable(t *testing.T) {
	s, c := revivalFixture()
	s.actor("self").Abilities = []string{"pharaoh"}
	s.actor("self").MP = 81
	c.NativeDefeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return ErrUnsupportedEffect },
		Apply:    func(*LifetimeState, TargetContext, string) error { t.Fatal("validation called Apply"); return nil },
	}}
	if err := s.ValidateRevival(&c, "gem_sacrifice_soul", "", &targetRNG{t: t}); err != nil || s.actor("fallen").HP != 0 {
		t.Fatal(err, s)
	}
	s.actor("self").Gear.AccessoryID = "item-110"
	before := s.callbackCopy()
	if _, err := s.ApplyRevival(&c, "gem_sacrifice_soul", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal("unsupported reset ran after healing", err, s)
	}
}
