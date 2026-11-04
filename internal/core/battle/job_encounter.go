package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// JobRoomContext is an operation-local Adventure port. TreasureIDs are owned,
// unopened boxes, projected as neutral targets only while resolving the helper.
// Validation is pure; Apply runs inside the enclosing command transaction.
type JobRoomContext struct {
	ActorID     string
	NormalStage bool
	Floor       int
	TreasureIDs []string
	Validate    func(string) error
	Apply       func(string) (TargetContext, error)
	// Cursor is updated only by the room owner's native all-member reset.
	Cursor TargetCursor
}

type JobEncounterContext struct {
	DamageContext
	Arguments          []string
	Room               *JobRoomContext
	DefeatContext      *DefeatContext
	Invoke             func(CallbackInvocation) error
	ResponseSuppressed bool
}

func (c *JobEncounterContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// treasureView preserves ordinary confusion, cover and fallback resolution
// without putting Adventure's claimable boxes into durable combat actors.
func (s *LifetimeState) treasureView(c *JobEncounterContext) (LifetimeState, TargetContext, error) {
	p, frame := *s, c.TargetContext
	p.Actors = slices.Clone(s.Actors)
	frame.EnemyIDs = slices.Clone(frame.EnemyIDs)
	for _, id := range c.Room.TreasureIDs {
		if id == "" || p.actor(id) != nil {
			return p, frame, ErrUnsupportedEffect
		}
		p.Actors = append(p.Actors, LifetimeActor{Participant: Participant{ID: id, TeamID: "job-treasure", HP: 1, MaxHP: 1}, NPC: true})
		frame.EnemyIDs = append(frame.EnemyIDs, id)
	}
	return p, frame, nil
}

func (s *LifetimeState) ValidateJobEncounter(c *JobEncounterContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	switch id {
	case "job-78-entry-2", "job-78-entry-6":
		if c.Room == nil || c.Room.ActorID != "" && c.Room.ActorID != c.ActorID {
			return ErrUnsupportedEffect
		}
		if id == "job-78-entry-2" && c.Room.NormalStage && c.Room.Floor >= 9 {
			return nil
		}
		if id == "job-78-entry-6" {
			p, frame, err := s.treasureView(c)
			if err != nil {
				return err
			}
			if err := p.validateTargetContext(frame, rng); err != nil {
				return err
			}
			child := c.DamageContext
			child.TargetContext = frame
			target := ""
			if len(c.Arguments) > 0 {
				target = c.Arguments[0]
			}
			reachable := false
			for _, e := range p.damageCandidates(&child, controlSymbol(target, "treasure-retry", CategoryUntyped)) {
				reachable = reachable || slices.Contains(c.Room.TreasureIDs, e.TargetID)
			}
			if !reachable {
				return nil
			}
		}
		if !c.Room.NormalStage || c.Room.Validate == nil || c.Room.Apply == nil {
			return ErrUnsupportedEffect
		}
		return c.Room.Validate(id)
	case "job-78-entry-4":
		if err := s.validateToolCursor(c.Cursor); err != nil {
			return err
		}
		p, child := s.callbackCopy(), *c
		if child.DefeatContext != nil {
			child.DamageContext = p.nativeDamageContext(child.DamageContext, child.DefeatContext, rng)
		}
		_, err := p.projectCallbackDamage(&child.DamageContext, child.DefeatContext, dualAgilityEffect(c.Cursor.ActorID, 50, CategoryBreath, true), 50, false, rng)
		return err
	case "job-79-entry-2":
		if s.actor(c.ActorID).NPC && c.Invoke == nil {
			return ErrUnsupportedEffect
		}
		return nil
	}
	return ErrUnsupportedEffect
}

// ApplyJobEncounter runs the intrinsic body only. Costs, action tails, response
// admission and durable receipts belong to the enclosing operation.
func (s *LifetimeState) ApplyJobEncounter(c *JobEncounterContext, id string, rng random.Generator) error {
	if err := s.ValidateJobEncounter(c, id, rng); err != nil {
		return err
	}
	switch id {
	case "job-78-entry-2":
		if c.Room.Floor >= 9 || rng.Float64()*3 >= 1 {
			return nil
		}
		frame, err := c.Room.Apply(id)
		if err == nil {
			c.TargetContext = frame
			c.Cursor = c.Room.Cursor
			if c.DefeatContext != nil {
				c.DefeatContext.Cursor = c.Cursor
			}
		}
		return err
	case "job-78-entry-6":
		p, frame, err := s.treasureView(c)
		if err != nil {
			return err
		}
		r, err := p.ResolveEnemyTarget(frame, controlSymbol(c.shift(), "treasure-retry", CategoryUntyped), rng)
		copy(s.Actors, p.Actors[:len(s.Actors)])
		if err != nil || !r.Resolved || !slices.Contains(c.Room.TreasureIDs, r.TargetID) {
			return err
		}
		ids := []string{c.ActorID}
		if rng.Float64()*5 < 3 {
			frame, err := c.Room.Apply(id)
			if err != nil {
				return err
			}
			c.TargetContext = frame
			ids = c.PartyIDs
		}
		for _, target := range ids {
			a := s.actor(target)
			if a.Jobs.JobID == "job-78" {
				a.Jobs.SP = 100
			}
			if a.Jobs.OldJobID == "job-78" {
				a.Jobs.OldSP = 100
			}
		}
	case "job-78-entry-4":
		if rng.Float64()*6 < 1 {
			s.actor(c.ActorID).Status, s.actor(c.ActorID).Tension = StatusDofuu, 1
			return nil
		}
		oldReset, oldDefeat := c.Reset, c.Defeat
		if c.DefeatContext != nil {
			c.DamageContext = s.nativeDamageContext(c.DamageContext, c.DefeatContext, rng)
		}
		defer func() { c.Reset, c.Defeat = oldReset, oldDefeat }()
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.Cursor.ActorID, 50, CategoryBreath, true), rng)
		if err != nil {
			return err
		}
		if c.DefeatContext != nil {
			c.Cursor = c.DefeatContext.Cursor
		}
		t := s.actor(r.TargetID)
		if r.Returned && t != nil && t.HP > 0 && !slices.Contains(c.PartyIDs, t.ID) && t.Temporary != TemporaryReflectBreath {
			c.EnemyIDs = append(slices.DeleteFunc(slices.Clone(c.EnemyIDs), func(id string) bool { return id == t.ID }), t.ID)
		}
	case "job-79-entry-2":
		if s.actor(c.ActorID).NPC {
			return c.Invoke(CallbackInvocation{Kind: "job-79-random"})
		}
		if rng.Float64()*2 < 1 {
			s.actor(c.ActorID).Status = StatusSleep
		}
		if len(c.EnemyIDs) > 0 {
			head := c.EnemyIDs[0]
			c.EnemyIDs = slices.Clone(c.EnemyIDs[1:])
			c.MemberOnlyIDs = append(slices.Clone(c.MemberOnlyIDs), head)
			members := make([]LifetimeActor, 0, len(s.Actors))
			for _, ids := range [][]string{c.PartyIDs, c.EnemyIDs, c.MemberOnlyIDs} {
				for _, target := range ids {
					members = append(members, *s.actor(target))
				}
			}
			s.Actors = members
		}
		c.ResponseSuppressed = true
		if s.SkillBans == nil {
			s.SkillBans = map[string]int{}
		}
		s.SkillBans[id] = 4 // The reached ordinary tail immediately ticks to three.
	}
	return nil
}
