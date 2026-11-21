package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// PlayerAction is one parsed submission entry whose callback the consumer has
// bound to its definitions. Validate is pure; Execute runs the bound callback.
type PlayerAction struct {
	EntryID string
	// Cost is charged after the callback: the first entry's listed cost after
	// item165/item249, the second entry's raw listed cost (_battle.cgi:761–785).
	Cost        int
	Whitelisted bool // the seven-entry additional-submission list
	Bow         bool // ticks the bow extended effect
	MirrorShare bool // preserves an actor reflection at normal-action expiry
	FallenSelf  bool // the only entry a fallen actor may execute
	// A custom entry's listed cost is 0 and it is never whitelisted, so its
	// bound ApplyPlayerCustomAction charges CMP in the source position.
	// Certify verifies static registration for potentially reachable callbacks
	// at admission without state-dependent validation.
	Certify  func(*LifetimeState) error
	Validate func(*LifetimeState) error
	Execute  func(*LifetimeState, random.Generator) (PlayerActionFacts, error)
}

// PlayerActionFacts are recorded by the bound callback's own contexts.
type PlayerActionFacts struct {
	Inert                  bool // source `$mes` return: the whole operation is inert
	Suppressed             bool // source `$npc_turn_skip`
	DamageCalls, HealCalls int
	End                    EndProcessing
	// Target is the context after the callback; the zero value keeps the
	// operation's own. Roster callbacks can rebuild the ordered side arrays.
	Target TargetContext
}

// PlayerOperation is rebuilt per request. Tail carries the actor's target
// context, requester level and defeat owner. Tail.EnemyIDs is the source enemy
// array: opponents fallen at load belong in MemberOnlyIDs, while defeats during
// the operation keep their positions, so the tail still runs after the last
// enemy falls (_battle.cgi:486–493,927). LevelUp performs the consumer's
// level-up branch when due; NPCComment reports a comment written this operation.
type PlayerOperation struct {
	Tail            PlayerTailContext
	Field           NamedFieldContext
	First           PlayerAction
	Second          *PlayerAction
	ResponseEnabled bool
	LevelUp         func(*LifetimeState, random.Generator) (bool, error)
	NPCComment      func() bool
}

// PlayerOperationResult reports reached blocks. ResponseReached means the
// eligible tail ran with ResponseEnabled, so the consumer processes at most one
// NPC response (_battle.cgi:1169–1171). Defeats
// reach the consumer in order through Tail.DefeatContext.Rewards, and consumed
// items stay on the actors' ConsumedItems.
type PlayerOperationResult struct {
	Accepted, NormalAction, Suppressed, ResponseReached bool
	Target                                              TargetContext // Final operation-local side order.
	LifetimeResult
}

// ApplyPlayerOperation executes one accepted player submission in source order
// (_battle.cgi:action). Unaccepted submissions, including a callback `$mes`,
// leave the state unchanged; any error restores the state before returning.
func (s *LifetimeState) ApplyPlayerOperation(op *PlayerOperation, rng random.Generator) (PlayerOperationResult, error) {
	if s == nil || op == nil || op.First.Validate == nil || op.First.Execute == nil {
		return PlayerOperationResult{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(op.Tail.TargetContext, rng); err != nil {
		return PlayerOperationResult{}, err
	}
	a := s.actor(op.Tail.ActorID)
	if a.NPC || op.Tail.RequesterLevel == nil || op.Tail.DefeatContext == nil {
		return PlayerOperationResult{}, ErrInvalidRequest
	}
	secondReachable := op.Second != nil && a.Extended == ExtendedAdditional && op.First.Whitelisted && op.Second.Whitelisted
	if op.Second != nil && (op.Second.Validate == nil || op.Second.Execute == nil) &&
		(secondReachable || op.Second.EntryID != "" || op.Second.Validate != nil || op.Second.Execute != nil || op.Second.Certify != nil) {
		return PlayerOperationResult{}, ErrInvalidRequest
	}
	if (a.HP <= 0 && !op.First.FallenSelf) || s.SkillBans[op.First.EntryID] > 0 {
		return PlayerOperationResult{}, nil
	}
	if op.First.Certify != nil {
		if err := op.First.Certify(s); err != nil {
			return PlayerOperationResult{}, err
		}
	}
	if err := op.First.Validate(s); err != nil {
		return PlayerOperationResult{}, err
	}
	if secondReachable && op.Second.Certify != nil {
		if err := op.Second.Certify(s); err != nil {
			return PlayerOperationResult{}, err
		}
	}
	// Tails write operation facts and the defeat cursor; the caller's op and
	// state stay unchanged on failure, and reuse never inherits earlier facts.
	work := *op
	defeat := *op.Tail.DefeatContext
	work.Tail.DefeatContext = &defeat
	saved := s.callbackCopy()
	result, err := s.applyPlayerOperation(&work, rng)
	if err != nil || !result.Accepted {
		*s = saved
		return PlayerOperationResult{}, err
	}
	result.Target = work.Tail.TargetContext
	return result, nil
}

func (s *LifetimeState) applyPlayerOperation(op *PlayerOperation, rng random.Generator) (PlayerOperationResult, error) {
	c := &op.Tail
	id := c.ActorID
	snapshot, err := s.CaptureEquipmentTail(c.TargetContext)
	if err != nil {
		return PlayerOperationResult{}, err
	}
	if c.Item210Fallen, err = s.CaptureItem210(c.TargetContext); err != nil {
		return PlayerOperationResult{}, err
	}
	var marked []string
	for _, member := range c.PartyIDs {
		if m := s.actor(member); m.HP <= 0 && m.Temporary == TemporaryEffect("death_mark") {
			marked = append(marked, member)
		}
	}
	fieldBefore, grave := s.Fields.Current(), s.SnapshotGrave()
	if fieldBefore == "sealed" {
		s.actor(id).EnterSealField()
	}
	defeat := func(target string) error {
		_, err := s.ApplyDefeat(c.DefeatContext, target, rng)
		return err
	}
	a := s.actor(id)
	if a.Status != "" && hasItem(a.ItemDefinitionIDs, "item-246") && rng.Float64() < .25 {
		a.Status = ""
	}
	decision, err := a.CheckIncapacitation(IncapacitationContext{Bypass: hasItem(a.ItemDefinitionIDs, "item-204")}, rng)
	if err != nil {
		return PlayerOperationResult{}, err
	}
	if decision.NeedsDefeat {
		if err := defeat(id); err != nil {
			return PlayerOperationResult{}, err
		}
	}
	result := PlayerOperationResult{Accepted: true, NormalAction: decision.NormalAction}
	c.Completion = LifetimeCompletion{Accepted: true, ActorID: id}
	if decision.NormalAction {
		message, err := s.applyPlayerNormalAction(op, defeat, rng)
		if err != nil || message {
			return PlayerOperationResult{}, err
		}
	}
	return s.finishPlayerOperation(op, result, snapshot, fieldBefore, grave, marked, defeat, rng)
}

// applyPlayerNormalAction is _battle.cgi:729–857. It returns true for `$mes`.
func (s *LifetimeState) applyPlayerNormalAction(op *PlayerOperation, defeat func(string) error, rng random.Generator) (bool, error) {
	c := &op.Tail
	a := s.actor(c.ActorID)
	// Capture before the first callback can replace the effect.
	additional := a.Extended == ExtendedAdditional
	c.NormalAction = true
	if err := a.BeginNormalAction(false, op.First.MirrorShare, hasItem(a.ItemDefinitionIDs, "item-163"), rng); err != nil {
		return false, err
	}
	if err := op.First.Validate(s); err != nil {
		return false, err
	}
	facts, err := op.First.Execute(s, rng)
	if err != nil || facts.Inert {
		return facts.Inert, err
	}
	s.recordPlayerFacts(c, facts)
	c.BowEffect = op.First.Bow
	charge := func(cost int) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-cost)
	}
	charge(op.First.Cost)
	if second := op.Second; second != nil && CanExecuteSecond(additional, op.First.Whitelisted, second.Whitelisted, s.actor(c.ActorID).MP, second.Cost) {
		if err := second.Validate(s); err != nil {
			return false, err
		}
		// The source ignores a second callback's `$mes`.
		facts, err := second.Execute(s, rng)
		if err != nil {
			return false, err
		}
		s.recordPlayerFacts(c, facts)
		charge(second.Cost)
	}
	deaths, err := s.ApplyPartyPoison(c.ActorID, c.PartyIDs, rng)
	if err != nil {
		return false, err
	}
	for _, id := range deaths {
		if err := defeat(id); err != nil {
			return false, err
		}
	}
	poison := func(statuses ...string) error {
		a := s.actor(c.ActorID)
		if !slices.Contains(statuses, a.Status) {
			return nil
		}
		dead, err := a.ApplyPoison(rng)
		if err != nil || !dead {
			return err
		}
		return defeat(c.ActorID)
	}
	if err := poison(StatusDeadlyPoison, StatusVirulentPoison); err != nil {
		return false, err
	}
	if a := s.actor(c.ActorID); a.HP > 0 && a.Temporary == TemporaryRegeneration {
		amount := int(float64(a.MaxHP) * (.1 + rng.Float64()*.1))
		if a.MaxHP > 999 {
			amount = int(rng.Float64() * 100)
		}
		a.HP = min(a.MaxHP, a.HP+amount)
	}
	if err := s.ApplyPlayerTail(c, "item-189-normal-tail", rng); err != nil {
		return false, err
	}
	if err := poison(StatusPoison); err != nil {
		return false, err
	}
	if a := s.actor(c.ActorID); a.HP > 0 && a.Temporary == TemporaryRage && !c.Completion.Suppressed {
		return false, s.statTension(&c.EffectContext, a, false, rng)
	}
	return false, nil
}

func (s *LifetimeState) recordPlayerFacts(c *PlayerTailContext, f PlayerActionFacts) {
	if f.Target.ActorID != "" {
		c.TargetContext = f.Target
		c.DefeatContext.TargetContext = f.Target
	}
	c.Completion.Suppressed = c.Completion.Suppressed || f.Suppressed
	c.DamageCalls += f.DamageCalls
	c.HealCalls += f.HealCalls
	if c.End.DarknessDebit == nil {
		c.End.DarknessDebit = f.End.DarknessDebit
	}
	c.End.ImmediateRiskCalls += f.End.ImmediateRiskCalls
	c.End.SkipPrefilter = c.End.SkipPrefilter || f.End.SkipPrefilter
}

// finishPlayerOperation is _battle.cgi:861–1172, reached by every accepted
// operation, including incapacitation and the actor's own defeat.
func (s *LifetimeState) finishPlayerOperation(op *PlayerOperation, result PlayerOperationResult, snapshot EquipmentTailSnapshot, fieldBefore string, grave *GraveSnapshot, marked []string, defeat func(string) error, rng random.Generator) (PlayerOperationResult, error) {
	c := &op.Tail
	equipment := func(ids ...string) error {
		e := EquipmentTailContext{DamageContext: c.DamageContext, Completion: c.Completion, Snapshot: snapshot, Defeat: c.DefeatContext}
		for _, id := range ids {
			if err := s.ApplyEquipmentTail(&e, id, rng); err != nil {
				return err
			}
		}
		c.DamageContext = e.DamageContext
		return nil
	}
	tails := func(ids ...string) error {
		for _, id := range ids {
			if err := s.ApplyPlayerTail(c, id, rng); err != nil {
				return err
			}
		}
		return nil
	}
	if err := equipment("item-164-operation-tail", "item-209-operation-tail"); err != nil {
		return result, err
	}
	op.Field.TargetContext = c.TargetContext
	if err := s.ApplyNamedFieldTail(&op.Field, NamedFieldTail{Before: fieldBefore, ResponseEnabled: op.ResponseEnabled, Suppressed: c.Completion.Suppressed, Grave: grave}, rng); err != nil {
		return result, err
	}
	c.TargetContext = op.Field.TargetContext
	c.Completion.EnemiesRemain = len(c.EnemyIDs) > 0
	s.Fields.FinishOperation(c.Completion.EnemiesRemain, c.Completion.Suppressed)
	if err := equipment("item-206-operation-tail"); err != nil {
		return result, err
	}
	if err := tails("item-210-revival-tail"); err != nil {
		return result, err
	}
	result.Suppressed = c.Completion.Suppressed
	if op.LevelUp != nil {
		leveled, err := op.LevelUp(s, rng)
		if err != nil {
			return result, err
		}
		c.Completion.LevelUp = leveled
	}
	c.Completion.NPCComment = op.NPCComment != nil && op.NPCComment()
	f := c.Completion
	if !f.EnemiesRemain || f.Suppressed || f.NPCComment || f.LevelUp {
		return result, nil
	}
	if err := tails("item-183-combat-tail", "item-194-combat-tail", "item-202-combat-tail", "item-198-combat-tail",
		"weapon-35-armor-37-tail", "weapon-69-armor-54-tail", "weapon-70-armor-55-tail"); err != nil {
		return result, err
	}
	if err := equipment("guardian-mp-tail"); err != nil {
		return result, err
	}
	if err := tails("weapon-30-tail", "weapon-54-tail"); err != nil {
		return result, err
	}
	if err := equipment("item-208-combat-tail"); err != nil {
		return result, err
	}
	if err := tails("cmp-recharge-tail"); err != nil {
		return result, err
	}
	recharged := s.actor(c.ActorID).AdvanceCMP(c.Completion.CMPAmount, c.Completion.NextCMPThreshold)
	if err := tails("frenzy-tail", "crown-tail"); err != nil {
		return result, err
	}
	if err := s.ApplyIncenseHealing(&c.EffectContext, rng); err != nil {
		return result, err
	}
	// Counters finish before item246 and the death mark; the end queue flushes last.
	completion := c.Completion
	completion.Tail = LifetimeTail{DamageCalls: c.DamageCalls, BowAction: c.BowEffect, SecondSubmitted: op.Second != nil && op.Second.Whitelisted}
	completion.CMPRecharged = &recharged
	completion.End = EndProcessing{}
	lifetime, err := s.CompleteOperation(completion, c.PartyIDs)
	if err != nil {
		return result, err
	}
	result.LifetimeResult = lifetime
	if err := equipment("item-246-combat-tail"); err != nil {
		return result, err
	}
	for _, id := range marked {
		if m := s.actor(id); m.HP > 0 {
			m.Temporary = ""
			if err := defeat(id); err != nil {
				return result, err
			}
		}
	}
	c.End.Flush(s.actor(c.ActorID))
	result.ResponseReached = op.ResponseEnabled
	return result, nil
}
