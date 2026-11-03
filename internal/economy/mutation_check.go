package economy

import "context"

// SetMutationCheck validates actor-owned activity under the character lock,
// before costs, inventory locks or callbacks. Configure before serving requests.
func (s *Service) SetMutationCheck(check func(context.Context, string) error) {
	s.mutationCheck = check
}

func (s *Service) checkMutation(ctx context.Context, actor string) error {
	if s.mutationCheck != nil {
		return s.mutationCheck(ctx, actor)
	}
	return nil
}
