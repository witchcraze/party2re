package challenge

import (
	"context"
	"errors"
	"time"
)

var ErrPartyUnavailable = errors.New("party admission is unavailable")

// Admission coordinates live party consent and character activity locks.
// Implemented at the composition root through the Party public service.
type Admission interface {
	AdmitParty(context.Context, string, string, []string, func(context.Context, string, []string) error) error
	AdmitSolo(context.Context, string, func(context.Context) error) error
}

func WithAdmission(admission Admission) Option {
	return func(s *Service) { s.admission = admission }
}

func (s *Service) ConfigureAdmission(admission Admission) { s.admission = admission }

// ErrStartOutcomeUnknown reports that admission failed after the session was
// published and its SQL start row could not be confirmed present or absent.
// State is preserved; callers observe the active session and must not replay the start.
var ErrStartOutcomeUnknown = errors.New("challenge start outcome could not be confirmed; observe the active session before retrying")

const startReconcileTimeout = 5 * time.Second

// reconcileStart resolves an admission error raised after session was
// published to Valkey. The SQL start row is the commit evidence: a present row
// means the start committed (the error was lost acknowledgement), a confirmed
// absent row means the transaction aborted and only this session is removed,
// and anything else is reported as unknown without deleting state. A commit
// that is still in flight server-side cannot be excluded without a durable
// receipt; see docs/architecture/transient-run-state.md.
func (s *Service) reconcileStart(ctx context.Context, session *ChallengeSession, admissionErr error) (*ChallengeSession, error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startReconcileTimeout)
	defer cancel()
	stored, err := s.repo.FindSessionByID(rctx, session.ID)
	switch {
	case err == nil && stored != nil:
		return session, nil
	case errors.Is(err, ErrSessionNotFound):
		return nil, errors.Join(admissionErr, s.discardPublished(rctx, session))
	default:
		return nil, errors.Join(ErrStartOutcomeUnknown, admissionErr, err)
	}
}

// discardPublished removes the published session only while it is still the
// leader's current run, so a replacement enrollment is never deleted.
func (s *Service) discardPublished(ctx context.Context, session *ChallengeSession) error {
	current, err := s.activeStore.GetActiveSession(ctx, session.CharacterID)
	if err != nil {
		return err
	}
	if current == nil || current.ID != session.ID {
		return nil
	}
	return s.activeStore.DeleteActiveSession(ctx, session.CharacterID)
}

// ContainsCharacter authorizes the leader or an enrolled participant.
func (run ChallengeSession) ContainsCharacter(id string) bool {
	if run.CharacterID == id {
		return true
	}
	for _, member := range run.Members {
		if member.CharacterID == id {
			return true
		}
	}
	return false
}
