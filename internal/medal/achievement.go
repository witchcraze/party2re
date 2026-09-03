package medal

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

//go:embed achievements.json
var defaultAchievementsData []byte

// MetricType identifies the gameplay metric being tracked for achievements.
type MetricType string

const (
	MetricAdventureVictories MetricType = "adventure_victories"
	MetricMonstersSlain      MetricType = "monsters_slain"
	MetricGoldEarned         MetricType = "gold_earned"
	MetricBossesSlain        MetricType = "bosses_slain"
	MetricPvPVictories       MetricType = "pvp_victories"
	MetricCasinoGames        MetricType = "casino_games"
	MetricAlchemyCrafts      MetricType = "alchemy_crafts"
)

var (
	ErrAchievementNotFound          = errors.New("achievement not found")
	ErrAchievementNotCompleted      = errors.New("achievement milestone not completed")
	ErrAchievementAlreadyClaimed    = errors.New("achievement reward already claimed")
	ErrInvalidMetric                = errors.New("invalid metric type")
	ErrInvalidAmount                = errors.New("amount must be positive")
	ErrAchievementRepoNotConfigured = errors.New("achievement repository not configured")
)

// Achievement represents a static lifetime milestone definition in the game catalog.
type Achievement struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Category          string     `json:"category"`
	Description       string     `json:"description"`
	Metric            MetricType `json:"metric"`
	Threshold         int        `json:"threshold"`
	MedalID           string     `json:"medal_id"`
	MedalName         string     `json:"medal_name"`
	MedalDescription  string     `json:"medal_description"`
	SmallMedalsReward int        `json:"small_medals_reward"`
}

// DomainEvent carries information about a completed gameplay action for achievement progress tracking.
type DomainEvent struct {
	CharacterID string     `json:"character_id"`
	Metric      MetricType `json:"metric"`
	Amount      int        `json:"amount"`
	OccurredAt  time.Time  `json:"occurred_at"`
}

// AchievementRecord represents the persistent progress state of a character for a specific achievement.
type AchievementRecord struct {
	CharacterID     string     `json:"character_id"`
	AchievementID   string     `json:"achievement_id"`
	CurrentProgress int        `json:"current_progress"`
	IsCompleted     bool       `json:"is_completed"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	IsClaimed       bool       `json:"is_claimed"`
	ClaimedAt       *time.Time `json:"claimed_at,omitempty"`
}

// AchievementProgress represents an enriched view of an achievement including catalog metadata and character progress.
type AchievementProgress struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Category             string     `json:"category"`
	Description          string     `json:"description"`
	Metric               MetricType `json:"metric"`
	Threshold            int        `json:"threshold"`
	CurrentProgress      int        `json:"current_progress"`
	CompletionPercentage float64    `json:"completion_percentage"`
	IsCompleted          bool       `json:"is_completed"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	IsClaimed            bool       `json:"is_claimed"`
	ClaimedAt            *time.Time `json:"claimed_at,omitempty"`
	MedalID              string     `json:"medal_id"`
	MedalName            string     `json:"medal_name"`
	MedalDescription     string     `json:"medal_description"`
	SmallMedalsReward    int        `json:"small_medals_reward"`
}

// CharacterMedal represents a commemorative medal awarded to a character upon claiming a completed achievement.
type CharacterMedal struct {
	CharacterID string    `json:"character_id"`
	MedalID     string    `json:"medal_id"`
	MedalName   string    `json:"medal_name"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	AwardedAt   time.Time `json:"awarded_at"`
}

// ClaimResult contains the results of claiming an achievement milestone reward.
type ClaimResult struct {
	AchievementID      string                  `json:"achievement_id"`
	AchievementName    string                  `json:"achievement_name"`
	Medal              CharacterMedal          `json:"medal"`
	SmallMedalsAwarded int                     `json:"small_medals_awarded"`
	Character          corecharacter.Character `json:"character"`
}

// InitialAchievements loads the embedded default achievement definitions.
func InitialAchievements() ([]Achievement, error) {
	var achievements []Achievement
	if err := json.Unmarshal(defaultAchievementsData, &achievements); err != nil {
		return nil, err
	}
	return achievements, nil
}

// LoadAchievementsFromFile loads achievement definitions from an external JSON file.
func LoadAchievementsFromFile(filePath string) ([]Achievement, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var achievements []Achievement
	if err := json.Unmarshal(data, &achievements); err != nil {
		return nil, err
	}
	return achievements, nil
}
