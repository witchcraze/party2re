package battle

import "github.com/witchcraze/party2re/internal/core/random"

func coverClonePayload(level int) HelperPayload {
	p := HelperPayload{Identity: "cover-clone", Name: "分身", Stats: [5]float64{2.5*float64(level) + 5, 500, 350, 50, 150}}
	if level >= 40 {
		p.Form, p.Stats = 1, [5]float64{3*float64(level) + 5, 500, 400, 180, 80}
	}
	if level >= 70 {
		p.Form, p.Stats = 2, [5]float64{3.5*float64(level) + 5, 500, 450, 200, 50}
	}
	return p
}

func (s *LifetimeState) applyCoverClone(c *BeastCloneContext, rng random.Generator) error {
	h := s.rosterIdentity("cover-clone")
	if h != nil {
		a := s.actor(c.ActorID)
		h.Temporary, a.Temporary = TemporaryCovering, TemporaryCover
		if a.HP >= a.MaxHP || h.HP <= 1 {
			return nil
		}
		if h.ID == a.ID {
			// A commanded clone can address its own identity through both roles.
			hp := a.HP
			a.HP = 1
			if hp > a.MaxHP/2 {
				a.HP = a.MaxHP
			}
			return nil
		}
		transfer := min(a.MaxHP-a.HP, h.HP-1)
		a.HP, h.HP = a.HP+transfer, h.HP-transfer
		return nil
	}
	child := RosterContext{DamageContext: c.DamageContext, RequesterItemID: c.RequesterItemID}
	r, err := s.AddHelper(&child, coverClonePayload(*c.RequesterLevel), rng)
	if err != nil {
		return err
	}
	c.DamageContext = child.DamageContext
	// AddHelper may reallocate the roster; reacquire both participants.
	a, h := s.actor(c.ActorID), s.actor(r.HelperID)
	h.Temporary, a.Temporary = TemporaryCovering, TemporaryCover
	h.Status, a.Status = a.Status, ""
	return nil
}
