package main

import (
	"context"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/database"
)

func retireChallenge(ctx context.Context, svc *challenge.Service, actor string) error {
	run, err := svc.GetActiveSession(ctx, actor)
	if err != nil || run == nil {
		return err
	}
	_, err = svc.ExitSession(ctx, actor, run.ID, run.Revision)
	return err
}

// Challenge owns the outer transaction; inject commit faults at that boundary.
type challengeCommitFault struct {
	challenge.TransitionRepository
	mode string
	err  error
}

func (f *challengeCommitFault) WithRunLock(ctx context.Context, id string, fn func(context.Context) error) error {
	if database.TxFromContext(ctx) != nil {
		return f.TransitionRepository.WithRunLock(ctx, id, fn)
	}
	mode := f.mode
	f.mode = ""
	if mode == "abort" {
		return f.TransitionRepository.WithRunLock(ctx, id, func(ctx context.Context) error {
			if err := fn(ctx); err != nil {
				return err
			}
			return f.err
		})
	}
	if err := f.TransitionRepository.WithRunLock(ctx, id, fn); err != nil {
		return err
	}
	if mode == "lost-ack" {
		return f.err
	}
	return nil
}
