package battle

import "github.com/witchcraze/party2re/internal/core/random"

// validateSacrificeDefeat checks the self-defeat that follows full party
// healing against its owner hooks without executing them.
func (s *LifetimeState) validateSacrificeDefeat(c *RevivalContext) error {
	if err := s.validateTargetContext(c.TargetContext, random.Default()); err != nil {
		return err
	}
	for _, a := range s.Actors {
		if a.MaxHP < 0 || a.MaxHP > 1e12 || a.MaxMP < 0 || a.MaxMP > 1e12 || a.MP < 0 || a.MP > 1e12 {
			return ErrInvalidParticipant
		}
	}
	p := s.callbackCopy()
	for _, target := range c.PartyIDs {
		if target != c.ActorID {
			p.actor(target).HP = p.actor(target).MaxHP
		}
	}
	defeat := *c.NativeDefeat
	defeat.TargetContext = c.TargetContext
	_, err := p.projectDefeatBranches(&defeat, c.ActorID, random.Default(), map[string]bool{}, true)
	return err
}
