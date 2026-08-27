package ranking_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/ranking"
)

func TestIsValidRankingType(t *testing.T) {
	tests := []struct {
		name     string
		input    ranking.RankingType
		expected bool
	}{
		{"level", ranking.RankingTypeLevel, true},
		{"player_wealth", ranking.RankingTypePlayerWealth, true},
		{"character_wealth", ranking.RankingTypeCharacterWealth, true},
		{"battle_victory", ranking.RankingTypeBattleVictory, true},
		{"pvp_victory", ranking.RankingTypePvPVictory, true},
		{"boss_defeat", ranking.RankingTypeBossDefeat, true},
		{"adventure_victory", ranking.RankingTypeAdventureVictory, true},
		{"job_mastery", ranking.RankingTypeJobMastery, true},
		{"job_popularity", ranking.RankingTypeJobPopularity, true},
		{"helper", ranking.RankingTypeHelper, true},
		{"rebirth", ranking.RankingTypeRebirth, true},
		{"small_medals", ranking.RankingTypeSmallMedals, true},
		{"invalid", ranking.RankingType("invalid_type"), false},
		{"empty", ranking.RankingType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ranking.IsValidRankingType(tt.input)
			if result != tt.expected {
				t.Fatalf("expected IsValidRankingType(%q)=%v, got %v", tt.input, tt.expected, result)
			}
		})
	}
}

func TestNormalizePagination(t *testing.T) {
	tests := []struct {
		name          string
		limit, offset int
		expLimit      int
		expOffset     int
	}{
		{"defaults on zero limit", 0, 0, ranking.DefaultLimit, 0},
		{"defaults on negative limit", -5, 10, ranking.DefaultLimit, 10},
		{"clamps max limit", 200, 5, ranking.MaxLimit, 5},
		{"valid limits within range", 50, 20, 50, 20},
		{"clamps negative offset", 10, -5, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset := ranking.NormalizePagination(tt.limit, tt.offset)
			if gotLimit != tt.expLimit || gotOffset != tt.expOffset {
				t.Fatalf("expected (%d, %d), got (%d, %d)", tt.expLimit, tt.expOffset, gotLimit, gotOffset)
			}
		})
	}
}
