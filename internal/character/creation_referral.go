package character

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// ReferralDelivery locks the existing passive recipient and appends the registration reward
// and notice using the ambient creation transaction.
type ReferralDelivery func(ctx context.Context, referrerID string, joining corecharacter.Character) error

func WithReferralDelivery(delivery ReferralDelivery) Option {
	return func(s *Service) { s.referralDelivery = delivery }
}
