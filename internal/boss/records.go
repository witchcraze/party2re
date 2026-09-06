package boss

import (
	"context"
	"time"
)

// ResetDailyAttemptsIfExpired resets used attempts when day has rolled over.
func (r *CharacterBossRecord) ResetDailyAttemptsIfExpired(today time.Time) {
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	resetDate := time.Date(r.DailyAttemptsResetAt.Year(), r.DailyAttemptsResetAt.Month(), r.DailyAttemptsResetAt.Day(), 0, 0, 0, 0, time.UTC)
	if todayDate.After(resetDate) {
		r.DailyAttemptsUsed = 0
		r.DailyAttemptsResetAt = todayDate
	}
}

// GetCharacterRecord retrieves the boss challenge progress record for a character.
func (s *Service) GetCharacterRecord(ctx context.Context, characterID string) (CharacterBossRecord, error) {
	if characterID == "" {
		return CharacterBossRecord{}, ErrCharacterNotFound
	}
	rec, err := s.repo.GetOrCreateRecord(ctx, characterID)
	if err != nil {
		return CharacterBossRecord{}, err
	}
	rec.ResetDailyAttemptsIfExpired(time.Now().UTC())
	return rec, nil
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
