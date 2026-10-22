package challenge

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRevisionConflict = errors.New("challenge revision or command inputs conflict")
	ErrRecoveryRequired = errors.New("challenge owned activity requires explicit recovery")
	ErrOutcomeUnknown   = errors.New("challenge command outcome could not be confirmed")
)

// CommandResult belongs to one actor and expected revision, even after departure.
type CommandResult struct {
	ActorID          string            `json:"actor_id"`
	Action           string            `json:"action"`
	ExpectedRevision int               `json:"expected_revision"`
	Session          ChallengeSession  `json:"session"`
	Round            *RoundResult      `json:"round,omitempty"`
	Settled          []ChallengeMember `json:"settled,omitempty"`
}

// Transition retains the exact publication until all required writes are acknowledged.
type Transition struct {
	RunID       string
	Revision    int
	Pending     bool
	Publication *ChallengeSession
	RemovedIDs  []string
	LegacyOwner string
	Deadline    time.Time
}

// TransitionRepository serializes every writer before reading live state.
// SaveTransition and EnrollRun require the context supplied by WithRunLock.
type TransitionRepository interface {
	WithRunLock(context.Context, string, func(context.Context) error) error
	FindTransition(context.Context, string) (Transition, error)
	FindClaim(context.Context, string) (string, error)
	FindCommandResult(context.Context, string, string, int) (*CommandResult, error)
	EnrollRun(context.Context, ChallengeSession) error
	SaveTransition(context.Context, Transition, *CommandResult) error
	AcknowledgePublication(context.Context, string, int) error
}

// RunStore publishes only committed, revision-qualified transitions. Reads never renew TTL.
type RunStore interface {
	SessionTTL() time.Duration
	ReadRun(context.Context, string) (*ChallengeSession, error)
	PublishRun(context.Context, Transition) error
	ReadLegacyRun(context.Context, string) (*ChallengeSession, time.Time, error)
}

type FullnessResetter interface {
	ResetFullness(context.Context, string) error
}

func WithFullnessResetter(owner FullnessResetter) Option {
	return func(s *Service) { s.fullness = owner }
}

func (s *Service) ConfigureFullnessResetter(owner FullnessResetter) { s.fullness = owner }

func WithTransitionRepository(repo TransitionRepository) Option {
	return func(s *Service) { s.transitions = repo }
}

func (s *Service) GetCommandResult(ctx context.Context, actor, runID string, revision int) (*CommandResult, error) {
	result, err := s.transitions.FindCommandResult(ctx, runID, actor, revision)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrForbidden
	}
	return result, nil
}

func (s *Service) publishPending(ctx context.Context, runID string) error {
	return s.transitions.WithRunLock(ctx, runID, func(ctx context.Context) error {
		t, err := s.transitions.FindTransition(ctx, runID)
		if err != nil || !t.Pending {
			return err
		}
		if err := s.runStore.PublishRun(ctx, t); err != nil {
			return err
		}
		return s.transitions.AcknowledgePublication(ctx, runID, t.Revision)
	})
}

func (s *Service) saveMutation(ctx context.Context, before ChallengeSession, result *CommandResult, removed []string) error {
	result.Session.Revision = before.Revision + 1
	result.Session.UpdatedAt = time.Now().UTC()
	return s.transitions.SaveTransition(ctx, Transition{RunID: before.ID, Revision: result.Session.Revision,
		Pending: true, Publication: &result.Session, RemovedIDs: removed,
		Deadline: time.Now().UTC().Add(s.runStore.SessionTTL())}, result)
}

// mutate returns a committed receipt on lost SQL acknowledgement; publication failure
// preserves that successful result with ErrRecoveryRequired for an explicit retry.
func (s *Service) mutate(ctx context.Context, actor, runID string, revision int, action string,
	compute func(context.Context, ChallengeSession) (*CommandResult, []string, error)) (*CommandResult, error) {
	if actor == "" {
		return nil, ErrCharacterNotFound
	}
	if runID == "" || revision < 1 {
		return nil, ErrRevisionConflict
	}
	var result *CommandResult
	err := s.transitions.WithRunLock(ctx, runID, func(ctx context.Context) error {
		admission, err := s.repo.FindSessionByID(ctx, runID)
		if err != nil {
			return err
		}
		if !admission.ContainsCharacter(actor) {
			return ErrForbidden
		}
		// Recovery may publish but never recomputes or pays the preceding command.
		if err := s.publishPending(ctx, runID); err != nil {
			return errors.Join(ErrRecoveryRequired, err)
		}
		stored, err := s.transitions.FindCommandResult(ctx, runID, actor, revision)
		if err != nil {
			return err
		}
		if stored != nil {
			if stored.Action != action {
				return ErrRevisionConflict
			}
			result = stored
			return nil
		}
		run, err := s.runStore.ReadRun(ctx, runID)
		if err != nil {
			return err
		}
		if run == nil {
			return ErrRecoveryRequired
		}
		if !run.ContainsCharacter(actor) {
			return ErrForbidden
		}
		if run.Revision != revision {
			return ErrRevisionConflict
		}
		coord, err := s.transitions.FindTransition(ctx, runID)
		if err != nil {
			return err
		}
		if coord.Revision != run.Revision {
			return ErrRevisionConflict
		}
		if run.Status != StatusActive {
			return ErrSessionNotActive
		}
		var removed []string
		result, removed, err = compute(ctx, *run)
		if err != nil {
			return err
		}
		result.ActorID, result.Action, result.ExpectedRevision = actor, action, revision
		return s.saveMutation(ctx, *run, result, removed)
	})
	if err != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startReconcileTimeout)
		defer cancel()
		stored, readErr := s.transitions.FindCommandResult(rctx, runID, actor, revision)
		if readErr != nil {
			return nil, errors.Join(ErrOutcomeUnknown, err, readErr)
		}
		if stored == nil {
			return nil, err
		}
		if stored.Action != action {
			return nil, ErrRevisionConflict
		}
		result = stored
	}
	if err := s.publishPending(ctx, runID); err != nil {
		return result, errors.Join(ErrRecoveryRequired, err)
	}
	return result, nil
}
