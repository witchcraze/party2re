package pvp

import (
	"context"
	"fmt"
	"sort"
)

// TransactionProvider can be injected into Service to orchestrate database transactions.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// WithTransactionProvider configures the transaction provider for Service.
func WithTransactionProvider(tp TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tp
	}
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func sortCharacterIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	var result []string
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

// refundMembers credits money to each character ID inside an atomic transaction
// with Rank 2 row locking in strictly ascending character ID order.
func (s *Service) refundMembers(ctx context.Context, memberIDs []string, amount int) error {
	if amount <= 0 || len(memberIDs) == 0 {
		return nil
	}
	sortedIDs := sortCharacterIDs(memberIDs)
	return s.runInTx(ctx, func(txCtx context.Context) error {
		for _, id := range sortedIDs {
			char, err := s.characters.FindByIDForUpdate(txCtx, id)
			if err != nil {
				return fmt.Errorf("find character %s for refund: %w", id, err)
			}
			if err := char.AddMoney(amount); err != nil {
				return fmt.Errorf("add refund money to %s: %w", id, err)
			}
			if err := s.characters.Update(txCtx, char); err != nil {
				return fmt.Errorf("update character %s refund: %w", id, err)
			}
		}
		return nil
	})
}

// awardPrizes distributes prize money and increments PvPWins for winners inside an atomic transaction
// with Rank 2 row locking in strictly ascending character ID order.
func (s *Service) awardPrizes(ctx context.Context, winnerIDs []string, prizePerMember int) ([]string, error) {
	if len(winnerIDs) == 0 {
		return nil, nil
	}
	sortedIDs := sortCharacterIDs(winnerIDs)
	var awardedIDs []string
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		awardedIDs = make([]string, 0, len(sortedIDs))
		for _, id := range sortedIDs {
			wChar, err := s.characters.FindByIDForUpdate(txCtx, id)
			if err != nil {
				return fmt.Errorf("find winner character %s for prize: %w", id, err)
			}
			if prizePerMember > 0 {
				if err := wChar.AddMoney(prizePerMember); err != nil {
					return fmt.Errorf("add prize money to %s: %w", id, err)
				}
			}
			wChar.PvPWins++
			if err := s.characters.Update(txCtx, wChar); err != nil {
				return fmt.Errorf("update winner character %s: %w", id, err)
			}
			awardedIDs = append(awardedIDs, wChar.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return awardedIDs, nil
}
