package battle

import "github.com/witchcraze/party2re/internal/core/random"

// DuelCrownContext carries operation facts; descendant hooks share the existing
// damage owner's admission contract. Arguments belong to the invocation frame.
type DuelCrownContext struct {
	DamageContext
	Arguments []string
}

// DuelCrownEntries preserves the six positions at _skill.cgi:5594–5720.
func DuelCrownEntries() []CallbackEntry {
	return []CallbackEntry{
		{ID: "job-74-entry-0", Name: "provoke", RequiredSP: 10},
		{ID: "job-74-entry-1", Name: "buckler", RequiredSP: 30, MPCost: 2},
		{ID: "job-74-entry-2", Name: "duel", RequiredSP: 50, MPCost: 10},
		{ID: "job-74-entry-3", Name: "neutral-charge", RequiredSP: 70, MPCost: 40},
		{ID: "job-74-entry-4", Name: "verdict", RequiredSP: 120, MPCost: 14},
		{ID: "job-74-entry-5", Name: "crown-flurry", RequiredSP: 150, MPCost: 45},
	}
}

func (c *DuelCrownContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ProjectDuelCrown projects only the exact inline stance; random outcomes stay
// behind the invocation owner's AfterEffects gate.
func (s *LifetimeState) ProjectDuelCrown(c *DuelCrownContext, id string) (bool, error) {
	if err := s.ValidateDuelCrown(c, id, random.Default()); err != nil {
		return false, err
	}
	if id != "job-74-entry-1" {
		return false, nil
	}
	s.actor(c.ActorID).Temporary = TemporaryReduceAttack
	return true, nil
}

// ApplyDuelCrown executes intrinsic bodies. Listed costs belong to the outer
// command; delegated/random invocations keep their existing cost semantics.
func (s *LifetimeState) ApplyDuelCrown(c *DuelCrownContext, id string, rng random.Generator) error {
	if err := s.ValidateDuelCrown(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	a := s.actor(c.ActorID)
	switch id {
	case "job-74-entry-0":
		for _, member := range s.Actors {
			if member.HP <= 0 || member.ID == c.ActorID || rng.Float64()*2 < 1 {
				continue
			}
			if err := s.statTension(&c.EffectContext, s.actor(member.ID), false, rng); err != nil {
				return err
			}
		}
	case "job-74-entry-1":
		a.Temporary = TemporaryReduceAttack
	case "job-74-entry-2":
		return s.applyDuel(c, rng)
	case "job-74-entry-3":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 150, CategoryUntyped, true, rng)
		return err
	case "job-74-entry-4":
		if rng.Float64()*3 < 1 {
			a.Temporary = TemporaryRevival
		} else {
			a.Status = []string{StatusDeadlyPoison, StatusDofuu}[int(rng.Float64()*2)]
			a.Tension = 1
		}
	case "job-74-entry-5":
		return s.applyCrownFlurry(c, rng)
	}
	return nil
}

func (s *LifetimeState) applyDuel(c *DuelCrownContext, rng random.Generator) error {
	r, err := s.ResolveEnemyTarget(c.TargetContext, TargetEffect{TargetID: c.shift(), Payload: CallbackPayload{Symbol: "duel"}}, rng)
	if err != nil {
		return err
	}
	if c.targetResources != nil {
		c.targetResources(r)
	}
	target := ""
	if r.Resolved {
		target = r.TargetID
	}
	if s.actor(target) == nil {
		// The source indexes members by enemy count; rand(0) still draws rand(1).
		i := int(rng.Float64() * float64(max(1, len(c.EnemyIDs))))
		target = s.Actors[i].ID
	}
	actor := c.ActorID
	a, t := s.actor(actor), s.actor(target)
	actorHP, targetHP, attack := a.HP, t.HP, a.Attack
	a.Attack = int(float64(attack) * 1.1)
	restored := false
	defer func() {
		if !restored {
			s.actor(actor).Attack = attack
		}
	}()
	for range 2 {
		if _, err := s.applyNormalStrike(&c.DamageContext, target, rng); err != nil {
			return err
		}
		if s.actor(actor).HP <= 0 || s.actor(target).HP <= 0 {
			break
		}
		if err := s.duelReply(c, target, actor, rng); err != nil {
			return err
		}
		if s.actor(actor).HP <= 0 || s.actor(target).HP <= 0 {
			break
		}
	}
	a, t = s.actor(actor), s.actor(target)
	a.Attack, restored = attack, true
	win := false
	switch {
	case a.HP <= 0:
		t.HP = targetHP
	case t.HP <= 0:
		a.HP, win = actorHP, true
	case targetHP-t.HP > actorHP-a.HP:
		a.HP = actorHP
		win = rng.Float64()*4 < 1
	case actorHP-a.HP > targetHP-t.HP:
		t.HP = targetHP
	default:
		a.HP, t.HP = actorHP, targetHP
	}
	if win {
		if _, err := s.ApplyStatUp(&c.EffectContext, TargetEffect{TargetID: actor, Payload: CallbackPayload{Number: .2}}, StatAttack, rng); err != nil {
			return err
		}
		a = s.actor(actor)
		a.Extended, a.ExtendedTurns = ExtendedCrown, 5
	}
	return nil
}

func (s *LifetimeState) duelReply(c *DuelCrownContext, actor, target string, rng random.Generator) error {
	original, npc := c.TargetContext, c.ActorNPC
	c.TargetContext = duelReplyContext(original, actor)
	c.ActorNPC = s.actor(actor).NPC
	defer func() {
		original.Item195Activated = c.Item195Activated
		c.TargetContext, c.ActorNPC = original, npc
	}()
	_, err := s.applyNormalStrike(&c.DamageContext, target, rng)
	return err
}

func duelReplyContext(c TargetContext, actor string) TargetContext {
	c.duelOriginID, c.ActorID = c.ActorID, actor
	c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
	return c
}

func (s *LifetimeState) applyCrownFlurry(c *DuelCrownContext, rng random.Generator) error {
	count, target := 3, ""
	if s.actor(c.ActorID).Extended == ExtendedCrown {
		count, target = 6, c.shift()
	}
	coefficient := .9
	for hit := 1; hit <= count; hit++ {
		a := s.actor(c.ActorID)
		if a.HP <= 0 {
			break
		}
		r, err := s.ApplyDamage(&c.DamageContext, TargetEffect{TargetID: target, Payload: CallbackPayload{Number: float64(a.Attack) * coefficient}, Category: CategoryPhysical}, rng)
		if err != nil {
			return err
		}
		if r.Returned && r.TargetID != "" {
			coefficient *= 1 + rng.Float64()*.1
			if rng.Float64()*2 < 1 && hit <= 3 && s.actor(c.ActorID).Extended != ExtendedCrown {
				count++
			}
			if rng.Float64()*3 < 1 {
				if _, err := s.ApplyRecoil(&c.EffectContext, &c.End, float64(s.actor(c.ActorID).Attack)*.075*coefficient, rng); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
