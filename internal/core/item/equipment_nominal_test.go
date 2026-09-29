package item

import (
	"testing"
)

func TestGetWeaponNominalStats(t *testing.T) {
	tests := []struct {
		defID      string
		wantPower  int
		wantWeight int
	}{
		{"weapon-01", 2, 0},
		{"weapon-02", 4, 2},
		{"weapon-03", 8, 6},
		{"weapon-08", 14, 9},
		{"weapon-13", 30, 20},
		{"weapon-25", 99, 66},
		{"weapon-67", 165, 80},
		{"weapon-68", 180, 0},
		{"item-001", 0, 0},
	}

	for _, tt := range tests {
		power, weight := GetWeaponNominalStats(tt.defID)
		if power != tt.wantPower || weight != tt.wantWeight {
			t.Errorf("GetWeaponNominalStats(%s) = (%d, %d), want (%d, %d)", tt.defID, power, weight, tt.wantPower, tt.wantWeight)
		}
	}
}

func TestGetArmorNominalStats(t *testing.T) {
	tests := []struct {
		defID       string
		wantDefense int
		wantWeight  int
	}{
		{"armor-01", 3, 0},
		{"armor-04", 12, 4},
		{"armor-07", 24, 8},
		{"armor-11", 43, 17},
		{"armor-29", 90, 34},
		{"item-116", 30, 0},
		{"item-119", 50, 0},
		{"item-121", 0, -60},
	}

	for _, tt := range tests {
		defense, weight := GetArmorNominalStats(tt.defID)
		if defense != tt.wantDefense || weight != tt.wantWeight {
			t.Errorf("GetArmorNominalStats(%s) = (%d, %d), want (%d, %d)", tt.defID, defense, weight, tt.wantDefense, tt.wantWeight)
		}
	}
}
