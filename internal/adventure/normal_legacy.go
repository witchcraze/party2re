package adventure

import "context"

// SetLegacyAdmission keeps the retained batch entry from overwriting enrolled
// run resources. Composition supplies the shared sorted-character admission.
func (s *Service) SetLegacyAdmission(admit func(context.Context, []string, func(context.Context) error) error) {
	s.legacyEntry = admit
}

func (s *Service) ExecuteCrawl(ctx context.Context, req DungeonCrawlRequest) (DungeonCrawlResult, error) {
	if s.legacyEntry == nil {
		return s.executeCrawl(ctx, req)
	}
	var result DungeonCrawlResult
	err := s.legacyEntry(ctx, req.CharacterIDs, func(ctx context.Context) error {
		var err error
		result, err = s.executeCrawl(ctx, req)
		return err
	})
	return result, err
}
