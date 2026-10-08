package party

import "context"

// GetActiveParty reads the live lobby and actor's membership without joining,
// refreshing ready timers, or consulting historical adventure logs.
func (s *Service) GetActiveParty(ctx context.Context, characterID string) (Party, Member, error) {
	return s.repo.GetActivePartyByCharacter(ctx, characterID)
}
