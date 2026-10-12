package battle

// LifetimeCompletion describes the reached operation blocks. Admission/receipts
// belong to the consumer: GET, replay, rejected and ineligible commands supply
// Accepted=false. Accepted incapacitated operations still reach applicable tails.
type LifetimeCompletion struct {
	Accepted         bool
	ActorID          string
	NPC              bool
	Suppressed       bool
	EnemiesRemain    bool
	NPCComment       bool
	LevelUp          bool
	Tail             LifetimeTail
	CMPAmount        int
	NextCMPThreshold int
	End              EndProcessing
}

type LifetimeResult struct {
	ExtendedExpired bool
	CMPRecharged    bool
	IncenseHolder   string
}

// CompleteOperation finishes counters and valid end processing before exposing
// the next saved state. Poison/normal-action effects precede this operation;
// source-specific modifier and death effects must already have been resolved.
// Player and NPC tails intentionally differ. No clock or round tick is involved.
func (s *LifetimeState) CompleteOperation(c LifetimeCompletion, partyIDs []string) (LifetimeResult, error) {
	var result LifetimeResult
	if !c.Accepted {
		return result, nil
	}
	actor := s.actor(c.ActorID)
	if actor == nil {
		return result, ErrInvalidParticipant
	}
	if err := c.End.Validate(); err != nil {
		return result, err
	}
	playerTail := !c.NPC && c.EnemiesRemain && !c.Suppressed && !c.NPCComment && !c.LevelUp
	if playerTail {
		for _, id := range partyIDs {
			if s.actor(id) == nil {
				return result, ErrInvalidParticipant
			}
		}
		if actor.CMPCharge >= actor.CMPNeedCharge && (c.CMPAmount < 1 || c.NextCMPThreshold < 3 || c.NextCMPThreshold > 9) {
			return result, ErrInvalidRequest
		}
	}
	if !c.NPC {
		s.Fields.FinishOperation(c.EnemiesRemain, c.Suppressed)
	}
	if c.NPC || playerTail {
		facts := c.Tail
		facts.NPC = c.NPC
		if playerTail {
			result.CMPRecharged = actor.AdvanceCMP(c.CMPAmount, c.NextCMPThreshold)
		}
		result.ExtendedExpired = actor.FinishExtended(facts)
		if playerTail {
			result.IncenseHolder = s.TickIncense(partyIDs)
			s.TickSkillBans()
		}
		// NPC flush is outside its normal-action block. Player flush is inside
		// the eligible tail; suppression is not a universal poison/effect skip.
		c.End.Flush(actor)
	}
	return result, nil
}

func (s *LifetimeState) actor(id string) *LifetimeActor {
	for i := range s.Actors {
		if s.Actors[i].ID == id {
			return &s.Actors[i]
		}
	}
	return nil
}

// EndProcessing is operation-local typed work, never part of LifetimeState.
// Only the valid tagged darkness debit is supported at the referenced revision.
// Bare CODE gems101/102 and enclosing closures must fail whole-entry admission;
// there is no supported queued _risk callback to repair or execute here.
type EndProcessing struct {
	DarknessDebit      *int
	ImmediateRiskCalls int
	SkipPrefilter      bool
}

func (e EndProcessing) Validate() error {
	if e.ImmediateRiskCalls < 0 || (e.DarknessDebit != nil && (*e.DarknessDebit < 0 || *e.DarknessDebit > 500)) {
		return ErrInvalidRequest
	}
	return nil
}

// RecordDarkness captures only the first qualifying hit, including a zero debit.
// Call at _damage:8984–8994 after its preceding gates; not on every HP mutation.
func (e *EndProcessing) RecordDarkness(damage int) {
	if e.DarknessDebit != nil {
		return
	}
	debit := min(500, max(0, damage/2))
	e.DarknessDebit = &debit
}

// Flush prefilters against immediate risk once, before execution. __risk does
// not increment _risk's counter, and cannot kill/revive a living actor.
func (e EndProcessing) Flush(a *LifetimeActor) {
	if e.DarknessDebit == nil || (!e.SkipPrefilter && e.ImmediateRiskCalls > 0) || a.HP <= 0 {
		return
	}
	a.HP = max(1, a.HP-*e.DarknessDebit)
}
