package ranking

import "context"

// boardSize is the length of every ranking.cgi board (party2/login.cgi write_ranking).
const boardSize = 10

type boardQuery func(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)

type pairedBoardDef struct {
	leaderType     RankingType
	dependentType  RankingType
	leaderQuery    boardQuery
	dependentQuery boardQuery
}

func (s *Service) pairedBoardDefs() []pairedBoardDef {
	return []pairedBoardDef{
		{
			leaderType:     RankingTypePvPVictory,
			dependentType:  RankingTypeMonsterKills,
			leaderQuery:    s.repo.GetPvPVictoryRanking,
			dependentQuery: s.repo.GetMonsterKillsRanking,
		},
		{
			leaderType:     RankingTypeMaoCount,
			dependentType:  RankingTypeHeroCount,
			leaderQuery:    s.repo.GetMaoCountRanking,
			dependentQuery: s.repo.GetHeroCountRanking,
		},
	}
}

func (s *Service) pairedBoardFor(t RankingType) (pairedBoardDef, bool) {
	for _, pair := range s.pairedBoardDefs() {
		if t == pair.leaderType || t == pair.dependentType {
			return pair, true
		}
	}
	return pairedBoardDef{}, false
}

func (s *Service) standaloneBoardQuery(t RankingType) (boardQuery, bool) {
	switch t {
	case RankingTypeHelper:
		return s.repo.GetHelperRanking, true
	case RankingTypeCasinoWins:
		return s.repo.GetCasinoWinsRanking, true
	case RankingTypeAlchemy:
		return s.repo.GetAlchemyRanking, true
	default:
		return nil, false
	}
}

func filterDependentBoard(leaders, candidates []CharacterRankingEntry) []CharacterRankingEntry {
	excluded := make(map[string]bool, len(leaders))
	for _, e := range leaders {
		excluded[e.CharacterID] = true
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
	return board
}

// loadPairedBoards computes a leader board and its dependent board from that single leader query.
func (s *Service) loadPairedBoards(ctx context.Context, leaderQuery, dependentQuery boardQuery) ([]CharacterRankingEntry, []CharacterRankingEntry, error) {
	leaders, _, err := leaderQuery(ctx, boardSize, 0)
	if err != nil {
		return nil, nil, err
	}
	if leaders == nil {
		leaders = []CharacterRankingEntry{}
	}
	// At most boardSize candidates are excluded, so twice the board always fills it.
	candidates, _, err := dependentQuery(ctx, 2*boardSize, 0)
	if err != nil {
		return nil, nil, err
	}
	return leaders, filterDependentBoard(leaders, candidates), nil
}

// loadSingleBoard computes an independent ranking.cgi board from current data.
func (s *Service) loadSingleBoard(ctx context.Context, query boardQuery) ([]CharacterRankingEntry, error) {
	entries, _, err := query(ctx, boardSize, 0)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []CharacterRankingEntry{}
	}
	return entries, nil
}

// getBoard pages a ranking.cgi board strictly from its published snapshot.
func (s *Service) getBoard(ctx context.Context, t RankingType, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getCharacterRankingPage(ctx, t, limit, offset)
}

// GetPvPVictoryRanking returns the 王者 board (kill_p) by strong-opponent PvP defeats.
func (s *Service) GetPvPVictoryRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypePvPVictory, limit, offset)
}

// GetMonsterKillsRanking returns the 英雄 board (kill_m) by defeated strong enemies.
func (s *Service) GetMonsterKillsRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeMonsterKills, limit, offset)
}

// GetMaoCountRanking returns the 魔王 board (mao_c) by unsealed demon king defeats.
func (s *Service) GetMaoCountRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeMaoCount, limit, offset)
}

// GetHeroCountRanking returns the 勇者 board (hero_c) by hero achievements.
func (s *Service) GetHeroCountRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeHeroCount, limit, offset)
}

// GetCasinoWinsRanking returns the 勝負師 board (cas_c) by casino showdown wins.
func (s *Service) GetCasinoWinsRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeCasinoWins, limit, offset)
}

// GetHelperRanking returns the 手助け board (help_c) by completed helper quests.
func (s *Service) GetHelperRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeHelper, limit, offset)
}

// GetAlchemyRanking returns the 錬金 board (alc_c) by alchemy syntheses.
func (s *Service) GetAlchemyRanking(ctx context.Context, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	return s.getBoard(ctx, RankingTypeAlchemy, limit, offset)
}
