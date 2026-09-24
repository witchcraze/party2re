package character_test

import (
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestCMPTierRate(t *testing.T) {
	tests := []struct {
		tier int
		want float64
	}{
		{tier: 0, want: 0.0},
		{tier: 1, want: 0.5},
		{tier: 2, want: 0.8},
		{tier: 3, want: 1.0},
		{tier: 4, want: 1.5},
		{tier: 5, want: 2.0},
		{tier: -1, want: 0.0},
		{tier: 6, want: 0.0},
	}

	for _, tt := range tests {
		got := corecharacter.CMPTierRate(tt.tier)
		if got != tt.want {
			t.Errorf("CMPTierRate(%d) = %v, want %v", tt.tier, got, tt.want)
		}
	}
}

func TestCalculateCMP(t *testing.T) {
	tests := []struct {
		name           string
		level          int
		jobTierRate    float64
		oldJobTierRate float64
		want           int
	}{
		{
			name:           "level 20 warrior (tier 5) without old job",
			level:          20,
			jobTierRate:    2.0,
			oldJobTierRate: 0.0,
			want:           40,
		},
		{
			name:           "level 50 warrior (tier 5) and swordsman (tier 5)",
			level:          50,
			jobTierRate:    2.0,
			oldJobTierRate: 2.0,
			want:           200,
		},
		{
			name:           "level 50 knight (tier 4) and monk (tier 3)",
			level:          50,
			jobTierRate:    1.5,
			oldJobTierRate: 1.0,
			want:           125, // 50 * 2.5 = 125
		},
		{
			name:           "level 33 with fractional product int truncation",
			level:          33,
			jobTierRate:    0.8, // tier 2
			oldJobTierRate: 0.5, // tier 1 -> sum = 1.3
			want:           42,  // 33 * 1.3 = 42.9 -> 42
		},
		{
			name:           "level over 99 is capped at 99 (original_lv rule)",
			level:          150,
			jobTierRate:    1.5,
			oldJobTierRate: 1.0,
			want:           247, // 99 * 2.5 = 247.5 -> 247
		},
		{
			name:           "level 0 or negative produces 0",
			level:          0,
			jobTierRate:    2.0,
			oldJobTierRate: 2.0,
			want:           0,
		},
		{
			name:           "negative level produces 0",
			level:          -5,
			jobTierRate:    2.0,
			oldJobTierRate: 2.0,
			want:           0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := corecharacter.CalculateCMP(tt.level, tt.jobTierRate, tt.oldJobTierRate)
			if got != tt.want {
				t.Errorf("CalculateCMP(%d, %v, %v) = %d, want %d", tt.level, tt.jobTierRate, tt.oldJobTierRate, got, tt.want)
			}
		})
	}
}

func TestCharacter_CMP(t *testing.T) {
	c := &corecharacter.Character{Level: 40}
	// job tier 4 (1.5) + old job tier 2 (0.8) -> rate = 2.3 -> 40 * 2.3 = 92
	if got := c.CMP(4, 2); got != 92 {
		t.Errorf("c.CMP(4, 2) = %d, want 92", got)
	}

	var nilChar *corecharacter.Character
	if got := nilChar.CMP(4, 2); got != 0 {
		t.Errorf("nil.CMP(4, 2) = %d, want 0", got)
	}
}
