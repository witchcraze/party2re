package depot

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// RemittanceSource says why gold is held; clients render the receipt message.
type RemittanceSource string

const RemittanceDepotTransfer RemittanceSource = "depot_transfer"

const RemittancePvPPrize RemittanceSource = "pvp_prize"

const RemittanceStorePayment RemittanceSource = "store_payment"

// Remittance is gold held for CharacterID until collected at the Depot.
type Remittance struct {
	ID          string
	CharacterID string
	Amount      int
	Source      RemittanceSource
	SenderName  string
}

// RemittanceRepository stores held gold. Mutations run under the recipient's
// character lock; ListRemittances returns the oldest first.
type RemittanceRepository interface {
	AddRemittance(ctx context.Context, value Remittance) error
	ListRemittances(ctx context.Context, characterID string) ([]Remittance, error)
	// SetRemittanceAmount keeps the remaining amount, deleting the row at 0.
	SetRemittanceAmount(ctx context.Context, characterID, remittanceID string, amount int) error
}

var errRemittancesUnconfigured = errors.New("depot remittance repository is required")

func WithRemittanceRepository(repo RemittanceRepository) Option {
	return func(s *Service) { s.remittances = repo }
}

// PendingRemittances lists gold held for the character, oldest first.
func (s *Service) PendingRemittances(ctx context.Context, characterID string) ([]Remittance, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidCharacterID
	}
	if s.remittances == nil {
		return nil, nil // Nothing can be held without a repository.
	}
	return s.remittances.ListRemittances(ctx, characterID)
}

// collectRemittances credits held gold oldest first up to the wallet cap and
// keeps the excess pending, returning the amounts taken. The caller holds the
// character lock and saves the character.
func (s *Service) collectRemittances(ctx context.Context, char *corecharacter.Character) ([]Remittance, error) {
	pending, err := s.PendingRemittances(ctx, char.ID)
	if err != nil {
		return nil, err
	}
	room := corecharacter.MaxMoney - char.Money
	var collected []Remittance
	for _, r := range pending {
		if room <= 0 {
			break
		}
		take := min(r.Amount, room)
		if err := s.remittances.SetRemittanceAmount(ctx, char.ID, r.ID, r.Amount-take); err != nil {
			return nil, err
		}
		if err := char.AddMoney(take); err != nil {
			return nil, err
		}
		room -= take
		r.Amount = take
		collected = append(collected, r)
	}
	return collected, nil
}
