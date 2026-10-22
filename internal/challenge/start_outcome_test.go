package challenge_test

import (
	"context"
	"errors"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"testing"
)

var errCommit = errors.New("commit acknowledgement lost")

type commitLossTransitions struct {
	challenge.TransitionRepository
	lose       bool
	unreadable bool
	cancel     context.CancelFunc
	depth      int
}

func (r *commitLossTransitions) WithRunLock(ctx context.Context, id string, fn func(context.Context) error) error {
	r.depth++
	defer func() { r.depth-- }()
	err := r.TransitionRepository.WithRunLock(ctx, id, fn)
	if err == nil && r.lose && r.depth == 1 {
		r.lose = false
		if r.cancel != nil {
			r.cancel()
		}
		return errCommit
	}
	return err
}
func (r *commitLossTransitions) FindCommandResult(ctx context.Context, id, actor string, rev int) (*challenge.CommandResult, error) {
	if r.unreadable {
		return nil, errors.New("receipt unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.TransitionRepository.FindCommandResult(ctx, id, actor, rev)
}

type publicationFailure struct {
	challenge.ActiveSessionStore
	after bool
	fail  bool
}

func (r *publicationFailure) PublishRun(ctx context.Context, tr challenge.Transition) error {
	if r.fail && !r.after {
		return errors.New("publication failed")
	}
	if err := r.ActiveSessionStore.PublishRun(ctx, tr); err != nil {
		return err
	}
	if r.fail {
		return errors.New("publication acknowledgement lost")
	}
	return nil
}

func TestStartRunCommitEvidenceAndPendingPublication(t *testing.T) {
	for _, kind := range []string{"commit loss", "commit unreadable", "canceled commit", "body failure", "publication acknowledgement"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transitions := &commitLossTransitions{TransitionRepository: challenge.NewMemoryTransitionRepository(), lose: true}
			store := &publicationFailure{ActiveSessionStore: challenge.NewMemorySessionRepository()}
			if kind == "commit unreadable" {
				transitions.unreadable = true
			}
			if kind == "canceled commit" {
				transitions.cancel = cancel
			}
			if kind == "body failure" || kind == "publication acknowledgement" {
				transitions.lose = false
				store.fail = true
				store.after = kind == "publication acknowledgement"
			}
			chars := &mockCharRepo{chars: map[string]corecharacter.Character{"lead": createPartyTestChar("lead", 30, 200, 100, 100)}}
			svc, err := challenge.NewService(newMockChallengeRepo(), chars, roundWinEngine{}, challenge.WithActiveSessionStore(store), challenge.WithTransitionRepository(transitions))
			if err != nil {
				t.Fatal(err)
			}
			run, err := svc.StartSession(ctx, "lead", "0")
			if kind == "commit unreadable" {
				if run != nil || !errors.Is(err, challenge.ErrOutcomeUnknown) {
					t.Fatalf("unknown: %+v %v", run, err)
				}
				return
			}
			if kind == "commit loss" {
				if err != nil || run == nil {
					t.Fatalf("receipt did not resolve commit: %+v %v", run, err)
				}
			}
			if kind == "body failure" || kind == "publication acknowledgement" {
				if run == nil || !errors.Is(err, challenge.ErrRecoveryRequired) {
					t.Fatalf("pending: %+v %v", run, err)
				}
				observed, err := svc.GetActiveSession(context.Background(), "lead")
				if err != nil || observed == nil || !observed.RecoveryPending {
					t.Fatalf("pending GET: %+v %v", observed, err)
				}
				// Reading must not acknowledge or retry the failed publication.
				tr, err := transitions.FindTransition(context.Background(), run.ID)
				if err != nil || !tr.Pending {
					t.Fatalf("GET mutated intent: %+v %v", tr, err)
				}
				store.fail = false
				_, next, err := svc.AdvanceRound(context.Background(), "lead", run.ID, run.Revision)
				if err != nil || next.CurrentRound != 2 {
					t.Fatalf("explicit recovery: %+v %v", next, err)
				}
			}
			if kind == "canceled commit" {
				// The committed command remains owned even if its response/publication is canceled.
				receipt, readErr := svc.GetCommandResult(context.Background(), "lead", run.ID, 0)
				if readErr != nil || receipt.Action != "start" {
					t.Fatalf("canceled receipt: %+v %v", receipt, readErr)
				}
			}
		})
	}
}
