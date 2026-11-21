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
		{"monster_kills", ranking.RankingTypeMonsterKills, true},
		{"mao_count", ranking.RankingTypeMaoCount, true},
		{"hero_count", ranking.RankingTypeHeroCount, true},
		{"pvp_victory", ranking.RankingTypePvPVictory, true},
		{"job_popularity", ranking.RankingTypeJobPopularity, true},
		{"helper", ranking.RankingTypeHelper, true},
		{"casino_wins", ranking.RankingTypeCasinoWins, true},
		{"alchemy", ranking.RankingTypeAlchemy, true},
		{"weekly_job_change", ranking.RankingTypeWeeklyJobChange, true},
		{"kill_p alias", "kill_p", true},
		{"kill_m alias", "kill_m", true},
		{"mao_c alias", "mao_c", true},
		{"hero_c alias", "hero_c", true},
		{"cas_c alias", "cas_c", true},
		{"help_c alias", "help_c", true},
		{"alc_c alias", "alc_c", true},
		{"week_job_change alias", "week_job_change", true},
		{"level removed", ranking.RankingType("level"), false},
		{"player_wealth removed", ranking.RankingType("player_wealth"), false},
		{"character_wealth removed", ranking.RankingType("character_wealth"), false},
		{"boss_defeat removed", ranking.RankingType("boss_defeat"), false},
		{"adventure_victory removed", ranking.RankingType("adventure_victory"), false},
		{"job_mastery removed", ranking.RankingType("job_mastery"), false},
		{"small_medals removed", ranking.RankingType("small_medals"), false},
		{"battle_victory purged", "battle_victory", false},
		{"rebirth", ranking.RankingType("rebirth"), false},
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

func TestRepositoryInterfaceComposition(t *testing.T) {
	repo := newMockRepo()
	var _ ranking.CharacterRankingRepository = repo
	var _ ranking.LeaderboardRepository = repo
	var _ ranking.SnapshotRepository = repo
	var _ ranking.Repository = repo
}
