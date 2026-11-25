package dungeon

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

var ErrForbidden = errors.New("character is not authorized for this expedition")

var ErrPartyUnavailable = errors.New("party admission is unavailable")

// ErrActorFallen rejects commands from a fallen character; only Escape remains.
var ErrActorFallen = errors.New("a fallen character cannot act")

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

// reconcileStart resolves a save or admission error raised once run may have
// been published.
// Dungeon admission performs no SQL writes, so such an error (e.g. a failed
// commit that only releases character locks) cannot undo the Valkey-owned run.
// The identity-verified run is therefore the start result; anything else is
// reported as unknown without deleting state or releasing a once quota.
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

// isAlive reports whether an enrolled character has HP left; a run without a
// roster snapshot carries only the leader's HP.
func (run ActiveExpedition) isAlive(id string) bool {
	for _, member := range run.Members {
		if member.CharacterID == id {
			return member.CurrentHP > 0
		}
	}
	return len(run.Members) == 0 && run.CharacterID == id && run.CurrentHP > 0
}

// anyAlive reports whether the party still has a member who can act.
func (run ActiveExpedition) anyAlive() bool {
	for _, member := range run.Members {
		if member.CurrentHP > 0 {
			return true
		}
	}
	return len(run.Members) == 0 && run.CurrentHP > 0
}

// ErrOnceQuotaUsed rejects a once-quota map while the leader's quota is consumed.
var ErrOnceQuotaUsed = errors.New("the leader has already used today's limited dungeon; sleep at home to enter again")

var errOnceQuotaUnavailable = errors.New("once quota is not configured")

// OnceQuota is the leader's once quota shared with Adventure
// (timer.CategoryDungeonOnce); Home wake resets it.
type OnceQuota interface {
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error)
}

func WithOnceQuota(quota OnceQuota) Option {
	return func(s *Service) { s.quota = quota }
}

// acquireOnce consumes the leader's quota for run, tokened by the run ID.
func (s *Service) acquireOnce(ctx context.Context, leaderID, runID string) error {
	if s.quota == nil {
		return errOnceQuotaUnavailable
	}
	ok, err := s.quota.TryLockWithToken(ctx, timer.CategoryDungeonOnce, leaderID, runID, 0)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOnceQuotaUsed
	}
	return nil
}

// releaseOnce returns the quota of a start that published no run.
func (s *Service) releaseOnce(ctx context.Context, leaderID, runID string) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startReconcileTimeout)
	defer cancel()
	//lint:ignore error-swallow compensatory release of an unpublished start; Home wake resets a stuck quota
	_, _ = s.quota.ReleaseLockWithToken(rctx, timer.CategoryDungeonOnce, leaderID, runID)
}

func (s *Service) onceUsed(ctx context.Context, leaderID string) (bool, error) {
	if s.quota == nil {
		return true, nil
	}
	return s.quota.IsLocked(ctx, timer.CategoryDungeonOnce, leaderID)
}
