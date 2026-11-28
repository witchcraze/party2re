package adventure_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
)

func TestCalculateExitFatigue_NormalAndCalmModes(t *testing.T) {
	cases := []struct {
		round      int
		wantNormal int
		wantCalm   int
	}{
		{round: -1, wantNormal: 1, wantCalm: 0},
		{round: 0, wantNormal: 1, wantCalm: 0},
		{round: 1, wantNormal: 4, wantCalm: 2},
		{round: 2, wantNormal: 7, wantCalm: 3},
		{round: 3, wantNormal: 10, wantCalm: 5},
		{round: 5, wantNormal: 16, wantCalm: 8},
		{round: 8, wantNormal: 25, wantCalm: 12},
		{round: 9, wantNormal: 28, wantCalm: 14},
		{round: 10, wantNormal: 31, wantCalm: 15},
		{round: 11, wantNormal: 34, wantCalm: 17},
	}

	for _, tc := range cases {
		for _, mode := range []string{"", adventure.CrawlModeNormal, adventure.CrawlModeTry, adventure.CrawlModeHard} {
			if got := adventure.CalculateExitFatigue(tc.round, mode); got != tc.wantNormal {
				t.Errorf("CalculateExitFatigue(round=%d, mode=%q) = %d, want %d", tc.round, mode, got, tc.wantNormal)
			}
		}
		if got := adventure.CalculateExitFatigue(tc.round, adventure.CrawlModeCalm); got != tc.wantCalm {
			t.Errorf("CalculateExitFatigue(round=%d, mode=calm) = %d, want %d", tc.round, got, tc.wantCalm)
		}
	}
}

func TestValidateCrawlMode(t *testing.T) {
	for _, valid := range []string{"", "normal", "calm", "try", "hard", "NORMAL", "CALM"} {
		got, err := adventure.ValidateCrawlMode(valid)
		if err != nil {
			t.Errorf("ValidateCrawlMode(%q) unexpected error: %v", valid, err)
		}
		if valid == "" && got != adventure.CrawlModeNormal {
			t.Errorf("ValidateCrawlMode(\"\") = %q, want normal", got)
		}
	}

	for _, invalid := range []string{"invalid", "unknown", "calmm"} {
		if _, err := adventure.ValidateCrawlMode(invalid); err == nil {
			t.Errorf("ValidateCrawlMode(%q) expected error, got nil", invalid)
		}
	}
}
