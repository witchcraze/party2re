package character

import "testing"

func TestValidateNeedJoin(t *testing.T) {
	char := Character{JobLevel: 3}
	char.Stats.HP = 1
	char.Stats.MaxHP = 100

	for _, tc := range []struct {
		condition string
		wantError string
	}{
		{"", ""},
		{"0", ""},
		{" \t", ""},
		{" hp_100_u ", ""},
		{"hp_100", ""},
		{"hp_100_u_extra", ""},
		{"hp_invalid_u", ""},
		{"hp_100_x", ""},
		{"lv_100_o", ""},
		{"hp_101_u", ""},
		{"hp_100_u", "HP must be under 100"},
		{"hp_100_o", ""},
		{"hp_101_o", "HP must be at least 101"},
		{"joblv_4_u", ""},
		{"joblv_3_u", "Job level must be under 3"},
		{"joblv_3_o", ""},
		{"joblv_4_o", "Job level must be at least 4"},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			err := ValidateNeedJoin(tc.condition, char)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("ValidateNeedJoin(%q) = %v, want nil", tc.condition, err)
				}
			} else if err == nil || err.Error() != tc.wantError {
				t.Fatalf("ValidateNeedJoin(%q) = %v, want %q", tc.condition, err, tc.wantError)
			}
		})
	}
}

func TestBattleRowRewards(t *testing.T) {
	exp, gold := Character{Level: 25, JobLevel: 3}.BattleRowRewards()
	if exp != 28 || gold != 12 {
		t.Fatalf("BattleRowRewards() = %d, %d; want 28, 12", exp, gold)
	}
}
