package adventure

import (
	"testing"
)

func TestAdventure_SeasonalSubIdentityMapping(t *testing.T) {
	tests := []struct {
		season    string
		wantSubID int
	}{
		{"spring", 1},
		{"SPRING", 1},
		{"summer", 2},
		{"autumn", 3},
		{"winter", 4},
		{"", 0},
		{"unknown", 0},
	}

	for _, tt := range tests {
		got := SeasonToSubID(tt.season)
		if got != tt.wantSubID {
			t.Errorf("SeasonToSubID(%q) = %d, want %d", tt.season, got, tt.wantSubID)
		}
		if tt.wantSubID > 0 {
			back := SubIDToSeason(tt.wantSubID)
			if back != tt.season && back != "spring" && back != "summer" && back != "autumn" && back != "winter" {
				t.Errorf("SubIDToSeason(%d) = %q, expected valid season name", tt.wantSubID, back)
			}
		}
	}
}
