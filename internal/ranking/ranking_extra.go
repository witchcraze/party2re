package ranking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var jstLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

// NextSundayMidnightJST calculates the next Sunday 00:00:00 JST occurrence strictly after t.
func NextSundayMidnightJST(t time.Time) time.Time {
	inJST := t.In(jstLocation)
	daysUntilSunday := (int(time.Sunday) - int(inJST.Weekday()) + 7) % 7
	midnight := time.Date(inJST.Year(), inJST.Month(), inJST.Day(), 0, 0, 0, 0, jstLocation).AddDate(0, 0, daysUntilSunday)
	if !midnight.After(inJST) {
		midnight = midnight.AddDate(0, 0, 7)
	}
	return midnight.UTC()
}

// GetCasinoWinsRanking returns character rankings sorted by casino showdown wins (cas_c).
func (s *Service) GetCasinoWinsRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeCasinoWins, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeCasinoWins,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetCasinoWinsRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeCasinoWins,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetAlchemyRanking returns character rankings sorted by total alchemy craft syntheses (alc_c).
func (s *Service) GetAlchemyRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeAlchemy, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeAlchemy,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetAlchemyRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeAlchemy,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetWeeklyJobChangeRanking returns character rankings sorted by weekly job change counts.
func (s *Service) GetWeeklyJobChangeRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeWeeklyJobChange, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeWeeklyJobChange,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetWeeklyJobChangeRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeWeeklyJobChange,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// RecordJobChange increments the active weekly job change count for a character.
func (s *Service) RecordJobChange(ctx context.Context, characterID string) error {
	if characterID == "" {
		return errors.New("character ID required")
	}
	return s.repo.IncrementJobChangeCount(ctx, characterID)
}

// RotateWeeklyJobChangeRanking freezes the active weekly job change leaderboards into a snapshot and resets counters.
func (s *Service) RotateWeeklyJobChangeRanking(ctx context.Context) error {
	now := s.nowFunc().UTC()
	entries, totalCount, err := s.repo.GetActiveWeeklyJobChangeRanking(ctx, 100, 0)
	if err != nil {
		return fmt.Errorf("query active weekly job changes for rotation: %w", err)
	}

	rawJSON, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal weekly job change snapshot: %w", err)
	}

	snapshot := RankingSnapshot{
		RankingType:  RankingTypeWeeklyJobChange,
		SnapshotData: string(rawJSON),
		TotalCount:   totalCount,
		CalculatedAt: now,
		UpdatedAt:    now,
	}

	if err := s.repo.SaveSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("save weekly job change snapshot: %w", err)
	}

	if s.valkeyCache != nil {
		_ = s.valkeyCache.Set(ctx, RankingTypeWeeklyJobChange, snapshot, s.cacheTTL)
	}

	s.storeCache(RankingTypeWeeklyJobChange, entries, totalCount, now)

	if err := s.repo.ResetWeeklyJobChanges(ctx); err != nil {
		return fmt.Errorf("reset weekly job changes: %w", err)
	}

	return nil
}

// GetLegends returns all Hall of Fame categories with their entry counts.
func (s *Service) GetLegends(ctx context.Context) ([]LegendCategoryInfo, error) {
	counts, err := s.repo.GetLegendCategoryCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get legend category counts: %w", err)
	}

	categories := AllLegendCategories()
	for i := range categories {
		categories[i].TotalCount = counts[categories[i].Category]
	}
	return categories, nil
}

// GetLegendCategory retrieves inductees for a specific Hall of Fame category.
func (s *Service) GetLegendCategory(ctx context.Context, category LegendCategory) (LegendCategoryPage, error) {
	if !IsValidLegendCategory(category) {
		return LegendCategoryPage{}, ErrInvalidLegendCategory
	}

	info, _ := GetLegendCategoryInfo(category)
	entries, err := s.repo.GetLegendInductees(ctx, category)
	if err != nil {
		return LegendCategoryPage{}, fmt.Errorf("get legend inductees for %s: %w", category, err)
	}
	if entries == nil {
		entries = []LegendEntry{}
	}

	return LegendCategoryPage{
		Category:    category,
		Title:       info.Title,
		Description: info.Description,
		Entries:     entries,
		Total:       len(entries),
	}, nil
}

// RecordLegend inducts a character into the permanent Hall of Fame. Returns true if newly inducted.
func (s *Service) RecordLegend(ctx context.Context, entry LegendEntry) (bool, error) {
	if !IsValidLegendCategory(entry.Category) {
		return false, ErrInvalidLegendCategory
	}
	if entry.CharacterID == "" {
		return false, errors.New("character ID required")
	}
	if entry.InductedAt.IsZero() {
		entry.InductedAt = s.nowFunc().UTC()
	}
	return s.repo.RecordLegend(ctx, entry)
}
