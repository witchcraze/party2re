package adventure

import (
	"context"
	"math"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// Milestone represents a gameplay achievement milestone unlocked through adventure clears.
type Milestone struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Threshold   int    `json:"threshold"`
	Unlocked    bool   `json:"unlocked"`
	Description string `json:"description"`
}

// StageStatData holds raw count data for a specific stage from the repository.
type StageStatData struct {
	StageID       string `json:"stage_id"`
	TotalAttempts int    `json:"total_attempts"`
	ClearCount    int    `json:"clear_count"`
}

// AggregatedStats holds raw summary counters aggregated from the persistence layer.
type AggregatedStats struct {
	TotalAdventures int             `json:"total_adventures"`
	TotalVictories  int             `json:"total_victories"`
	TotalDefeats    int             `json:"total_defeats"`
	TotalDraws      int             `json:"total_draws"`
	TotalTurns      int             `json:"total_turns"`
	TotalExpEarned  int             `json:"total_exp_earned"`
	TotalGoldEarned int             `json:"total_gold_earned"`
	StageStats      []StageStatData `json:"stage_stats"`
}

// StageClearStat represents clear and attempt statistics for a specific stage with its human-readable name.
type StageClearStat struct {
	StageID       string `json:"stage_id"`
	StageName     string `json:"stage_name"`
	ClearCount    int    `json:"clear_count"`
	TotalAttempts int    `json:"total_attempts"`
}

// AdventureChronicle provides an aggregated statistical summary of a character's adventure history.
type AdventureChronicle struct {
	CharacterID     string           `json:"character_id"`
	TotalAdventures int              `json:"total_adventures"`
	TotalVictories  int              `json:"total_victories"`
	TotalDefeats    int              `json:"total_defeats"`
	TotalDraws      int              `json:"total_draws"`
	WinRate         float64          `json:"win_rate"`
	TotalTurns      int              `json:"total_turns"`
	TotalExpEarned  int              `json:"total_exp_earned"`
	TotalGoldEarned int              `json:"total_gold_earned"`
	Stages          []StageClearStat `json:"stages"`
	Milestones      []Milestone      `json:"milestones"`
}

// AdventureHistoryEntry represents an individual historical adventure record.
type AdventureHistoryEntry struct {
	ID                 string             `json:"id"`
	CharacterID        string             `json:"character_id"`
	StageID            string             `json:"stage_id"`
	StageName          string             `json:"stage_name"`
	MonsterID          string             `json:"monster_id"`
	MonsterName        string             `json:"monster_name"`
	StartedAt          time.Time          `json:"started_at"`
	AvailableAt        time.Time          `json:"available_at"`
	Outcome            corebattle.Outcome `json:"outcome"`
	BattleTurns        int                `json:"battle_turns"`
	RewardExperience   int                `json:"reward_experience"`
	RewardCurrency     int                `json:"reward_currency"`
	RewardItemID       string             `json:"reward_item_id,omitempty"`
	RewardItemQuantity int                `json:"reward_item_quantity,omitempty"`
	Resolved           bool               `json:"resolved"`
	Claimed            bool               `json:"claimed"`
}

// PaginatedAdventures represents a paginated list of adventure history entries.
type PaginatedAdventures struct {
	CharacterID string                  `json:"character_id"`
	Adventures  []AdventureHistoryEntry `json:"adventures"`
	Total       int                     `json:"total"`
	Limit       int                     `json:"limit"`
	Offset      int                     `json:"offset"`
}

// NormalizePagination ensures limit and offset stay within allowed system boundaries.
func NormalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// DefaultMilestones evaluates milestone unlocking states based on total adventure victories.
func DefaultMilestones(totalVictories int) []Milestone {
	milestoneDefs := []struct {
		key         string
		name        string
		threshold   int
		description string
	}{
		{
			key:         "try_mode",
			name:        "トライモード (Try Mode)",
			threshold:   50,
			description: "クエストにトライモードを設定可能になる",
		},
		{
			key:         "image_setting",
			name:        "イメージ設定 (Image Setting)",
			threshold:   100,
			description: "ほーむからイメージを設定可能になる",
		},
		{
			key:         "calm_mode",
			name:        "カームモード (Calm Mode)",
			threshold:   150,
			description: "クエストにカームモードを設定可能になる",
		},
		{
			key:         "hard_mode",
			name:        "ハードモード (Hard Mode)",
			threshold:   300,
			description: "クエストにハードモードを設定可能になる",
		},
		{
			key:         "avatar_setting",
			name:        "アバター設定 (Avatar Setting)",
			threshold:   500,
			description: "イメージからアバターを設定可能になる",
		},
		{
			key:         "extreme_mode",
			name:        "エクストリームモード (Extreme Mode)",
			threshold:   1000,
			description: "クエストにエクストリームモードを設定可能になる",
		},
	}

	result := make([]Milestone, 0, len(milestoneDefs))
	for _, def := range milestoneDefs {
		result = append(result, Milestone{
			Key:         def.key,
			Name:        def.name,
			Threshold:   def.threshold,
			Unlocked:    totalVictories >= def.threshold,
			Description: def.description,
		})
	}
	return result
}

// ListHistory retrieves paginated adventure history for a character, enriched with catalog metadata.
func (s *Service) ListHistory(ctx context.Context, characterID string, limit, offset int) (PaginatedAdventures, error) {
	if characterID == "" {
		return PaginatedAdventures{}, corecharacter.ErrNotFound
	}
	if _, err := s.characters.FindByID(ctx, characterID); err != nil {
		return PaginatedAdventures{}, err
	}

	limit, offset = NormalizePagination(limit, offset)

	adventures, total, err := s.adventures.ListByCharacterID(ctx, characterID, limit, offset)
	if err != nil {
		return PaginatedAdventures{}, err
	}

	entries := make([]AdventureHistoryEntry, 0, len(adventures))
	for _, adv := range adventures {
		stageID := adv.StageID
		if stageID == "" {
			stageID = adv.Type
		}
		stageName := stageID
		if s.stages != nil {
			if st, err := s.stages.FindByID(stageID); err == nil {
				stageName = st.Name
			}
		}

		monsterID := adv.MonsterID
		monsterName := monsterID
		if monsterID == "" {
			// If monster ID was not explicitly on the struct, infer from battle participant
			if adv.BattleResult.WinnerID == adv.CharacterID {
				monsterID = adv.BattleResult.LoserID
			} else if adv.BattleResult.LoserID == adv.CharacterID {
				monsterID = adv.BattleResult.WinnerID
			}
		}
		if s.monsters != nil && monsterID != "" {
			if m, err := s.monsters.FindByID(monsterID); err == nil {
				monsterName = m.Name
			}
		}

		entries = append(entries, AdventureHistoryEntry{
			ID:                 adv.ID,
			CharacterID:        adv.CharacterID,
			StageID:            stageID,
			StageName:          stageName,
			MonsterID:          monsterID,
			MonsterName:        monsterName,
			StartedAt:          adv.StartedAt,
			AvailableAt:        adv.AvailableAt,
			Outcome:            adv.BattleResult.Outcome,
			BattleTurns:        adv.BattleResult.Turns,
			RewardExperience:   adv.BattleResult.Reward.Experience,
			RewardCurrency:     adv.BattleResult.Reward.Currency,
			RewardItemID:       adv.BattleResult.Reward.ItemDefinitionID,
			RewardItemQuantity: adv.BattleResult.Reward.ItemQuantity,
			Resolved:           adv.Resolved,
			Claimed:            adv.Claimed,
		})
	}

	return PaginatedAdventures{
		CharacterID: characterID,
		Adventures:  entries,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	}, nil
}

// GetChronicle computes an aggregated statistical summary of past adventure runs and unlocked milestones.
func (s *Service) GetChronicle(ctx context.Context, characterID string) (AdventureChronicle, error) {
	if characterID == "" {
		return AdventureChronicle{}, corecharacter.ErrNotFound
	}
	if _, err := s.characters.FindByID(ctx, characterID); err != nil {
		return AdventureChronicle{}, err
	}

	rawStats, err := s.adventures.GetAggregatedStats(ctx, characterID)
	if err != nil {
		return AdventureChronicle{}, err
	}

	var winRate float64
	if rawStats.TotalAdventures > 0 {
		winRate = math.Round((float64(rawStats.TotalVictories)/float64(rawStats.TotalAdventures))*10000) / 10000
	}

	stages := make([]StageClearStat, 0, len(rawStats.StageStats))
	for _, ss := range rawStats.StageStats {
		stageName := ss.StageID
		if s.stages != nil {
			if st, err := s.stages.FindByID(ss.StageID); err == nil {
				stageName = st.Name
			}
		}
		stages = append(stages, StageClearStat{
			StageID:       ss.StageID,
			StageName:     stageName,
			ClearCount:    ss.ClearCount,
			TotalAttempts: ss.TotalAttempts,
		})
	}

	milestones := DefaultMilestones(rawStats.TotalVictories)

	return AdventureChronicle{
		CharacterID:     characterID,
		TotalAdventures: rawStats.TotalAdventures,
		TotalVictories:  rawStats.TotalVictories,
		TotalDefeats:    rawStats.TotalDefeats,
		TotalDraws:      rawStats.TotalDraws,
		WinRate:         winRate,
		TotalTurns:      rawStats.TotalTurns,
		TotalExpEarned:  rawStats.TotalExpEarned,
		TotalGoldEarned: rawStats.TotalGoldEarned,
		Stages:          stages,
		Milestones:      milestones,
	}, nil
}
