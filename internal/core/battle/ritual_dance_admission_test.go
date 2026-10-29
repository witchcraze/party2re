package battle

import "testing"

func TestGroupDanceResetBoundsPreserveModifierOrder(t *testing.T) {
	// Source damage modifiers run before an optional item190 direct conversion.
	// Random modifier endpoints must combine a192's high roll with a weapon's
	// low activation roll; low-HP weapon bounds must leave input HP unchanged.
	for _, row := range []struct {
		name    string
		items   []string
		tmp     TemporaryEffect
		power   float64
		direct  bool
		maximum int
	}{
		{"ordinary then direct", []string{"item-149", "item-190"}, "", 120, false, 324},
		{"direct", []string{"item-149", "item-190"}, "", 100, true, 198},
		{"combined draws", []string{"item-192", "weapon-37"}, TemporaryDefense, 50, true, 160},
		{"earlier HP loss", []string{"weapon-68"}, "", 100, true, 215},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c := damageFixture()
			a, target := &s.Actors[0], &s.Actors[1]
			a.ItemDefinitionIDs, a.HP, a.MaxHP, a.Tension = row.items, 500, 1000, 1
			target.Temporary = row.tmp
			e := groupDanceEffect(a, false, row.direct)
			e.Payload.Number = row.power
			for _, hp := range []int{row.maximum, row.maximum + 1} {
				target.HP = hp
				if got := s.groupDanceMayReset(&c, target, e); got != (hp == row.maximum) || a.HP != 500 || target.HP != hp {
					t.Fatal(row, hp, got, s)
				}
			}
		})
	}
}
