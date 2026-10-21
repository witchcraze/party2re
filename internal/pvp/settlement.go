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

func (s *Service) deductBet(ctx context.Context, characterID string, amount int) error {
	c, err := s.characters.FindByIDForUpdate(ctx, characterID)
	if err != nil {
		return err
	}
	if err := c.DeductMoney(amount); err != nil {
		return ErrInsufficientBetFunds
	}
	return s.characters.Update(ctx, c)
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

// refundMembers uses the financial transition's transaction and ascending Rank 2 locks.
func (s *Service) refundMembers(ctx context.Context, memberIDs []string, amount int) error {
	if amount <= 0 || len(memberIDs) == 0 {
		return nil
	}
	sortedIDs := sortCharacterIDs(memberIDs)
	for _, id := range sortedIDs {
		char, err := s.characters.FindByIDForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("find character %s for refund: %w", id, err)
		}
		if err := char.AddMoney(amount); err != nil {
			return fmt.Errorf("add refund money to %s: %w", id, err)
		}
		if err := s.characters.Update(ctx, char); err != nil {
			return fmt.Errorf("update character %s refund: %w", id, err)
		}
	}
	return nil
}

// awardPrizes uses the financial transition's transaction and ascending Rank 2 locks.
func (s *Service) awardPrizes(ctx context.Context, winnerIDs []string, prizePerMember int) error {
	sortedIDs := sortCharacterIDs(winnerIDs)
	for _, id := range sortedIDs {
		wChar, err := s.characters.FindByIDForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("find winner character %s for prize: %w", id, err)
		}
		if prizePerMember > 0 {
			if err := wChar.AddMoney(prizePerMember); err != nil {
				return fmt.Errorf("add prize money to %s: %w", id, err)
			}
		}
		wChar.PvPWins++
		if err := s.characters.Update(ctx, wChar); err != nil {
			return fmt.Errorf("update winner character %s: %w", id, err)
		}
	}
	return nil
}
