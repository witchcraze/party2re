package character

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// ReferralDelivery appends the registration reward and notice using the creation
// transaction, after the service has locked the existing referrer character.
type ReferralDelivery func(ctx context.Context, referrerID string, joining corecharacter.Character) error

func WithReferralDelivery(delivery ReferralDelivery) Option {
	return func(s *Service) { s.referralDelivery = delivery }
}
