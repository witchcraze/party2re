package dungeon

import (
	"context"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// commitStep saves an action's step and then settles the run's pending
// credits. The acting member's stock (params.StockDrain) joins the credits in
// the same revision-checked step, as get_stock ends a living member's action
// (_battle.cgi:912).
func (s *Service) commitStep(ctx context.Context, exp *ActiveExpedition, ownerID string, params StepParams) (StepOutcome, error) {
	if id := params.StockDrain; id != "" {
		for _, m := range exp.Members {
			if m.CharacterID != id {
				continue
			}
			stock := corebattle.Reward{Experience: m.AccumulatedExp, Currency: m.AccumulatedGold, Crystals: m.AccumulatedCrystals}
			if stock == (corebattle.Reward{}) {
				break
			}
			var err error
			params.Credits, err = mergeCredit(params.Credits, DefeatCredit{ID: fmt.Sprintf("r%d:%s", exp.Revision, id), CharacterID: id, Reward: stock})
			if err != nil {
				return StepOutcome{}, err
			}
		}
	}
	out, err := s.activeStore.Step(ctx, ownerID, params)
	if err != nil {
		return StepOutcome{}, err
	}
	if err := s.settleCredits(ctx, &out.Expedition); err != nil {
		return StepOutcome{}, err
	}
	return out, nil
}

// settleCredits commits the run's pending credits to SQL, then drops them from
// the run. A failure leaves them pending; the next mutation or Escape retries,
// and the SQL receipt per credit ID prevents a second grant.
func (s *Service) settleCredits(ctx context.Context, exp *ActiveExpedition) error {
	if len(exp.Credits) == 0 {
		return nil
	}
	if err := s.repo.SettleDefeatCredits(ctx, exp.ID, exp.Credits); err != nil {
		return err
	}
	ids := make([]string, len(exp.Credits))
	for i, c := range exp.Credits {
		ids[i] = c.ID
	}
	if err := s.activeStore.AckCredits(ctx, exp.CharacterID, exp.ID, ids); err != nil {
		return err
	}
	exp.Credits = nil
	return nil
}
