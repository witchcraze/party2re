package character

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestCreateReferralUsesCreationTransaction(t *testing.T) {
	for _, target := range []string{"", "missing", " referrer "} {
		t.Run(target, func(t *testing.T) {
			repo := newMockRepository()
			repo.characters["referrer"] = corecharacter.Character{ID: "referrer", PlayerID: "player-1"}
			type txKey struct{}
			calls := 0
			service, err := NewService(repo,
				WithTransactionProvider(&mockTransactionProvider{fn: func(ctx context.Context, fn func(context.Context) error) error {
					return fn(context.WithValue(ctx, txKey{}, true))
				}}),
				WithReferralDelivery(func(ctx context.Context, referrerID string, joining corecharacter.Character) error {
					if _, err := repo.FindByIDForUpdate(ctx, referrerID); errors.Is(err, ErrNotFound) {
						return nil
					} else if err != nil {
						return err
					}
					calls++
					if ctx.Value(txKey{}) != true || referrerID != "referrer" || repo.characters[joining.ID].ID != joining.ID {
						t.Fatalf("delivery outside saved creation transaction: referrer=%q joining=%+v", referrerID, joining)
					}
					return nil
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{JobID: "job-01", Gender: "m", ReferrerCharacterID: target})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if target == " referrer " {
				want = 1
			}
			if calls != want {
				t.Fatalf("deliveries=%d, want %d", calls, want)
			}
		})
	}
}

func TestCreateReferralErrors(t *testing.T) {
	failure := errors.New("referral failure")
	for _, stage := range []string{"delivery", "unconfigured", "no_transaction"} {
		t.Run(stage, func(t *testing.T) {
			repo := newMockRepository()
			repo.characters["referrer"] = corecharacter.Character{ID: "referrer"}
			opts := []Option{WithTransactionProvider(&mockTransactionProvider{})}
			if stage == "no_transaction" {
				opts = nil
			}
			if stage != "unconfigured" {
				opts = append(opts, WithReferralDelivery(func(context.Context, string, corecharacter.Character) error { return failure }))
			}
			service, err := NewService(repo, opts...)
			if err != nil {
				t.Fatal(err)
			}
			created, err := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{JobID: "job-01", Gender: "m", ReferrerCharacterID: "referrer"})
			if err == nil || created.ID != "" {
				t.Fatalf("created=%+v err=%v", created, err)
			}
			if stage == "delivery" && !errors.Is(err, failure) {
				t.Fatalf("lost referral error: %v", err)
			}
		})
	}
}
