package boss

import (
	"context"
)

// GetCharacterRecord retrieves the boss challenge progress record for a character.
func (s *Service) GetCharacterRecord(ctx context.Context, characterID string) (CharacterBossRecord, error) {
	if characterID == "" {
		return CharacterBossRecord{}, ErrCharacterNotFound
	}
	return s.repo.GetOrCreateRecord(ctx, characterID)
}

// GetHistory retrieves recent boss challenge history for a character.
func (s *Service) GetHistory(ctx context.Context, characterID string, limit int) ([]BossChallengeHistory, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetHistory(ctx, characterID, limit)
}

// GetLeaderboard retrieves top boss defeat rankings.
func (s *Service) GetLeaderboard(ctx context.Context, limit int) ([]BossLeaderboardEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.GetLeaderboard(ctx, limit)
}
