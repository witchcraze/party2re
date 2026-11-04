package character

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// guardedRecipientRepository models the actor-mutation guard on the primary repository.
type guardedRecipientRepository struct {
	*mockRepository
	err error
}

func (r guardedRecipientRepository) FindByIDForUpdate(context.Context, string) (corecharacter.Character, error) {
	return corecharacter.Character{}, r.err
}

// FindRecipientByIDForUpdate is the unguarded passive-recipient lock.
func (r guardedRecipientRepository) FindRecipientByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.mockRepository.FindByIDForUpdate(ctx, id)
}

func TestService_CreateWithOptionsLocksPassiveReferrerWithoutActorGuard(t *testing.T) {
	raw := newMockRepository()
	referrer := corecharacter.Character{ID: "referrer-1", Name: "Referrer", PlayerID: "player-a"}
	raw.characters[referrer.ID] = referrer
	guardErr := errors.New("actor guard must not apply to referral recipient")
	delivered := 0
	service, err := NewService(guardedRecipientRepository{mockRepository: raw, err: guardErr},
		WithTransactionProvider(&mockTransactionProvider{}),
		WithReferralDelivery(func(context.Context, string, corecharacter.Character) error {
			delivered++
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateWithOptions(context.Background(), "player-b", "Newcomer", CreationOptions{
		JobID: "job-01", Gender: "m", ReferrerCharacterID: referrer.ID,
	}); err != nil {
		t.Fatalf("referred registration rejected: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("referral deliveries=%d, want 1", delivered)
	}
}
