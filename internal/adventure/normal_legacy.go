package adventure

import "context"

// SetLegacyAdmission keeps the retained batch entry from overwriting enrolled
// run resources. Composition supplies the shared sorted-character admission.
func (s *Service) SetLegacyAdmission(admit func(context.Context, []string, func(context.Context) error) error) {
	s.legacyEntry = admit
}

func (s *Service) ExecuteCrawl(ctx context.Context, req DungeonCrawlRequest) (DungeonCrawlResult, error) {
	var result DungeonCrawlResult
	crawl := func(ctx context.Context) error {
		var err error
		result, err = s.executeCrawl(ctx, req)
		return err
	}
	var err error
	if s.legacyEntry == nil {
		err = crawl(ctx)
	} else {
		err = s.legacyEntry(ctx, req.CharacterIDs, crawl)
	}
	if err == nil && result.UnsealDemonKing && s.unsealHook != nil {
		// The batch crawl has no quest name ($p_name).
		//lint:ignore error-swallow best-effort room opening after commit returns nothing to propagate
		s.unsealHook(ctx, "", result.Unsealers)
	}
	return result, err
}
