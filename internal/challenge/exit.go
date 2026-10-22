package challenge

import (
	"context"
)

type TreasureAward struct {
	ItemID      string `json:"item_id"`
	RecipientID string `json:"recipient_id"`
	Delivered   bool   `json:"delivered"`
}

// ExitSession settles only the executing participant and preserves survivors verbatim.
func (s *Service) ExitSession(ctx context.Context, actor, runID string, revision int) (*CommandResult, error) {
	return s.mutate(ctx, actor, runID, revision, "exit", func(ctx context.Context, run ChallengeSession) (*CommandResult, []string, error) {
		var departing ChallengeMember
		remaining := make([]ChallengeMember, 0, len(run.Members)-1)
		for _, m := range run.Members {
			if m.CharacterID == actor {
				departing = m
			} else {
				remaining = append(remaining, m)
			}
		}
		settlement := run
		settlement.Members = []ChallengeMember{departing}
		settlement.Status = StatusClaimed
		if err := s.repo.FinalizeSession(ctx, settlement, departing.AccumulatedExp, departing.AccumulatedGold, nil, run.CurrentRound-1); err != nil {
			return nil, nil, err
		}
		if s.fullness != nil {
			if err := s.fullness.ResetFullness(ctx, actor); err != nil {
				return nil, nil, err
			}
		}
		for i := range run.TreasureAwards {
			if run.TreasureAwards[i].RecipientID == actor {
				run.TreasureAwards[i].Delivered = true
			}
		}
		run.Members = remaining
		if len(remaining) == 0 {
			run.Status = StatusClaimed
		} else if run.CharacterID == actor {
			run.CharacterID = remaining[0].CharacterID
			run.CharacterCurrentHP = remaining[0].CharacterCurrentHP
		}
		return &CommandResult{Session: run, Settled: settlement.Members}, []string{actor}, nil
	})
}
