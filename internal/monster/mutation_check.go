package monster

import "context"

// SetMutationCheck validates the initiating actor under the character lock.
// Passive recipients and committed reward deliveries do not call this check.
// Configure before serving requests.
func (s *Service) SetMutationCheck(check func(context.Context, string) error) {
	s.mutationCheck = check
}

func (s *Service) checkMutation(ctx context.Context, actor string) error {
	if s.mutationCheck != nil {
		return s.mutationCheck(ctx, actor)
	}
	return nil
}
