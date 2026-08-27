package ranking

import (
	"errors"
	"time"
)

// RankingType identifies the category of leaderboard.
type RankingType string

const (
	RankingTypeLevel            RankingType = "level"
	RankingTypePlayerWealth     RankingType = "player_wealth"
	RankingTypeCharacterWealth  RankingType = "character_wealth"
	RankingTypeBattleVictory    RankingType = "battle_victory"
	RankingTypePvPVictory       RankingType = "pvp_victory"
	RankingTypeBossDefeat       RankingType = "boss_defeat"
	RankingTypeAdventureVictory RankingType = "adventure_victory"
	RankingTypeJobMastery       RankingType = "job_mastery"
	RankingTypeJobPopularity    RankingType = "job_popularity"
	RankingTypeHelper           RankingType = "helper"
	RankingTypeRebirth          RankingType = "rebirth"
	RankingTypeSmallMedals      RankingType = "small_medals"
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
	RebirthCount   int    `json:"rebirth_count"`
	Score          int64  `json:"score"`
	SecondaryScore int64  `json:"secondary_score,omitempty"`
}

// PlayerWealthRankingEntry represents a player's aggregated wealth ranking.
type PlayerWealthRankingEntry struct {
	Rank            int    `json:"rank"`
	PlayerID        string `json:"player_id"`
	Username        string `json:"username"`
	TotalWealth     int64  `json:"total_wealth"`
	BankBalance     int64  `json:"bank_balance"`
	CharactersMoney int64  `json:"characters_money"`
	CharacterCount  int    `json:"character_count"`
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

// IsValidRankingType returns true if the specified ranking type is supported.
func IsValidRankingType(t RankingType) bool {
	switch t {
	case RankingTypeLevel,
		RankingTypePlayerWealth,
		RankingTypeCharacterWealth,
		RankingTypeBattleVictory,
		RankingTypePvPVictory,
		RankingTypeBossDefeat,
		RankingTypeAdventureVictory,
		RankingTypeJobMastery,
		RankingTypeJobPopularity,
		RankingTypeHelper,
		RankingTypeRebirth,
		RankingTypeSmallMedals:
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
