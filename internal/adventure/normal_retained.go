package adventure

import "context"

// retainedDue adds the actor's disposed-timeout balances to the stock being
// released by the current command. Idle cleanup retains unpaid earnings instead
// of paying them, so the next eligible stock release is their only settlement.
func (s *NormalRunService) retainedDue(ctx context.Context, actor string, pending RunEarnings) ([]RetainedEarnings, RunEarnings, error) {
	retained, err := s.cfg.Store.FindRetainedEarnings(ctx, actor)
	if err != nil {
		return nil, RunEarnings{}, err
	}
	due := pending
	for _, entry := range retained {
		if due, err = addRunEarnings(due, entry.Recipient.Pending); err != nil {
			return nil, RunEarnings{}, err
		}
	}
	return retained, due, nil
}

// settleRetained moves paid balances to Applied in the settlement transaction.
// Try stock stays in Conditional, and the committed penalty is not rewritten.
func (s *NormalRunService) settleRetained(ctx context.Context, actor string, retained []RetainedEarnings) error {
	for _, entry := range retained {
		p := entry.Recipient
		var err error
		if p.Applied, err = addRunEarnings(p.Applied, p.Pending); err != nil {
			return err
		}
		p.Pending = RunEarnings{}
		if err := s.cfg.Store.SaveRetainedRecipient(ctx, entry.RunID, actor, p); err != nil {
			return err
		}
	}
	return nil
}
