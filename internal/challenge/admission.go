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

const startReconcileTimeout = 5 * time.Second

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
