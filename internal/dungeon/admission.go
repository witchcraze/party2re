package dungeon

import (
	"context"
	"errors"
	"time"
)

var ErrForbidden = errors.New("character is not authorized for this expedition")

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

// ErrStartOutcomeUnknown reports that admission failed after the expedition was
// published and the published run could not be verified afterwards. State is
// preserved; callers observe the active expedition and must not replay the start.
var ErrStartOutcomeUnknown = errors.New("expedition start outcome could not be confirmed; observe the active expedition before retrying")

const startReconcileTimeout = 5 * time.Second

// reconcileStart resolves an admission error raised after run was published.
// Dungeon admission performs no SQL writes, so such an error (e.g. a failed
// commit that only releases character locks) cannot undo the Valkey-owned run.
// The identity-verified run is therefore the start result; anything else is
// reported as unknown without deleting state.
func (s *Service) reconcileStart(ctx context.Context, run *ActiveExpedition, admissionErr error) (*ActiveExpedition, error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startReconcileTimeout)
	defer cancel()
	current, err := s.activeStore.GetActiveExpedition(rctx, run.CharacterID)
	if err == nil && current != nil && current.ID == run.ID {
		return run, nil
	}
	return nil, errors.Join(ErrStartOutcomeUnknown, admissionErr, err)
}

// ContainsCharacter authorizes the leader or an enrolled participant.
func (run ActiveExpedition) ContainsCharacter(id string) bool {
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
