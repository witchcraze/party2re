package main

import (
	"context"
	"sort"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/party"
)

type combatAdmission struct {
	party interface {
		WithReadyRoster(context.Context, string, string, []string, func(context.Context, string, []string) error) error
	}
	characters interface {
		FindByIDForUpdate(context.Context, string) (corecharacter.Character, error)
	}
	tx   economy.TransactionProvider
	busy func(context.Context, string) error
}

func (a combatAdmission) AdmitParty(ctx context.Context, actor, partyID string, claimed []string, start func(context.Context, string, []string) error) error {
	return a.party.WithReadyRoster(ctx, actor, partyID, claimed, func(ctx context.Context, resolved string, ids []string) error {
		return a.withMembers(ctx, ids, func(ctx context.Context) error { return start(ctx, resolved, ids) })
	})
}

func (a combatAdmission) AdmitSolo(ctx context.Context, actor string, start func(context.Context) error) error {
	return a.withMembers(ctx, []string{actor}, start)
}

// withMembers serializes solo and cooperative starts across both combat owners.
// Transaction: RunInTx. Lock Order: characters(2), sorted by ID.
func (a combatAdmission) withMembers(ctx context.Context, ids []string, start func(context.Context) error) error {
	lockIDs := append([]string(nil), ids...)
	sort.Strings(lockIDs)
	return a.tx.RunInTx(ctx, func(ctx context.Context) error {
		for _, id := range lockIDs {
			char, err := a.characters.FindByIDForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if char.Stats.HP <= 0 {
				return party.ErrCharacterUnconscious
			}
			if char.Tired >= 100 {
				return party.ErrCharacterExhausted
			}
			if char.PendingWake {
				return party.ErrMemberBusy
			}
		}
		for _, id := range ids {
			if err := a.busy(ctx, id); err != nil {
				return err
			}
		}
		return start(ctx)
	})
}
