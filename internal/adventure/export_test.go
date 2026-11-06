package adventure

import "context"

// SettlePostBattleForTest exposes the internal settlePostBattle method to tests.
func (s *Service) SettlePostBattleForTest(ctx context.Context, req DungeonCrawlRequest, result *DungeonCrawlResult) error {
	return s.settlePostBattle(ctx, req, result)
}
