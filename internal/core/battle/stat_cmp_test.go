package battle

import "testing"

func TestCMPReductionThresholdImmunityAndUnclampedDebit(t *testing.T) {
	// _skill.cgi:9252–9290: mc_mp supplies the stop threshold, but c_mp
	// deliberately skips the final floor clamp and the ordinary MP debit cap.
	for _, row := range []struct {
		cmp          int
		tension      float64
		item         string
		want, amount int
	}{
		{20, 1, "", 20, 0}, {21, 1, "", 17, 4}, {100, 1, "", 80, 20},
		{100, 8, "", -60, 160}, {100, 1, "item-159", 100, 0},
	} {
		s, c := recoveryFixture()
		s.actor("self").Tension = row.tension
		s.actor("enemy").CMP, s.actor("enemy").MaxCMP = row.cmp, 100
		if row.item != "" {
			s.actor("enemy").ItemDefinitionIDs = []string{row.item}
		}
		r, err := s.ApplyStatDown(&c, controlEffect("enemy", .2, CategoryUntyped), StatCMP, &targetRNG{t: t})
		if err != nil || s.actor("enemy").CMP != row.want || r.Amount != row.amount {
			t.Fatal(row, r, err, s)
		}
	}
}
