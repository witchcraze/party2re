package ranking

import (
	"errors"
	"time"
)

// RankingType identifies the category of leaderboard.
type RankingType string

const (
	RankingTypePvPVictory      RankingType = "pvp_victory"
	RankingTypeJobPopularity   RankingType = "job_popularity"
	RankingTypeHelper          RankingType = "helper"
	RankingTypeCasinoWins      RankingType = "casino_wins"
	RankingTypeAlchemy         RankingType = "alchemy"
	RankingTypeWeeklyJobChange RankingType = "weekly_job_change"
	RankingTypeMonsterKills    RankingType = "monster_kills"
	RankingTypeMaoCount        RankingType = "mao_count"
	RankingTypeHeroCount       RankingType = "hero_count"
)

var (
	ErrInvalidRankingType = errors.New("invalid ranking type")
	ErrSnapshotNotFound   = errors.New("ranking snapshot not found")
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// CharacterRankingEntry represents a character's position and stats in a character-based leaderboard.
type CharacterRankingEntry struct {
	Rank           int    `json:"rank"`
	CharacterID    string `json:"character_id"`
	CharacterName  string `json:"character_name"`
	PlayerID       string `json:"player_id"`
	PlayerUsername string `json:"player_username,omitempty"`
	JobID          string `json:"job_id"`
	Gender         string `json:"gender"`
	Level          int    `json:"level"`
	Experience     int    `json:"experience"`
	SP             int    `json:"sp"`
	Score          int64  `json:"score"`
	SecondaryScore int64  `json:"secondary_score,omitempty"`
}

// JobPopularityEntry represents the distribution and popularity of a job across characters.
type JobPopularityEntry struct {
	Rank        int     `json:"rank"`
	JobID       string  `json:"job_id"`
	TotalCount  int     `json:"total_count"`
	MaleCount   int     `json:"male_count"`
	FemaleCount int     `json:"female_count"`
	Percentage  float64 `json:"percentage"`
}

// RankingPage is a generic paginated ranking response container.
type RankingPage[T any] struct {
	RankingType  RankingType `json:"ranking_type"`
	Entries      []T         `json:"entries"`
	Total        int         `json:"total"`
	Limit        int         `json:"limit"`
	Offset       int         `json:"offset"`
	CalculatedAt time.Time   `json:"calculated_at"`
	IsSnapshot   bool        `json:"is_snapshot"`
}

// RankingSnapshot represents persisted/cached ranking data.
type RankingSnapshot struct {
	RankingType  RankingType `json:"ranking_type"`
	SnapshotData string      `json:"snapshot_data"`
	TotalCount   int         `json:"total_count"`
	CalculatedAt time.Time   `json:"calculated_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// NormalizeRankingType converts aliases (e.g. legacy Perl CGI identifiers) to canonical RankingType.
func NormalizeRankingType(t RankingType) RankingType {
	switch t {
	case "kill_p", "pvp-victory":
		return RankingTypePvPVictory
	case "kill_m", "monster-kills":
		return RankingTypeMonsterKills
	case "mao_c", "mao-count":
		return RankingTypeMaoCount
	case "hero_c", "hero-count":
		return RankingTypeHeroCount
	case "cas_c", "casino-wins":
		return RankingTypeCasinoWins
	case "alc_c":
		return RankingTypeAlchemy
	case "help_c", "helpers":
		return RankingTypeHelper
	case "week_job_change", "weekly-job-change":
		return RankingTypeWeeklyJobChange
	default:
		return t
	}
}

// IsValidRankingType returns true if the specified ranking type is supported.
func IsValidRankingType(t RankingType) bool {
	switch NormalizeRankingType(t) {
	case RankingTypePvPVictory,
		RankingTypeJobPopularity,
		RankingTypeHelper,
		RankingTypeCasinoWins,
		RankingTypeAlchemy,
		RankingTypeWeeklyJobChange,
		RankingTypeMonsterKills,
		RankingTypeMaoCount,
		RankingTypeHeroCount:
		return true
	default:
		return false
	}
}

// NormalizePagination ensures limit and offset are within valid bounds.
func NormalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = DefaultLimit
	} else if limit > MaxLimit {
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
