package challenge

import (
	"context"
	"errors"
	"github.com/witchcraze/party2re/internal/id"
)

func (s *Service) startRun(ctx context.Context, actor string, start func(context.Context, string) (*ChallengeSession, error)) (*ChallengeSession, error) {
	runID := id.New()
	var session *ChallengeSession
	err := s.transitions.WithRunLock(ctx, runID, func(ctx context.Context) error { var err error; session, err = start(ctx, runID); return err })
	if err != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startReconcileTimeout)
		defer cancel()
		result, readErr := s.transitions.FindCommandResult(rctx, runID, actor, 0)
		if readErr != nil {
			return nil, errors.Join(ErrOutcomeUnknown, err, readErr)
		}
		if result == nil {
			return nil, err
		}
		session = &result.Session
	}
	if err := s.publishPending(ctx, runID); err != nil {
		return session, errors.Join(ErrRecoveryRequired, err)
	}
	return session, nil
}

// MigrateSession is an explicit command, run only after old writers are quiesced.
// It preserves the old deadline; neither observation nor migration is activity.
func (s *Service) MigrateSession(ctx context.Context, actor, runID string) error {
	err := s.transitions.WithRunLock(ctx, runID, func(ctx context.Context) error {
		admission, err := s.repo.FindSessionByID(ctx, runID)
		if err != nil {
			return err
		}
		if !admission.ContainsCharacter(actor) {
			return ErrForbidden
		}
		t, err := s.transitions.FindTransition(ctx, runID)
		if err != nil {
			return err
		}
		if t.Revision > 0 {
			return nil
		}
		run, deadline, err := s.runStore.ReadLegacyRun(ctx, admission.CharacterID)
		if err != nil {
			return err
		}
		if run.ID != runID || run.Status != StatusActive || len(run.Members) == 0 || len(run.Members) != len(admission.Members) {
			return ErrRecoveryRequired
		}
		for i, m := range run.Members {
			if m.CharacterID != admission.Members[i].CharacterID || m.CharacterCurrentMP == nil {
				return ErrRecoveryRequired
			}
		}
		run.Revision = 1
		if len(run.AccumulatedItems) > 0 && len(run.TreasureAwards) == 0 {
			for _, item := range run.AccumulatedItems {
				run.TreasureAwards = append(run.TreasureAwards, TreasureAward{ItemID: item, RecipientID: admission.CharacterID})
			}
		}
		if err := s.transitions.EnrollRun(ctx, *run); err != nil {
			return err
		}
		return s.transitions.SaveTransition(ctx, Transition{RunID: runID, Revision: 1, Pending: true, Publication: run, Deadline: deadline, LegacyOwner: admission.CharacterID}, &CommandResult{ActorID: actor, Action: "migrate", ExpectedRevision: 0, Session: *run})
	})
	if err != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startReconcileTimeout)
		defer cancel()
		result, readErr := s.transitions.FindCommandResult(rctx, runID, actor, 0)
		if readErr != nil {
			return errors.Join(ErrOutcomeUnknown, err, readErr)
		}
		if result == nil || result.Action != "migrate" {
			return err
		}
	}
	return s.publishPending(ctx, runID)
}
