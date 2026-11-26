package ranking

import "context"

// boardSize is the length of every ranking.cgi board (party2/login.cgi write_ranking).
const boardSize = 10

type boardQuery func(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)

// boardQueries returns a ranking.cgi board's ordered query and, when the board excludes
// another board's leaders, that leaders' query.
func (s *Service) boardQueries(t RankingType) (query, leaders boardQuery, ok bool) {
	switch t {
	case RankingTypePvPVictory:
		return s.repo.GetPvPVictoryRanking, nil, true
	case RankingTypeMonsterKills:
		return s.repo.GetMonsterKillsRanking, s.repo.GetPvPVictoryRanking, true
	case RankingTypeMaoCount:
		return s.repo.GetMaoCountRanking, nil, true
	case RankingTypeHeroCount:
		return s.repo.GetHeroCountRanking, s.repo.GetMaoCountRanking, true
	case RankingTypeCasinoWins:
		return s.repo.GetCasinoWinsRanking, nil, true
	case RankingTypeHelper:
		return s.repo.GetHelperRanking, nil, true
	case RankingTypeAlchemy:
		return s.repo.GetAlchemyRanking, nil, true
	default:
		return nil, nil, false
	}
}

// loadBoard computes a ranking.cgi board from current data.
func (s *Service) loadBoard(ctx context.Context, t RankingType) ([]CharacterRankingEntry, error) {
	query, leaders, ok := s.boardQueries(t)
	if !ok {
		return nil, ErrInvalidRankingType
	}
	if leaders == nil {
		entries, _, err := query(ctx, boardSize, 0)
		return entries, err
	}
	top, _, err := leaders(ctx, boardSize, 0)
	if err != nil {
		return nil, err
	}
	excluded := make(map[string]bool, len(top))
	for _, e := range top {
		excluded[e.CharacterID] = true
	}
	// At most boardSize candidates are excluded, so twice the board always fills it.
	candidates, _, err := query(ctx, 2*boardSize, 0)
	if err != nil {
		return nil, err
	}
	board := make([]CharacterRankingEntry, 0, boardSize)
	for _, e := range candidates {
		if excluded[e.CharacterID] {
			continue
		}
		e.Rank = len(board) + 1
		board = append(board, e)
		if len(board) == boardSize {
			break
		}
	}
	return board, nil
}

// getBoard pages a ranking.cgi board from its snapshot or, on a miss or request, from current data.
func (s *Service) getBoard(ctx context.Context, t RankingType, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	page := RankingPage[CharacterRankingEntry]{RankingType: t, Limit: limit, Offset: offset}
	if useSnapshot {
		if entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, t, limit, offset); found {
			page.Entries, page.Total, page.CalculatedAt, page.IsSnapshot = entries, total, calcTime, true
			return page, nil
		}
	}
	board, err := s.loadBoard(ctx, t)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}
	page.Entries, page.Total, page.CalculatedAt = paginateSlice(board, limit, offset), len(board), s.nowFunc().UTC()
	return page, nil
}

// GetPvPVictoryRanking returns the 王者 board (kill_p) by strong-opponent PvP defeats.
func (s *Service) GetPvPVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypePvPVictory, limit, offset, useSnapshot)
}

// GetMonsterKillsRanking returns the 英雄 board (kill_m) by defeated strong enemies.
func (s *Service) GetMonsterKillsRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeMonsterKills, limit, offset, useSnapshot)
}

// GetMaoCountRanking returns the 魔王 board (mao_c) by unsealed demon king defeats.
func (s *Service) GetMaoCountRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeMaoCount, limit, offset, useSnapshot)
}

// GetHeroCountRanking returns the 勇者 board (hero_c) by hero achievements.
func (s *Service) GetHeroCountRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeHeroCount, limit, offset, useSnapshot)
}

// GetCasinoWinsRanking returns the 勝負師 board (cas_c) by casino showdown wins.
func (s *Service) GetCasinoWinsRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeCasinoWins, limit, offset, useSnapshot)
}

// GetHelperRanking returns the 手助け board (help_c) by completed helper quests.
func (s *Service) GetHelperRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeHelper, limit, offset, useSnapshot)
}

// GetAlchemyRanking returns the 錬金 board (alc_c) by alchemy syntheses.
func (s *Service) GetAlchemyRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeAlchemy, limit, offset, useSnapshot)
}
